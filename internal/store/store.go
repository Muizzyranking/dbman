// Package store persists managed-database metadata in
// ~/.cache/dbman/instances.json — the facts Docker can't provide
// (passwords, friendly names, port mappings).
package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Instance is the persisted metadata for one managed database.
// Docker holds runtime state; this holds what `docker inspect` can't:
// password, logical type, friendly name.
type Instance struct {
	Name          string    `json:"name"`
	Type          string    `json:"type"` // postgres | redis | mysql | mongo
	ContainerName string    `json:"container_name"`
	Image         string    `json:"image"`
	Host          string    `json:"host"`
	Port          int       `json:"port"`
	User          string    `json:"user,omitempty"`
	Password      string    `json:"password,omitempty"`
	Database      string    `json:"database,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

func Path() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		home, herr := os.UserHomeDir()
		if herr != nil {
			return "", err
		}
		base = filepath.Join(home, ".cache")
	}
	return filepath.Join(base, "dbman", "instances.json"), nil
}

func Load() ([]Instance, string, error) {
	p, err := Path()
	if err != nil {
		return nil, "", err
	}
	if _, err := os.Stat(p); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return nil, p, err
		}
		if err := os.WriteFile(p, []byte("[]"), 0o644); err != nil {
			return nil, p, err
		}
		return []Instance{}, p, nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, p, err
	}
	if len(data) == 0 {
		return []Instance{}, p, nil
	}
	var list []Instance
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, p, err
	}
	if list == nil {
		list = []Instance{}
	}
	return list, p, nil
}

func Save(list []Instance) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

func Find(list []Instance, name string) *Instance {
	for i := range list {
		if list[i].Name == name {
			return &list[i]
		}
	}
	return nil
}

func Upsert(list []Instance, inst Instance) []Instance {
	for i := range list {
		if list[i].Name == inst.Name {
			list[i] = inst
			return list
		}
	}
	return append(list, inst)
}

func Remove(list []Instance, name string) []Instance {
	out := list[:0]
	for _, v := range list {
		if v.Name != name {
			out = append(out, v)
		}
	}
	return out
}
