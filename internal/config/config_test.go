package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFromPathMissingFileReturnsDefault(t *testing.T) {
	t.Parallel()

	cfg, err := loadFromPath(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("loadFromPath returned error: %v", err)
	}
	if cfg != Default() {
		t.Fatalf("expected default config, got %+v", cfg)
	}
}

func TestSaveToPathAndLoadFromPathRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "cfg", "config.json")
	want := Config{
		Context:   "dev",
		Namespace: "team-a",
	}

	if err := saveToPath(path, want); err != nil {
		t.Fatalf("saveToPath returned error: %v", err)
	}

	got, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("loadFromPath returned error: %v", err)
	}
	if got != want {
		t.Fatalf("loaded config mismatch: got %+v want %+v", got, want)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("unexpected config permissions: got %o want 600", perm)
	}
}

func TestLoadFromPathInvalidJSONWrapsError(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{not-json"), 0o600); err != nil {
		t.Fatalf("write bad config: %v", err)
	}

	_, err := loadFromPath(path)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "decode config") {
		t.Fatalf("expected wrapped decode error, got: %v", err)
	}
}

func TestSaveAndLoadUseDefaultPathInHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	want := Config{
		Context:   "prod",
		Namespace: "team-b",
	}
	if err := Save(want); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if got != want {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}

	path := filepath.Join(home, ".kontrol", "config.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected config file at default path: %v", err)
	}
}

func TestLoadReturnsDefaultWhenConfigAbsentAtDefaultPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	got, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if got != Default() {
		t.Fatalf("Load() = %+v, want default %+v", got, Default())
	}
}
