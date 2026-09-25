# mergo

A native Mermaid diagram renderer for the terminal, written in pure Go. Renders Mermaid diagrams with a professional-quality rasterizer and displays them using the Kitty graphics protocol or half-block Unicode characters.

## Features

- **Native rendering**: Pure Go rasterizer with no external dependencies on browsers or JavaScript
- **Kitty graphics protocol**: Crisp, pixel-perfect rendering in Kitty terminal
- **Half-block fallback**: Beautiful truecolor rendering using Unicode half-blocks (▀) in any terminal
- **Interactive TUI**: Pan, zoom, switch between diagrams, cycle themes on the fly
- **Markdown support**: Extract and render all `mermaid` code blocks from `.md` files
- **Theme support**: Built-in themes (default, dark, forest, neutral, charm) with live switching
- **Export to PNG**: Render diagrams to PNG files with configurable scaling
- **Stdin support**: Pipe Mermaid diagrams to render them directly

## Supported Diagram Types

All modern Mermaid diagram types are supported:

- **Flowchart** (flowchart TD/LR, graph)
- **Sequence diagrams** (sequence, message)
- **Class diagrams** (class)
- **State diagrams** (state, stateDiagram)
- **ER diagrams** (erDiagram, entity)
- **Gantt charts** (gantt)
- **Pie charts** (pie)
- **Git graph** (gitGraph)
- **Journey** (journey)
- **Mindmap** (mindmap)
- **Timeline** (timeline)
- **XY chart** (xychart)
- **Quadrant chart** (quadrant)

## Installation

```bash
go install github.com/mark3labs/mergo@latest
```

Or from source:

```bash
git clone https://github.com/mark3labs/mergo
cd mergo
go build .
./mergo -h
```

## Usage

### Interactive viewer (default)

```bash
# View a Mermaid diagram
mergo examples/pie/basic.mmd

# View with a specific theme
mergo -t dark examples/flowchart/basic.mmd

# View from stdin
cat diagram.mmd | mergo

# View all diagrams in a Markdown file
mergo document.md
```

### Non-interactive export

```bash
# Export to PNG
mergo -o diagram.png examples/pie/basic.mmd

# Export with custom scaling
mergo -o diagram.png --scale 3 examples/pie/basic.mmd

# Print to stdout (sized to terminal)
mergo -p examples/pie/basic.mmd

# Export a specific diagram from a Markdown file
mergo -o diagram2.png --index 1 document.md
```

## Keybindings (Interactive Mode)

| Key | Action |
|-----|--------|
| `q`, `Ctrl+C` | Quit |
| `+` / `-` | Zoom in/out |
| `1` | Fit to window (actual size) |
| `f`, `0` | Fit diagram in view |
| `h`, `j`, `k`, `l` or arrows | Pan (left/down/up/right) |
| `Tab`, `Shift+Tab` or `n`, `p` | Next/previous diagram (when multiple) |
| `t` | Cycle theme |
| `r` | Toggle renderer (Kitty/half-block) |
| `s` | Save current diagram as PNG |
| `R` | Reload source file |
| `?` | Show help |

## Command-line Options

```
Usage:
  mergo [flags] [file ...]

Flags:
  -t, --theme string        diagram theme (default "default")
      --renderer string     renderer: auto, kitty, or halfblock (default "auto")
  -o, --output file         write rendered diagram to PNG and exit (non-interactive)
  -p, --print              print diagram to stdout and exit (non-interactive)
      --index N            diagram index for export/print (0-indexed, default 0)
      --scale N            render scale for export/print (default 2)
      --font path          font file path (e.g. /path/to/font.ttf)
      --font-bold path     bold font file path
      --no-shadows         disable drop shadows
  -h, --help               show help
      --version            show version

Themes:
  - default
  - dark
  - forest
  - neutral
  - charm
```

## Terminal Support

### Kitty Terminal (Recommended)

Automatic detection of Kitty graphics protocol support. The renderer sends a probe query at startup and falls back gracefully if not supported.

Environment hints for auto-detection:
- `TERM=xterm-kitty`
- `KITTY_WINDOW_ID` environment variable

### Half-block Fallback

All terminals with truecolor support (16 million colors) are supported via Unicode half-blocks. Includes:
- Modern terminals: Kitty (when graphics disabled), WezTerm, iTerm2, GNOME Terminal, etc.
- SSH sessions with proper terminal support
- Terminal multiplexers (tmux, screen) with passthrough configuration

### Terminal Detection

The `auto` renderer (default) detects capabilities at startup:

1. Sends Kitty graphics capability probe
2. Requests primary device attributes (DA1)
3. Queries terminal cell pixel size
4. Falls back to half-blocks after ~1.5 second timeout

For tmux, graphics escapes are wrapped in DCS passthrough and cell size is queried via `ioctl`.

## Architecture

### Rendering Pipeline

```
Mermaid source
    ↓
diagram.Preprocess (strip comments, detect type)
    ↓
<type>.Render (parse diagram to scene.Scene)
    ↓
scene.Render (rasterize at device pixel size)
    ↓
[graphics.renderKitty or graphics.renderHalfBlock]
    ↓
Terminal display
```

### Key Packages

- **`internal/scene`**: Vector primitives, text layout, and rasterization (via gg)
- **`internal/mermaid`**: Public API and diagram type registry
- **`internal/diagram`**: Diagram preprocessing and rendering dispatch
- **`internal/theme`**: Color themes and styling
- **`internal/tui`**: Terminal UI, graphics rendering, viewport management
- **`internal/diagrams/*`**: Diagram-type-specific renderers

## Development

### Building

```bash
go build .
```

### Running tests

```bash
go test -v ./internal/tui/...
```

### Testing with a real terminal

```bash
# In Kitty terminal
./mergo examples/pie/basic.mmd

# Force half-block rendering
MERGO_RENDERER=halfblock ./mergo examples/pie/basic.mmd

# Render to PNG
./mergo -o /tmp/test.png examples/pie/basic.mmd
```

### Key Implementation Details

**Kitty Graphics Protocol**:
- Uses Unicode placeholder protocol (U+10EEEE with row/col diacritics)
- Double-buffering with image IDs 0 and 1 to avoid flicker
- Chunked transmission with base64 encoding
- Automatic cleanup on exit

**Half-block Rendering**:
- Area-average downsampling from raster image
- Each cell = one half-block with foreground (top) and background (bottom) colors
- Full truecolor (24-bit RGB) support
- Properly handles gamma-aware color averaging (when available)

**Viewport Math**:
- Maintains aspect ratio fit with letterboxing
- Clamped pan to keep content visible
- Smooth zoom with configurable limits (0.01–100×)
- Scene-relative coordinates

## Known Limitations

1. **Terminal support**: Requires a terminal with true graphics or truecolor support (very rare to be missing in 2024)
2. **File watching**: Only checks mtime every 500ms
3. **Rendering performance**: Very large diagrams may take a moment to render
4. **Fonts**: Custom fonts work best with `-o` / `-p` export modes; TUI font switching is planned
5. **Incomplete diagram types**: Some advanced Mermaid features (actor backgrounds, complex clusters) may render differently than the JS implementation

## Performance

- **Parse time**: ~1–5ms for typical diagrams
- **Render time**: 10–100ms at 2× scale depending on complexity
- **Memory**: ~50–200MB for interactive mode (diagram cache + renderer state)

## License

MIT

## Contributing

Contributions welcome! Please open issues for bugs or feature requests, and PRs for code changes.
