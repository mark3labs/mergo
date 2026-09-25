package pie

import (
	"fmt"
	"image/color"
	"math"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

func init() {
	diagram.Register(diagram.Type{
		Name:   "pie",
		Detect: diagram.Keyword("pie"),
		Render: Render,
	})
}

// Render renders a pie chart.
func Render(src string, cfg *diagram.Config) (*scene.Scene, error) {
	doc, err := Parse(src)
	if err != nil {
		return nil, err
	}

	th := cfg.Theme
	sc := diagram.NewScene(th)

	if len(doc.Slices) == 0 {
		return sc, nil
	}

	// Calculate total
	total := 0.0
	for _, s := range doc.Slices {
		if s.Value > 0 {
			total += s.Value
		}
	}
	if total <= 0 {
		return sc, nil
	}

	// Get text position from config
	textPosition := cfg.Float("pie", "textPosition", 0.75)

	// Draw pie chart centered
	centerX := 150.0
	centerY := 120.0
	radius := 80.0

	// Draw slices
	angle := -math.Pi / 2 // Start at top
	legendX := centerX + radius + 80
	legendY := centerY - (float64(len(doc.Slices)) * 20 / 2)

	for i, slice := range doc.Slices {
		sliceAngle := (slice.Value / total) * 2 * math.Pi
		endAngle := angle + sliceAngle

		// Draw slice
		drawPieSlice(sc, centerX, centerY, radius, angle, endAngle, th.PaletteColor(i), th)

		// Draw percentage label inside slice
		if slice.Value/total > 0.05 { // Only for slices > 5%
			drawSliceLabel(sc, centerX, centerY, radius, angle, endAngle, slice.Value, total, textPosition, th)
		}

		// Draw legend
		drawLegendEntry(sc, legendX, legendY+float64(i)*24, slice.Label, slice.Value, doc.ShowData, i, th)

		angle = endAngle
	}

	// Draw title if present
	if doc.Title != "" {
		titleFont := diagram.BoldFont(th, 1.125)
		sc.Add(scene.NewText(centerX, centerY-radius-40, doc.Title, titleFont, th.TitleColor, scene.AnchorMiddle, scene.VAlignBottom))
	}

	sc.Fit(diagram.Pad)
	return sc, nil
}

func drawPieSlice(sc *scene.Scene, cx, cy, r float64, startAngle, endAngle float64, fill color.RGBA, th *theme.Theme) {
	// Draw a pie slice using a path with arc
	p := scene.NewPath(scene.Style{
		Fill:        fill,
		Stroke:      th.Background,
		StrokeWidth: 1.5,
	})

	startX := cx + r*math.Cos(startAngle)
	startY := cy + r*math.Sin(startAngle)

	p.MoveTo(cx, cy).
		LineTo(startX, startY).
		Arc(cx, cy, r, r, startAngle, endAngle, true).
		LineTo(cx, cy).
		Close()

	sc.Add(p)
}

func drawSliceLabel(sc *scene.Scene, cx, cy, r float64, startAngle, endAngle float64, value, total float64, textPosition float64, th *theme.Theme) {
	// Label position at textPosition radius (0..1)
	midAngle := (startAngle + endAngle) / 2
	labelR := r * textPosition
	labelX := cx + labelR*math.Cos(midAngle)
	labelY := cy + labelR*math.Sin(midAngle)

	percent := (value / total) * 100
	label := fmt.Sprintf("%.0f%%", percent)

	font := diagram.Font(th, 0.85)
	sc.Add(scene.NewText(labelX, labelY, label, font, th.TextColor, scene.AnchorMiddle, scene.VAlignMiddle))
}

func drawLegendEntry(sc *scene.Scene, x, y float64, label string, value float64, showData bool, index int, th *theme.Theme) {
	// Draw colored square
	squareSize := 12.0
	st := scene.Style{
		Fill:        th.PaletteColor(index),
		Stroke:      th.TextColor,
		StrokeWidth: 0.5,
	}
	sc.Add(scene.RectPath(x, y-squareSize/2, squareSize, squareSize, 1, st))

	// Draw text
	font := diagram.Font(th, 0.9)
	text := label
	if showData {
		text = fmt.Sprintf("%s (%.2g)", label, value)
	}
	sc.Add(scene.NewText(x+20, y, text, font, th.TextColor, scene.AnchorStart, scene.VAlignMiddle))
}
