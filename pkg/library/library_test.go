package library

import (
	"path/filepath"
	"testing"
)

func TestValidSlug(t *testing.T) {
	for _, s := range []string{"community-457c1b43", "muse-passport-2", "100-3", "ai-31"} {
		if !ValidSlug(s) {
			t.Errorf("%q should be valid", s)
		}
	}
	for _, s := range []string{"", "..", "../x", "a/b", "A", "x..y", ".hidden"} {
		if ValidSlug(s) {
			t.Errorf("%q should be invalid", s)
		}
	}
}

func TestResolveStaysInsideLibrary(t *testing.T) {
	l := Library{Root: "/lib"}
	ok := map[string]string{
		"plays/a/cover.webp":     "/lib/plays/a/cover.webp",
		"avatars/x.webp":         "/lib/avatars/x.webp",
		"plays/a/gallery/0.webp": "/lib/plays/a/gallery/0.webp",
	}
	for in, want := range ok {
		got, valid := l.Resolve(in)
		if !valid || got != filepath.FromSlash(want) {
			t.Errorf("Resolve(%q) = %q %v", in, got, valid)
		}
	}
	for _, in := range []string{"../etc/passwd.webp", "plays/a/firmware.bin", "index.json", "settings.webp", "plays/../../x.webp"} {
		if got, valid := l.Resolve(in); valid {
			t.Errorf("Resolve(%q) should be refused, got %q", in, got)
		}
	}
}
