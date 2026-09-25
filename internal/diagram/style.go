package diagram

import (
	"image/color"
	"math"
	"strconv"
	"strings"

	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

// ParseCSS parses Mermaid style strings like
// "fill:#f9f,stroke:#333,stroke-width:4px" (commas or semicolons as
// separators; commas inside parentheses are kept).
func ParseCSS(s string) map[string]string {
	out := map[string]string{}
	depth := 0
	var parts []string
	var cur strings.Builder
	for _, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',', ';':
			if depth == 0 {
				parts = append(parts, cur.String())
				cur.Reset()
				continue
			}
		}
		cur.WriteRune(r)
	}
	parts = append(parts, cur.String())
	for _, p := range parts {
		k, v, ok := strings.Cut(p, ":")
		if !ok {
			continue
		}
		k = strings.ToLower(strings.TrimSpace(k))
		v = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(v), "!important"))
		if k != "" {
			out[k] = v
		}
	}
	return out
}

// NodeStyle is a resolved box style.
type NodeStyle struct {
	Fill, Stroke, Text color.RGBA
	StrokeWidth        float64
	Dash               []float64
	Bold, Italic       bool
	FontSize           float64
	FillSet, TextSet   bool
}

// Apply applies CSS properties to the style.
func (ns *NodeStyle) Apply(css map[string]string) {
	for k, v := range css {
		switch k {
		case "fill", "background", "background-color":
			if c, err := theme.ParseColor(v); err == nil {
				ns.Fill = c
				ns.FillSet = true
			}
		case "stroke", "border-color":
			if c, err := theme.ParseColor(v); err == nil {
				ns.Stroke = c
			}
		case "color":
			if c, err := theme.ParseColor(v); err == nil {
				ns.Text = c
				ns.TextSet = true
			}
		case "stroke-width":
			if f, ok := ParsePx(v); ok {
				ns.StrokeWidth = f
			}
		case "stroke-dasharray":
			ns.Dash = ParseDash(v)
		case "font-weight":
			ns.Bold = v == "bold" || v == "bolder" || v == "600" || v == "700" || v == "800" || v == "900"
		case "font-style":
			ns.Italic = v == "italic" || v == "oblique"
		case "font-size":
			if f, ok := ParsePx(v); ok && f > 4 && f < 100 {
				ns.FontSize = f
			}
		}
	}
}

// FixContrast picks a readable text color when a custom fill was set
// without a text color.
func (ns *NodeStyle) FixContrast() {
	if ns.FillSet && !ns.TextSet && math.Abs(theme.Luminance(ns.Fill)-theme.Luminance(ns.Text)) < 0.3 {
		ns.Text = theme.ContrastText(ns.Fill)
	}
}

// Box returns a scene style for a filled box.
func (ns NodeStyle) Box(shadow bool) scene.Style {
	return scene.Style{Fill: ns.Fill, Stroke: ns.Stroke, StrokeWidth: ns.StrokeWidth, Dash: ns.Dash, Shadow: shadow}
}

// ParsePx parses "4px" / "4".
func ParsePx(v string) (float64, bool) {
	v = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(v), "px"))
	f, err := strconv.ParseFloat(v, 64)
	return f, err == nil
}

// ParseDash parses a stroke-dasharray value.
func ParseDash(v string) []float64 {
	var out []float64
	for _, f := range strings.FieldsFunc(v, func(r rune) bool { return r == ' ' || r == ',' }) {
		if x, ok := ParsePx(f); ok && x > 0 {
			out = append(out, x)
		}
	}
	if len(out) == 1 {
		out = append(out, out[0])
	}
	return out
}

// EdgeLabel returns the items for an edge label centered at (x, y).
func EdgeLabel(x, y float64, text string, f scene.Font, th *theme.Theme) []scene.Item {
	if text == "" {
		return nil
	}
	w, h := scene.MeasureBlock(text, f, 0)
	w += 12
	h += 6
	bg := th.EdgeLabelBg
	if bg.A == 255 {
		bg.A = 235
	}
	return []scene.Item{
		scene.RectPath(x-w/2, y-h/2, w, h, 3, scene.Style{Fill: bg}),
		scene.NewText(x, y, text, f, th.TextColor, scene.AnchorMiddle, scene.VAlignMiddle),
	}
}

// LabelSize returns the size an edge label occupies (for layout).
func LabelSize(text string, f scene.Font) (float64, float64) {
	if text == "" {
		return 0, 0
	}
	w, h := scene.MeasureBlock(text, f, 0)
	return w + 12, h + 6
}

// EndLabelPos returns a position for a small label (e.g. cardinality)
// near the end of a polyline: offset along the last segment and to the
// side.
func EndLabelPos(pts []scene.Point, atStart bool, along, side float64) scene.Point {
	if len(pts) < 2 {
		if len(pts) == 1 {
			return pts[0]
		}
		return scene.Point{}
	}
	a, b := pts[len(pts)-1], pts[len(pts)-2]
	if atStart {
		a, b = pts[0], pts[1]
	}
	d := b.Sub(a).Norm()
	n := scene.Point{X: -d.Y, Y: d.X}
	return a.Add(d.Mul(along)).Add(n.Mul(side))
}
