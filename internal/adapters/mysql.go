package adapters

import (
	"fmt"

	"github.com/muizzyranking/dbman/internal/config"
	"github.com/muizzyranking/dbman/internal/docker"
	"github.com/muizzyranking/dbman/internal/store"
)

// MySQL covers both mysql and mariadb images via config options.
type MySQL struct{}

func init() { register(MySQL{}) }

func (MySQL) Name() string      { return "mysql" }
func (MySQL) Aliases() []string { return []string{"mariadb"} }
func (MySQL) DefaultPort() int  { return 3306 }
func (MySQL) DefaultUser(cfg config.Config) string {
	if d, ok := cfg.Defaults["mysql"]; ok && d.User != "" {
		return d.User
	}
	return "root"
}
func (MySQL) DefaultDB(cfg config.Config) string {
	if d, ok := cfg.Defaults["mysql"]; ok && d.DB != "" {
		return d.DB
	}
	return "app"
}
func (MySQL) ContainerSpec(cfg config.Config, inst store.Instance) docker.Spec {
	return docker.Spec{
		Image:         inst.Image,
		Name:          inst.ContainerName,
		HostPort:      inst.Port,
		ContainerPort: 3306,
		Env: map[string]string{
			"MYSQL_ROOT_PASSWORD": inst.Password,
			"MYSQL_DATABASE":      inst.Database,
		},
		Network: cfg.DockerNetwork,
	}
}
func (MySQL) ConnectionURLs(inst store.Instance) map[string]string {
	return map[string]string{
		"DATABASE_URL": fmt.Sprintf("mysql://%s:%s@%s:%d/%s",
			inst.User, inst.Password, inst.Host, inst.Port, inst.Database),
	}
}
func (MySQL) EnvBlock(inst store.Instance) string {
	urls := MySQL{}.ConnectionURLs(inst)
	return fmt.Sprintf("MYSQL_HOST=%s\nMYSQL_PORT=%d\nMYSQL_USER=%s\nMYSQL_PASSWORD=%s\nMYSQL_DATABASE=%s\nDATABASE_URL=%s",
		inst.Host, inst.Port, inst.User, inst.Password, inst.Database, urls["DATABASE_URL"])
}
func (MySQL) ConnectCmd(inst store.Instance) []string {
	return []string{"mysql", "-u", inst.User, "-p" + inst.Password, inst.Database}
}
func (MySQL) ContainerShellCmd() []string { return []string{"/bin/sh"} }
func (MySQL) PreviewCommands(inst store.Instance) []string {
	return []string{
		"SHOW TABLES;  -- run inside Connect",
		"SHOW DATABASES;",
	}
}
