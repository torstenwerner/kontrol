package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	configDirName  = ".kontrol"
	configFileName = "config.json"
)

// Config stores persisted selections.
type Config struct {
	Context             string            `json:"context"`
	Namespace           string            `json:"namespace"`
	NamespacesByContext map[string]string `json:"namespaces_by_context,omitempty"`
}

// Default returns the default config values.
func Default() Config {
	return Config{
		NamespacesByContext: map[string]string{},
	}
}

// Load reads persisted config from ~/.kontrol/config.json.
// If the file does not exist, Default() is returned.
func Load() (Config, error) {
	path, err := defaultPath()
	if err != nil {
		return Default(), err
	}
	return loadFromPath(path)
}

// Save persists config to ~/.kontrol/config.json.
func Save(cfg Config) error {
	path, err := defaultPath()
	if err != nil {
		return err
	}
	return saveToPath(path, cfg)
}

func defaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return filepath.Join(home, configDirName, configFileName), nil
}

func loadFromPath(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Default(), nil
		}
		return Default(), fmt.Errorf("read config %q: %w", path, err)
	}

	cfg := Default()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Default(), fmt.Errorf("decode config %q: %w", path, err)
	}
	normalize(&cfg)
	return cfg, nil
}

func saveToPath(path string, cfg Config) error {
	normalize(&cfg)

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir %q: %w", filepath.Dir(path), err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config %q: %w", path, err)
	}
	return nil
}

func normalize(cfg *Config) {
	if cfg.NamespacesByContext == nil {
		cfg.NamespacesByContext = map[string]string{}
	}
	if cfg.Context == "" {
		return
	}
	if namespace, ok := cfg.NamespacesByContext[cfg.Context]; ok && namespace != "" {
		cfg.Namespace = namespace
		return
	}
	if cfg.Namespace != "" {
		cfg.NamespacesByContext[cfg.Context] = cfg.Namespace
	}
}
