// Package cli implements the dbman Cobra commands. It shares the config,
// store, Docker client, and adapters with the TUI.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/muizzyranking/dbman/internal/adapters"
	"github.com/muizzyranking/dbman/internal/config"
	"github.com/muizzyranking/dbman/internal/docker"
	"github.com/muizzyranking/dbman/internal/store"
	"github.com/muizzyranking/dbman/internal/util"
)

type ctx struct {
	cfg  config.Config
	list []store.Instance
	dock docker.Client
}

// Version is set at release time via:
// go build -ldflags "-X github.com/muizzyranking/dbman/internal/cli.Version=v1.2.3"
var Version = "dev"

func loadCtx() (*ctx, error) {
	cfg, _, err := config.Load()
	if err != nil {
		return nil, err
	}
	list, _, err := store.Load()
	if err != nil {
		return nil, err
	}
	return &ctx{cfg: cfg, list: list, dock: docker.NewCLIClient()}, nil
}

func findInstance(c *ctx, name string) (store.Instance, adapters.Adapter, error) {
	inst := store.Find(c.list, name)
	if inst == nil {
		return store.Instance{}, nil, fmt.Errorf("no instance named %q (dbman list)", name)
	}
	_, a, err := adapters.Resolve(inst.Type)
	if err != nil {
		return *inst, nil, err
	}
	return *inst, a, nil
}

// requireRunning guards commands that need a live container (connect, shell,
// logs, tables) and explains how to fix it instead of a raw docker error.
func requireRunning(c *ctx, inst store.Instance) error {
	st := c.dock.ContainerStatus(inst.ContainerName)
	if st.Running {
		return nil
	}
	state := st.State
	if state == "" || state == "notfound" {
		return fmt.Errorf("dbman: %q container %q not found — was it removed outside dbman? (dbman list)", inst.Name, inst.ContainerName)
	}
	return fmt.Errorf("dbman: %q is not running (state: %s). Start it first: dbman start %s", inst.Name, state, inst.Name)
}

func mask(s string, show bool) string {
	if show {
		return s
	}
	if s == "" {
		return "(empty)"
	}
	return "********"
}

// pickPort implements standard_first -> range fallback.
func pickPort(c *ctx, a adapters.Adapter, want int) (int, error) {
	if want != 0 {
		if !util.FreePort(want) || c.dock.PortInUseBy(want) != "" {
			by := c.dock.PortInUseBy(want)
			suggest := util.FindFreePort(c.cfg.PortRangeStart)
			if by != "" {
				return 0, fmt.Errorf("port %d is in use by container %q; try --port %d", want, by, suggest)
			}
			return 0, fmt.Errorf("port %d is in use on this host; try --port %d", want, suggest)
		}
		return want, nil
	}
	if c.cfg.PortStrategy == "standard_first" {
		std := a.DefaultPort()
		if util.FreePort(std) && c.dock.PortInUseBy(std) == "" {
			return std, nil
		}
	}
	return util.FindFreePort(c.cfg.PortRangeStart), nil
}

func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:     "dbman",
		Short:   "TUI + CLI manager for local Docker databases",
		Long:    "dbman manages per-project postgres/redis/mysql/mongo containers. Run with no args to open the TUI.",
		Version: Version,
	}

	root.AddCommand(listCmd(), infoCmd(), envCmd(), createCmd(),
		startCmd(), stopCmd(), restartCmd(), rmCmd(),
		connectCmd(), shellCmd(), logsCmd(), tablesCmd(), configCmd())

	return root
}

func listCmd() *cobra.Command {
	jsonOut := false
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List managed databases",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := loadCtx()
			if err != nil {
				return err
			}
			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(c.list)
			}
			if len(c.list) == 0 {
				fmt.Println("No databases yet. Create one: dbman create --type postgres --name myproj")
				return nil
			}
			fmt.Printf("%-18s %-9s %-24s %-6s %-9s\n", "NAME", "TYPE", "CONTAINER", "PORT", "STATUS")
			for _, inst := range c.list {
				st := c.dock.ContainerStatus(inst.ContainerName)
				fmt.Printf("%-18s %-9s %-24s %-6d %-9s\n",
					inst.Name, inst.Type, inst.ContainerName, inst.Port, st.State)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output JSON")
	return cmd
}

