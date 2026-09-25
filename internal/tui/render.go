package tui

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"image"
	"image/png"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mergo/internal/mermaid"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

// sceneKey identifies a parsed scene.
type sceneKey struct {
	diag  int
	hash  uint64
	theme string
}

func hashSource(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

// sceneEntry is a cached parse result.
type sceneEntry struct {
	sc  *scene.Scene
	err error
}

// parseScene parses and lays out a diagram.
func parseScene(src, themeName string) (sc *scene.Scene, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("internal error: %v", r)
		}
	}()
	th, err := theme.Get(themeName)
	if err != nil {
		th = theme.Default()
	}
	return mermaid.Render(src, th)
}

// renderJob is a snapshot of everything needed to rasterize one frame.
type renderJob struct {
	gen       int
	sc        *scene.Scene
	mode      Renderer
	cols      int
	rows      int
	cell      CellSize
	cam       camera
	noShadows bool
	imgID     int
	tmux      bool
}

// renderResult is the output of a render job.
type renderResult struct {
	gen   int
	mode  Renderer
	lines []string // body lines (placeholders or half blocks)
	raw   string   // escape sequences to write before showing lines (kitty)
	imgID int
	err   error
}

// halfBlockSupersample is the supersampling factor used for half blocks.
const halfBlockSupersample = 3

// viewPixels returns the size of the view in terminal pixels.
func viewPixels(cols, rows int, cell CellSize) (float64, float64) {
	return float64(cols * cell.W), float64(rows * cell.H)
}

// run executes the job.
func (j renderJob) run() (res renderResult) {
	res = renderResult{gen: j.gen, mode: j.mode, imgID: j.imgID}
	defer func() {
		if r := recover(); r != nil {
			res.err = fmt.Errorf("render failed: %v", r)
		}
	}()
	if j.sc == nil || j.cols <= 0 || j.rows <= 0 {
		return res
	}
	pw, ph := viewPixels(j.cols, j.rows, j.cell)
	switch j.mode {
	case RendererKitty:
		vp := j.cam.Viewport(pw, ph, j.cam.zoom)
		img := j.sc.Render(scene.RenderOptions{Scale: j.cam.zoom, Viewport: vp, NoShadows: j.noShadows})
		data, err := encodePNG(img)
		if err != nil {
			res.err = err
			return res
		}
		res.raw = kittyDelete(j.imgID, j.tmux) +
			kittyTransmit(j.imgID, data, j.tmux) +
			kittyVirtualPlacement(j.imgID, j.cols, j.rows, j.tmux)
		res.lines = placeholderGrid(j.imgID, j.cols, j.rows)
	default:
		// Half blocks: one cell = 1 x 2 "pixels". A terminal pixel maps to
		// 1/cell.W half-block pixels horizontally.
		s := j.cam.zoom / float64(j.cell.W)
		hw, hh := float64(j.cols), float64(j.rows*2)
		vp := j.cam.Viewport(hw, hh, s)
		ss := float64(halfBlockSupersample)
		img := j.sc.Render(scene.RenderOptions{Scale: s * ss, Viewport: vp, NoShadows: j.noShadows})
		res.lines = halfBlocks(img, j.cols, j.rows)
	}
	return res
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// savePath returns where "save" writes the PNG for a diagram.
func savePath(d *Diagram) string {
	if d.Path == "-" {
		return fmt.Sprintf("diagram-%d.png", d.Block+1)
	}
	base := strings.TrimSuffix(d.Path, filepath.Ext(d.Path))
	if isMarkdown(d.Path) {
		return fmt.Sprintf("%s-%d.png", base, d.Block+1)
	}
	return base + ".png"
}
