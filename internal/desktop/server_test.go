package desktop

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
