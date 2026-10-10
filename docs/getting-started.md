# Getting started

## Requirements

- Go 1.21+ (to build)
- Docker Engine running locally (`docker ps` works)

## Install

One-liner (Linux/macOS):

```bash
curl -fsSL https://raw.githubusercontent.com/muizzyranking/dbman/main/install.sh | sh
```

From source:

```bash
git clone https://github.com/muizzyranking/dbman.git && cd dbman
go build -o dbman .
# optional: put it on PATH
sudo mv dbman /usr/local/bin/dbman
```

## First run

Running anything creates the two files dbman needs:

- `~/.config/dbman/config.json` — settings, theme, default images
- `~/.cache/dbman/instances.json` — your databases (passwords, ports, containers)

```bash
dbman config     # prints both paths
dbman list       # empty at first
```

## Your first database

```bash
dbman create --type postgres --name myproj-pg
```

dbman picks the default image (`postgres:16-alpine`), generates a password,
and assigns a port (standard `5432` when free, else from `55000+`). If the image
isn't local, it pulls first — in the TUI a spinner shows the pull progress so
the UI never freezes.

Redis example:

```bash
dbman create --type redis --name myproj-cache
```

## Daily flow

1. `dbman` — opens the TUI: databases left, preview right.
2. `enter` on one — preview tabs Info / Env / URLs.
3. `y` — copies the env block; paste into your project's `.env`.
4. `enter` again — full DB console (`psql`, `redis-cli`, …). `exit` returns.
5. `s` — start/stop. `D` — delete (volume goes too, unless kept).

CLI equivalents exist for everything — see [CLI reference](cli.md).

## Next steps

- [TUI guide](tui.md) to learn the panes and keys (`?` lists them in-app too).
- [Configuration](configuration.md) to set your images (e.g. pgvector), ports,
  network, and theme.
