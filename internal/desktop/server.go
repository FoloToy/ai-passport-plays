// Package desktop serves the embedded UI, library media and the local JSON API.
// In the app it is the Wails asset handler, so no network port is opened.
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
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/FoloToy/ai-passport-plays/frontend"
	"github.com/FoloToy/ai-passport-plays/internal/device"
	"github.com/FoloToy/ai-passport-plays/internal/factory"
	"github.com/FoloToy/ai-passport-plays/internal/flasher"
	"github.com/FoloToy/ai-passport-plays/internal/library"
	"github.com/FoloToy/ai-passport-plays/internal/settings"
	"github.com/FoloToy/ai-passport-plays/internal/syncer"
)

type Server struct {
	Version  string
	Settings *settings.Store
	Sync     *syncer.Manager
	Flash    *flasher.Manager
	Worker   device.Runner
	Token    string
	// StateDir is the private app folder; the factory firmware is unpacked under it.
	StateDir string

	// Native hooks; nil in headless development mode.
	ChooseDir func(title, current string) (string, error)
	Reveal    func(path string) error
	OpenURL   func(url string) error
	// Windows 7/8 edition only: report and install the USB serial driver.
	DriverStatus  func() string
	InstallDriver func() error

	systemLanguage string

	mu       sync.Mutex
	catalog  []byte
	index    *library.Index
	indexDir string
}

func New(version string, st *settings.Store, sm *syncer.Manager, fm *flasher.Manager, w device.Runner) *Server {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	s := &Server{Version: version, Settings: st, Sync: sm, Flash: fm, Worker: w, Token: hex.EncodeToString(b), systemLanguage: settings.SystemLanguage()}
	sm.OnIndex = s.Invalidate
	return s
}

func (s *Server) library() library.Library { return library.Library{Root: s.Settings.Get().LibraryDir} }

// Invalidate drops the cached catalog after a sync or library move.
func (s *Server) Invalidate() {
	s.mu.Lock()
	s.catalog, s.index, s.indexDir = nil, nil, ""
	s.mu.Unlock()
}

func (s *Server) loadIndex() (*library.Index, []byte, error) {
	lib := s.library()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.index != nil && s.indexDir == lib.Root {
		return s.index, s.catalog, nil
	}
	if lib.Root == "" {
		return nil, nil, os.ErrNotExist
	}
	idx, err := lib.LoadIndex()
	if err != nil {
		return nil, nil, err
	}
	b, err := json.Marshal(idx)
	if err != nil {
		return nil, nil, err
	}
	s.index, s.catalog, s.indexDir = idx, b, lib.Root
	return idx, b, nil
}

