package syncer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FoloToy/ai-passport-desktop/pkg/library"
)

// fakeImage returns a minimal ESP32-C3 merged image header followed by payload.
func fakeImage(seed byte, n int) []byte {
	b := make([]byte, n)
	b[0] = 0xE9
	b[12] = 0x05
	for i := 16; i < n; i++ {
		b[i] = seed
	}
	return b
}

type fakeSite struct {
	mu        sync.Mutex
	plays     []map[string]any
	firmware  map[string][]byte
	limitOnce map[string]bool // slug -> return 429 once
	downloads atomic.Int32
	media     atomic.Int32
}

func newFakeSite(n int) *fakeSite {
	s := &fakeSite{firmware: map[string][]byte{}, limitOnce: map[string]bool{}}
	for i := 1; i <= n; i++ {
		slug := fmt.Sprintf("play-%d", i)
		img := fakeImage(byte(i), 4096+i)
		sum := sha256.Sum256(img)
		s.firmware[slug] = img
		cat := "games"
		if i%2 == 0 {
			cat = "learning"
		}
		s.plays = append(s.plays, map[string]any{
			"id": i, "slug": slug, "source": "community", "revisionId": 100 + i,
			"title":       map[string]string{"zh": "玩法" + strconv.Itoa(i), "en": "Play " + strconv.Itoa(i)},
			"description": map[string]string{"zh": "描述", "en": "desc"},
			"categoryKey": cat, "tags": []string{}, "author": "dev",
			"authorProfile": map[string]any{"name": "dev", "avatarUrl": "/api/avatars/abc.webp"},
			"image":         "/api/media/plays/" + slug + ".webp?v=1",
			"thumbnail":     "/api/media/plays/" + slug + "/thumbnail.webp?v=1",
			"images":        []string{"/api/media/plays/" + slug + "/gallery/0.webp?v=1"},
			"publishedAt":   "2026-10-01T00:00:00Z", "downloads": i,
			"firmware": map[string]any{"available": true, "size": len(img), "sha256": hex.EncodeToString(sum[:]),
				"url": "/api/download/community/" + slug, "format": "esp-merged-0x0"},
		})
	}
	return s
}

func (s *fakeSite) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case r.URL.Path == "/api/plays":
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		offset, _ := strconv.Atoi(q.Get("offset"))
		list := s.plays
		if q.Get("tag") == "entry:family" {
			list = s.plays[:1]
		}
		end := min(offset+limit, len(list))
		page := []map[string]any{}
		if offset < len(list) {
			page = list[offset:end]
		}
		resp := map[string]any{
			"ok": true, "plays": page,
			"pagination":     map[string]any{"total": len(list), "hasMore": end < len(list)},
			"categoryCounts": map[string]int{},
			"discoveryTags":  []map[string]any{{"key": "must-play", "name": map[string]string{"zh": "必玩", "en": "Must"}, "enabled": true}},
			"discoveryTabs":  []map[string]any{{"key": "entry:family", "name": map[string]string{"zh": "亲子", "en": "Family"}, "enabled": true, "visible": true}},
		}
		if q.Get("bootstrap") == "true" {
			resp["bootstrap"] = map[string]any{"recommendations": []map[string]any{{"id": 2}}}
		}
		_ = json.NewEncoder(w).Encode(resp)
	case len(r.URL.Path) > len("/api/download/community/") && r.URL.Path[:len("/api/download/community/")] == "/api/download/community/":
		slug := r.URL.Path[len("/api/download/community/"):]
		if s.limitOnce[slug] {
			delete(s.limitOnce, slug)
			w.Header().Set("Retry-After", "0")
			http.Error(w, `{"detail":"busy"}`, http.StatusTooManyRequests)
			return
		}
		s.downloads.Add(1)
		_, _ = w.Write(s.firmware[slug])
	case len(r.URL.Path) > len("/api/media/") && r.URL.Path[:len("/api/media/")] == "/api/media/", len(r.URL.Path) > len("/api/avatars/") && r.URL.Path[:len("/api/avatars/")] == "/api/avatars/":
		s.media.Add(1)
		_, _ = w.Write([]byte("RIFFxxxxWEBP"))
	default:
		http.NotFound(w, r)
	}
}

