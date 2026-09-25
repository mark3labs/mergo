package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissing(t *testing.T) {
	s, err := LoadFrom(filepath.Join(t.TempDir(), "nope", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if s != (Settings{}) {
		t.Fatalf("got %+v, want zero settings", s)
	}
}

func TestUpdateRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "mergo", "config.json")
	if err := UpdateAt(p, func(s *Settings) { s.Theme = "dracula" }); err != nil {
		t.Fatal(err)
	}
	s, err := LoadFrom(p)
	if err != nil {
		t.Fatal(err)
	}
	if s.Theme != "dracula" {
		t.Fatalf("theme = %q, want dracula", s.Theme)
	}
	entries, err := os.ReadDir(filepath.Dir(p))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("leftover temp files: %v", entries)
	}
}

func TestUpdatePreservesUnknownKeys(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(`{"theme":"nord","future":{"x":1}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateAt(p, func(s *Settings) {
		if s.Theme != "nord" {
			t.Errorf("mutate saw theme %q, want nord", s.Theme)
		}
		s.Theme = "gruvbox"
	}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["theme"] != "gruvbox" || raw["future"] == nil {
		t.Fatalf("got %s", data)
	}
}

func TestLoadInvalid(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFrom(p); err == nil {
		t.Fatal("expected an error for invalid JSON")
	}
}

func TestPathXDG(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "mergo", "config.json"); p != want {
		t.Fatalf("path = %q, want %q", p, want)
	}
	if err := Update(func(s *Settings) { s.Theme = "dark" }); err != nil {
		t.Fatal(err)
	}
	if s, err := Load(); err != nil || s.Theme != "dark" {
		t.Fatalf("got %+v, %v", s, err)
	}
}

func TestPathRelativeXDGIgnored(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "relative/dir")
	p, err := Path()
	if err != nil {
		t.Skip("no user config dir:", err)
	}
	if !filepath.IsAbs(p) {
		t.Fatalf("path = %q, want an absolute path", p)
	}
}
