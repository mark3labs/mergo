package tui

import (
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

// Renderer selects how images are shown.
type Renderer int

const (
	// RendererAuto probes the terminal and picks kitty if available.
	RendererAuto Renderer = iota
	// RendererKitty uses the kitty graphics protocol.
	RendererKitty
	// RendererHalfBlock uses truecolor half block characters.
	RendererHalfBlock
)

// ParseRenderer parses "auto", "kitty" or "halfblock".
func ParseRenderer(s string) (Renderer, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "auto":
		return RendererAuto, true
	case "kitty", "graphics":
		return RendererKitty, true
	case "halfblock", "half-block", "half", "blocks", "ansi":
		return RendererHalfBlock, true
	}
	return RendererAuto, false
}

func (r Renderer) String() string {
	switch r {
	case RendererKitty:
		return "kitty"
	case RendererHalfBlock:
		return "half-block"
	}
	return "auto"
}

// kittyHint reports whether the environment suggests a terminal that
// implements the kitty graphics protocol (including Unicode placeholders).
func kittyHint(env func(string) string) bool {
	if env == nil {
		env = os.Getenv
	}
	term := strings.ToLower(env("TERM"))
	prog := strings.ToLower(env("TERM_PROGRAM"))
	switch {
	case strings.Contains(term, "kitty"), env("KITTY_WINDOW_ID") != "":
		return true
	case strings.Contains(term, "ghostty"), prog == "ghostty", env("GHOSTTY_RESOURCES_DIR") != "":
		return true
	case prog == "wezterm", env("WEZTERM_EXECUTABLE") != "":
		// WezTerm implements the protocol, but not Unicode placeholders.
		return false
	}
	return false
}

// inTmux reports whether we run inside tmux.
func inTmux() bool { return os.Getenv("TMUX") != "" }

// CellSize is the pixel size of one terminal cell.
type CellSize struct{ W, H int }

// Valid reports whether the size looks plausible.
func (c CellSize) Valid() bool { return c.W >= 2 && c.H >= 4 && c.W < 200 && c.H < 400 }

// defaultCellSize is used when the terminal doesn't tell us.
var defaultCellSize = CellSize{W: 9, H: 18}

// ttyCellSize queries the cell size via TIOCGWINSZ on the given fd.
func ttyCellSize(fd uintptr) (CellSize, bool) {
	ws, err := unix.IoctlGetWinsize(int(fd), unix.TIOCGWINSZ)
	if err != nil || ws.Col == 0 || ws.Row == 0 || ws.Xpixel == 0 || ws.Ypixel == 0 {
		return CellSize{}, false
	}
	cs := CellSize{W: int(ws.Xpixel) / int(ws.Col), H: int(ws.Ypixel) / int(ws.Row)}
	return cs, cs.Valid()
}

// terminalCellSize tries stdout, stdin and /dev/tty.
func terminalCellSize() CellSize {
	for _, f := range []*os.File{os.Stdout, os.Stdin, os.Stderr} {
		if cs, ok := ttyCellSize(f.Fd()); ok {
			return cs
		}
	}
	if f, err := os.Open("/dev/tty"); err == nil {
		defer func() { _ = f.Close() }()
		if cs, ok := ttyCellSize(f.Fd()); ok {
			return cs
		}
	}
	return defaultCellSize
}
