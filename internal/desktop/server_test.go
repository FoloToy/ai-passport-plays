package desktop

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FoloToy/ai-passport-plays/internal/factory"
	"github.com/FoloToy/ai-passport-plays/internal/flasher"
	"github.com/FoloToy/ai-passport-plays/internal/settings"
	"github.com/FoloToy/ai-passport-plays/internal/syncer"
	"github.com/FoloToy/ai-passport-plays/internal/worker"
)

func newServer(t *testing.T) (*Server, string) {
	t.Helper()
	st, err := settings.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	lib := t.TempDir()
	if _, err := st.Update(func(c *settings.Settings) { c.LibraryDir = lib }); err != nil {
		t.Fatal(err)
	}
	sm := syncer.New(syncer.NewClient("http://127.0.0.1:1", "test"))
	return New("test", st, sm, flasher.New(worker.Runner{}), worker.Runner{}), lib
}

func do(s *Server, method, path, token, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		r.Header.Set("X-Plays-Token", token)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func TestAPIRequiresToken(t *testing.T) {
	s, _ := newServer(t)
	if w := do(s, "GET", "/api/state", "", ""); w.Code != http.StatusForbidden {
		t.Fatalf("missing token: %d", w.Code)
	}
	if w := do(s, "GET", "/api/state", "wrong", ""); w.Code != http.StatusForbidden {
		t.Fatalf("wrong token: %d", w.Code)
	}
	if w := do(s, "GET", "/api/state", s.Token, ""); w.Code != 200 {
		t.Fatalf("valid token: %d", w.Code)
	}
}

func TestIndexPageCarriesToken(t *testing.T) {
	s, _ := newServer(t)
	w := do(s, "GET", "/", "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), s.Token) {
		t.Fatal("index page must embed the session token")
	}
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "default-src 'self'") {
		t.Fatal("CSP header missing")
	}
}

func TestMediaServesOnlyLibraryImages(t *testing.T) {
	s, lib := newServer(t)
	dir := filepath.Join(lib, "plays", "demo")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "cover.webp"), []byte("img"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "firmware.bin"), []byte("fw"), 0o644)
	if w := do(s, "GET", "/media/plays/demo/cover.webp", "", ""); w.Code != 200 || w.Body.String() != "img" {
		t.Fatalf("cover: %d", w.Code)
	}
	for _, p := range []string{"/media/plays/demo/firmware.bin", "/media/../settings.json", "/media/plays/../../etc/passwd.webp"} {
		if w := do(s, "GET", p, "", ""); w.Code != 404 {
			t.Errorf("%s should be 404, got %d", p, w.Code)
		}
	}
}

func TestSetLibraryRequiresAbsolutePath(t *testing.T) {
	s, _ := newServer(t)
	w := do(s, "POST", "/api/library/set", s.Token, `{"dir":"relative/path","mode":"switch"}`)
	if w.Code != 400 || !strings.Contains(w.Body.String(), `"code":"path"`) {
		t.Fatalf("relative path accepted: %d %s", w.Code, w.Body)
	}
	target := filepath.Join(t.TempDir(), "new")
	w = do(s, "POST", "/api/library/set", s.Token, `{"dir":"`+filepath.ToSlash(target)+`","mode":"switch"}`)
	if w.Code != 200 || s.Settings.Get().LibraryDir != filepath.Clean(target) {
		t.Fatalf("switch failed: %d %s", w.Code, w.Body)
	}
}

func TestFlashRefusesMissingFirmware(t *testing.T) {
	s, _ := newServer(t)
	w := do(s, "POST", "/api/flash/start", s.Token, `{"slug":"nope","port":"/dev/cu.x"}`)
	if w.Code != 400 {
		t.Fatalf("flash without index should fail: %d", w.Code)
	}
}

// WebKit (via Wails) delivers POST bodies without Content-Length.
func TestBodyReadWithoutContentLength(t *testing.T) {
	s, _ := newServer(t)
	target := filepath.Join(t.TempDir(), "plays")
	r := httptest.NewRequest("POST", "/api/library/set", strings.NewReader(`{"dir":"`+filepath.ToSlash(target)+`","mode":"switch"}`))
	r.ContentLength = 0
	r.Header.Del("Content-Length")
	r.Header.Set("X-Plays-Token", s.Token)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 200 || s.Settings.Get().LibraryDir != filepath.Clean(target) {
		t.Fatalf("body ignored without Content-Length: %d %s", w.Code, w.Body)
	}
}

func TestRestoreUnpacksFirmwareAndRunsOneJob(t *testing.T) {
	s, _ := newServer(t)
	s.StateDir = t.TempDir()
	backups := t.TempDir()
	if w := do(s, "POST", "/api/backup/set", s.Token, `{"dir":"relative/path"}`); w.Code == 200 {
		t.Fatal("relative backup folder accepted")
	}
	if w := do(s, "POST", "/api/backup/set", s.Token, `{"dir":"`+filepath.ToSlash(backups)+`"}`); w.Code != 200 {
		t.Fatalf("set backup: %d %s", w.Code, w.Body)
	}
	if got := s.Settings.Get().Backups(); filepath.Clean(got) != filepath.Clean(backups) {
		t.Fatalf("backup dir %q", got)
	}
	if w := do(s, "POST", "/api/restore/start", s.Token, `{"fullBackup":false}`); w.Code != 200 {
		t.Fatalf("start: %d %s", w.Code, w.Body)
	}
	if _, err := os.Stat(filepath.Join(s.StateDir, "factory-firmware", factory.Version, "trae_card.bin")); err != nil {
		t.Fatal("factory firmware was not unpacked:", err)
	}
	// The test server has no worker, so the job fails on its own; it must report a restore.
	deadline := time.Now().Add(3 * time.Second)
	for s.Flash.Active() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if st := s.Flash.Snapshot(); st.Kind != "restore" || st.Stage != "failed" {
		t.Fatalf("unexpected state %+v", st)
	}
	if w := do(s, "POST", "/api/restore/cancel", s.Token, `{}`); w.Code == 200 {
		t.Fatal("cancel accepted with nothing running")
	}
}

func TestRestoreNeedsStateDir(t *testing.T) {
	s, _ := newServer(t)
	if w := do(s, "POST", "/api/restore/start", s.Token, `{}`); w.Code == 200 || !strings.Contains(w.Body.String(), "firmware") {
		t.Fatalf("want firmware error, got %d %s", w.Code, w.Body)
	}
}

func TestFirmwareBackupStartsReadOnlyJob(t *testing.T) {
	s, _ := newServer(t)
	dir := t.TempDir()
	if _, err := s.Settings.Update(func(c *settings.Settings) { c.BackupDir = dir }); err != nil {
		t.Fatal(err)
	}
	if w := do(s, "POST", "/api/firmware-backup/start", s.Token, `{}`); w.Code != 200 {
		t.Fatalf("start: %d %s", w.Code, w.Body)
	}
	if st := s.Flash.Snapshot(); st.Kind != "backup" {
		t.Fatalf("unexpected kind %q", st.Kind)
	}
}
