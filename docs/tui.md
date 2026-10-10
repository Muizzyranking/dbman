# TUI guide

Run `dbman` with no arguments (or `dbman tui`) to open the interface.

## Layout

```
 ◤ dbman  ·  docker databases  ·  3                    ? all keys
 ╭──────────────╮╭───────────────────────────────────╮
 │ ❯ databases  ││ ❯ demo-pg  ● running              │
 │ / filter     ││                                   │
 │ ───────────  ││  1 Info  2 Env  3 URLs            │
 │ ▸ ● demo-pg  ││ ───────────────────────────────── │
 │   ○ demo-red ││ container  dbman-demo-pg          │
 │              ││ image      postgres:16-alpine      │
 ╰──────────────╯╰───────────────────────────────────╯
 ▸ enter open …
 j/k move  ·  enter open  ·  tab preview  ·  …  ·  ? all keys  ·  q quit
```

- **Left pane** — database list: `●` running (green) / `○` stopped, full name,
  colored `[type]` pill, host port. `/` filters.
- **Right pane** — preview of the selected database, tabs Info / Env / URLs.
- **Footer** — one line, always ends with `? all keys`. The active pane has a
  **thick** accent border; the idle one is thin and muted.
- Passwords are masked (`********`) everywhere for screen sharing.

## Keys

Press `?` anywhere for the in-app list. `h/j/k/l` never switch panes —
only `tab` does.

| Key | Where | Action |
|-----|-------|--------|
| `tab` | anywhere | move between list ↔ preview |
| `j/k`, `g/G`, `ctrl+d/u` | list | move / first-last / half page |
| `enter` | list | jump to preview |
| `n` | list | new database |
| `s` / `R` | list+preview | start-stop toggle / restart |
| `D` | list | delete (confirm; volume removed unless kept) |
| `/`, `enter`/`esc` | list | filter / apply / clear |
| `r` | list+preview | refresh from Docker |
| `1–3`, `h/l` | preview | Info / Env / URLs tabs |
| `enter` | preview | full DB console (`exit` returns) |
| `E` | preview | copy exact `docker exec` connect command |
| `x` | preview | container OS shell (`exit` returns) |
| `L` | preview | follow logs (`ctrl+c` returns) |
| `p` | preview | reveal / hide password |
| `y` | preview | copy env block or URLs (real values, even masked) |
| `Y` | preview | copy password only |
| `?` | anywhere | keybinding help |
| `q`/`esc` | anywhere | back / quit |

`enter`, `x`, and `L` need a **running** container — on a stopped one you get
a plain message (`… is not running (exited) — press s to start`) instead of a
flashing terminal. Same guard exists in the CLI.

## Create dialog

`tab` moves fields, `enter` saves. The **type** row and the **image** field
cycle with `←/→`: images come from `defaults.<engine>.options` in your
[config](configuration.md); typing free text always works for custom tags.
Switching type resets the image to that engine's default. First-time images
pull with a spinner overlay — the UI stays responsive.
