package adapters

import (
	"fmt"

	"github.com/muizzyranking/dbman/internal/config"
	"github.com/muizzyranking/dbman/internal/docker"
	"github.com/muizzyranking/dbman/internal/store"
)

type Mongo struct{}

func init() { register(Mongo{}) }

func (Mongo) Name() string      { return "mongo" }
func (Mongo) Aliases() []string { return []string{"mongodb"} }
func (Mongo) DefaultPort() int  { return 27017 }
func (Mongo) DefaultUser(cfg config.Config) string {
	if d, ok := cfg.Defaults["mongo"]; ok && d.User != "" {
		return d.User
	}
	return "root"
}
func (Mongo) DefaultDB(cfg config.Config) string {
	if d, ok := cfg.Defaults["mongo"]; ok && d.DB != "" {
		return d.DB
	}
	return "app"
}
func (Mongo) ContainerSpec(cfg config.Config, inst store.Instance) docker.Spec {
	return docker.Spec{
		Image:         inst.Image,
		Name:          inst.ContainerName,
		HostPort:      inst.Port,
		ContainerPort: 27017,
		Env: map[string]string{
			"MONGO_INITDB_ROOT_USERNAME": inst.User,
			"MONGO_INITDB_ROOT_PASSWORD": inst.Password,
			"MONGO_INITDB_DATABASE":      inst.Database,
		},
		Network: cfg.DockerNetwork,
	}
}
func (Mongo) ConnectionURLs(inst store.Instance) map[string]string {
	return map[string]string{
		"MONGO_URL": fmt.Sprintf("mongodb://%s:%s@%s:%d/%s?authSource=admin",
			inst.User, inst.Password, inst.Host, inst.Port, inst.Database),
	}
}
func (Mongo) EnvBlock(inst store.Instance) string {
	urls := Mongo{}.ConnectionURLs(inst)
	return fmt.Sprintf("MONGO_HOST=%s\nMONGO_PORT=%d\nMONGO_USER=%s\nMONGO_PASSWORD=%s\nMONGO_DB=%s\nMONGO_URL=%s",
		inst.Host, inst.Port, inst.User, inst.Password, inst.Database, urls["MONGO_URL"])
}
func (Mongo) ConnectCmd(inst store.Instance) []string {
	url := fmt.Sprintf("mongodb://%s:%s@localhost:27017/%s?authSource=admin",
		inst.User, inst.Password, inst.Database)
	return []string{"mongosh", url}
}
func (Mongo) ContainerShellCmd() []string { return []string{"/bin/sh"} }
func (Mongo) PreviewCommands(inst store.Instance) []string {
	return []string{
		"show collections  // run inside Connect",
		"db.<collection>.find().limit(20)",
	}
}
