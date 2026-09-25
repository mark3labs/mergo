// Package timeline implements Mermaid timeline diagrams.
package timeline

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

func init() {
	diagram.Register(diagram.Type{
		Name:   "timeline",
		Detect: diagram.Keyword("timeline"),
		Render: Render,
	})
}

// Period is a time period with events.
type Period struct {
	Label   string
	Events  []string
	Section int // -1 if none
}

// Timeline is a parsed timeline.
type Timeline struct {
	Title    string
	Vertical bool
	Sections []string
	Periods  []*Period
}

// Parse parses timeline source.
func Parse(src string) (*Timeline, error) {
	t := &Timeline{}
	header := false
	section := -1
	var last *Period
	for i, raw := range strings.Split(src, "\n") {
		ln := i + 1
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		if !header {
			if !strings.HasPrefix(line, "timeline") {
				return nil, fmt.Errorf("line %d: expected 'timeline'", ln)
			}
			rest := strings.ToUpper(strings.TrimSpace(strings.TrimPrefix(line, "timeline")))
			t.Vertical = rest == "TD" || rest == "TB"
			header = true
			continue
		}
		if strings.HasPrefix(line, "title ") {
			t.Title = diagram.CleanLabel(line[6:])
			continue
		}
		if strings.HasPrefix(line, "section ") {
			t.Sections = append(t.Sections, diagram.CleanLabel(line[8:]))
			section = len(t.Sections) - 1
			continue
		}
		if strings.HasPrefix(line, "accTitle") || strings.HasPrefix(line, "accDescr") {
			continue
		}
		parts := strings.Split(line, ":")
		if strings.HasPrefix(line, ":") {
			if last == nil {
				return nil, fmt.Errorf("line %d: event without a time period", ln)
			}
			for _, e := range parts[1:] {
				if e = strings.TrimSpace(e); e != "" {
					last.Events = append(last.Events, diagram.CleanLabel(e))
				}
			}
			continue
		}
		p := &Period{Label: diagram.CleanLabel(parts[0]), Section: section}
		for _, e := range parts[1:] {
			if e = strings.TrimSpace(e); e != "" {
				p.Events = append(p.Events, diagram.CleanLabel(e))
			}
		}
		t.Periods = append(t.Periods, p)
		last = p
	}
	return t, nil
}

const (
	colW   = 170.0
	colGap = 24.0
	boxPad = 10.0
)

// Render renders a timeline.
func Render(src string, cfg *diagram.Config) (*scene.Scene, error) {
	t, err := Parse(src)
	if err != nil {
		return nil, err
	}
	th := cfg.Theme
	sc := diagram.NewScene(th)
	title := t.Title
	if title == "" {
		title = cfg.Title
	}
	if len(t.Periods) == 0 {
		sc.Add(scene.NewText(0, 0, "(empty timeline)", diagram.Font(th, 1), th.TextColor, scene.AnchorMiddle, scene.VAlignMiddle))
		return diagram.Finish(sc, th, title), nil
	}
	multi := !cfg.Bool("timeline", "disableMulticolor", false)
	periodFont := scene.Font{Size: th.FontSize, Bold: true}
	eventFont := scene.Font{Size: th.FontSize * 0.88}
	secFont := scene.Font{Size: th.FontSize * 1.02, Bold: true}

	colorOf := func(i int, p *Period) (color.RGBA, color.RGBA) {
		idx := i
		if len(t.Sections) > 0 && p.Section >= 0 {
			idx = p.Section
		}
		if !multi {
			idx = 0
		}
		c := th.PaletteColor(idx)
		return c, th.PaletteTextColor(idx)
	}

	type box struct {
		txt  string
		w, h float64
	}
	measure := func(s string, f scene.Font, w float64) box {
		txt := scene.WrapText(s, f, w-2*boxPad)
		_, h := scene.MeasureBlock(txt, f, 0)
		return box{txt: txt, w: w, h: h + 2*boxPad}
	}

	if t.Vertical {
		return renderVertical(t, th, sc, title, colorOf, periodFont, eventFont, secFont), nil
	}

	// horizontal layout
	secH := 0.0
	if len(t.Sections) > 0 {
		for _, s := range t.Sections {
			secH = math.Max(secH, measure(s, secFont, colW).h)
		}
	}
	periodH := 0.0
	for _, p := range t.Periods {
		periodH = math.Max(periodH, measure(p.Label, periodFont, colW).h)
	}
	yPeriod := 0.0
	if secH > 0 {
		yPeriod = secH + 16
	}
	yLine := yPeriod + periodH + 34
	x := 0.0
	xs := make([]float64, len(t.Periods))
	for i := range t.Periods {
		xs[i] = x
		x += colW + colGap
	}
	totalW := x - colGap

	// sections: header boxes spanning their periods
	for si, s := range t.Sections {
		first, last := -1, -1
		for i, p := range t.Periods {
			if p.Section == si {
				if first < 0 {
					first = i
				}
				last = i
			}
		}
		if first < 0 {
			continue
		}
		c, tc := th.PaletteColor(si), th.PaletteTextColor(si)
		if !multi {
			c, tc = th.PaletteColor(0), th.PaletteTextColor(0)
		}
		sx := xs[first]
		sw := xs[last] + colW - sx
		b := measure(s, secFont, sw)
		sc.Add(scene.RectPath(sx, 0, sw, secH, 8, scene.Style{Fill: theme.Darken(c, 0.08), Shadow: true}))
		sc.Add(scene.NewText(sx+sw/2, secH/2, b.txt, secFont, tc, scene.AnchorMiddle, scene.VAlignMiddle))
	}

	// the time axis
	lineCol := th.LineColor
	sc.Add(scene.Edge([]scene.Point{{X: -10, Y: yLine}, {X: totalW + 30, Y: yLine}}, scene.EdgeOpts{
		Style: scene.Style{Stroke: lineCol, StrokeWidth: 3}, End: scene.MarkerArrow, Marker: scene.MarkerOpts{Size: 14},
	})...)

	for i, p := range t.Periods {
		c, tc := colorOf(i, p)
		x := xs[i]
		b := measure(p.Label, periodFont, colW)
		py := yPeriod + (periodH - b.h)
		sc.Add(scene.RectPath(x, py, colW, b.h, 8, scene.Style{Fill: c, Stroke: theme.Darken(c, 0.15), StrokeWidth: 1, Shadow: true}))
		sc.Add(scene.NewText(x+colW/2, py+b.h/2, b.txt, periodFont, tc, scene.AnchorMiddle, scene.VAlignMiddle))
		// connector from the period box through the axis to the events
		cx := x + colW/2
		sc.Add(scene.Circle(cx, yLine, 6, scene.Style{Fill: c, Stroke: th.Background, StrokeWidth: 2}))
		ey := yLine + 30
		evFill := theme.Mix(c, th.Background, 0.35)
		evText := theme.ContrastText(evFill)
		var boxes []box
		for _, e := range p.Events {
			boxes = append(boxes, measure(e, eventFont, colW-16))
		}
		if len(boxes) > 0 {
			bottom := ey
			for _, b := range boxes {
				bottom += b.h + 12
			}
			sc.Add(scene.Line(cx, py+b.h, cx, bottom-12, scene.Style{Stroke: theme.WithAlpha(c, 200), StrokeWidth: 2, Dash: []float64{4, 4}}))
		} else {
			sc.Add(scene.Line(cx, py+b.h, cx, yLine, scene.Style{Stroke: theme.WithAlpha(c, 200), StrokeWidth: 2, Dash: []float64{4, 4}}))
		}
		for _, b := range boxes {
			bx := x + 8
			sc.Add(scene.RectPath(bx, ey, b.w, b.h, 6, scene.Style{Fill: evFill, Stroke: c, StrokeWidth: 1.2, Shadow: true}))
			sc.Add(scene.NewText(bx+b.w/2, ey+b.h/2, b.txt, eventFont, evText, scene.AnchorMiddle, scene.VAlignMiddle))
			ey += b.h + 12
		}
	}
	return diagram.Finish(sc, th, title), nil
}

