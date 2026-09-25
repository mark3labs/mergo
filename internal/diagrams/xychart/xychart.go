// Package xychart implements Mermaid XY charts (bar and line).
package xychart

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

func init() {
	diagram.Register(diagram.Type{
		Name:   "xychart",
		Detect: diagram.Keyword("xychart-beta", "xychart"),
		Render: Render,
	})
}

// Axis is a chart axis (categorical or numeric).
type Axis struct {
	Title      string
	Categories []string
	Min, Max   float64
	HasRange   bool
}

// Series is a data series.
type Series struct {
	Kind   string // "bar" or "line"
	Title  string
	Values []float64
}

// Chart is a parsed XY chart.
type Chart struct {
	Title      string
	Horizontal bool
	X, Y       Axis
	Series     []*Series
}

var (
	rangeRe = regexp.MustCompile(`^(-?[\d.]+)\s*-->\s*(-?[\d.]+)$`)
)

// splitTitle splits an optional leading (quoted) title from the rest.
func splitTitle(s string) (string, string) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, `"`) {
		if j := strings.Index(s[1:], `"`); j >= 0 {
			return s[1 : j+1], strings.TrimSpace(s[j+2:])
		}
	}
	if strings.HasPrefix(s, "[") || rangeRe.MatchString(s) {
		return "", s
	}
	// unquoted single-word title followed by data
	if i := strings.IndexAny(s, " \t"); i > 0 {
		rest := strings.TrimSpace(s[i:])
		if strings.HasPrefix(rest, "[") || rangeRe.MatchString(rest) {
			return s[:i], rest
		}
	}
	return s, ""
}

