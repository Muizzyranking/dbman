# Configuration — what you actually change, and why

File: `~/.config/dbman/config.json`. You never *have* to touch it: dbman
creates it with working defaults on first run, and every `create` field can be
overridden per-database (`--image`, `--port`, …). Edit the config when you want
a **new default for all future databases**.

```bash
dbman config               # where the files live
dbman config --show        # view current settings
dbman config --reset       # restore defaults (backs up to config.json.bak)
```

## Recipe 1: default to pgvector (or PostGIS)

**You want:** every new postgres to include the pgvector extension without
passing `--image` each time, plus a dropdown of alternates in the TUI.

```json
"postgres": {
  "image": "pgvector/pgvector:pg16",
  "options": ["pgvector/pgvector:pg16", "postgres:16-alpine", "postgis/postgis:16-3.4"],
  "user": "postgres",
  "db": "postgres"
}
```

- `image` is the default when the field/flag is empty.
- `options` is the `←/→` dropdown in the TUI create dialog. Free-typed custom
  tags always work too. Keep `image` among the entries so cycling returns to it.

## Recipe 2: run MariaDB instead of MySQL

**You want:** `--type mysql` (or `mariadb`) to spin up MariaDB.

```json
"mysql": {
  "image": "mariadb:11",
  "options": ["mariadb:11", "mysql:8", "mysql:8.0"],
  "user": "root",
  "db": "app"
}
```

Same adapter, same env block — only the image changes. Existing databases keep
whatever image they were created with; this only affects new ones.

## Recipe 3: control ports

**You want:** predictable ports, or to avoid clashing with a system postgres.

- Default (`"port_strategy": "standard_first"`) tries the engine's standard
  port first — `5432`, `6379`, `3306`, `27017` — and falls back to the range
  when busy. Good for "just works like production".
- `"port_strategy": "range"` always allocates from `"port_range_start"`
  (default `55000`, scans upward). Good when standard ports belong to other
  services. Change the base if `55000+` collides with something of yours.
- An explicit `--port` / port field always wins; if busy, dbman errors with a
  free suggestion instead of killing anything.

## Recipe 4: put databases on your own network

**You want:** dbman containers reachable by name from your app containers.

```bash
docker network create dbman-net
```

```json
"docker_network": "dbman-net"
```

Empty (`""`) means the default bridge. The network must exist before `create`.

## Recipe 5: make it yours (theme + icons)

**You want:** different colors, or plain text instead of nerd-font glyphs.

```json
"theme": { "name": "midnight", "accent": "#FF7B72", "success": "#3FB950",
           "warning": "#D29922", "error": "#F85149", "muted": "#8B949E" },
"icons": false
```

`accent` drives selection, the focused (thick) pane border, and the active
tab. `icons: false` drops the database glyph if your terminal has no Nerd
Font (you'd see boxes otherwise).

## Reference: every key

| Key | Type | Default | Notes |
|-----|------|---------|-------|
| `theme.name` | string | `"dark"` | Label only, shown in the header. |
| `theme.accent/success/warning/error/muted` | `"#RRGGBB"` | see example | Invalid values render unpredictably — keep hex. |
| `port_strategy` | enum | `"standard_first"` | `"standard_first"` or `"range"`; anything else behaves like `"range"`. |
| `port_range_start` | int | `55000` | Base for auto-assigned ports. |
| `docker_network` | string | `""` | `""` = default bridge, else passed as `--network`. |
| `icons` | bool | `true` | Nerd-font glyph tinted per engine; `false` = plain text. |
| `defaults.<engine>.image` | string | per engine | Any valid image reference (tags, digests, private registries). |
| `defaults.<engine>.options` | string[] | per engine | TUI dropdown choices. Empty/missing = free text only. |
| `defaults.<engine>.user` | string | per engine | Empty field fallback (`postgres`/`root`; redis has no user). |
| `defaults.<engine>.db` | string | per engine | Empty field fallback (`postgres`/`app`/`0` for redis). |

`defaults` keys are `postgres`, `redis`, `mysql`, `mongo`. Aliases reuse the
canonical entry: `mariadb` → `mysql`, `valkey` → `redis`, `mongodb` → `mongo`.

### Per-engine behavior worth knowing

- **postgres**: sets `POSTGRES_USER/PASSWORD/DB`. Env block has
  `POSTGRES_*` + `DATABASE_URL` (`postgres://…`) + `ASYNC_URL`
  (`postgresql+asyncpg://…`).
- **redis**: empty password = **no auth** and is *not* auto-generated.
  Env block: `REDIS_HOST/PORT/URL`.
- **mysql**: sets `MYSQL_ROOT_PASSWORD` + `MYSQL_DATABASE`. Env block:
  `MYSQL_*` + `DATABASE_URL` (`mysql://…`).
- **mongo**: sets `MONGO_INITDB_*` (seeded on first start with an empty
  volume). Env block: `MONGO_*` + `MONGO_URL` (`…?authSource=admin`).
- Passwords auto-generate (16 chars) when empty, except redis. Busy explicit
  ports error with a suggestion. Delete drops container **and** volume unless
  `--keep-volume` / the TUI checkbox.

## State file (`~/.cache/dbman/instances.json`)

Not configuration — it's your data: `{name, type, container_name, image,
host, port, user, password, database, created_at}` per database. Managed by
dbman; edit only to recover. Duplicate `name`s are ignored on load.
