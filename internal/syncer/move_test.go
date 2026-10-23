package syncer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMoveRestoresSourceWhenSettingsCannotBeSaved(t *testing.T) {
	root := t.TempDir()
	from, to := filepath.Join(root, "from"), filepath.Join(root, "to")
	if err := os.MkdirAll(from, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(from, "index.json"), []byte("catalog"), 0o644); err != nil {
		t.Fatal(err)
	}
	want := errors.New("settings disk full")
	m := New(nil)
	if err := m.move(context.Background(), from, to, func() error { return want }); !errors.Is(err, want) {
		t.Fatalf("got %v, want settings failure", err)
	}
	if b, err := os.ReadFile(filepath.Join(from, "index.json")); err != nil || string(b) != "catalog" {
		t.Fatalf("original library unavailable after failed move: %q, %v", b, err)
	}
	if _, err := os.Stat(to); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected destination after rollback: %v", err)
	}
}
