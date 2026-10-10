# dbman — local Docker database manager

[![ci](https://github.com/muizzyranking/dbman/actions/workflows/ci.yml/badge.svg)](https://github.com/muizzyranking/dbman/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/muizzyranking/dbman)](https://github.com/muizzyranking/dbman/releases)

A TUI + CLI to manage per-project dev databases (postgres, redis, mysql/MariaDB,
mongo) as Docker containers. Create a database per project, copy the connection
block into your `.env`, shell into it when needed, stop/delete it when done.

## Install

One-liner (Linux/macOS, installs to `~/.local/bin`):

```bash
curl -fsSL https://raw.githubusercontent.com/muizzyranking/dbman/main/install.sh | sh
```

Or pick your way ([details](docs/getting-started.md)):

```bash
go install github.com/muizzyranking/dbman@latest   # needs Go
# Windows / manual: download the zip from the releases page
```

From source:

```bash
git clone https://github.com/muizzyranking/dbman.git && cd dbman
go build -o dbman .
```

## Quickstart

```bash
go build -o dbman .
./dbman create --type postgres --name myproj-pg
./dbman                 # opens the TUI
```

First run creates `~/.config/dbman/config.json` and `~/.cache/dbman/instances.json`.

## Docs

- [Getting started](docs/getting-started.md) — install, first database, daily flow
- [TUI guide](docs/tui.md) — layout, panes, all keybindings
- [CLI reference](docs/cli.md) — every command with examples
- [Configuration](docs/configuration.md) — every config option and its values
- [Adapters](docs/adapters.md) — supported engines, env blocks, adding a new DB
- [Troubleshooting](docs/troubleshooting.md) — common errors and fixes

## One-minute tour

```
dbman list                        # all managed databases + status
dbman info myproj-pg              # host/port/user/password/urls
dbman env myproj-pg --copy        # env block to clipboard
dbman connect myproj-pg           # psql / redis-cli / mysql / mongosh
dbman stop myproj-pg              # s in the TUI toggles start/stop
dbman rm myproj-pg                # container + volume + record
```

In the TUI: `tab` moves between the list and the preview, `?` shows all keys,
`p` reveals the masked password, `y` copies, `E` copies the exact
`docker exec` connect command.