func infoCmd() *cobra.Command {
	showPw := false
	jsonOut := false
	cmd := &cobra.Command{
		Use:   "info <name>",
		Short: "Show connection info for one database",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := loadCtx()
			if err != nil {
				return err
			}
			inst, a, err := findInstance(c, args[0])
			if err != nil {
				return err
			}
			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(inst)
			}
			st := c.dock.ContainerStatus(inst.ContainerName)
			fmt.Printf("Name:      %s\nType:      %s\nContainer: %s\nImage:     %s\nHost:      %s\nPort:      %d\nUser:      %s\nPassword:  %s\nDB:        %s\nStatus:    %s\n",
				inst.Name, inst.Type, inst.ContainerName, inst.Image,
				inst.Host, inst.Port, inst.User, mask(inst.Password, showPw),
				inst.Database, st.State)
			fmt.Println("\nURLs:")
			for k, v := range a.ConnectionURLs(inst) {
				fmt.Printf("  %s=%s\n", k, v)
			}
			fmt.Println("\nEnv block:")
			fmt.Println(a.EnvBlock(inst))
			return nil
		},
	}
	cmd.Flags().BoolVar(&showPw, "show-password", false, "reveal password")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output JSON")
	return cmd
}

func envCmd() *cobra.Command {
	doCopy := false
	export := ""
	cmd := &cobra.Command{
		Use:   "env <name>",
		Short: "Print (and optionally copy/export) the env block",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := loadCtx()
			if err != nil {
				return err
			}
			inst, a, err := findInstance(c, args[0])
			if err != nil {
				return err
			}
			block := a.EnvBlock(inst)
			fmt.Println(block)
			if doCopy {
				_ = util.CopyToClipboard(block)
				fmt.Fprintln(os.Stderr, "(copied to clipboard)")
			}
			if export != "" {
				if err := os.WriteFile(export, []byte(block+"\n"), 0o644); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "(wrote %s)\n", export)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&doCopy, "copy", false, "copy to clipboard")
	cmd.Flags().StringVar(&export, "export", "", "write env block to file (e.g. .env)")
	return cmd
}

func createCmd() *cobra.Command {
	var typ, name, image, user, password, db string
	var port int
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new database container",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := loadCtx()
			if err != nil {
				return err
			}
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			canon, a, err := adapters.Resolve(typ)
			if err != nil {
				return err
			}
			if store.Find(c.list, name) != nil {
				return fmt.Errorf("instance %q already exists", name)
			}
			if image == "" {
				if d, ok := c.cfg.Defaults[canon]; ok {
					image = d.Image
				}
			}
			if image == "" {
				return fmt.Errorf("--image is required (no default for %s)", canon)
			}
			if user == "" {
				user = a.DefaultUser(c.cfg)
			}
			if db == "" {
				db = a.DefaultDB(c.cfg)
				if canon == "postgres" && db == "" {
					db = name
				}
			}
			if password == "" && canon != "redis" {
				password = util.GeneratePassword(16)
			}
			hostPort, err := pickPort(c, a, port)
			if err != nil {
				return err
			}
			inst := store.Instance{
				Name:          name,
				Type:          canon,
				ContainerName: "dbman-" + strings.ToLower(strings.ReplaceAll(name, " ", "-")),
				Image:         image,
				Host:          "127.0.0.1",
				Port:          hostPort,
				User:          user,
				Password:      password,
				Database:      db,
				CreatedAt:     time.Now(),
			}
			spec := a.ContainerSpec(c.cfg, inst)
			if !c.dock.ImageExists(inst.Image) {
				fmt.Printf("Pulling image %s (first run may take a while) ...\n", inst.Image)
				if err := c.dock.Pull(inst.Image); err != nil {
					return fmt.Errorf("pull %s: %w", inst.Image, err)
				}
			}
			fmt.Printf("Creating %s (%s) on 127.0.0.1:%d ...\n", inst.Name, inst.Image, inst.Port)
			if err := c.dock.Run(spec); err != nil {
				return err
			}
			c.list = store.Upsert(c.list, inst)
			if err := store.Save(c.list); err != nil {
				return err
			}
			fmt.Println("Created. Connection info:")
			fmt.Println(a.EnvBlock(inst))
			return nil
		},
	}
	cmd.Flags().StringVar(&typ, "type", "", "required: postgres|redis|mysql|mariadb|mongo")
	cmd.Flags().StringVar(&name, "name", "", "required: friendly name, e.g. myproj-pg")
	cmd.Flags().StringVar(&image, "image", "", "optional: docker image (default from config)")
	cmd.Flags().StringVar(&user, "user", "", "optional: db user")
	cmd.Flags().StringVar(&password, "password", "", "optional: generated if empty (except redis)")
	cmd.Flags().StringVar(&db, "db", "", "optional: database name")
	cmd.Flags().IntVar(&port, "port", 0, "optional: host port (auto-assigned if 0)")
	return cmd
}

