// Package adapters defines the per-engine Adapter interface and registry.
// Each database engine implements Adapter once and self-registers, so new
// databases plug in without touching the TUI, CLI, or Docker layers.
package adapters

import (
	"fmt"
	"strings"

	"github.com/muizzyranking/dbman/internal/config"
	"github.com/muizzyranking/dbman/internal/docker"
	"github.com/muizzyranking/dbman/internal/store"
)

// Adapter is implemented once per database engine. To add a new DB,
// implement this interface and register it in Registry below.
type Adapter interface {
	// Name is the canonical type key, e.g. "postgres".
	Name() string
	// Aliases match extra --type values to this adapter, e.g. "mariadb" -> mysql.
	Aliases() []string
	DefaultPort() int
	DefaultUser(cfg config.Config) string
	DefaultDB(cfg config.Config) string
	// ContainerSpec builds the `docker run` spec for an instance.
	ContainerSpec(cfg config.Config, inst store.Instance) docker.Spec
	// ConnectionURLs returns copy-paste URLs, e.g. DATABASE_URL.
	ConnectionURLs(inst store.Instance) map[string]string
	// EnvBlock returns the copy-paste env block.
	EnvBlock(inst store.Instance) string
	// ConnectCmd is the DB console command run via `docker exec`
	// (psql, redis-cli, ...). This is what you meant by "shell":
	// entering the DB and running DB commands.
	ConnectCmd(inst store.Instance) []string
	// ContainerShellCmd is the OS shell inside the container (/bin/sh).
	ContainerShellCmd() []string
	// TableQuery returns read-only SQL/commands used by `tables` preview.
	// Each adapter documents its own; interactive work happens in Connect.
	PreviewCommands(inst store.Instance) []string
}

var Registry = map[string]Adapter{}

func register(a Adapter) {
	Registry[a.Name()] = a
	for _, al := range a.Aliases() {
		Registry[al] = a
	}
}

// Resolve maps a --type value (including aliases like mariadb) to an adapter.
// Returns canonical type key + adapter.
func Resolve(typ string) (string, Adapter, error) {
	t := strings.ToLower(strings.TrimSpace(typ))
	if t == "" {
		return "", nil, fmt.Errorf("db type is required (postgres|redis|mysql|mariadb|mongo)")
	}
	if t == "mariadb" {
		t = "mysql"
	}
	a, ok := Registry[t]
	if !ok {
		return "", nil, fmt.Errorf("unknown db type %q (postgres|redis|mysql|mariadb|mongo)", typ)
	}
	return a.Name(), a, nil
}

// Types returns the canonical type names for help text.
func Types() []string { return []string{"postgres", "redis", "mysql", "mongo"} }

func pgURL(scheme string, inst store.Instance) string {
	return fmt.Sprintf("%s://%s:%s@%s:%d/%s",
		scheme, inst.User, inst.Password, inst.Host, inst.Port, inst.Database)
}
