package tui

import (
	"os"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
	"golang.org/x/sys/unix"
)

// DarkBackground asks the terminal whether its background is dark, like
// gopyter does. Call it before the TUI starts so the query can't race the
// program's input reader. Without a terminal to ask (or when running in the
// background, where touching the terminal would stop the process) it
// assumes dark.
func DarkBackground() bool {
	// stdin may carry the diagram and stdout the PNG: talk to the
	// controlling terminal directly.
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return true
	}
	defer func() { _ = tty.Close() }()
	if !term.IsTerminal(tty.Fd()) || !foreground(tty) {
		return true
	}
	return lipgloss.HasDarkBackground(tty, tty)
}

// foreground reports whether this process is in the terminal's foreground
// process group.
func foreground(tty *os.File) bool {
	pgrp, err := unix.IoctlGetInt(int(tty.Fd()), unix.TIOCGPGRP)
	return err == nil && pgrp == unix.Getpgrp()
}

// Renderer selects how images are shown.
type Renderer int

const (
	// RendererAuto probes the terminal and picks kitty if available.
	RendererAuto Renderer = iota
	// RendererKitty uses the kitty graphics protocol.
	RendererKitty
	// RendererHalfBlock uses truecolor half block characters.
	RendererHalfBlock
	// RendererSixel uses sixel graphics.
	RendererSixel
)

// ParseRenderer parses "auto", "kitty", "sixel" or "halfblock".
func ParseRenderer(s string) (Renderer, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "auto":
		return RendererAuto, true
	case "kitty", "graphics":
		return RendererKitty, true
	case "sixel", "sixels":
		return RendererSixel, true
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
	case RendererSixel:
		return "sixel"
	}
	return "auto"
}

// Placement selects how kitty images are positioned on screen.
type Placement int

const (
	// PlacementAuto picks the best placement for the environment.
	PlacementAuto Placement = iota
	// PlacementUnicode uses virtual placements + Unicode placeholder cells
	// (kitty, Ghostty; tmux with allow-passthrough).
	PlacementUnicode
	// PlacementDirect places the image at a cursor position with a=p. Used
	// for terminals and multiplexers that implement the graphics protocol
	// but not Unicode placeholders (zellij >= 0.45, WezTerm, Konsole).
	PlacementDirect
)

// ParsePlacement parses "auto", "unicode"/"placeholder" or "direct".
func ParsePlacement(s string) (Placement, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "auto":
		return PlacementAuto, true
	case "unicode", "placeholder", "placeholders":
		return PlacementUnicode, true
	case "direct":
		return PlacementDirect, true
	}
	return PlacementAuto, false
}

func (p Placement) String() string {
	switch p {
	case PlacementUnicode:
		return "unicode"
	case PlacementDirect:
		return "direct"
	}
	return "auto"
}

// inZellij reports whether we run inside a zellij session.
func inZellij(env func(string) string) bool {
	if env == nil {
		env = os.Getenv
	}
	return env("ZELLIJ") != "" || env("ZELLIJ_SESSION_NAME") != ""
}

// resolvePlacement picks the placement strategy for the environment.
// zellij implements the kitty protocol itself (no passthrough needed) but
// does not support Unicode placeholders, and it forwards the outer
// terminal's environment (e.g. KITTY_WINDOW_ID), so it must be checked
// first.
func resolvePlacement(p Placement, env func(string) string) Placement {
	if p != PlacementAuto {
		return p
	}
	if env == nil {
		env = os.Getenv
	}
	switch {
	case inZellij(env):
		return PlacementDirect
	case env("TMUX") != "":
		// tmux does not track kitty placements; placeholders are the only
		// thing that survives redraws (requires allow-passthrough).
		return PlacementUnicode
	case kittyHint(env):
		return PlacementUnicode
	}
	return PlacementDirect
}

// graphicsHint reports whether the environment suggests any terminal that
// can display kitty graphics (with or without Unicode placeholders).
func graphicsHint(env func(string) string) bool {
	if env == nil {
		env = os.Getenv
	}
	prog := strings.ToLower(env("TERM_PROGRAM"))
	return kittyHint(env) || prog == "wezterm" || env("WEZTERM_EXECUTABLE") != "" || env("KONSOLE_VERSION") != ""
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

// sixelHint reports whether the environment suggests a terminal known to
// display sixel graphics. Used by print mode when the terminal can't be
// asked (the interactive viewer relies on the device attributes).
func sixelHint(env func(string) string) bool {
	if env == nil {
		env = os.Getenv
	}
	term := strings.ToLower(env("TERM"))
	prog := strings.ToLower(env("TERM_PROGRAM"))
	for _, t := range []string{"foot", "mlterm", "contour", "yaft", "sixel"} {
		if strings.Contains(term, t) {
			return true
		}
	}
	switch prog {
	case "iterm.app", "mintty", "contour", "blackbox":
		return true
	}
	return false
}

// hasSixel reports whether primary device attributes advertise sixel
// graphics (attribute 4).
func hasSixel(attrs []int) bool {
	return slices.Contains(attrs, 4)
}

// querySixel asks the controlling terminal for its primary device
// attributes and reports whether they include sixel graphics. Like
// DarkBackground, it only talks to a foreground terminal.
func querySixel(timeout time.Duration) bool {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return false
	}
	defer func() { _ = tty.Close() }()
	if !term.IsTerminal(tty.Fd()) || !foreground(tty) {
		return false
	}
	state, err := term.MakeRaw(tty.Fd())
	if err != nil {
		return false
	}
	defer func() { _ = term.Restore(tty.Fd(), state) }()

	rd, err := uv.NewCancelReader(tty)
	if err != nil {
		return false
	}
	defer func() { _ = rd.Close() }()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-done:
		case <-time.After(timeout):
			rd.Cancel()
		}
	}()
	if _, err := tty.WriteString(ansi.RequestPrimaryDeviceAttributes); err != nil {
		return false
	}

	pa := ansi.NewParser()
	var buf [256]byte
	var st byte
	for {
		n, err := rd.Read(buf[:])
		if err != nil {
			return false
		}
		p := buf[:n]
		for len(p) > 0 {
			seq, _, read, ns := ansi.DecodeSequence(p, st, pa)
			st = ns
			p = p[read:]
			if ns != ansi.NormalState || !ansi.HasCsiPrefix(seq) || pa.Command() != ansi.Command('?', 0, 'c') {
				continue
			}
			var attrs []int
			for _, prm := range pa.Params() {
				attrs = append(attrs, prm.Param(0))
			}
			return hasSixel(attrs)
		}
	}
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
