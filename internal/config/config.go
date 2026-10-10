// Package config loads and saves ~/.config/dbman/config.json, creating it
// with defaults on first run.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Theme struct {
	Name    string `json:"name"`
	Accent  string `json:"accent"`
	Success string `json:"success"`
	Warning string `json:"warning"`
	Error   string `json:"error"`
	Muted   string `json:"muted"`
}

type DBDefaults struct {
	Image   string   `json:"image"`
	Options []string `json:"options"`
	User    string   `json:"user,omitempty"`
	DB      string   `json:"db,omitempty"`
}

type Config struct {
	Theme          Theme                 `json:"theme"`
	PortStrategy   string                `json:"port_strategy"` // "standard_first" | "range"
	PortRangeStart int                   `json:"port_range_start"`
	DockerNetwork  string                `json:"docker_network"`
	Icons          bool                  `json:"icons"`
	Defaults       map[string]DBDefaults `json:"defaults"`
}

func DefaultConfig() Config {
	return Config{
		Theme: Theme{
			Name:    "dark",
			Accent:  "#7C6CFF",
			Success: "#3FB950",
			Warning: "#D29922",
			Error:   "#F85149",
			Muted:   "#8B949E",
		},
		PortStrategy:   "standard_first",
		PortRangeStart: 55000,
		DockerNetwork:  "",
		Icons:          true,
		Defaults: map[string]DBDefaults{
			"postgres": {
				Image:   "postgres:16-alpine",
				Options: []string{"postgres:16-alpine", "pgvector/pgvector:pg16", "postgres:15-alpine"},
				User:    "postgres",
				DB:      "postgres",
			},
			"redis": {
				Image:   "redis:7-alpine",
				Options: []string{"redis:7-alpine", "redis:7", "valkey/valkey:8"},
			},
			"mysql": {
				Image:   "mysql:8",
				Options: []string{"mysql:8", "mariadb:11", "mysql:8.0"},
				User:    "root",
				DB:      "app",
			},
			"mongo": {
				Image:   "mongo:7",
				Options: []string{"mongo:7", "mongo:6"},
				User:    "root",
				DB:      "app",
			},
		},
	}
}

func Path() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		home, herr := os.UserHomeDir()
		if herr != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "dbman", "config.json"), nil
}

// Load creates a default config on first run.
func Load() (Config, string, error) {
	p, err := Path()
	if err != nil {
		return DefaultConfig(), "", err
	}
	if _, err := os.Stat(p); os.IsNotExist(err) {
		cfg := DefaultConfig()
		if err := Save(cfg); err != nil {
			return cfg, p, err
		}
		return cfg, p, nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return DefaultConfig(), p, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig(), p, err
	}
	// icons defaults to true; only false when explicitly set.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err == nil {
		if _, ok := raw["icons"]; !ok {
			cfg.Icons = true
		}
	}
	if cfg.Defaults == nil {
		def := DefaultConfig()
		cfg.Defaults = def.Defaults
	}
	if cfg.PortRangeStart == 0 {
		cfg.PortRangeStart = 55000
	}
	if cfg.PortStrategy == "" {
		cfg.PortStrategy = "standard_first"
	}
	return cfg, p, nil
}

func Save(cfg Config) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}