func parseList(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		return nil, fmt.Errorf("expected [ ... ]")
	}
	body := s[1 : len(s)-1]
	var out []string
	var cur strings.Builder
	inQ := false
	for _, r := range body {
		switch {
		case r == '"':
			inQ = !inQ
		case r == ',' && !inQ:
			out = append(out, strings.TrimSpace(cur.String()))
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	if strings.TrimSpace(cur.String()) != "" || len(out) > 0 {
		out = append(out, strings.TrimSpace(cur.String()))
	}
	return out, nil
}

func parseAxis(s string, ln int) (Axis, error) {
	var a Axis
	title, rest := splitTitle(s)
	a.Title = diagram.CleanLabel(title)
	if rest == "" {
		return a, nil
	}
	if m := rangeRe.FindStringSubmatch(rest); m != nil {
		a.Min, _ = strconv.ParseFloat(m[1], 64)
		a.Max, _ = strconv.ParseFloat(m[2], 64)
		a.HasRange = true
		return a, nil
	}
	cats, err := parseList(rest)
	if err != nil {
		return a, fmt.Errorf("line %d: invalid axis definition %q", ln, s)
	}
	for _, c := range cats {
		a.Categories = append(a.Categories, diagram.CleanLabel(c))
	}
	return a, nil
}

// Parse parses xychart source.
func Parse(src string) (*Chart, error) {
	c := &Chart{}
	header := false
	for i, raw := range strings.Split(src, "\n") {
		ln := i + 1
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		if !header {
			if !strings.HasPrefix(line, "xychart") {
				return nil, fmt.Errorf("line %d: expected 'xychart-beta'", ln)
			}
			c.Horizontal = strings.Contains(line, "horizontal")
			header = true
			continue
		}
		kw, rest := cutWord(line)
		var err error
		switch kw {
		case "title":
			t, _ := splitTitle(rest)
			c.Title = diagram.CleanLabel(t)
		case "x-axis":
			c.X, err = parseAxis(rest, ln)
		case "y-axis":
			c.Y, err = parseAxis(rest, ln)
		case "bar", "line":
			title, data := splitTitle(rest)
			vals, perr := parseList(data)
			if perr != nil {
				return nil, fmt.Errorf("line %d: %s data must be a list like [1, 2, 3]", ln, kw)
			}
			s := &Series{Kind: kw, Title: diagram.CleanLabel(title)}
			for _, v := range vals {
				f, ferr := strconv.ParseFloat(strings.TrimSpace(v), 64)
				if ferr != nil {
					return nil, fmt.Errorf("line %d: invalid number %q", ln, v)
				}
				s.Values = append(s.Values, f)
			}
			c.Series = append(c.Series, s)
		default:
			return nil, fmt.Errorf("line %d: unknown statement %q", ln, kw)
		}
		if err != nil {
			return nil, err
		}
	}
	return c, nil
}

func cutWord(s string) (string, string) {
	s = strings.TrimSpace(s)
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimSpace(s[i+1:])
}

// niceTicks returns nicely rounded ticks covering [lo, hi].
func niceTicks(lo, hi float64, n int) (float64, float64, []float64) {
	if hi <= lo {
		hi = lo + 1
	}
	span := hi - lo
	step := math.Pow(10, math.Floor(math.Log10(span/float64(n))))
	for _, m := range []float64{1, 2, 2.5, 5, 10} {
		if span/(step*m) <= float64(n) {
			step *= m
			break
		}
	}
	nlo := math.Floor(lo/step+1e-9) * step
	nhi := math.Ceil(hi/step-1e-9) * step
	var ticks []float64
	for v := nlo; v <= nhi+step/2; v += step {
		ticks = append(ticks, math.Round(v/step)*step)
	}
	return nlo, nhi, ticks
}

func fmtNum(v float64) string {
	if math.Abs(v) >= 1e6 {
		return strconv.FormatFloat(v/1e6, 'f', -1, 64) + "M"
	}
	if math.Abs(v) >= 1e4 && math.Mod(v, 1000) == 0 {
		return strconv.FormatFloat(v/1e3, 'f', -1, 64) + "k"
	}
	return strconv.FormatFloat(math.Round(v*1000)/1000, 'f', -1, 64)
}

// Render renders an XY chart.
func Render(src string, cfg *diagram.Config) (*scene.Scene, error) {
	c, err := Parse(src)
	if err != nil {
		return nil, err
	}
	th := cfg.Theme
	sc := diagram.NewScene(th)
	W := cfg.Float("xyChart", "width", 700)
	H := cfg.Float("xyChart", "height", 500)
	showData := cfg.Bool("xyChart", "showDataLabel", false)
	tickFont := scene.Font{Size: th.FontSize * 0.8}
	titleFont := scene.Font{Size: th.FontSize * 0.9, Bold: true}

	// categories
	n := 0
	for _, s := range c.Series {
		n = max(n, len(s.Values))
	}
	cats := c.X.Categories
	numericX := len(cats) == 0
	if numericX {
		// numeric x axis: evenly spaced points between min and max
		for i := 0; i < n; i++ {
			v := float64(i + 1)
			if c.X.HasRange && n > 1 {
				v = c.X.Min + (c.X.Max-c.X.Min)*float64(i)/float64(n-1)
			}
			cats = append(cats, fmtNum(v))
		}
	}
	if n < len(cats) {
		n = len(cats)
	}
	if n == 0 {
		sc.Add(scene.NewText(0, 0, "(no data)", tickFont, th.TextColor, scene.AnchorMiddle, scene.VAlignMiddle))
		return diagram.Finish(sc, th, c.Title), nil
	}

	// value range
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, s := range c.Series {
		for _, v := range s.Values {
			lo, hi = math.Min(lo, v), math.Max(hi, v)
		}
	}
	if c.Y.HasRange {
		lo, hi = c.Y.Min, c.Y.Max
	} else {
		if math.IsInf(lo, 1) {
			lo, hi = 0, 1
		}
		if lo > 0 {
			lo = 0
		}
	}
	vlo, vhi, ticks := niceTicks(lo, hi, 6)
	if c.Y.HasRange {
		vlo, vhi = lo, hi
		_, _, ticks = niceTicks(lo, hi, 6)
		var in []float64
		for _, t := range ticks {
			if t >= lo-1e-9 && t <= hi+1e-9 {
				in = append(in, t)
			}
		}
		ticks = in
	}

	// geometry (value axis vertical unless horizontal)
	valLabelW := 0.0
	for _, t := range ticks {
		valLabelW = math.Max(valLabelW, scene.MeasureText(fmtNum(t), tickFont))
	}
	catW := 0.0
	for _, cname := range cats {
		catW = math.Max(catW, scene.MeasureText(cname, tickFont))
	}
	plotW, plotH := W, H
	var catRotate bool
	var left, bottom float64
	if !c.Horizontal {
		left = valLabelW + 14
		if c.Y.Title != "" {
			left += th.FontSize + 10
		}
		plotW = W - left - 10
		band := plotW / float64(n)
		catRotate = catW+8 > band
		bottom = th.FontSize + 12
		if catRotate {
			bottom = catW*0.72 + 20
		}
		if c.X.Title != "" {
			bottom += th.FontSize + 10
		}
		plotH = H - bottom
	} else {
		left = catW + 14
		if c.X.Title != "" {
			left += th.FontSize + 10
		}
		plotW = W - left - 10
		bottom = th.FontSize + 12
		if c.Y.Title != "" {
			bottom += th.FontSize + 10
		}
		plotH = H - bottom
	}
	px0, py0 := left, 0.0 // plot top-left
	px1, py1 := px0+plotW, py0+plotH
	grid := scene.Style{Stroke: theme.WithAlpha(th.GridColor, 200), StrokeWidth: 1}
	axis := scene.Style{Stroke: theme.Mix(th.TextColor, th.Background, 0.35), StrokeWidth: 1.2}

	// value mapping
	valPos := func(v float64) float64 {
		t := (v - vlo) / (vhi - vlo)
		if c.Horizontal {
			return px0 + t*plotW
		}
		return py1 - t*plotH
	}
	var band float64
	catCenter := func(i int) float64 {
		if c.Horizontal {
			return py0 + (float64(i)+0.5)*band
		}
		return px0 + (float64(i)+0.5)*band
	}
	if c.Horizontal {
		band = plotH / float64(n)
	} else {
		band = plotW / float64(n)
	}

	// grid & value ticks
	for _, t := range ticks {
		p := valPos(t)
		if c.Horizontal {
			sc.Add(scene.Line(p, py0, p, py1, grid))
			sc.Add(scene.NewText(p, py1+8, fmtNum(t), tickFont, th.TextColor, scene.AnchorMiddle, scene.VAlignTop))
		} else {
			sc.Add(scene.Line(px0, p, px1, p, grid))
			sc.Add(scene.NewText(px0-8, p, fmtNum(t), tickFont, th.TextColor, scene.AnchorEnd, scene.VAlignMiddle))
		}
	}
	// category labels
	for i, cname := range cats {
		p := catCenter(i)
		if c.Horizontal {
			sc.Add(scene.NewText(px0-8, p, cname, tickFont, th.TextColor, scene.AnchorEnd, scene.VAlignMiddle))
		} else if catRotate {
			sc.Add(&scene.Text{X: p + 4, Y: py1 + 10, S: cname, Font: tickFont, Color: th.TextColor, Anchor: scene.AnchorEnd, VAlign: scene.VAlignMiddle, Rotate: -45})
		} else {
			sc.Add(scene.NewText(p, py1+8, cname, tickFont, th.TextColor, scene.AnchorMiddle, scene.VAlignTop))
		}
	}
	// axis titles
	if c.X.Title != "" {
		if c.Horizontal {
			sc.Add(&scene.Text{X: 4, Y: (py0 + py1) / 2, S: c.X.Title, Font: titleFont, Color: th.TextColor, Anchor: scene.AnchorMiddle, VAlign: scene.VAlignTop, Rotate: -90})
		} else {
			sc.Add(scene.NewText((px0+px1)/2, H, c.X.Title, titleFont, th.TextColor, scene.AnchorMiddle, scene.VAlignBottom))
		}
	}
	if c.Y.Title != "" {
		if c.Horizontal {
			sc.Add(scene.NewText((px0+px1)/2, H, c.Y.Title, titleFont, th.TextColor, scene.AnchorMiddle, scene.VAlignBottom))
		} else {
			sc.Add(&scene.Text{X: 4, Y: (py0 + py1) / 2, S: c.Y.Title, Font: titleFont, Color: th.TextColor, Anchor: scene.AnchorMiddle, VAlign: scene.VAlignTop, Rotate: -90})
		}
	}

	// bars (grouped)
	var bars []*Series
	for _, s := range c.Series {
		if s.Kind == "bar" {
			bars = append(bars, s)
		}
	}
	zero := valPos(math.Max(vlo, math.Min(0, vhi)))
	dataFont := scene.Font{Size: th.FontSize * 0.72, Bold: true}
	groupW := band * 0.72
	for bi, s := range bars {
		col := th.ChartColor(seriesIndex(c, s))
		bw := groupW / float64(len(bars))
		for i, v := range s.Values {
			if i >= n {
				break
			}
			start := catCenter(i) - groupW/2 + float64(bi)*bw
			p := valPos(v)
			var rx, ry, rw, rh float64
			if c.Horizontal {
				rx, rw = math.Min(zero, p), math.Abs(p-zero)
				ry, rh = start+1, bw-2
			} else {
				ry, rh = math.Min(zero, p), math.Abs(p-zero)
				rx, rw = start+1, bw-2
			}
			sc.Add(scene.RectPath(rx, ry, rw, rh, math.Min(3, math.Min(rw, rh)/2), scene.Style{Fill: col}))
			if showData {
				if c.Horizontal {
					sc.Add(scene.NewText(rx+rw+4, ry+rh/2, fmtNum(v), dataFont, th.TextColor, scene.AnchorStart, scene.VAlignMiddle))
				} else {
					sc.Add(scene.NewText(rx+rw/2, ry-4, fmtNum(v), dataFont, th.TextColor, scene.AnchorMiddle, scene.VAlignBottom))
				}
			}
		}
	}
	// lines
	for _, s := range c.Series {
		if s.Kind != "line" {
			continue
		}
		col := th.ChartColor(seriesIndex(c, s))
		var pts []scene.Point
		for i, v := range s.Values {
			if i >= n {
				break
			}
			if c.Horizontal {
				pts = append(pts, scene.Pt(valPos(v), catCenter(i)))
			} else {
				pts = append(pts, scene.Pt(catCenter(i), valPos(v)))
			}
		}
		sc.Add(scene.NewPath(scene.Style{Stroke: col, StrokeWidth: 3, RoundCaps: true}).Polyline(pts...))
		for i, p := range pts {
			sc.Add(scene.Circle(p.X, p.Y, 4.5, scene.Style{Fill: th.Background, Stroke: col, StrokeWidth: 2.5}))
			if showData {
				sc.Add(scene.NewText(p.X, p.Y-9, fmtNum(s.Values[i]), dataFont, th.TextColor, scene.AnchorMiddle, scene.VAlignBottom))
			}
		}
	}
	// axes
	if c.Horizontal {
		sc.Add(scene.Line(px0, py0, px0, py1, axis), scene.Line(px0, py1, px1, py1, axis))
	} else {
		sc.Add(scene.Line(px0, py1, px1, py1, axis), scene.Line(px0, py0, px0, py1, axis))
	}

	// legend for named series
	var named []*Series
	for _, s := range c.Series {
		if s.Title != "" {
			named = append(named, s)
		}
	}
	if len(named) > 0 {
		lf := scene.Font{Size: th.FontSize * 0.85}
		x := px0
		y := -24.0
		for _, s := range named {
			col := th.ChartColor(seriesIndex(c, s))
			if s.Kind == "line" {
				sc.Add(scene.Line(x, y, x+18, y, scene.Style{Stroke: col, StrokeWidth: 3, RoundCaps: true}))
			} else {
				sc.Add(scene.RectPath(x, y-7, 18, 14, 2, scene.Style{Fill: col}))
			}
			sc.Add(scene.NewText(x+24, y, s.Title, lf, th.TextColor, scene.AnchorStart, scene.VAlignMiddle))
			x += 24 + scene.MeasureText(s.Title, lf) + 22
		}
	}
	title := c.Title
	if title == "" {
		title = cfg.Title
	}
	return diagram.Finish(sc, th, title), nil
}

func seriesIndex(c *Chart, s *Series) int {
	for i, x := range c.Series {
		if x == s {
			return i
		}
	}
	return 0
}
