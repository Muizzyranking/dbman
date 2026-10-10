package adapters

import (
	"fmt"

	"github.com/muizzyranking/dbman/internal/config"
	"github.com/muizzyranking/dbman/internal/docker"
	"github.com/muizzyranking/dbman/internal/store"
)

type Redis struct{}

func init() { register(Redis{}) }

func (Redis) Name() string                         { return "redis" }
func (Redis) Aliases() []string                    { return []string{"valkey"} }
func (Redis) DefaultPort() int                     { return 6379 }
func (Redis) DefaultUser(cfg config.Config) string { return "" }
func (Redis) DefaultDB(cfg config.Config) string   { return "0" }
func (Redis) ContainerSpec(cfg config.Config, inst store.Instance) docker.Spec {
	spec := docker.Spec{
		Image:         inst.Image,
		Name:          inst.ContainerName,
		HostPort:      inst.Port,
		ContainerPort: 6379,
		Env:           map[string]string{},
		Network:       cfg.DockerNetwork,
	}
	if inst.Password != "" {
		spec.Command = []string{"redis-server", "--requirepass", inst.Password}
	}
	return spec
}
func (Redis) ConnectionURLs(inst store.Instance) map[string]string {
	if inst.Password != "" {
		return map[string]string{
			"REDIS_URL": fmt.Sprintf("redis://:%s@%s:%d/%s", inst.Password, inst.Host, inst.Port, inst.Database),
		}
	}
	return map[string]string{
		"REDIS_URL": fmt.Sprintf("redis://%s:%d/%s", inst.Host, inst.Port, inst.Database),
	}
}
func (Redis) EnvBlock(inst store.Instance) string {
	urls := Redis{}.ConnectionURLs(inst)
	return fmt.Sprintf("REDIS_HOST=%s\nREDIS_PORT=%d\nREDIS_URL=%s",
		inst.Host, inst.Port, urls["REDIS_URL"])
}
func (Redis) ConnectCmd(inst store.Instance) []string {
	if inst.Password != "" {
		return []string{"redis-cli", "-a", inst.Password}
	}
	return []string{"redis-cli"}
}
func (Redis) ContainerShellCmd() []string { return []string{"/bin/sh"} }
func (Redis) PreviewCommands(inst store.Instance) []string {
	return []string{
		"SCAN 0  -- list keys (run inside Connect)",
		"KEYS *  -- small DBs only",
	}
}