// CheckUpdatesInBackground asks the community whether newer plays exist.
func (s *Server) CheckUpdatesInBackground() {
	idx, _, err := s.loadIndex()
	if err != nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		s.Sync.CheckUpdates(ctx, idx)
	}()
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
	p, ok := s.library().Resolve(strings.TrimPrefix(r.URL.Path, "/media/"))
	if !ok || s.library().Root == "" {
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
			_, b, err := s.loadIndex()
			if err != nil {
				reply(w, 404, apiError{Code: "no_index", Message: "library has no index"})
				return
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write(b)
		case "/api/ports":
			ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
			defer cancel()
			ports, err := s.Worker.Ports(ctx)
			if err != nil {
				fail(w, "worker", err)
				return
			}
			reply(w, 200, ports)
		case "/api/ready":
			slugs, version := s.Sync.Ready()
			reply(w, 200, map[string]any{"slugs": slugs, "version": version})
		case "/api/library/usage":
			lib := s.library()
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
	code := "other"
	switch r.URL.Path {
	case "/api/settings":
		_, err = s.Settings.Update(func(c *settings.Settings) { c.Language = body.Language })
	case "/api/library/choose":
		if s.ChooseDir == nil {
			err, code = errors.New("native folder picker unavailable"), "no_picker"
			break
		}
		var dir string
		dir, err = s.ChooseDir("选择玩法存储目录 / Choose library folder", s.Settings.Get().LibraryDir)
		out = map[string]string{"dir": dir}
	case "/api/backup/choose":
		if s.ChooseDir == nil {
			err, code = errors.New("native folder picker unavailable"), "no_picker"
			break
		}
		var dir string
		if dir, err = s.ChooseDir("选择备份目录 / Choose backup folder", s.Settings.Get().Backups()); err == nil && dir != "" {
			code, err = s.setBackupDir(dir)
		}
	case "/api/backup/set":
		code, err = s.setBackupDir(body.Dir)
	case "/api/restore/start":
		code, err = s.startRestore(body.FullBackup == nil || *body.FullBackup)
	case "/api/firmware-backup/start":
		code, err = s.startBackup()
	case "/api/driver/install":
		if s.InstallDriver == nil {
			err, code = errors.New("this edition does not install drivers"), "driver"
		} else if err = s.InstallDriver(); err != nil {
			code = "driver"
		}
	case "/api/restore/cancel":
		if !s.Flash.Cancel() {
			err, code = errors.New("the restore can no longer be stopped"), "not_cancellable"
		}
	case "/api/library/set":
		code, err = s.setLibrary(body.Dir, body.Mode)
	case "/api/sync/start":
		if s.Flash.Active() {
			err, code = errors.New("flashing in progress"), "busy"
			break
		}
		dir := s.Settings.Get().LibraryDir
		if dir == "" {
			err, code = errors.New("choose a library folder first"), "no_library"
			break
		}
		if err = s.Sync.Start(dir); err != nil {
			code = err.Error()
		}
	case "/api/sync/cancel":
		s.Sync.Cancel()
	case "/api/update/check":
		s.CheckUpdatesInBackground()
	case "/api/firmware/fetch":
		if err = s.Sync.FetchOne(s.Settings.Get().LibraryDir, body.Slug); err != nil {
			code = err.Error()
			if errors.Is(err, syncer.ErrNoFirmware) {
				code = "no_firmware"
			}
		}
	case "/api/flash/start":
		code, err = s.startFlash(body.Slug, body.Port)
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
		fail(w, code, err)
		return
	}
	reply(w, 200, out)
}

func (s *Server) state() map[string]any {
	cfg := s.Settings.Get()
	lib := library.Library{Root: cfg.LibraryDir}
	info := map[string]any{"dir": cfg.LibraryDir, "defaultDir": settings.DefaultLibraryDir(), "configured": cfg.LibraryDir != "", "hasIndex": false}
	if idx, _, err := s.loadIndex(); err == nil {
		ready, listed := 0, 0
		for _, p := range idx.Plays {
			if !p.Removed {
				listed++
				if p.Ready {
					ready++
				}
			}
		}
		info["hasIndex"], info["syncedAt"], info["complete"], info["plays"], info["ready"] = true, idx.SyncedAt, idx.Complete, listed, ready
	}
	target := lib.Root
	if target == "" {
		target = settings.DefaultLibraryDir()
	}
	if free, ok := syncer.FreeSpace(target); ok {
		info["free"] = free
	}
	return map[string]any{
		"version":        s.Version,
		"platform":       runtime.GOOS,
		"worker":         s.Worker.Available(),
		"native":         s.ChooseDir != nil,
		"driver":         s.driverStatus(),
		"language":       cfg.Language,
		"systemLanguage": s.systemLanguage,
		"library":        info,
		"sync":           s.Sync.Snapshot(),
		"update":         s.Sync.Update(),
		"fetch":          s.Sync.Fetch(),
		"ready":          readyVersion(s.Sync),
		"flash":          s.Flash.Snapshot(),
		"restore": map[string]any{
			"backupDir":        cfg.Backups(),
			"defaultBackupDir": settings.DefaultBackupDir(),
			"firmware":         factory.Version,
		},
	}
}

func (s *Server) setLibrary(dir, mode string) (string, error) {
	if s.Sync.Running() || s.Flash.Active() {
		return "busy", errors.New("another task is running")
	}
	dir = strings.TrimSpace(dir)
	if dir == "" || !filepath.IsAbs(dir) {
		return "path", errors.New("choose an absolute folder path")
	}
	dir = filepath.Clean(dir)
	cur := s.Settings.Get().LibraryDir
	commit := func() error {
		_, err := s.Settings.Update(func(c *settings.Settings) { c.LibraryDir = dir })
		s.Invalidate()
		s.Sync.ResetReady()
		return err
	}
	curLib := library.Library{Root: cur}
	hasData := cur != "" && (curLib.HasIndex() || dirExists(filepath.Join(cur, "plays")))
	if mode == "move" && hasData && cur != dir {
		if err := s.Sync.Move(cur, dir, commit); err != nil {
			if errors.Is(err, syncer.ErrTargetNotEmpty) {
				return "not_empty", err
			}
			return "path", err
		}
		return "", nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "path", err
	}
	if err := commit(); err != nil {
		return "other", err
	}
	return "", nil
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func (s *Server) play(slug string) (*library.Play, error) {
	if !library.ValidSlug(slug) {
		return nil, errors.New("invalid play")
	}
	idx, _, err := s.loadIndex()
	if err != nil {
		return nil, err
	}
	for i := range idx.Plays {
		if idx.Plays[i].Slug == slug {
			return &idx.Plays[i], nil
		}
	}
	return nil, errors.New("play not found")
}

func (s *Server) startFlash(slug, port string) (string, error) {
	if s.Sync.Running() && s.Sync.Snapshot().Kind == "move" {
		return "busy", errors.New("library is being moved")
	}
	if _, err := s.play(slug); err != nil {
		return "not_found", err
	}
	sha, size, ok := syncer.LocalFirmware(s.library(), slug)
	if !ok {
		return "not_ready", errors.New("firmware not downloaded")
	}
	if port == "" {
		return "port", errors.New("choose a serial port")
	}
	job := flasher.Job{Slug: slug, Port: port, Image: s.library().FirmwarePath(slug), SHA256: sha, Size: size, Baud: 460800,
		BackupDir: s.Settings.Get().Backups()}
	if err := s.Flash.Start(job); err != nil {
		return "busy", err
	}
	return "", nil
}

func (s *Server) startRestore(fullBackup bool) (string, error) {
	if s.StateDir == "" {
		return "firmware", errors.New("no folder for the factory firmware")
	}
	fw, err := factory.Extract(filepath.Join(s.StateDir, "factory-firmware"))
	if err != nil {
		return "firmware", err
	}
	job := flasher.RestoreJob{FirmwareDir: fw, BackupDir: s.Settings.Get().Backups(), FullBackup: fullBackup}
	if err := os.MkdirAll(job.BackupDir, 0o755); err != nil {
		return "backup_dir", err
	}
	if err := s.Flash.StartRestore(job); err != nil {
		return "busy", err
	}
	return "", nil
}

// startBackup saves the whole flash of the connected device; it never writes to it.
func (s *Server) startBackup() (string, error) {
	job := flasher.RestoreJob{BackupDir: s.Settings.Get().Backups(), Backup: true}
	if err := os.MkdirAll(job.BackupDir, 0o755); err != nil {
		return "backup_dir", err
	}
	if err := s.Flash.StartRestore(job); err != nil {
		return "busy", err
	}
	return "", nil
}

func (s *Server) setBackupDir(dir string) (string, error) {
	if s.Flash.Active() {
		return "busy", errors.New("a device job is running")
	}
	dir = strings.TrimSpace(dir)
	if dir == "" || !filepath.IsAbs(dir) {
		return "path", errors.New("choose an absolute folder path")
	}
	dir = filepath.Clean(dir)
	if dir == filepath.Clean(settings.DefaultBackupDir()) {
		dir = ""
	}
	_, err := s.Settings.Update(func(c *settings.Settings) { c.BackupDir = dir })
	return "other", err
}

func (s *Server) reveal(target, slug string) error {
	if s.Reveal == nil {
		return errors.New("not available")
	}
	switch target {
	case "backup":
		dir := s.Settings.Get().Backups()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		return s.Reveal(dir)
	case "restore-session":
		// Only the folder the last restore reported; never a path from the page.
		dir := s.Flash.Snapshot().Session
		if dir == "" {
			return errors.New("no restore backup yet")
		}
		return s.Reveal(dir)
	}
	lib := s.library()
	if lib.Root == "" {
		return errors.New("no library")
	}
	if target == "play" {
		if !library.ValidSlug(slug) {
			return errors.New("invalid play")
		}
		return s.Reveal(lib.PlayDir(slug))
	}
	return s.Reveal(lib.Root)
}

func (s *Server) openWeb(slug string) error {
	if s.OpenURL == nil {
		return errors.New("not available")
	}
	p, err := s.play(slug)
	if err != nil {
		return err
	}
	return s.OpenURL(s.Sync.Client.Origin + "/plays/" + strconv.Itoa(p.ID) + "/")
}

func readyVersion(m *syncer.Manager) int {
	_, v := m.Ready()
	return v
}

// driverStatus is "" when the edition has no driver support.
func (s *Server) driverStatus() string {
	if s.DriverStatus == nil {
		return ""
	}
	return s.DriverStatus()
}
