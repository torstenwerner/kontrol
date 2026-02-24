package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadFromPathMissingFileReturnsDefault(t *testing.T) {
	t.Parallel()

	cfg, err := loadFromPath(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("loadFromPath returned error: %v", err)
	}
	if !reflect.DeepEqual(cfg, Default()) {
		t.Fatalf("expected default config, got %+v", cfg)
	}
}

func TestSaveToPathAndLoadFromPathRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "cfg", "config.json")
	want := Config{
		Context:   "dev",
		Namespace: "team-a",
		NamespacesByContext: map[string]string{
			"dev": "team-a",
		},
	}

	if err := saveToPath(path, want); err != nil {
		t.Fatalf("saveToPath returned error: %v", err)
	}

	got, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("loadFromPath returned error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
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
		NamespacesByContext: map[string]string{
			"prod": "team-b",
		},
	}
	if err := Save(want); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
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
	if !reflect.DeepEqual(got, Default()) {
		t.Fatalf("Load() = %+v, want default %+v", got, Default())
	}
}

func TestLoadFromPathBackfillsNamespacesByContext(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"context":"dev","namespace":"team-a"}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("loadFromPath returned error: %v", err)
	}

	want := Config{
		Context:   "dev",
		Namespace: "team-a",
		NamespacesByContext: map[string]string{
			"dev": "team-a",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadFromPathUsesNamespaceFromContextMap(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"context":"dev","namespace":"old","namespaces_by_context":{"dev":"team-a"}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("loadFromPath returned error: %v", err)
	}

	if got.Namespace != "team-a" {
		t.Fatalf("Namespace = %q, want %q", got.Namespace, "team-a")
	}
}

func TestLoadFromPathBackfillsEmptyMappedNamespaceFromLegacyField(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"context":"dev","namespace":"team-a","namespaces_by_context":{"dev":""}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("loadFromPath returned error: %v", err)
	}
	if got.Namespace != "team-a" {
		t.Fatalf("Namespace = %q, want %q", got.Namespace, "team-a")
	}
	if got.NamespacesByContext["dev"] != "team-a" {
		t.Fatalf("NamespacesByContext[dev] = %q, want %q", got.NamespacesByContext["dev"], "team-a")
	}
}

func TestLoadFromPathKeepsLegacyNamespaceWhenContextMissing(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"namespace":"team-a"}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("loadFromPath returned error: %v", err)
	}
	if got.Context != "" {
		t.Fatalf("Context = %q, want empty", got.Context)
	}
	if got.Namespace != "team-a" {
		t.Fatalf("Namespace = %q, want %q", got.Namespace, "team-a")
	}
	if len(got.NamespacesByContext) != 0 {
		t.Fatalf("NamespacesByContext = %+v, want empty", got.NamespacesByContext)
	}
}