func waitIdle(t *testing.T, m *Manager) State {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if s := m.Snapshot(); !s.Running {
			return s
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("sync did not finish")
	return State{}
}

func TestFullThenIncrementalSync(t *testing.T) {
	throttleBase = 50 * time.Millisecond
	site := newFakeSite(7)
	site.limitOnce["play-3"] = true
	srv := httptest.NewServer(site)
	defer srv.Close()
	root := t.TempDir()
	m := New(NewClient(srv.URL, "test"))
	indexed := 0
	m.OnIndex = func() { indexed++ }

	if err := m.Start(root); err != nil {
		t.Fatal(err)
	}
	st := waitIdle(t, m)
	if st.Phase != "done" || st.Failed != 0 || st.Added != 7 {
		t.Fatalf("unexpected state %+v", st)
	}
	if indexed < 2 {
		t.Fatalf("index should be written after both stages, got %d", indexed)
	}
	lib := library.Library{Root: root}
	idx, err := lib.LoadIndex()
	if err != nil {
		t.Fatal(err)
	}
	if !idx.Complete || len(idx.Plays) != 7 {
		t.Fatalf("index incomplete: complete=%v plays=%d", idx.Complete, len(idx.Plays))
	}
	for _, p := range idx.Plays {
		if !p.Ready || p.Cover == "" || p.Thumb == "" || len(p.Gallery) != 1 || p.Author.Avatar == "" {
			t.Fatalf("play not fully stored: %+v", p)
		}
	}
	if len(idx.Topics) != 2 || idx.Topics[1].Key != "family" || len(idx.Topics[1].IDs) != 1 {
		t.Fatalf("topics: %+v", idx.Topics)
	}
	if len(idx.Recommendations) != 1 || idx.Recommendations[0] != 2 {
		t.Fatalf("recommendations: %v", idx.Recommendations)
	}
	if got := site.downloads.Load(); got != 7 {
		t.Fatalf("expected 7 firmware downloads, got %d", got)
	}

	// Second run: nothing changed, so nothing is downloaded again.
	media := site.media.Load()
	if err := m.Start(root); err != nil {
		t.Fatal(err)
	}
	st = waitIdle(t, m)
	if st.Phase != "done" || site.downloads.Load() != 7 || site.media.Load() != media {
		t.Fatalf("incremental sync re-downloaded: %+v downloads=%d", st, site.downloads.Load())
	}

	// A play disappears from the site: it is kept locally and marked removed.
	site.mu.Lock()
	site.plays = site.plays[1:]
	site.mu.Unlock()
	if err := m.Start(root); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, m)
	idx, _ = lib.LoadIndex()
	removed := 0
	for _, p := range idx.Plays {
		if p.Removed {
			removed++
			if p.Slug != "play-1" || !p.Ready {
				t.Fatalf("removed play wrong: %+v", p)
			}
		}
	}
	if removed != 1 {
		t.Fatalf("expected one removed play, got %d", removed)
	}
}

func TestCorruptFirmwareIsRejected(t *testing.T) {
	site := newFakeSite(2)
	site.firmware["play-2"] = fakeImage(9, 5000) // size and hash no longer match the catalog
	srv := httptest.NewServer(site)
	defer srv.Close()
	root := t.TempDir()
	m := New(NewClient(srv.URL, "test"))
	if err := m.Start(root); err != nil {
		t.Fatal(err)
	}
	st := waitIdle(t, m)
	if st.Failed != 1 || len(st.Failures) != 1 || st.Failures[0].Slug != "play-2" {
		t.Fatalf("expected play-2 to fail: %+v", st)
	}
	if _, err := os.Stat(filepath.Join(root, "plays", "play-2", "firmware.bin")); !os.IsNotExist(err) {
		t.Fatal("corrupt firmware must not be stored")
	}
	idx, _ := library.Library{Root: root}.LoadIndex()
	if idx.Complete {
		t.Fatal("index must be marked incomplete")
	}
}

func TestFetchOneOnDemand(t *testing.T) {
	site := newFakeSite(1)
	srv := httptest.NewServer(site)
	defer srv.Close()
	root := t.TempDir()
	lib := library.Library{Root: root}
	m := New(NewClient(srv.URL, "test"))
	// Catalog only: write the meta file the way stage 1 does, without firmware.
	plays, _, err := m.Client.listAll(t.Context(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.syncCatalog(t.Context(), lib, &plays[0])
	if _, _, ok := LocalFirmware(lib, "play-1"); ok {
		t.Fatal("firmware should not exist yet")
	}
	if err := m.FetchOne(root, "play-1"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for m.Fetch().Running && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if f := m.Fetch(); !f.Done {
		t.Fatalf("fetch failed: %+v", f)
	}
	if _, _, ok := LocalFirmware(lib, "play-1"); !ok {
		t.Fatal("firmware should be ready")
	}
	if slugs, _ := m.Ready(); len(slugs) != 1 || slugs[0] != "play-1" {
		t.Fatalf("ready overlay: %v", slugs)
	}
}

func TestMoveLibrary(t *testing.T) {
	from := filepath.Join(t.TempDir(), "a")
	to := filepath.Join(t.TempDir(), "b")
	if err := os.MkdirAll(filepath.Join(from, "plays", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(from, "plays", "x", "firmware.bin"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New(NewClient("http://127.0.0.1:1", "test"))
	committed := false
	if err := m.Move(from, to, func() error { committed = true; return nil }); err != nil {
		t.Fatal(err)
	}
	st := waitIdle(t, m)
	if st.Phase != "done" || !committed {
		t.Fatalf("move failed: %+v", st)
	}
	if b, err := os.ReadFile(filepath.Join(to, "plays", "x", "firmware.bin")); err != nil || string(b) != "data" {
		t.Fatal("data not moved")
	}
	if err := CheckMoveTarget(to, filepath.Join(to, "inner")); err == nil {
		t.Fatal("nested target must be refused")
	}
	other := t.TempDir()
	_ = os.WriteFile(filepath.Join(other, "keep.txt"), []byte("x"), 0o644)
	if err := CheckMoveTarget(to, other); err != ErrTargetNotEmpty {
		t.Fatalf("non-empty target must be refused, got %v", err)
	}
}

func TestThrottleBacksOff(t *testing.T) {
	throttleBase, throttleMax = time.Second, 4*time.Second
	var th throttle
	a := time.Until(th.hit(0))
	b := time.Until(th.hit(0))
	c := time.Until(th.hit(0))
	d := time.Until(th.hit(10 * time.Second))
	if a > 1100*time.Millisecond || b < 1900*time.Millisecond || c < 3900*time.Millisecond || c > 4100*time.Millisecond || d < 9*time.Second {
		t.Fatalf("unexpected backoff %v %v %v %v", a, b, c, d)
	}
	throttleBase, throttleMax = 30*time.Second, 10*time.Minute
}
