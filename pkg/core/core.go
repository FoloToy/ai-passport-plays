// Package core holds the app logic shared by every front end: the desktop app's
// local JSON API (internal/desktop) and the native Windows 7 client
// (FoloToy/ai-passport-desktop-windows7). It owns the settings, the play library
// index, the sync manager and the device job manager, and applies the same checks
// whichever front end calls it.
package core

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/FoloToy/ai-passport-desktop/pkg/device"
	"github.com/FoloToy/ai-passport-desktop/pkg/factory"
	"github.com/FoloToy/ai-passport-desktop/pkg/flasher"
	"github.com/FoloToy/ai-passport-desktop/pkg/library"
	"github.com/FoloToy/ai-passport-desktop/pkg/settings"
	"github.com/FoloToy/ai-passport-desktop/pkg/syncer"
)

// Error carries a stable code the UI translates (err_<code> in the desktop UI).
type Error struct {
	Code string
	Err  error
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

func fail(code string, err error) error { return &Error{Code: code, Err: err} }

// Code returns the error's code, or "other".
func Code(err error) string {
	var e *Error
	if errors.As(err, &e) && e.Code != "" {
		return e.Code
	}
	return "other"
}

type Core struct {
	Version  string
	Settings *settings.Store
	Sync     *syncer.Manager
	Flash    *flasher.Manager
	Runner   device.Runner
	// StateDir is the private app folder; the factory firmware is unpacked under it.
	StateDir string

	mu       sync.Mutex
	catalog  []byte
	index    *library.Index
	indexDir string
	onIndex  []func()
}

// New wires the managers together. stateDir is the private settings folder.
func New(version, stateDir string, st *settings.Store, sm *syncer.Manager, fm *flasher.Manager, r device.Runner) *Core {
	c := &Core{Version: version, StateDir: stateDir, Settings: st, Sync: sm, Flash: fm, Runner: r}
	sm.OnIndex = c.Invalidate
	return c
}

// OnIndexChange registers f to run (on a background goroutine) whenever the
// library index is reloaded after a sync or a library change.
func (c *Core) OnIndexChange(f func()) {
	c.mu.Lock()
	c.onIndex = append(c.onIndex, f)
	c.mu.Unlock()
}

func (c *Core) Library() library.Library { return library.Library{Root: c.Settings.Get().LibraryDir} }

// Invalidate drops the cached index after a sync or library move.
func (c *Core) Invalidate() {
	c.mu.Lock()
	c.catalog, c.index, c.indexDir = nil, nil, ""
	hooks := append([]func(){}, c.onIndex...)
	c.mu.Unlock()
	for _, f := range hooks {
		f()
	}
}

// Index returns the library index and its JSON encoding, cached until Invalidate.
// It returns os.ErrNotExist when no library folder is set.
func (c *Core) Index() (*library.Index, []byte, error) {
	lib := c.Library()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.index != nil && c.indexDir == lib.Root {
		return c.index, c.catalog, nil
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
	c.index, c.catalog, c.indexDir = idx, b, lib.Root
	return idx, b, nil
}

// CheckUpdatesInBackground asks the community whether newer plays exist.
func (c *Core) CheckUpdatesInBackground() {
	idx, _, err := c.Index()
	if err != nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		c.Sync.CheckUpdates(ctx, idx)
	}()
}

// LibraryInfo summarises the library for status displays.
type LibraryInfo struct {
	Dir        string    `json:"dir"`
	DefaultDir string    `json:"defaultDir"`
	Configured bool      `json:"configured"`
	HasIndex   bool      `json:"hasIndex"`
	SyncedAt   time.Time `json:"syncedAt,omitzero"`
	Complete   bool      `json:"complete"`
	Plays      int       `json:"plays"`
	Ready      int       `json:"ready"`
	Free       uint64    `json:"free,omitempty"`
}

func (c *Core) LibraryInfo() LibraryInfo {
	dir := c.Settings.Get().LibraryDir
	info := LibraryInfo{Dir: dir, DefaultDir: settings.DefaultLibraryDir(), Configured: dir != ""}
	if idx, _, err := c.Index(); err == nil {
		info.HasIndex, info.SyncedAt, info.Complete = true, idx.SyncedAt, idx.Complete
		for _, p := range idx.Plays {
			if !p.Removed {
				info.Plays++
				if p.Ready {
					info.Ready++
				}
			}
		}
	}
	target := dir
	if target == "" {
		target = settings.DefaultLibraryDir()
	}
	info.Free, _ = syncer.FreeSpace(target)
	return info
}

// StartSync downloads or updates the whole library.
func (c *Core) StartSync() error {
	if c.Flash.Active() {
		return fail("busy", errors.New("flashing in progress"))
	}
	dir := c.Settings.Get().LibraryDir
	if dir == "" {
		return fail("no_library", errors.New("choose a library folder first"))
	}
	if err := c.Sync.Start(dir); err != nil {
		return fail(err.Error(), err)
	}
	return nil
}

// FetchFirmware downloads one play's firmware on demand.
func (c *Core) FetchFirmware(slug string) error {
	if err := c.Sync.FetchOne(c.Settings.Get().LibraryDir, slug); err != nil {
		if errors.Is(err, syncer.ErrNoFirmware) {
			return fail("no_firmware", err)
		}
		return fail(err.Error(), err)
	}
	return nil
}

// SetLibrary switches to dir, or moves the current library there when mode is "move".
func (c *Core) SetLibrary(dir, mode string) error {
	if c.Sync.Running() || c.Flash.Active() {
		return fail("busy", errors.New("another task is running"))
	}
	dir = strings.TrimSpace(dir)
	if dir == "" || !filepath.IsAbs(dir) {
		return fail("path", errors.New("choose an absolute folder path"))
	}
	dir = filepath.Clean(dir)
	cur := c.Settings.Get().LibraryDir
	commit := func() error {
		_, err := c.Settings.Update(func(s *settings.Settings) { s.LibraryDir = dir })
		c.Invalidate()
		c.Sync.ResetReady()
		return err
	}
	curLib := library.Library{Root: cur}
	hasData := cur != "" && (curLib.HasIndex() || dirExists(filepath.Join(cur, "plays")))
	if mode == "move" && hasData && cur != dir {
		if err := c.Sync.Move(cur, dir, commit); err != nil {
			if errors.Is(err, syncer.ErrTargetNotEmpty) {
				return fail("not_empty", err)
			}
			return fail("path", err)
		}
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fail("path", err)
	}
	if err := commit(); err != nil {
		return fail("other", err)
	}
	return nil
}

// HasLibraryData reports whether the current library folder holds plays, so a
// front end can ask whether to move them when the folder changes.
func (c *Core) HasLibraryData() bool {
	cur := c.Settings.Get().LibraryDir
	return cur != "" && (library.Library{Root: cur}.HasIndex() || dirExists(filepath.Join(cur, "plays")))
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// Play finds a play in the index by slug.
func (c *Core) Play(slug string) (*library.Play, error) {
	if !library.ValidSlug(slug) {
		return nil, errors.New("invalid play")
	}
	idx, _, err := c.Index()
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

// FirmwareReady reports whether the play's firmware has been downloaded and checked.
func (c *Core) FirmwareReady(slug string) bool {
	_, _, ok := syncer.LocalFirmware(c.Library(), slug)
	return ok
}

// StartFlash writes a downloaded play to the device on port.
func (c *Core) StartFlash(slug, port string) error {
	if c.Sync.Running() && c.Sync.Snapshot().Kind == "move" {
		return fail("busy", errors.New("library is being moved"))
	}
	if _, err := c.Play(slug); err != nil {
		return fail("not_found", err)
	}
	sha, size, ok := syncer.LocalFirmware(c.Library(), slug)
	if !ok {
		return fail("not_ready", errors.New("firmware not downloaded"))
	}
	if port == "" {
		return fail("port", errors.New("choose a serial port"))
	}
	job := flasher.Job{Slug: slug, Port: port, Image: c.Library().FirmwarePath(slug), SHA256: sha, Size: size, Baud: 460800,
		BackupDir: c.Settings.Get().Backups()}
	if err := c.Flash.Start(job); err != nil {
		return fail("busy", err)
	}
	return nil
}

// StartRestore restores the bundled factory firmware. The flow never writes the
// cardid identity area; see pkg/device and pkg/factory.
func (c *Core) StartRestore(fullBackup bool) error {
	if c.StateDir == "" {
		return fail("firmware", errors.New("no folder for the factory firmware"))
	}
	fw, err := factory.Extract(filepath.Join(c.StateDir, "factory-firmware"))
	if err != nil {
		return fail("firmware", err)
	}
	job := flasher.RestoreJob{FirmwareDir: fw, BackupDir: c.Settings.Get().Backups(), FullBackup: fullBackup}
	if err := os.MkdirAll(job.BackupDir, 0o755); err != nil {
		return fail("backup_dir", err)
	}
	if err := c.Flash.StartRestore(job); err != nil {
		return fail("busy", err)
	}
	return nil
}

// StartBackup saves the whole flash of the connected device; it never writes to it.
func (c *Core) StartBackup() error {
	job := flasher.RestoreJob{BackupDir: c.Settings.Get().Backups(), Backup: true}
	if err := os.MkdirAll(job.BackupDir, 0o755); err != nil {
		return fail("backup_dir", err)
	}
	if err := c.Flash.StartRestore(job); err != nil {
		return fail("busy", err)
	}
	return nil
}

// CancelRestore stops a restore or backup; it fails once writing has started.
func (c *Core) CancelRestore() error {
	if !c.Flash.Cancel() {
		return fail("not_cancellable", errors.New("the restore can no longer be stopped"))
	}
	return nil
}

// SetBackupDir changes where backups go; the default folder is stored as "".
func (c *Core) SetBackupDir(dir string) error {
	if c.Flash.Active() {
		return fail("busy", errors.New("a device job is running"))
	}
	dir = strings.TrimSpace(dir)
	if dir == "" || !filepath.IsAbs(dir) {
		return fail("path", errors.New("choose an absolute folder path"))
	}
	dir = filepath.Clean(dir)
	if dir == filepath.Clean(settings.DefaultBackupDir()) {
		dir = ""
	}
	if _, err := c.Settings.Update(func(s *settings.Settings) { s.BackupDir = dir }); err != nil {
		return fail("other", err)
	}
	return nil
}

// SetLanguage stores "zh", "en" or "" (follow the system).
func (c *Core) SetLanguage(lang string) error {
	_, err := c.Settings.Update(func(s *settings.Settings) { s.Language = lang })
	return err
}

// RevealPath resolves a folder to show: "backup", "restore-session", "play" (with
// slug) or the library root. It never accepts a path from the caller.
func (c *Core) RevealPath(target, slug string) (string, error) {
	switch target {
	case "backup":
		dir := c.Settings.Get().Backups()
		return dir, os.MkdirAll(dir, 0o755)
	case "restore-session":
		// Only the folder the last restore reported.
		dir := c.Flash.Snapshot().Session
		if dir == "" {
			return "", errors.New("no restore backup yet")
		}
		return dir, nil
	}
	lib := c.Library()
	if lib.Root == "" {
		return "", errors.New("no library")
	}
	if target == "play" {
		if !library.ValidSlug(slug) {
			return "", errors.New("invalid play")
		}
		return lib.PlayDir(slug), nil
	}
	return lib.Root, nil
}

// PlayURL is the play's page on the community site.
func (c *Core) PlayURL(slug string) (string, error) {
	p, err := c.Play(slug)
	if err != nil {
		return "", err
	}
	return c.Sync.Client.Origin + "/plays/" + strconv.Itoa(p.ID) + "/", nil
}

// ReadyVersion changes whenever the set of downloaded firmware changes.
func (c *Core) ReadyVersion() int {
	_, v := c.Sync.Ready()
	return v
}
