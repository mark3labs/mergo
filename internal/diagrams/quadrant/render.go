package quadrant

import (
	"fmt"
	"image/color"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

func init() {
	diagram.Register(diagram.Type{
		Name:   "quadrantChart",
		Detect: diagram.Keyword("quadrantChart"),
		Render: Render,
	})
}

// Render renders a quadrant chart.
func Render(src string, cfg *diagram.Config) (*scene.Scene, error) {
	doc, err := Parse(src)
	if err != nil {
		return nil, err
	}

	th := cfg.Theme
	sc := diagram.NewScene(th)

	// Layout
	padding := 60.0
	width := 400.0
	height := 350.0

	// Quadrant dimensions
	centerX := padding + width/2
	centerY := padding + height/2
	quadrantW := width / 2
	quadrantH := height / 2

	// Draw quadrants
	drawQuadrant(sc, centerX-quadrantW, centerY-quadrantH, quadrantW, quadrantH,
		th.PaletteColor(0), doc.Quadrant2, th) // top-left
	drawQuadrant(sc, centerX, centerY-quadrantH, quadrantW, quadrantH,
		th.PaletteColor(1), doc.Quadrant1, th) // top-right
	drawQuadrant(sc, centerX-quadrantW, centerY, quadrantW, quadrantH,
		th.PaletteColor(2), doc.Quadrant3, th) // bottom-left
	drawQuadrant(sc, centerX, centerY, quadrantW, quadrantH,
		th.PaletteColor(3), doc.Quadrant4, th) // bottom-right

	// Draw axes
	drawAxes(sc, centerX, centerY, width, height, th, doc)

	// Draw points
	for i, pt := range doc.Points {
		// Scale point to [0,1] coordinates and then to pixel coordinates
		px := centerX - quadrantW + (pt.X * quadrantW)
		py := centerY - (pt.Y * quadrantH)

		color := th.PaletteColor(i)
		if pt.Style.Color != "" {
			if c, err := theme.ParseColor(pt.Style.Color); err == nil {
				color = c
			}
		}

		radius := pt.Style.Radius
		if radius == 0 {
			radius = 5
		}

		// Draw point circle
		st := scene.Style{
			Fill:        color,
			Stroke:      th.TextColor,
			StrokeWidth: 1.5,
		}
		if pt.Style.StrokeColor != "" {
			if c, err := theme.ParseColor(pt.Style.StrokeColor); err == nil {
				st.Stroke = c
			}
		}
		if pt.Style.StrokeWidth != "" {
			var w float64
			fmt.Sscanf(pt.Style.StrokeWidth, "%f", &w)
			if w > 0 {
				st.StrokeWidth = w
			}
		}

		sc.Add(scene.Circle(px, py, radius, st))

		// Draw label
		font := diagram.Font(th, 0.85)
		sc.Add(scene.NewText(px, py+radius+12, pt.Name, font, th.TextColor,
			scene.AnchorMiddle, scene.VAlignTop))
	}

	// Add title
	if doc.Title != "" {
		titleFont := diagram.BoldFont(th, 1.125)
		sc.Add(scene.NewText(centerX, padding-30, doc.Title, titleFont, th.TitleColor,
			scene.AnchorMiddle, scene.VAlignBottom))
	}

	sc.Fit(diagram.Pad)
	return sc, nil
}

func drawQuadrant(sc *scene.Scene, x, y, w, h float64, fill color.RGBA, label string, th *theme.Theme) {
	// Draw quadrant background
	st := scene.Style{
		Fill:        theme.WithAlpha(fill, 40),
		Stroke:      theme.WithAlpha(fill, 100),
		StrokeWidth: 1,
	}
	sc.Add(scene.RectPath(x, y, w, h, 0, st))

	// Draw label
	if label != "" {
		font := diagram.Font(th, 0.9)
		// Place label in upper left of quadrant, semi-transparent
		sc.Add(scene.NewText(x+8, y+20, label, font, theme.WithAlpha(fill, 180),
			scene.AnchorStart, scene.VAlignTop))
	}
}

func drawAxes(sc *scene.Scene, cx, cy, w, h float64, th *theme.Theme, doc *Document) {
	// X-axis
	axisStyle := scene.Style{
		Stroke:      th.LineColor,
		StrokeWidth: 2,
	}
	sc.Add(scene.Line(cx-w/2, cy, cx+w/2, cy, axisStyle))
	// Y-axis
	sc.Add(scene.Line(cx, cy-h/2, cx, cy+h/2, axisStyle))

	// Axis labels
	labelFont := diagram.Font(th, 0.85)

	// X-axis labels
	xMinLabel := "Low"
	xMaxLabel := "High"
	if doc.XAxis.Min != "" {
		xMinLabel = doc.XAxis.Min
	}
	if doc.XAxis.Max != "" {
		xMaxLabel = doc.XAxis.Max
	}
	sc.Add(scene.NewText(cx-w/2-20, cy+20, xMinLabel, labelFont, th.TextColor,
		scene.AnchorEnd, scene.VAlignTop))
	sc.Add(scene.NewText(cx+w/2+20, cy+20, xMaxLabel, labelFont, th.TextColor,
		scene.AnchorStart, scene.VAlignTop))

	// Y-axis labels
	yMinLabel := "Low"
	yMaxLabel := "High"
	if doc.YAxis.Min != "" {
		yMinLabel = doc.YAxis.Min
	}
	if doc.YAxis.Max != "" {
		yMaxLabel = doc.YAxis.Max
	}
	sc.Add(scene.NewText(cx-30, cy+h/2+10, yMinLabel, labelFont, th.TextColor,
		scene.AnchorEnd, scene.VAlignTop))
	sc.Add(scene.NewText(cx-30, cy-h/2-10, yMaxLabel, labelFont, th.TextColor,
		scene.AnchorEnd, scene.VAlignBottom))
}
