package tui

import (
	"math"

	"github.com/mark3labs/mergo/internal/scene"
)

// camera describes which part of a scene is shown. Zoom is expressed in
// "kitty pixels" (terminal pixels) per scene unit so that it is
// independent of the renderer; half-block rendering divides by the cell
// width.
type camera struct {
	zoom   float64 // terminal pixels per scene unit
	cx, cy float64 // scene coordinate at the center of the view
	fit    bool    // follow the fit zoom on resize
	init   bool
}

const (
	minZoom    = 0.05
	maxZoom    = 24.0
	maxFitZoom = 2.5
	zoomStep   = 1.25
)

// fitZoom returns the zoom that makes a scene of size sw x sh fit into a
// view of pw x ph terminal pixels with a small margin.
func fitZoom(sw, sh, pw, ph float64) float64 {
	if sw <= 0 || sh <= 0 || pw <= 0 || ph <= 0 {
		return 1
	}
	z := math.Min(pw/sw, ph/sh) * 0.94
	return clampf(z, minZoom, maxFitZoom)
}

// Fit centers the scene and picks the fit zoom.
func (c *camera) Fit(sw, sh, pw, ph float64) {
	c.zoom = fitZoom(sw, sh, pw, ph)
	c.cx, c.cy = sw/2, sh/2
	c.fit = true
	c.init = true
}

// Zoom multiplies the zoom by f keeping the scene point under the view
// position (ax, ay) (terminal pixels relative to the view's top-left)
// fixed.
func (c *camera) Zoom(f, ax, ay, pw, ph float64) {
	nz := clampf(c.zoom*f, minZoom, maxZoom)
	if nz == c.zoom {
		return
	}
	// scene point under the anchor
	sx := c.cx + (ax-pw/2)/c.zoom
	sy := c.cy + (ay-ph/2)/c.zoom
	c.zoom = nz
	c.cx = sx - (ax-pw/2)/nz
	c.cy = sy - (ay-ph/2)/nz
	c.fit = false
}

// SetZoom sets an absolute zoom around the view center.
func (c *camera) SetZoom(z float64) {
	c.zoom = clampf(z, minZoom, maxZoom)
	c.fit = false
}

// Pan moves the view by (dx, dy) terminal pixels.
func (c *camera) Pan(dx, dy float64) {
	c.cx += dx / c.zoom
	c.cy += dy / c.zoom
	c.fit = false
}

// Clamp keeps the view over the scene: if the scene is smaller than the
// view along an axis it is centered, otherwise the view may not scroll
// past the scene edges.
func (c *camera) Clamp(sw, sh, pw, ph float64) {
	vw, vh := pw/c.zoom, ph/c.zoom
	c.cx = clampAxis(c.cx, sw, vw)
	c.cy = clampAxis(c.cy, sh, vh)
}

func clampAxis(center, size, view float64) float64 {
	if view >= size {
		return size / 2
	}
	return clampf(center, view/2, size-view/2)
}

// Viewport returns the visible scene rectangle for a view of pw x ph
// pixels at the given pixels-per-unit scale.
func (c *camera) Viewport(pw, ph, scale float64) scene.Rect {
	vw, vh := pw/scale, ph/scale
	return scene.Rect{X: c.cx - vw/2, Y: c.cy - vh/2, W: vw, H: vh}
}

func clampf(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
