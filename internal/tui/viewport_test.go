package tui

import (
	"math"
	"testing"

	"github.com/mark3labs/mergo/internal/scene"
)

func TestViewportFit(t *testing.T) {
	tests := []struct {
		name          string
		viewW, viewH  int
		sceneW, sceneH float64
		expectedScale float64
	}{
		{
			name:          "square scene, square viewport",
			viewW: 10, viewH: 10,
			sceneW: 100, sceneH: 100,
			expectedScale: 10,
		},
		{
			name:          "wide scene, square viewport",
			viewW: 10, viewH: 10,
			sceneW: 200, sceneH: 100,
			expectedScale: 20, // Limited by width (scene wider than viewport)
		},
		{
			name:          "tall scene, square viewport",
			viewW: 10, viewH: 10,
			sceneW: 100, sceneH: 200,
			expectedScale: 20, // Limited by height (scene taller than viewport)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vp := NewViewport(tt.viewW, tt.viewH)
			vp.SetScene(tt.sceneW, tt.sceneH)

			if math.Abs(vp.Scale-tt.expectedScale) > 0.01 {
				t.Errorf("Scale: got %.2f, want %.2f", vp.Scale, tt.expectedScale)
			}
		})
	}
}

func TestViewportClamp(t *testing.T) {
	vp := NewViewport(10, 10)
	vp.SetScene(100, 100)

	// Pan out of bounds
	vp.X = -50
	vp.Y = -50
	vp.Clamp()

	if vp.X != 0 || vp.Y != 0 {
		t.Errorf("Lower bounds not clamped: X=%.0f, Y=%.0f", vp.X, vp.Y)
	}

	// Pan to upper bounds
	vp.X = 500
	vp.Y = 500
	vp.Clamp()

	viewW := float64(vp.Width) * vp.Scale
	viewH := float64(vp.Height) * vp.Scale

	if vp.X+viewW > vp.sceneWidth || vp.Y+viewH > vp.sceneHeight {
		t.Errorf("Upper bounds not clamped: X=%.0f, Y=%.0f", vp.X, vp.Y)
	}
}

func TestViewportPan(t *testing.T) {
	vp := NewViewport(100, 100)
	vp.SetScene(1000, 1000)
	vp.Scale = 1

	initialX := vp.X
	vp.PanRight(10)

	if vp.X-initialX < 9 || vp.X-initialX > 11 {
		t.Errorf("PanRight: expected ~10 units, got %.1f", vp.X-initialX)
	}
}

func TestViewportRect(t *testing.T) {
	vp := NewViewport(10, 10)
	vp.SetScene(100, 100)

	rect := vp.Rect()
	if !isRect(rect, scene.Rect{W: float64(vp.Width) * vp.Scale, H: float64(vp.Height) * vp.Scale}) {
		t.Errorf("Rect dimensions don't match viewport: %+v", rect)
	}
}

func isRect(a, b scene.Rect) bool {
	return math.Abs(a.W-b.W) < 0.01 && math.Abs(a.H-b.H) < 0.01
}

func TestViewportZoom(t *testing.T) {
	vp := NewViewport(100, 100)
	vp.SetScene(1000, 1000)

	initialScale := vp.Scale
	vp.ZoomIn()
	if vp.Scale >= initialScale {
		t.Errorf("ZoomIn didn't decrease scale")
	}

	vp.SetScale(initialScale)
	vp.ZoomOut()
	if vp.Scale <= initialScale {
		t.Errorf("ZoomOut didn't increase scale")
	}
}
