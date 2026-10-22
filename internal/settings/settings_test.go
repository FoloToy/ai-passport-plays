package settings

import "testing"

func TestUpdatePersists(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Get().Language != "auto" {
		t.Fatal("default language should be auto")
	}
	if _, err := s.Update(func(c *Settings) { c.Language = "en"; c.LibraryDir = "/x" }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(func(c *Settings) { c.Language = "fr" }); err == nil {
		t.Fatal("invalid language accepted")
	}
	again, _ := Open(dir)
	if got := again.Get(); got.Language != "en" || got.LibraryDir != "/x" {
		t.Fatalf("not persisted: %+v", got)
	}
}

func TestLanguageFrom(t *testing.T) {
	for in, want := range map[string]string{"zh-Hans-CN": "zh", "zh_TW": "zh", "en-CN": "en", "": ""} {
		if got := languageFrom(in); got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
}
