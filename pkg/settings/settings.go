// Package settings stores the small amount of private app configuration.
// Plays, images and firmware never live here; they live in the library directory.
package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

type Settings struct {
	// LibraryDir is where plays, images and firmware are stored. Empty until the user confirms one.
	LibraryDir string `json:"libraryDir"`
	// Language is "auto", "zh" or "en".
	Language string `json:"language"`
	// BackupDir keeps factory-restore backups and identity-area copies. Empty means DefaultBackupDir.
	BackupDir string `json:"backupDir,omitempty"`
}

// DefaultBackupDir matches the standalone factory restore tool, so earlier backups stay together.
func DefaultBackupDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	root := filepath.Join(home, "Documents")
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		root = home
	}
	return filepath.Join(root, "FoloToy恢复出厂备份")
}

// Backups returns the folder to use for device backups.
func (c Settings) Backups() string {
	if c.BackupDir != "" {
		return c.BackupDir
	}
	return DefaultBackupDir()
}

type Store struct {
	path string
	mu   sync.Mutex
	cur  Settings
}

// DefaultLibraryDir is suggested on first launch.
func DefaultLibraryDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Documents", "AI Passport Plays")
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{path: filepath.Join(dir, "settings.json"), cur: Settings{Language: "auto"}}
	b, err := os.ReadFile(s.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		var loaded Settings
		if json.Unmarshal(b, &loaded) == nil {
			s.cur = loaded
		}
	}
	if !validLanguage(s.cur.Language) {
		s.cur.Language = "auto"
	}
	return s, nil
}

func validLanguage(l string) bool { return l == "auto" || l == "zh" || l == "en" }

func (s *Store) Get() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur
}

func (s *Store) Update(change func(*Settings)) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.cur
	change(&next)
	if !validLanguage(next.Language) {
		return s.cur, errors.New("invalid language")
	}
	if err := WriteJSONAtomic(s.path, next); err != nil {
		return s.cur, err
	}
	s.cur = next
	return next, nil
}

// WriteJSONAtomic writes v to path through a temporary file and rename,
// so readers never see a half-written file.
func WriteJSONAtomic(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}
