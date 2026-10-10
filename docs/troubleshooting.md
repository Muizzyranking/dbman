# Troubleshooting

## `dbman: "X" is not running (state: exited)`

`connect`, `shell`, `logs`, and `tables` need a live container. Start it:

```bash
dbman start X        # or press s on it in the TUI
```

The TUI shows the same message in the status line instead of flashing a
terminal. Nothing is broken — exited is the normal state after a reboot or
`dbman stop`.

## `port N is in use …`

Something (another dbman DB or another app) holds the port. Either free it or
take the suggested free port. dbman never kills host processes. Two dbman
instances with the same explicit port can't run at the same time — give one a
different `--port`.

## Create seems stuck on a new image

First-time images download (`docker pull`) — in the TUI a spinner overlay
shows `pulling <image>` and the UI stays responsive; the CLI prints
`Pulling image …`. If it fails, you get the pull error (usually network or a
wrong tag). Re-run after fixing.

## TUI shows ghost/duplicate rows after start/stop

Fixed: Docker's own stdout used to smear the terminal. If you still see
artifacts, press `r` (refresh) or resize the terminal; your
`~/.cache/dbman/instances.json` is the source of truth (`dbman list`).

## Query returns nothing on a fresh database

Correct — a new database has no tables/keys yet. `SHOW TABLES` on an empty
MySQL returns zero rows; `SCAN 0` on empty Redis returns nothing. Create schema
via `dbman connect X` first, then `dbman tables X --preview <table>`.

## `docker exec` / logs window flashes and closes

Happens when the container isn't running (see above) — now guarded with a
message. If it happens on a *running* container, run the printed command
(`dbman connect X --print`) manually to see Docker's raw error.

## Boxes/glyphs look wrong

Your terminal lacks a Nerd Font. Either install one or set `"icons": false`
in `~/.config/dbman/config.json`.

## Reset everything

```bash
dbman rm <name>              # per database (drops its volume too)
rm ~/.config/dbman/config.json      # regenerates defaults on next run
docker ps -a --filter name=dbman-   # leftovers created outside dbman
```
