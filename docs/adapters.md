# Adapters

Each engine implements the `Adapter` interface
(`internal/adapters/adapter.go`) and self-registers. The TUI, CLI, and Docker
layers only talk to the interface, so new databases plug in without touching
them.

## Supported engines

| `--type` | Container port | Console (`connect`) | Preview (`tables`) |
|----------|---------------|---------------------|--------------------|
| `postgres` | 5432 | `psql -U <user> -d <db>` | `pg_tables` list, `SELECT … LIMIT 20` |
| `redis` (+`valkey`) | 6379 | `redis-cli [-a <pw>]` | `SCAN`, `GET` |
| `mysql` (+`mariadb`) | 3306 | `mysql -u <user> -p… <db>` | `SHOW TABLES`, `SELECT … LIMIT 20` |
| `mongo` (+`mongodb`) | 27017 | `mongosh <url>` | `getCollectionNames`, `find().limit(20)` |

Env blocks and URLs per engine are listed in [Configuration](configuration.md).

## Adding a new engine

1. Create `internal/adapters/<name>.go` with a struct implementing:
   - `Name()` + `Aliases()` — canonical key and extra `--type` spellings
   - `DefaultPort()` / `DefaultUser(cfg)` / `DefaultDB(cfg)`
   - `ContainerSpec(cfg, inst)` — the `docker run` spec (image, ports, env)
   - `ConnectionURLs(inst)` + `EnvBlock(inst)` — copy-paste output
   - `ConnectCmd(inst)` — argv for `docker exec` console
   - `ContainerShellCmd()` — usually `["/bin/sh"]`
   - `PreviewCommands(inst)` — documented read-only hints
2. Register it: `func init() { register(MyDB{}) }`.
3. Add a `defaults.<name>` entry to `DefaultConfig()` in
   `internal/config/config.go` (image, options, user, db).
4. Extend `previewData` in `internal/cli/cli.go` for `tables` support.

`Types()` (used by the TUI type picker) derives from the canonical names —
no further wiring needed.
