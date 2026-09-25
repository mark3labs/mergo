package tui

import (
	"math"

	"github.com/mark3labs/mergo/internal/scene"
)

// Viewport manages pan and zoom state.
type Viewport struct {
	Width  int     // Terminal cell width
	Height int     // Terminal cell height
	X      float64 // Pan offset in scene units
	Y      float64
	Scale  float64 // Zoom level (scene units per device pixel)

	sceneWidth  float64
	sceneHeight float64
}

// NewViewport creates a new viewport.
func NewViewport(w, h int) Viewport {
	return Viewport{
		Width:  w,
		Height: h,
		Scale:  1,
	}
}

// SetScene sets the scene bounds and fits the viewport.
func (vp *Viewport) SetScene(w, h float64) {
	vp.sceneWidth = w
	vp.sceneHeight = h
	vp.Fit()
}

// Rect returns the scene region currently visible.
func (vp *Viewport) Rect() scene.Rect {
	return scene.Rect{
		X: vp.X,
		Y: vp.Y,
		W: float64(vp.Width) * vp.Scale,
		H: float64(vp.Height) * vp.Scale,
	}
}

// Fit scales and positions to show the entire scene centered and letterboxed.
func (vp *Viewport) Fit() {
	if vp.sceneWidth <= 0 || vp.sceneHeight <= 0 {
		vp.Scale = 1
		vp.X = 0
		vp.Y = 0
		return
	}

	viewportAspect := float64(vp.Width) / float64(vp.Height)
	sceneAspect := vp.sceneWidth / vp.sceneHeight

	var scale float64
	if viewportAspect > sceneAspect {
		// Fit height
		scale = vp.sceneHeight / float64(vp.Height)
	} else {
		// Fit width
		scale = vp.sceneWidth / float64(vp.Width)
	}

	vp.Scale = math.Max(scale, 0.01)
	vp.Center()
}

// Center positions the scene centered in the viewport.
func (vp *Viewport) Center() {
	viewWidth := float64(vp.Width) * vp.Scale
	viewHeight := float64(vp.Height) * vp.Scale

	vp.X = (vp.sceneWidth - viewWidth) / 2
	vp.Y = (vp.sceneHeight - viewHeight) / 2

	vp.Clamp()
}

// Clamp ensures the viewport doesn't go out of bounds.
func (vp *Viewport) Clamp() {
	viewWidth := float64(vp.Width) * vp.Scale
	viewHeight := float64(vp.Height) * vp.Scale

	if vp.X < 0 {
		vp.X = 0
	}
	if vp.Y < 0 {
		vp.Y = 0
	}
	if vp.X+viewWidth > vp.sceneWidth {
		vp.X = vp.sceneWidth - viewWidth
		if vp.X < 0 {
			vp.X = 0
		}
	}
	if vp.Y+viewHeight > vp.sceneHeight {
		vp.Y = vp.sceneHeight - viewHeight
		if vp.Y < 0 {
			vp.Y = 0
		}
	}
}

// Pan by the given amount in screen pixels.
func (vp *Viewport) Pan(dx, dy float64) {
	vp.X -= dx * vp.Scale
	vp.Y -= dy * vp.Scale
	vp.Clamp()
}

// PanLeft pans left by dx pixels.
func (vp *Viewport) PanLeft(dx float64) {
	vp.Pan(dx, 0)
}

// PanRight pans right by dx pixels.
func (vp *Viewport) PanRight(dx float64) {
	vp.Pan(-dx, 0)
}

// PanUp pans up by dy pixels.
func (vp *Viewport) PanUp(dy float64) {
	vp.Pan(0, dy)
}

// PanDown pans down by dy pixels.
func (vp *Viewport) PanDown(dy float64) {
	vp.Pan(0, -dy)
}

// ZoomIn increases zoom.
func (vp *Viewport) ZoomIn() {
	vp.SetScale(vp.Scale / 1.2)
}

// ZoomOut decreases zoom.
func (vp *Viewport) ZoomOut() {
	vp.SetScale(vp.Scale * 1.2)
}

// SetScale sets the zoom level directly (scene units per device pixel).
func (vp *Viewport) SetScale(s float64) {
	s = math.Max(s, 0.01)
	s = math.Min(s, 100)
	vp.Scale = s
	vp.Clamp()
}

// CellSize represents the pixel dimensions of a terminal cell.
type CellSize struct {
	Width  int
	Height int
}
