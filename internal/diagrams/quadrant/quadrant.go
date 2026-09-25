// Package quadrant implements Mermaid quadrant charts.
package quadrant

import (
	"fmt"
	"image/color"
	"regexp"
	"strconv"
	"strings"

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

// Point is a data point.
type Point struct {
	Label       string
	X, Y        float64
	Class       string
	Radius      float64
	Color       *color.RGBA
	StrokeColor *color.RGBA
	StrokeWidth float64
}

// Chart is a parsed quadrant chart.
type Chart struct {
	Title       string
	XLow, XHigh string
	YLow, YHigh string
	Quadrants   [4]string
	Points      []*Point
	ClassDefs   map[string]map[string]string
}

var pointRe = regexp.MustCompile(`^(.+?)(?::::(\S+))?\s*:\s*\[\s*([-\d.]+)\s*,\s*([-\d.]+)\s*\]\s*(.*)$`)

func parseAxis(s string) (string, string) {
	lo, hi, found := strings.Cut(s, "-->")
	if !found {
		return diagram.CleanLabel(s), ""
	}
	return diagram.CleanLabel(lo), diagram.CleanLabel(hi)
}

// Parse parses quadrant chart source.
func Parse(src string) (*Chart, error) {
	c := &Chart{ClassDefs: map[string]map[string]string{}}
	header := false
	for i, raw := range strings.Split(src, "\n") {
		ln := i + 1
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		if !header {
			if !strings.HasPrefix(line, "quadrantChart") {
				return nil, fmt.Errorf("line %d: expected 'quadrantChart'", ln)
			}
			header = true
			continue
		}
		kw, rest := cutWord(line)
		switch kw {
		case "title":
			c.Title = diagram.CleanLabel(rest)
			continue
		case "x-axis":
			c.XLow, c.XHigh = parseAxis(rest)
			continue
		case "y-axis":
			c.YLow, c.YHigh = parseAxis(rest)
			continue
		case "quadrant-1", "quadrant-2", "quadrant-3", "quadrant-4":
			c.Quadrants[kw[9]-'1'] = diagram.CleanLabel(rest)
			continue
		case "classDef":
			name, props := cutWord(rest)
			c.ClassDefs[name] = parseProps(props)
			continue
		case "accTitle", "accDescr":
			continue
		}
		m := pointRe.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("line %d: unrecognized statement %q", ln, line)
		}
		x, errx := strconv.ParseFloat(m[3], 64)
		y, erry := strconv.ParseFloat(m[4], 64)
		if errx != nil || erry != nil {
			return nil, fmt.Errorf("line %d: invalid point coordinates", ln)
		}
		if x < 0 || x > 1 || y < 0 || y > 1 {
			return nil, fmt.Errorf("line %d: point coordinates must be between 0 and 1", ln)
		}
		p := &Point{Label: diagram.CleanLabel(m[1]), X: x, Y: y, Class: m[2]}
		applyProps(p, parseProps(m[5]))
		c.Points = append(c.Points, p)
	}
	for _, p := range c.Points {
		if p.Class != "" {
			if def, ok := c.ClassDefs[p.Class]; ok {
				// explicit point styles win over the class
				cp := *p
				applyProps(p, def)
				if cp.Radius != 0 {
					p.Radius = cp.Radius
				}
				if cp.Color != nil {
					p.Color = cp.Color
				}
				if cp.StrokeColor != nil {
					p.StrokeColor = cp.StrokeColor
				}
				if cp.StrokeWidth != 0 {
					p.StrokeWidth = cp.StrokeWidth
				}
			}
		}
	}
	return c, nil
}