func simpleLifecycle(use, action, past string, fn func(docker.Client, string) error) *cobra.Command {
	return &cobra.Command{
		Use:   use + " <name>",
		Short: action + " a managed database container",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := loadCtx()
			if err != nil {
				return err
			}
			inst, _, err := findInstance(c, args[0])
			if err != nil {
				return err
			}
			if err := fn(c.dock, inst.ContainerName); err != nil {
				return err
			}
			fmt.Printf("%s %s\n", past, inst.Name)
			return nil
		},
	}
}

func startCmd() *cobra.Command {
	return simpleLifecycle("start", "Start", "started", func(d docker.Client, c string) error { return d.Start(c) })
}
func stopCmd() *cobra.Command {
	return simpleLifecycle("stop", "Stop", "stopped", func(d docker.Client, c string) error { return d.Stop(c) })
}
func restartCmd() *cobra.Command {
	return simpleLifecycle("restart", "Restart", "restarted", func(d docker.Client, c string) error { return d.Restart(c) })
}

func rmCmd() *cobra.Command {
	keepVol := false
	cmd := &cobra.Command{
		Use:   "rm <name>",
		Short: "Delete a database (container + record). Removes volume by default.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := loadCtx()
			if err != nil {
				return err
			}
			inst, _, err := findInstance(c, args[0])
			if err != nil {
				return err
			}
			if err := c.dock.Remove(inst.ContainerName, !keepVol); err != nil {
				return err
			}
			c.list = store.Remove(c.list, inst.Name)
			if err := store.Save(c.list); err != nil {
				return err
			}
			fmt.Printf("removed %s\n", inst.Name)
			return nil
		},
	}
	cmd.Flags().BoolVar(&keepVol, "keep-volume", false, "keep docker volume/data")
	return cmd
}

func quoteJoin(parts []string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.ContainsAny(p, " \t\"'") {
			out = append(out, "'"+strings.ReplaceAll(p, "'", "'\\''")+"'")
		} else {
			out = append(out, p)
		}
	}
	return strings.Join(out, " ")
}

func connectCmd() *cobra.Command {
	printOnly := false
	cmd := &cobra.Command{
		Use:   "connect <name>",
		Short: "Open the DB console (psql, redis-cli, mysql, mongosh) via docker exec",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := loadCtx()
			if err != nil {
				return err
			}
			inst, a, err := findInstance(c, args[0])
			if err != nil {
				return err
			}
			parts := append([]string{"docker", "exec", "-it", inst.ContainerName}, a.ConnectCmd(inst)...)
			if printOnly {
				fmt.Println(quoteJoin(parts))
				return nil
			}
			if err := requireRunning(c, inst); err != nil {
				return err
			}
			return c.dock.ExecInteractive(inst.ContainerName, a.ConnectCmd(inst)...)
		},
	}
	cmd.Flags().BoolVar(&printOnly, "print", false, "print the docker exec command instead of attaching")
	return cmd
}

func shellCmd() *cobra.Command {
	printOnly := false
	cmd := &cobra.Command{
		Use:   "shell <name>",
		Short: "Open an OS shell (/bin/sh) inside the container",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := loadCtx()
			if err != nil {
				return err
			}
			inst, a, err := findInstance(c, args[0])
			if err != nil {
				return err
			}
			parts := append([]string{"docker", "exec", "-it", inst.ContainerName}, a.ContainerShellCmd()...)
			if printOnly {
				fmt.Println(quoteJoin(parts))
				return nil
			}
			if err := requireRunning(c, inst); err != nil {
				return err
			}
			return c.dock.ExecInteractive(inst.ContainerName, a.ContainerShellCmd()...)
		},
	}
	cmd.Flags().BoolVar(&printOnly, "print", false, "print the docker exec command instead of attaching")
	return cmd
}

