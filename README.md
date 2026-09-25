# mergo

**Beautiful Mermaid diagrams in your terminal.** Real graphics, not ASCII art.

mergo parses [Mermaid](https://mermaid.js.org) diagrams, lays them out and
rasterizes them **natively in Go** — no browser, no Node.js, no
`mermaid-cli`. The result is displayed in the terminal using the
[kitty graphics protocol](https://sw.kovidgoyal.net/kitty/graphics-protocol/)
(kitty, Ghostty, …) with an automatic fallback to true-color half blocks
(`▀`) everywhere else.

Built with the Charm stack: [Bubble Tea v2](https://github.com/charmbracelet/bubbletea),
[Lip Gloss v2](https://github.com/charmbracelet/lipgloss),
[Ultraviolet](https://github.com/charmbracelet/ultraviolet),
[Bubbles](https://github.com/charmbracelet/bubbles) and
[Fang](https://github.com/charmbracelet/fang).

## Features

- **13 diagram types**: flowchart, sequence, class, state, ER, gantt, pie,
  mindmap, gitGraph, timeline, user journey, quadrant chart and XY chart.
- **Mermaid syntax**: front matter (`title`, `config`), `%%{init: …}%%`
  directives, `themeVariables`, comments, `classDef`/`class`/`style`/`:::`,
  `linkStyle`, markdown strings, `<br>`, entity codes (`#quot;`, `#9829;`).
- **Professional output**: layered (Sugiyama) graph layout with nested
  clusters, spline edges, proper arrowheads, crow's feet, UML markers,
  anti-aliased text, subtle drop shadows.
- **Themes**: the same 23 themes as [gopyter](https://github.com/mark3labs/gopyter)
  and kit (`mergo`, `catppuccin`, `dracula`, `nord`, `gruvbox`, `tokyonight`, …),
  each with a light and a dark variant that follows the terminal background.
  The viewer's bars, panels and picker are drawn in the theme's colors too.
  Mermaid's `default`, `dark`, `forest` and `neutral` stay available through
  `%%{init: {"theme": "…"}}%%` directives.
- **Interactive viewer**: zoom, pan (keys or mouse drag / wheel), fit, switch
  between diagrams, live reload on save, theme picker with live preview,
  export PNG.
- **Kitty graphics via Unicode placeholders**: the image is transmitted once
  and drawn with placeholder cells, so it cooperates perfectly with Bubble
  Tea's cell renderer (overlays, help, error panels all compose on top).
- **Half-block fallback**: gamma-correct, supersampled rendering to `▀`
  cells built with Ultraviolet — looks good in any true-color terminal.
- **Markdown aware**: every ` ```mermaid ` block in a `.md` file becomes a tab.
- **Non-interactive modes**: `--print` (like `cat` for diagrams) and
  `--output file.png`.

## Install

Prebuilt binaries for Linux and macOS (amd64, arm64). The script verifies
the release checksum and installs to `~/.local/bin` by default:

```sh
curl -fsSL https://raw.githubusercontent.com/mark3labs/mergo/master/install.sh | bash
# a specific version, or another directory:
curl -fsSL https://raw.githubusercontent.com/mark3labs/mergo/master/install.sh | bash -s -- --version v0.1.0 --bin-dir /usr/local/bin
```

Or with Go:

```sh
go install github.com/mark3labs/mergo@latest
```

## Usage

```sh
mergo diagram.mmd                 # interactive viewer
mergo README.md                   # every mermaid block, tab to switch
cat flow.mmd | mergo              # from stdin
mergo -t dracula flow.mmd         # use a theme for this run
mergo -p flow.mmd                 # print inline and exit
mergo -o flow.png --scale 3 flow.mmd   # export PNG
mergo -o flow.png --background light flow.mmd  # light variant
mergo types                       # list diagram types and themes
mergo themes                      # list themes, marking the active one
```

| Flag | Description |
| --- | --- |
| `-t, --theme` | theme for this run (see `mergo themes`), overriding the saved one |
| `--background` | `auto` (default: follow the terminal), `dark` or `light` theme variant |
| `-r, --renderer` | `auto` (default), `kitty` or `halfblock` |
| `--kitty-placement` | `auto` (default), `unicode` (placeholders) or `direct` |
| `-p, --print` | print inline and exit |
| `-o, --output` | export PNG (`-` for stdout) |
| `-i, --index` | which diagram of a multi-diagram input to print/export |
| `-w, --width` | width in cells for `--print` |
| `--scale` | pixel scale for PNG export (default 2) |
| `--font`, `--font-bold` | use your own TTF/OTF fonts |
| `--no-shadows` | flat rendering |
| `--no-watch` | disable live reload |

### Keys

| Key | Action |
| --- | --- |
| `+` / `=` / `i`, `-` / `o`, mouse wheel | zoom (wheel zooms at the pointer) |
| `h j k l` / arrows, mouse drag, shift+wheel | pan |
| `0` / `f` | fit to window |
| `1` | 100% (one diagram pixel per terminal pixel) |
| `tab` / `n`, `shift+tab` / `p` | next / previous diagram |
| `t` | theme picker: `↑↓` previews, `enter` applies and saves, `esc` cancels |
| `r` | toggle kitty / half-block renderer |
| `s` | save the current diagram as PNG next to its source |
| `R` | reload |
| `?` | help |
| `q` | quit |

### Settings

The theme picked in the viewer is saved to
`$XDG_CONFIG_HOME/mergo/config.json` (usually `~/.config/mergo/config.json`)
and used by default by the viewer, `--print` and `--output`. Without
`XDG_CONFIG_HOME` the platform default is used (`~/Library/Application
Support/mergo` on macOS).

## Terminal support

`auto` mode sends a kitty graphics query followed by a device attributes
request; if the terminal acknowledges the query, kitty graphics are used,
otherwise half blocks.

Kitty images are positioned in one of two ways (`--kitty-placement`):

- **unicode** – a virtual placement plus a grid of Unicode placeholder
  cells. The image is part of the cell grid, so it survives redraws and
  composes with overlays. Requires kitty or Ghostty.
- **direct** – the image is placed at the body origin with `a=p`
  (cursor saved/restored, `C=1`, z-index below non-default backgrounds so
  help/error overlays still cover it). Used where the protocol is
  implemented without placeholders.

`auto` picks **direct** inside zellij (checked first, because zellij passes
the outer terminal's `KITTY_WINDOW_ID`/`TERM` through), **unicode** in
kitty, Ghostty and tmux, and **direct** everywhere else that answers the
graphics query.

| Terminal | Renderer |
| --- | --- |
| kitty, Ghostty | kitty graphics (Unicode placeholders) |
| zellij ≥ 0.45 inside a kitty-protocol terminal | kitty graphics (direct placement, no passthrough needed) |
| WezTerm, Konsole | kitty graphics (direct placement) |
| iTerm2, Alacritty, foot, GNOME Terminal, Windows Terminal, … | half blocks |
| tmux | half blocks by default; with `set -g allow-passthrough on` inside kitty/Ghostty use `-r kitty` |

zellij has to be allowed to use the protocol (`support_kitty_graphics_protocol`
is on by default), and the host terminal must support it.

## Supported syntax (highlights)

- **flowchart / graph** – all classic shapes and the v11 `A@{ shape: … }`
  shapes (doc, docs, cyl, h-cyl, lin-cyl, delay, notch-rect, hourglass, bolt,
  flag, tri, …), all link types (`-->`, `---`, `-.->`, `==>`, `~~~`, `--o`,
  `--x`, `<-->`, longer variants, text on links), chaining and `&`,
  subgraphs (nested, with their own `direction`, as edge endpoints),
  `classDef` / `class` / `style` / `linkStyle`, `curve` config.
- **sequence** – participants/actors with aliases, participant types
  (boundary, control, entity, database, collections, queue), all arrow types,
  activations (nested), notes, `loop`/`alt`/`opt`/`par`/`critical`/`break`,
  `rect` highlights, `box` groups, `create`/`destroy`, `autonumber`.
- **class** – members, visibility, static/abstract, generics (`~T~`),
  annotations, all relationships with cardinalities and labels, namespaces,
  notes, styling.
- **state** – composite states (nested, per-scope `[*]`), concurrency
  regions (`--`), fork/join/choice, notes, styling.
- **erDiagram** – attributes with keys and comments, all cardinalities
  (symbols and words), identifying/non-identifying relationships, aliases.
- **gantt** – dayjs `dateFormat`, strftime `axisFormat`, `tickInterval`,
  `excludes`/`includes`/`weekend`, `after`/`until`, durations, milestones,
  `vert` markers, `crit`/`done`/`active`, `displayMode compact`,
  `todayMarker`, `inclusiveEndDates`, `topAxis`.
- **pie**, **mindmap**, **gitGraph** (LR/TB/BT, merges, cherry-picks, tags,
  commit types), **timeline** (LR/TD), **journey**, **quadrantChart**,
  **xychart-beta** (bars, lines, horizontal, data labels).

## Development

```sh
go test -race ./...
golangci-lint run ./...
go run ./cmd/mmdpng -ascii 140 examples/flowchart/cicd.mmd   # ASCII preview of a render
go run ./cmd/mmdpng -theme nord examples/state/concurrency.mmd out.png
```

See [docs/ENGINE.md](docs/ENGINE.md) for the architecture of the rendering
engine.
