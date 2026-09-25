---
description: Visually verify the viewer TUI in tmux (keys, mouse, overlays) for a change
---

Build mergo and check the terminal viewer in a real tmux session, then report exactly what is on screen. Scenario or change to verify (optional): $@

Use this after any change to `internal/tui` (rendering, keys, mouse, help, theme picker, status bar), and whenever unit tests can't show what the user will actually see. For changes to how a diagram itself is drawn, prefer the render check at the end.

## Steps

1. **Build to /tmp** (never into the repo):

       go build -o /tmp/mergo .

2. **Start a detached session** at a fixed size. Use a file from `examples/` (`examples/showcase.md` when you need several diagrams to tab between). Use an isolated config dir so the theme picker doesn't overwrite the user's settings. tmux does not pass kitty graphics through by default, so force half blocks:

       tmux kill-session -t mg 2>/dev/null
       tmux new-session -d -s mg -x 100 -y 30 "XDG_CONFIG_HOME=/tmp/mergo-tui-check COLORTERM=truecolor /tmp/mergo -r halfblock examples/showcase.md"
       sleep 1

   Also check a narrow size (for example `-x 64 -y 20`) when layout, the status bar, the help overlay or the theme picker changed

3. **Drive the UI**:
   - Keys (see `keyMap` in `internal/tui/styles.go`): `tmux send-keys -t mg -l '+'` (zoom in), `-`, `0` (fit), `1` (100%), `h j k l` (pan), `Tab` / `BTab` (next / previous diagram), `t` (theme picker), `r` (renderer), `R` (reload), `?` (help), `q` (quit)
   - Named keys: `tmux send-keys -t mg Escape`, `Enter`, `Tab`, `BTab` (shift+tab), `Up`, `Down`
   - Send `Escape` on its own and wait about 0.2s before the next key; otherwise the next key is read as an alt+key combination
   - Don't press `s` unless you mean it: it writes a PNG next to the source file
   - Mouse, as SGR sequences. Coordinates are **1-based** (0-based screen x/y + 1); `M` is press and `m` is release:

         tmux send-keys -t mg -l $'\e[<0;12;4M'$'\e[<0;12;4m'   # left click at x=11, y=3
         tmux send-keys -t mg -l $'\e[<65;40;15M'                # wheel down (zoom out at the pointer; 64 = up)
         tmux send-keys -t mg -l $'\e[<0;40;15M'$'\e[<32;50;18M'$'\e[<0;50;18m'   # drag to pan

   - To target an element (a theme picker entry, say), find its coordinates from a capture instead of guessing, e.g. a Python one-liner over `tmux capture-pane -t mg -p` that prints the line number and `.index('nord')`
   - `sleep` after actions: about 0.3s for UI updates, about 1s after zoom, theme or diagram changes (the viewport is re-rasterized)

4. **Capture and inspect**:
   - Text: `tmux capture-pane -t mg -p` (use `sed -n 'A,Bp'` to show the relevant rows). Half-block images show as `▀` rows; check the diagram fills the body and isn't clipped by the header or status bar
   - Styles and colors: `tmux capture-pane -t mg -p -e`. Look for `38;2;R;G;B` / `48;2;R;G;B` codes to confirm theme colors in the bars, panels and the selected picker entry
   - Check:
     - the header (diagram title, index / count) and status bar (zoom, renderer, theme, hints)
     - overlays (help, theme picker, errors, toasts) centered and not clipped
     - the theme picker previews on `Up`/`Down`, `Enter` keeps the theme, `Escape` restores the previous one
     - nothing left on screen from a previous frame

5. **Clean up**: `tmux kill-session -t mg` and `rm -rf /tmp/mergo-tui-check`. If the app was told to quit, confirm it exited (`tmux has-session -t mg` fails). Remove any stray PNGs a test `s` press wrote (`git status --short`)

6. **Report**: paste the relevant captured screen regions for each step, state pass or fail against the expected behavior, and describe any visual defect precisely (row, column, what's wrong)

## Render check (diagram output, no TUI)

For changes to a parser, layout, scene or theme:

    go run ./cmd/mmdpng -ascii 140 examples/<type>/<file>.mmd
    go run . -o /tmp/out.png -t <theme> examples/<type>/<file>.mmd

Run it on every example of the affected type (and `--background light` for theme work). Paste the ASCII previews, and look at the PNGs when shapes, arrowheads or text placement matter; ASCII only shows coarse layout.

## Guidelines

- Always capture before claiming something works. Screenshots in text form are the evidence
- Test both the keyboard and the mouse path for interactive features; they should behave the same
- If a capture looks wrong, first rule out an off-by-one in your own mouse coordinates before concluding the app is broken
- The kitty renderer can't be checked in plain tmux. If the change is kitty-specific, say so and ask the user to verify in kitty or Ghostty
