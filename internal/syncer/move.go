package syncer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/FoloToy/ai-passport-plays/internal/library"
)

// ErrTargetNotEmpty means the destination already holds other files.
var ErrTargetNotEmpty = errors.New("target folder is not empty")

func isEmptyDir(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if e.Name() != ".DS_Store" {
			return false, nil
		}
	}
	return true, nil
}

func within(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// CheckMoveTarget validates a destination before any work starts.
func CheckMoveTarget(from, to string) error {
	from, _ = filepath.Abs(from)
	to, _ = filepath.Abs(to)
	if from == to {
		return errors.New("same folder")
	}
	if within(to, from) || within(from, to) {
		return errors.New("folders must not contain each other")
	}
	empty, err := isEmptyDir(to)
	if err != nil {
		return err
	}
	if !empty {
		return ErrTargetNotEmpty
	}
	return nil
}

// Move relocates the library from one folder to another in the background.
// commit runs after the data is in place and before the old copy is removed.
func (m *Manager) Move(from, to string, commit func() error) error {
	if err := CheckMoveTarget(from, to); err != nil {
		return err
	}
	ctx, err := m.begin("move")
	if err != nil {
		return err
	}
	go func() {
		if err := m.move(ctx, from, to, commit); err != nil {
			if ctx.Err() != nil {
				m.finish("cancelled", "", nil)
				return
			}
			m.finish("failed", CodeTarget, err)
			return
		}
		if m.OnIndex != nil {
			m.OnIndex()
		}
		m.finish("done", "", nil)
	}()
	return nil
}

func (m *Manager) move(ctx context.Context, from, to string, commit func() error) error {
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(to, ".DS_Store"))
	_ = os.Remove(to) // only succeeds when empty; rename needs the target absent
	if err := os.Rename(from, to); err == nil {
		return commit()
	}
	// Different volume: copy, switch, then delete the old copy.
	total, files := library.Library{Root: from}.Usage()
	m.set(func(s *State) { s.BytesTotal = total; s.Total = files })
	if err := m.copyTree(ctx, from, to); err != nil {
		_ = os.RemoveAll(to)
		return err
	}
	if err := commit(); err != nil {
		_ = os.RemoveAll(to)
		return err
	}
	return os.RemoveAll(from)
}

func (m *Manager) copyTree(ctx context.Context, from, to string) error {
	return filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		rel, err := filepath.Rel(from, p)
		if err != nil {
			return err
		}
		dest := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if err := copyFile(p, dest, &countingWriter{m: m}); err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		m.set(func(s *State) { s.Done++ })
		return nil
	})
}

type countingWriter struct{ m *Manager }

func (c *countingWriter) Write(p []byte) (int, error) {
	c.m.bytes.Add(int64(len(p)))
	return len(p), nil
}

func copyFile(src, dst string, progress io.Writer) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	_, err = io.Copy(io.MultiWriter(out, progress), in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}
