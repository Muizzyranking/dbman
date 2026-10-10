# CLI reference

`dbman` with no args opens the [TUI](tui.md). Everything below also works there.

## Databases

```bash
dbman list [--json]
dbman create --type postgres|redis|mysql|mariadb|mongo --name NAME \
  [--image IMG] [--user U] [--password P] [--db DB] [--port PORT]
dbman start|stop|restart <name>
dbman rm <name> [--keep-volume]     # container + record; volume removed by default
```

`create` fills blanks from [config](configuration.md): default image, default
user/db, generated password (except redis, where empty = no auth), and an
auto-assigned port (`standard_first` tries `5432/6379/3306/27017`, else
`55000+`). A busy explicit `--port` is an error with a free-port suggestion —
dbman never kills host processes. Missing images are pulled first.

## Info & connection

```bash
dbman info <name> [--show-password] [--json]
dbman env <name> [--copy] [--export .env]
dbman tables <name> [--preview TABLE]   # read-only top-20 preview
dbman config [--show|--reset]           # paths, contents, restore defaults
dbman --version                         # release version (dev for local builds)
```

## Connecting — yes, from the CLI too

```bash
dbman connect <name>            # psql / redis-cli / mysql / mongosh via docker exec
dbman connect <name> --print    # print the command instead of attaching (copy it)
dbman shell <name> [--print]    # /bin/sh inside the container
dbman logs <name> [-f] [--tail 200]
```

`connect`, `shell`, `logs`, and `tables` require a running container and say so
plainly otherwise:

```
Error: dbman: "myproj-pg" is not running (state: exited). Start it first: dbman start myproj-pg
```

`--print` outputs the exact command, e.g.:

```
docker exec -it dbman-myproj-pg psql -U postgres -d postgres
```

The TUI copies the same string with `E`.
