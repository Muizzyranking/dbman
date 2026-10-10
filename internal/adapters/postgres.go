package adapters

import (
	"fmt"

	"github.com/muizzyranking/dbman/internal/config"
	"github.com/muizzyranking/dbman/internal/docker"
	"github.com/muizzyranking/dbman/internal/store"
)

type Postgres struct{}

func init() { register(Postgres{}) }

func (Postgres) Name() string      { return "postgres" }
func (Postgres) Aliases() []string { return nil }
func (Postgres) DefaultPort() int  { return 5432 }
func (Postgres) DefaultUser(cfg config.Config) string {
	if d, ok := cfg.Defaults["postgres"]; ok && d.User != "" {
		return d.User
	}
	return "postgres"
}
func (Postgres) DefaultDB(cfg config.Config) string {
	if d, ok := cfg.Defaults["postgres"]; ok && d.DB != "" {
		return d.DB
	}
	return "postgres"
}
func (Postgres) ContainerSpec(cfg config.Config, inst store.Instance) docker.Spec {
	return docker.Spec{
		Image:         inst.Image,
		Name:          inst.ContainerName,
		HostPort:      inst.Port,
		ContainerPort: 5432,
		Env: map[string]string{
			"POSTGRES_USER":     inst.User,
			"POSTGRES_PASSWORD": inst.Password,
			"POSTGRES_DB":       inst.Database,
		},
		Network: cfg.DockerNetwork,
	}
}
func (Postgres) ConnectionURLs(inst store.Instance) map[string]string {
	return map[string]string{
		"DATABASE_URL": pgURL("postgres", inst),
		"ASYNC_URL":    pgURL("postgresql+asyncpg", inst),
	}
}
func (Postgres) EnvBlock(inst store.Instance) string {
	urls := Postgres{}.ConnectionURLs(inst)
	return fmt.Sprintf("POSTGRES_HOST=%s\nPOSTGRES_PORT=%d\nPOSTGRES_USER=%s\nPOSTGRES_PASSWORD=%s\nPOSTGRES_DB=%s\nDATABASE_URL=%s",
		inst.Host, inst.Port, inst.User, inst.Password, inst.Database, urls["DATABASE_URL"])
}
func (Postgres) ConnectCmd(inst store.Instance) []string {
	// DB console (what you called "shell"): enter postgres and run SQL.
	return []string{"psql", "-U", inst.User, "-d", inst.Database}
}
func (Postgres) ContainerShellCmd() []string { return []string{"/bin/sh"} }
func (Postgres) PreviewCommands(inst store.Instance) []string {
	return []string{
		"\\dt  -- list tables (run inside Connect)",
		"SELECT tablename FROM pg_tables WHERE schemaname='public';",
	}
}