func parseProps(s string) map[string]string {
	out := map[string]string{}
	for part := range strings.SplitSeq(s, ",") {
		k, v, ok := strings.Cut(part, ":")
		if ok {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return out
}

func applyProps(p *Point, props map[string]string) {
	for k, v := range props {
		switch k {
		case "radius":
			if f, ok := diagram.ParsePx(v); ok {
				p.Radius = f
			}
		case "color":
			if c, err := theme.ParseColor(v); err == nil {
				p.Color = &c
			}
		case "stroke-color":
			if c, err := theme.ParseColor(v); err == nil {
				p.StrokeColor = &c
			}
		case "stroke-width":
			if f, ok := diagram.ParsePx(v); ok {
				p.StrokeWidth = f
			}
		}
	}
}

func cutWord(s string) (string, string) {
	s = strings.TrimSpace(s)
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimSpace(s[i+1:])
}

// Render renders a quadrant chart.
func Render(src string, cfg *diagram.Config) (*scene.Scene, error) {
	c, err := Parse(src)
	if err != nil {
		return nil, err
	}
	th := cfg.Theme
	sc := diagram.NewScene(th)
	size := cfg.Float("quadrantChart", "chartWidth", 500)
	half := size / 2
	x0, y0 := 40.0, 0.0
	labelFont := scene.Font{Size: th.FontSize * 1.0, Bold: true}
	axisFont := scene.Font{Size: th.FontSize * 0.9}
	pointFont := scene.Font{Size: th.FontSize * 0.8}

	// quadrant fills: q1 top-right, q2 top-left, q3 bottom-left, q4 bottom-right
	base := th.PrimaryColor
	fills := [4]color.RGBA{
		base,
		theme.Mix(base, th.Background, 0.35),
		theme.Mix(base, th.Background, 0.6),
		theme.Mix(base, th.Background, 0.35),
	}
	if th.Dark {
		fills = [4]color.RGBA{
			theme.Lighten(base, 0.1), theme.Mix(base, th.Background, 0.3),
			theme.Mix(base, th.Background, 0.55), theme.Mix(base, th.Background, 0.3),
		}
	}
	if tv, ok := cfg.Raw["themeVariables"].(map[string]any); ok {
		for i := range 4 {
			if s, ok := tv[fmt.Sprintf("quadrant%dFill", i+1)].(string); ok {
				if col, err := theme.ParseColor(s); err == nil {
					fills[i] = col
				}
			}
		}
	}
	rects := [4]scene.Rect{
		{X: x0 + half, Y: y0, W: half, H: half},
		{X: x0, Y: y0, W: half, H: half},
		{X: x0, Y: y0 + half, W: half, H: half},
		{X: x0 + half, Y: y0 + half, W: half, H: half},
	}
	for i, r := range rects {
		sc.Add(scene.RectPath(r.X, r.Y, r.W, r.H, 0, scene.Style{Fill: fills[i]}))
	}
	border := th.PrimaryBorderColor
	sc.Add(scene.RectPath(x0, y0, size, size, 0, scene.Style{Stroke: border, StrokeWidth: 2}))
	sc.Add(scene.Line(x0+half, y0, x0+half, y0+size, scene.Style{Stroke: border, StrokeWidth: 1.2}))
	sc.Add(scene.Line(x0, y0+half, x0+size, y0+half, scene.Style{Stroke: border, StrokeWidth: 1.2}))

	// quadrant labels (top-centered when points exist, else centered)
	for i, txt := range c.Quadrants {
		if txt == "" {
			continue
		}
		r := rects[i]
		tc := theme.ContrastText(fills[i])
		wrapped := scene.WrapText(txt, labelFont, r.W-20)
		if len(c.Points) > 0 {
			sc.Add(scene.NewText(r.X+r.W/2, r.Y+12, wrapped, labelFont, theme.WithAlpha(tc, 200), scene.AnchorMiddle, scene.VAlignTop))
		} else {
			sc.Add(scene.NewText(r.X+r.W/2, r.Y+r.H/2, wrapped, labelFont, theme.WithAlpha(tc, 200), scene.AnchorMiddle, scene.VAlignMiddle))
		}
	}
	// axis labels
	if c.XHigh == "" {
		sc.Add(scene.NewText(x0+half, y0+size+10, c.XLow, axisFont, th.TextColor, scene.AnchorMiddle, scene.VAlignTop))
	} else {
		sc.Add(scene.NewText(x0+half/2, y0+size+10, c.XLow, axisFont, th.TextColor, scene.AnchorMiddle, scene.VAlignTop))
		sc.Add(scene.NewText(x0+half*1.5, y0+size+10, c.XHigh, axisFont, th.TextColor, scene.AnchorMiddle, scene.VAlignTop))
	}
	yl := func(txt string, cy float64) {
		if txt != "" {
			sc.Add(&scene.Text{X: x0 - 12, Y: cy, S: txt, Font: axisFont, Color: th.TextColor, Anchor: scene.AnchorMiddle, VAlign: scene.VAlignBottom, Rotate: -90})
		}
	}
	if c.YHigh == "" {
		yl(c.YLow, y0+half)
	} else {
		yl(c.YLow, y0+half*1.5)
		yl(c.YHigh, y0+half/2)
	}

	// points
	for _, p := range c.Points {
		px := x0 + p.X*size
		py := y0 + (1-p.Y)*size
		rad := p.Radius
		if rad == 0 {
			rad = 5
		}
		fill := th.ChartColor(0)
		if p.Color != nil {
			fill = *p.Color
		}
		st := scene.Style{Fill: fill, Stroke: theme.Darken(fill, 0.3), StrokeWidth: 1, Shadow: true}
		if p.StrokeColor != nil {
			st.Stroke = *p.StrokeColor
		}
		if p.StrokeWidth > 0 {
			st.StrokeWidth = p.StrokeWidth
		}
		sc.Add(scene.Circle(px, py, rad, st))
		ly := py + rad + 4
		va := scene.VAlignTop
		if py+rad+22 > y0+size {
			ly, va = py-rad-4, scene.VAlignBottom
		}
		sc.Add(scene.NewText(px, ly, p.Label, pointFont, th.TextColor, scene.AnchorMiddle, va))
	}
	title := c.Title
	if title == "" {
		title = cfg.Title
	}
	return diagram.Finish(sc, th, title), nil
}