// renderVertical draws a top-down timeline: periods on the left of a
// vertical axis and events to the right.
func renderVertical(t *Timeline, th *theme.Theme, sc *scene.Scene, title string,
	colorOf func(int, *Period) (color.RGBA, color.RGBA), periodFont, eventFont, secFont scene.Font) *scene.Scene {
	axisX := colW + 40
	y := 0.0
	var items []scene.Item
	add := func(it ...scene.Item) { items = append(items, it...) }
	curSec := -2
	for i, p := range t.Periods {
		if p.Section != curSec && p.Section >= 0 {
			curSec = p.Section
			c := th.PaletteColor(p.Section)
			txt := t.Sections[p.Section]
			_, h := scene.MeasureBlock(txt, secFont, 0)
			add(scene.RectPath(0, y, axisX+colW+60, h+14, 8, scene.Style{Fill: theme.Darken(c, 0.08)}))
			add(scene.NewText(12, y+(h+14)/2, txt, secFont, theme.ContrastText(theme.Darken(c, 0.08)), scene.AnchorStart, scene.VAlignMiddle))
			y += h + 26
		}
		c, tc := colorOf(i, p)
		ptxt := scene.WrapText(p.Label, periodFont, colW-20)
		_, ph := scene.MeasureBlock(ptxt, periodFont, 0)
		ph += 20
		add(scene.RectPath(0, y, colW, ph, 8, scene.Style{Fill: c, Shadow: true}))
		add(scene.NewText(colW/2, y+ph/2, ptxt, periodFont, tc, scene.AnchorMiddle, scene.VAlignMiddle))
		add(scene.Circle(axisX, y+ph/2, 6, scene.Style{Fill: c, Stroke: th.Background, StrokeWidth: 2}))
		ey := y
		for _, e := range p.Events {
			txt := scene.WrapText(e, eventFont, colW+20)
			_, eh := scene.MeasureBlock(txt, eventFont, 0)
			eh += 16
			fill := theme.Mix(c, th.Background, 0.35)
			add(scene.RectPath(axisX+26, ey, colW+40, eh, 6, scene.Style{Fill: fill, Stroke: c, StrokeWidth: 1.2}))
			add(scene.NewText(axisX+36, ey+eh/2, txt, eventFont, theme.ContrastText(fill), scene.AnchorStart, scene.VAlignMiddle))
			ey += eh + 8
		}
		y = math.Max(y+ph, ey-8) + 18
	}
	sc.Add(scene.Edge([]scene.Point{{X: axisX, Y: -10}, {X: axisX, Y: y + 10}}, scene.EdgeOpts{
		Style: scene.Style{Stroke: th.LineColor, StrokeWidth: 3}, End: scene.MarkerArrow, Marker: scene.MarkerOpts{Size: 14},
	})...)
	sc.Add(items...)
	return diagram.Finish(sc, th, title)
}
