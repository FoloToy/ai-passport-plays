// Package desktop serves the embedded UI, library media and the local JSON API.
// In the app it is the Wails asset handler, so no network port is opened. The app
// logic itself lives in pkg/core, which the Windows 7 client shares.
package desktop

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/FoloToy/ai-passport-desktop/frontend"
	"github.com/FoloToy/ai-passport-desktop/pkg/core"
	"github.com/FoloToy/ai-passport-desktop/pkg/device"
	"github.com/FoloToy/ai-passport-desktop/pkg/factory"
	"github.com/FoloToy/ai-passport-desktop/pkg/flasher"
	"github.com/FoloToy/ai-passport-desktop/pkg/settings"
	"github.com/FoloToy/ai-passport-desktop/pkg/syncer"
)

type Server struct {
	*core.Core
	Token string

	// Native hooks; nil in headless development mode.
	ChooseDir func(title, current string) (string, error)
	Reveal    func(path string) error
	OpenURL   func(url string) error

	systemLanguage string
}

func New(version string, st *settings.Store, sm *syncer.Manager, fm *flasher.Manager, w device.Runner) *Server {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return &Server{Core: core.New(version, "", st, sm, fm, w), Token: hex.EncodeToString(b), systemLanguage: settings.SystemLanguage()}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
	switch {
	case strings.HasPrefix(r.URL.Path, "/api/"):
		w.Header().Set("Cache-Control", "no-store")
		if r.Header.Get("X-Plays-Token") != s.Token {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		s.api(w, r)
	case strings.HasPrefix(r.URL.Path, "/media/"):
		s.media(w, r)
	case r.Method != http.MethodGet && r.Method != http.MethodHead:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	case r.URL.Path == "/" || r.URL.Path == "/index.html":
		b, _ := frontend.Files.ReadFile("index.html")
		page := strings.NewReplacer("__TOKEN__", s.Token, "__NATIVE__", strconv.FormatBool(s.ChooseDir != nil), "__VERSION__", s.Version).Replace(string(b))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = io.WriteString(w, page)
	default:
		w.Header().Set("Cache-Control", "no-cache")
		http.FileServer(http.FS(fs.FS(frontend.Files))).ServeHTTP(w, r)
	}
}

func (s *Server) media(w http.ResponseWriter, r *http.Request) {
	lib := s.Library()
	p, ok := lib.Resolve(strings.TrimPrefix(r.URL.Path, "/media/"))
	if !ok || lib.Root == "" {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(p)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, st.Name(), st.ModTime(), f)
}

func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"error"`
}

func fail(w http.ResponseWriter, code string, err error) {
	reply(w, http.StatusBadRequest, apiError{Code: code, Message: err.Error()})
}

func (s *Server) api(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		switch r.URL.Path {
		case "/api/state":
			reply(w, 200, s.state())
		case "/api/catalog":
			_, b, err := s.Index()
			if err != nil {
				reply(w, 404, apiError{Code: "no_index", Message: "library has no index"})
				return
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write(b)
		case "/api/ports":
			ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
			defer cancel()
			ports, err := s.Runner.Ports(ctx)
			if err != nil {
				fail(w, "worker", err)
				return
			}
			reply(w, 200, ports)
		case "/api/ready":
			slugs, version := s.Sync.Ready()
			reply(w, 200, map[string]any{"slugs": slugs, "version": version})
		case "/api/library/usage":
			lib := s.Library()
			size, files := lib.Usage()
			free, _ := syncer.FreeSpace(lib.Root)
			reply(w, 200, map[string]any{"bytes": size, "files": files, "free": free})
		default:
			http.NotFound(w, r)
		}
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var body struct {
		Language string `json:"language"`
		Dir      string `json:"dir"`
		Mode     string `json:"mode"`
		Slug     string `json:"slug"`
		Port     string `json:"port"`
		Target   string `json:"target"`
		// FullBackup defaults to true when omitted.
		FullBackup *bool `json:"fullBackup"`
	}
	// Always read the body: WebKit's custom-scheme requests carry no Content-Length,
	// so Wails reports ContentLength 0 even when a JSON body is present.
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			fail(w, "request", err)
			return
		}
	}
	var out any = map[string]bool{"ok": true}
	var err error
	switch r.URL.Path {
	case "/api/settings":
		err = s.SetLanguage(body.Language)
	case "/api/library/choose":
		if s.ChooseDir == nil {
			fail(w, "no_picker", errors.New("native folder picker unavailable"))
			return
		}
		var dir string
		dir, err = s.ChooseDir("选择玩法存储目录 / Choose library folder", s.Settings.Get().LibraryDir)
		out = map[string]string{"dir": dir}
	case "/api/backup/choose":
		if s.ChooseDir == nil {
			fail(w, "no_picker", errors.New("native folder picker unavailable"))
			return
		}
		var dir string
		if dir, err = s.ChooseDir("选择备份目录 / Choose backup folder", s.Settings.Get().Backups()); err == nil && dir != "" {
			err = s.SetBackupDir(dir)
		}
	case "/api/backup/set":
		err = s.SetBackupDir(body.Dir)
	case "/api/restore/start":
		err = s.StartRestore(body.FullBackup == nil || *body.FullBackup)
	case "/api/firmware-backup/start":
		err = s.StartBackup()
	case "/api/restore/cancel":
		err = s.CancelRestore()
	case "/api/library/set":
		err = s.SetLibrary(body.Dir, body.Mode)
	case "/api/sync/start":
		err = s.StartSync()
	case "/api/sync/cancel":
		s.Sync.Cancel()
	case "/api/update/check":
		s.CheckUpdatesInBackground()
	case "/api/firmware/fetch":
		err = s.FetchFirmware(body.Slug)
	case "/api/flash/start":
		err = s.StartFlash(body.Slug, body.Port)
	case "/api/flash/reset":
		s.Flash.Reset()
	case "/api/reveal":
		err = s.reveal(body.Target, body.Slug)
	case "/api/open-web":
		err = s.openWeb(body.Slug)
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, core.Code(err), err)
		return
	}
	reply(w, 200, out)
}

func (s *Server) state() map[string]any {
	cfg := s.Settings.Get()
	return map[string]any{
		"version":        s.Version,
		"platform":       runtime.GOOS,
		"worker":         s.Runner.Available(),
		"native":         s.ChooseDir != nil,
		"language":       cfg.Language,
		"systemLanguage": s.systemLanguage,
		"library":        s.LibraryInfo(),
		"sync":           s.Sync.Snapshot(),
		"update":         s.Sync.Update(),
		"fetch":          s.Sync.Fetch(),
		"ready":          s.ReadyVersion(),
		"flash":          s.Flash.Snapshot(),
		"restore": map[string]any{
			"backupDir":        cfg.Backups(),
			"defaultBackupDir": settings.DefaultBackupDir(),
			"firmware":         factory.Version,
		},
	}
}

func (s *Server) reveal(target, slug string) error {
	if s.Reveal == nil {
		return errors.New("not available")
	}
	dir, err := s.RevealPath(target, slug)
	if err != nil {
		return err
	}
	return s.Reveal(dir)
}

func (s *Server) openWeb(slug string) error {
	if s.OpenURL == nil {
		return errors.New("not available")
	}
	url, err := s.PlayURL(slug)
	if err != nil {
		return err
	}
	return s.OpenURL(url)
}
