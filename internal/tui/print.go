package tui

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"

	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

// Run starts the interactive viewer.
func Run(ctx context.Context, paths []string, diagrams []*Diagram, opts Options) error {
	m := NewModel(paths, diagrams, opts)
	p := tea.NewProgram(m, tea.WithContext(ctx))
	_, err := p.Run()
	return err
}

// PrintOptions configures Print.
type PrintOptions struct {
	Theme     string
	Renderer  Renderer
	NoShadows bool
	// Width in cells (0 = terminal width).
	Width int
	// MaxHeight in cells (0 = unlimited).
	MaxHeight int
}

// Print renders a diagram inline to w (like cat for diagrams).
func Print(w io.Writer, d *Diagram, o PrintOptions) error {
	th, err := theme.Get(o.Theme)
	if err != nil {
		return err
	}
	sc, err := parseScene(d.Source, th.Name)
	if err != nil {
		return err
	}
	cols := o.Width
	if cols <= 0 {
		cols = 100
		if tw, _, err := term.GetSize(os.Stdout.Fd()); err == nil && tw > 0 {
			cols = tw
		}
	}
	cell := terminalCellSize()
	mode := o.Renderer
	if mode == RendererAuto {
		if graphicsHint(nil) {
			mode = RendererKitty
		} else {
			mode = RendererHalfBlock
		}
	}

	// Never upscale small diagrams too much: at most 1.5 terminal px per unit.
	pxW := float64(cols * cell.W)
	zoom := math.Min(pxW/sc.Width, 1.5)
	imgW := sc.Width * zoom
	imgH := sc.Height * zoom
	cols = max(int(math.Ceil(imgW/float64(cell.W))), 1)
	rows := max(int(math.Ceil(imgH/float64(cell.H))), 1)
	if o.MaxHeight > 0 && rows > o.MaxHeight {
		f := float64(o.MaxHeight) / float64(rows)
		zoom *= f
		cols = max(int(math.Ceil(sc.Width*zoom/float64(cell.W))), 1)
		rows = o.MaxHeight
	}

	switch mode {
	case RendererKitty:
		img := sc.Render(scene.RenderOptions{Scale: zoom, NoShadows: o.NoShadows})
		data, err := encodePNG(img)
		if err != nil {
			return err
		}
		id := imageIDBase() + 7
		_, err = fmt.Fprint(w, kittyTransmitDisplay(id, data, cols, rows, inTmux() && !inZellij(nil))+"\n")
		return err
	default:
		// half blocks: cols x rows*2 pixels, supersampled
		s := float64(cols) / sc.Width
		rows = max(int(math.Ceil(sc.Height*s/2)), 1)
		if o.MaxHeight > 0 && rows > o.MaxHeight {
			s *= float64(o.MaxHeight) / float64(rows)
			rows = o.MaxHeight
			cols = max(int(math.Ceil(sc.Width*s)), 1)
		}
		vp := scene.Rect{W: float64(cols) / s, H: float64(rows*2) / s}
		img := sc.Render(scene.RenderOptions{Scale: s * halfBlockSupersample, Viewport: vp, NoShadows: o.NoShadows})
		lines := halfBlocks(img, cols, rows)
		_, err := fmt.Fprintln(w, strings.Join(lines, "\x1b[0m\n")+"\x1b[0m")
		return err
	}
}

// Export renders a diagram to a PNG file.
func Export(path string, d *Diagram, themeName string, scale float64, noShadows bool) error {
	sc, err := parseScene(d.Source, themeName)
	if err != nil {
		return err
	}
	if scale <= 0 {
		scale = 2
	}
	data, err := encodePNG(sc.Render(scene.RenderOptions{Scale: scale, NoShadows: noShadows}))
	if err != nil {
		return err
	}
	if path == "-" {
		_, err = os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