func logsCmd() *cobra.Command {
	follow := false
	tail := "200"
	cmd := &cobra.Command{
		Use:   "logs <name>",
		Short: "Show container logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := loadCtx()
			if err != nil {
				return err
			}
			inst, _, err := findInstance(c, args[0])
			if err != nil {
				return err
			}
			if err := requireRunning(c, inst); err != nil {
				return err
			}
			return c.dock.Logs(inst.ContainerName, follow, tail)
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow log output")
	cmd.Flags().StringVar(&tail, "tail", "200", "lines of recent logs")
	return cmd
}

func tablesCmd() *cobra.Command {
	preview := ""
	cmd := &cobra.Command{
		Use:   "tables <name>",
		Short: "Read-only preview: list tables/keys/collections (top 20)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := loadCtx()
			if err != nil {
				return err
			}
			inst, _, err := findInstance(c, args[0])
			if err != nil {
				return err
			}
			if err := requireRunning(c, inst); err != nil {
				return err
			}
			out, err := previewData(c.dock, inst, preview)
			if err != nil {
				return err
			}
			fmt.Println(out)
			return nil
		},
	}
	cmd.Flags().StringVar(&preview, "preview", "", "preview top 20 rows of a table/collection/key")
	return cmd
}

func previewData(d docker.Client, inst store.Instance, preview string) (string, error) {
	switch inst.Type {
	case "postgres":
		if preview != "" {
			q := fmt.Sprintf("SELECT * FROM %s LIMIT 20;", preview)
			return d.ExecCapture(inst.ContainerName, "psql", "-U", inst.User, "-d", inst.Database, "-c", q)
		}
		return d.ExecCapture(inst.ContainerName, "psql", "-U", inst.User, "-d", inst.Database,
			"-c", "SELECT tablename FROM pg_tables WHERE schemaname='public';")
	case "mysql":
		if preview != "" {
			return d.ExecCapture(inst.ContainerName, "mysql", "-u", inst.User, "-p"+inst.Password,
				"-e", fmt.Sprintf("SELECT * FROM %s LIMIT 20;", preview), inst.Database)
		}
		return d.ExecCapture(inst.ContainerName, "mysql", "-u", inst.User, "-p"+inst.Password,
			"-e", "SHOW TABLES;", inst.Database)
	case "redis":
		if preview != "" {
			if inst.Password != "" {
				return d.ExecCapture(inst.ContainerName, "redis-cli", "-a", inst.Password, "GET", preview)
			}
			return d.ExecCapture(inst.ContainerName, "redis-cli", "GET", preview)
		}
		if inst.Password != "" {
			return d.ExecCapture(inst.ContainerName, "redis-cli", "-a", inst.Password, "SCAN", "0", "COUNT", "50")
		}
		return d.ExecCapture(inst.ContainerName, "redis-cli", "SCAN", "0", "COUNT", "50")
	case "mongo":
		url := fmt.Sprintf("mongodb://%s:%s@localhost:27017/%s?authSource=admin", inst.User, inst.Password, inst.Database)
		if preview != "" {
			return d.ExecCapture(inst.ContainerName, "mongosh", url, "--quiet",
				"--eval", fmt.Sprintf("db.getCollection('%s').find().limit(20).toArray()", preview))
		}
		return d.ExecCapture(inst.ContainerName, "mongosh", url, "--quiet",
			"--eval", "db.getCollectionNames()")
	default:
		return "", fmt.Errorf("unknown type %q", inst.Type)
	}
}

func configCmd() *cobra.Command {
	show := false
	reset := false
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show config/cache paths, contents, or reset to defaults",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, cfgPath, _ := config.Load()
			_, cachePath, _ := store.Load()
			if reset {
				backup := cfgPath + ".bak"
				if data, err := os.ReadFile(cfgPath); err == nil {
					_ = os.WriteFile(backup, data, 0o644)
					fmt.Printf("backed up to %s\n", backup)
				}
				if err := config.Save(config.DefaultConfig()); err != nil {
					return err
				}
				fmt.Printf("reset defaults in %s\n", cfgPath)
				return nil
			}
			if show {
				data, err := json.MarshalIndent(cfg, "", "  ")
				if err != nil {
					return err
				}
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("config: %s\ncache:  %s\n", cfgPath, cachePath)
			return nil
		},
	}
	cmd.Flags().BoolVar(&show, "show", false, "print the config file contents")
	cmd.Flags().BoolVar(&reset, "reset", false, "reset config to defaults (backs up old file)")
	return cmd
}
