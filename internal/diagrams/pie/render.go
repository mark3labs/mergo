package pie

import (
	"fmt"
	"math"
	"strconv"

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
	title := doc.Title
	if title == "" {
		title = cfg.Title
	}

	total := 0.0
	for _, s := range doc.Slices {
		if s.Value > 0 {
			total += s.Value
		}
	}
	if len(doc.Slices) == 0 || total <= 0 {
		sc.Add(scene.NewText(0, 0, "(no data)", diagram.Font(th, 1), th.TextColor, scene.AnchorMiddle, scene.VAlignMiddle))
		return diagram.Finish(sc, th, title), nil
	}

	textPos := cfg.Float("pie", "textPosition", 0.75)
	const radius = 170.0
	cx, cy := radius, radius
	outline := th.Background
	if c, ok := cfg.Raw["themeVariables"].(map[string]any); ok {
		if s, ok := c["pieStrokeColor"].(string); ok {
			if col, err := theme.ParseColor(s); err == nil {
				outline = col
			}
		}
	}

	// slices
	angle := -math.Pi / 2
	labelFont := scene.Font{Size: th.FontSize * 0.95, Bold: true}
	var labels []scene.Item
	visible := 0
	for i, s := range doc.Slices {
		if s.Value <= 0 {
			continue
		}
		frac := s.Value / total
		end := angle + frac*2*math.Pi
		fill := th.PaletteColor(i)
		p := scene.NewPath(scene.Style{Fill: fill, Stroke: outline, StrokeWidth: 2})
		if frac >= 0.9999 {
			p.Circle(cx, cy, radius)
		} else {
			p.MoveTo(cx, cy)
			p.Arc(cx, cy, radius, radius, angle, end, true)
			p.Close()
		}
		sc.Add(p)
		if frac >= 0.03 {
			mid := (angle + end) / 2
			lx := cx + radius*textPos*math.Cos(mid)
			ly := cy + radius*textPos*math.Sin(mid)
			if frac >= 0.9999 {
				lx, ly = cx, cy
			}
			txt := formatPercent(frac * 100)
			labels = append(labels, scene.NewText(lx, ly, txt, labelFont, theme.ContrastText(fill), scene.AnchorMiddle, scene.VAlignMiddle))
		}
		angle = end
		visible++
	}
	// subtle outer ring
	sc.Add(scene.Circle(cx, cy, radius, scene.Style{Stroke: theme.WithAlpha(th.TextColor, 50), StrokeWidth: 1}))
	sc.Add(labels...)

	// legend
	legendFont := scene.Font{Size: th.FontSize}
	lh := 28.0
	box := 18.0
	lx := cx + radius + 50
	ly := cy - float64(len(doc.Slices))*lh/2 + lh/2
	for i, s := range doc.Slices {
		y := ly + float64(i)*lh
		fill := th.PaletteColor(i)
		sc.Add(scene.RectPath(lx, y-box/2, box, box, 3, scene.Style{Fill: fill, Stroke: theme.WithAlpha(th.TextColor, 90), StrokeWidth: 1}))
		txt := s.Label
		if doc.ShowData {
			txt = fmt.Sprintf("%s [%s]", s.Label, formatNumber(s.Value))
		}
		sc.Add(scene.NewText(lx+box+10, y, txt, legendFont, th.TextColor, scene.AnchorStart, scene.VAlignMiddle))
	}
	return diagram.Finish(sc, th, title), nil
}

func formatPercent(p float64) string {
	if p >= 10 || math.Abs(p-math.Round(p)) < 0.05 {
		return fmt.Sprintf("%.0f%%", p)
	}
	return fmt.Sprintf("%.1f%%", p)
}

func formatNumber(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
