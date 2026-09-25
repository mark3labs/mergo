// Package gantt implements Mermaid gantt charts.
package gantt

import (
	"image/color"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

func init() {
	diagram.Register(diagram.Type{
		Name:   "gantt",
		Detect: diagram.Keyword("gantt"),
		Render: Render,
	})
}

// Now is the clock used for relative dates and the today marker
// (overridable in tests).
var Now = time.Now

// Render renders a gantt chart.
func Render(src string, cfg *diagram.Config) (*scene.Scene, error) {
	now := Now().UTC()
	c, err := Parse(src, now)
	if err != nil {
		return nil, err
	}
	th := cfg.Theme
	sc := diagram.NewScene(th)
	title := c.Title
	if title == "" {
		title = cfg.Title
	}
	if len(c.Tasks) == 0 {
		sc.Add(scene.NewText(0, 0, "(no tasks)", diagram.Font(th, 1), th.TextColor, scene.AnchorMiddle, scene.VAlignMiddle))
		return diagram.Finish(sc, th, title), nil
	}
	r := &renderer{c: c, th: th, cfg: cfg, now: now}
	r.draw(sc)
	return diagram.Finish(sc, th, title), nil
}

type renderer struct {
	c   *Chart
	th  *theme.Theme
	cfg *diagram.Config
	now time.Time

	min, max time.Time
	x0, w    float64 // chart area
	font     scene.Font
	secFont  scene.Font
	axisFont scene.Font
}

func (r *renderer) xOf(t time.Time) float64 {
	span := r.max.Sub(r.min).Seconds()
	if span <= 0 {
		return r.x0
	}
	return r.x0 + t.Sub(r.min).Seconds()/span*r.w
}

type row struct {
	section int
	tasks   []*Task
}

func (r *renderer) draw(sc *scene.Scene) {
	c, th := r.c, r.th
	r.font = scene.Font{Size: th.FontSize * 0.82}
	r.secFont = scene.Font{Size: th.FontSize * 0.85, Bold: true}
	r.axisFont = scene.Font{Size: th.FontSize * 0.75}

	// time range
	first := true
	for _, t := range c.Tasks {
		if first {
			r.min, r.max = t.Start, t.End
			first = false
		}
		if t.Start.Before(r.min) {
			r.min = t.Start
		}
		if t.End.After(r.max) {
			r.max = t.End
		}
	}
	if !r.max.After(r.min) {
		r.max = r.min.Add(24 * time.Hour)
	}

	// rows: one per task, or packed lanes in compact mode (vert markers
	// don't take a row)
	var rows []row
	for s := range c.Sections {
		var tasks []*Task
		for _, t := range c.Tasks {
			if t.Section == s && !t.Vert {
				tasks = append(tasks, t)
			}
		}
		if c.Compact {
			var lanes [][]*Task
			sorted := append([]*Task(nil), tasks...)
			sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Start.Before(sorted[j].Start) })
		outer:
			for _, t := range sorted {
				for li := range lanes {
					last := lanes[li][len(lanes[li])-1]
					if !t.Start.Before(last.End) {
						lanes[li] = append(lanes[li], t)
						continue outer
					}
				}
				lanes = append(lanes, []*Task{t})
			}
			for _, l := range lanes {
				rows = append(rows, row{section: s, tasks: l})
			}
			if len(lanes) == 0 {
				rows = append(rows, row{section: s})
			}
		} else {
			for _, t := range tasks {
				rows = append(rows, row{section: s, tasks: []*Task{t}})
			}
		}
	}

	// geometry
	barH := r.cfg.Float("gantt", "barHeight", 24)
	gap := r.cfg.Float("gantt", "barGap", 8)
	rowH := barH + gap
	secW := 0.0
	for _, s := range c.Sections {
		secW = math.Max(secW, scene.MeasureText(s, r.secFont))
	}
	r.x0 = math.Max(secW+32, 60)
	r.w = r.cfg.Float("gantt", "chartWidth", 1100)
	axisH := 26.0
	top := 0.0
	if c.TopAxis {
		top = axisH
	}
	chartTop := top + 6
	chartH := float64(len(rows)) * rowH
	chartBottom := chartTop + chartH

	// section bands
	secBkg := th.SectionBkg
	if len(secBkg) == 0 {
		secBkg = []color.RGBA{th.TertiaryColor}
	}
	y := chartTop
	for i := 0; i < len(rows); {
		s := rows[i].section
		j := i
		for j < len(rows) && rows[j].section == s {
			j++
		}
		h := float64(j-i) * rowH
		bg := secBkg[s%len(secBkg)]
		if bg.A > 0 {
			sc.Add(scene.RectPath(0, y, r.x0+r.w, h, 0, scene.Style{Fill: bg}))
		}
		if c.Sections[s] != "" {
			txt := scene.WrapText(c.Sections[s], r.secFont, r.x0-20)
			sc.Add(scene.NewText(12, y+h/2, txt, r.secFont, th.TextColor, scene.AnchorStart, scene.VAlignMiddle))
		}
		y += h
		i = j
	}

	// excluded days
	days := r.max.Sub(r.min).Hours() / 24
	if c.hasExcludes() && days <= 400 {
		exc := theme.WithAlpha(th.ExcludeBkg, 200)
		for d := startOfDay(r.min); d.Before(r.max); d = d.AddDate(0, 0, 1) {
			if c.Excluded(d) {
				xa, xb := math.Max(r.xOf(d), r.x0), math.Min(r.xOf(d.AddDate(0, 0, 1)), r.x0+r.w)
				if xb > xa {
					sc.Add(scene.RectPath(xa, chartTop, xb-xa, chartH, 0, scene.Style{Fill: exc}))
				}
			}
		}
	}

	// grid + axis
	ticks, format := r.ticks()
	grid := scene.Style{Stroke: th.GridColor, StrokeWidth: 1}
	for _, t := range ticks {
		x := r.xOf(t)
		if x < r.x0-0.5 || x > r.x0+r.w+0.5 {
			continue
		}
		sc.Add(scene.Line(x, chartTop, x, chartBottom, grid))
		label := Strftime(format, t)
		sc.Add(scene.NewText(x, chartBottom+8, label, r.axisFont, th.TextColor, scene.AnchorMiddle, scene.VAlignTop))
		if c.TopAxis {
			sc.Add(scene.NewText(x, chartTop-6, label, r.axisFont, th.TextColor, scene.AnchorMiddle, scene.VAlignBottom))
		}
	}
	sc.Add(scene.Line(r.x0, chartBottom, r.x0+r.w, chartBottom, scene.Style{Stroke: theme.Mix(th.GridColor, th.TextColor, 0.4), StrokeWidth: 1}))

	// today marker
	if c.TodayMarker != "off" && !r.now.Before(r.min) && !r.now.After(r.max) {
		st := scene.Style{Stroke: th.TodayLine, StrokeWidth: 2}
		css := diagram.ParseCSS(c.TodayMarker)
		if v, ok := css["stroke"]; ok {
			if col, err := theme.ParseColor(v); err == nil {
				st.Stroke = col
			}
		}
		if v, ok := css["stroke-width"]; ok {
			if f, ok := diagram.ParsePx(v); ok {
				st.StrokeWidth = f
			}
		}
		x := r.xOf(r.now)
		sc.Add(scene.Line(x, chartTop-4, x, chartBottom+4, st))
	}

	// tasks
	for ri, rw := range rows {
		ry := chartTop + float64(ri)*rowH + gap/2
		for _, t := range rw.tasks {
			r.drawTask(sc, t, ry, barH)
		}
	}

	// vertical markers
	for _, t := range c.Tasks {
		if !t.Vert {
			continue
		}
		x := r.xOf(t.Start)
		col := th.CritBorder
		sc.Add(scene.Line(x, chartTop-4, x, chartBottom+4, scene.Style{Stroke: col, StrokeWidth: 2, Dash: []float64{6, 4}}))
		sc.Add(scene.NewText(x, chartTop-8, t.Name, r.font, col, scene.AnchorMiddle, scene.VAlignBottom))
	}
	_ = axisH
}

func (r *renderer) colors(t *Task) (fill, stroke, text color.RGBA) {
	th := r.th
	fill, stroke, text = th.TaskBkg, th.TaskBorder, th.TaskText
	switch {
	case t.Crit && t.Done:
		fill, stroke, text = th.DoneTask, th.CritBorder, theme.ContrastText(th.DoneTask)
	case t.Crit && t.Active:
		fill, stroke, text = th.ActiveTask, th.CritBorder, theme.ContrastText(th.ActiveTask)
	case t.Crit:
		fill, stroke, text = th.CritTask, th.CritBorder, theme.ContrastText(th.CritTask)
	case t.Done:
		fill, stroke, text = th.DoneTask, th.DoneBorder, theme.ContrastText(th.DoneTask)
	case t.Active:
		fill, stroke, text = th.ActiveTask, th.ActiveBorder, theme.ContrastText(th.ActiveTask)
	}
	if math.Abs(theme.Luminance(fill)-theme.Luminance(text)) < 0.3 {
		text = theme.ContrastText(fill)
	}
	return
}

func (r *renderer) drawTask(sc *scene.Scene, t *Task, y, h float64) {
	th := r.th
	fill, stroke, text := r.colors(t)
	cy := y + h/2
	if t.Milestone {
		x := r.xOf(t.Start)
		if t.End.After(t.Start) {
			x = (r.xOf(t.Start) + r.xOf(t.End)) / 2
		}
		s := h * 0.5
		sc.Add(scene.NewPath(scene.Style{Fill: fill, Stroke: stroke, StrokeWidth: 1.3, Shadow: true}).Polygon(
			scene.Pt(x, cy-s), scene.Pt(x+s, cy), scene.Pt(x, cy+s), scene.Pt(x-s, cy)))
		sc.Add(scene.NewText(x+s+8, cy, t.Name, r.font, th.TextColor, scene.AnchorStart, scene.VAlignMiddle))
		return
	}
	x1, x2 := r.xOf(t.Start), r.xOf(t.End)
	w := math.Max(x2-x1, 3)
	sc.Add(scene.RectPath(x1, y, w, h, 4, scene.Style{Fill: fill, Stroke: stroke, StrokeWidth: 1.2, Shadow: true}))
	tw := scene.MeasureText(t.Name, r.font)
	switch {
	case tw+12 <= w:
		sc.Add(scene.NewText(x1+w/2, cy, t.Name, r.font, text, scene.AnchorMiddle, scene.VAlignMiddle))
	case x2+8+tw <= r.x0+r.w+120:
		sc.Add(scene.NewText(x2+8, cy, t.Name, r.font, th.TextColor, scene.AnchorStart, scene.VAlignMiddle))
	default:
		sc.Add(scene.NewText(x1-8, cy, t.Name, r.font, th.TextColor, scene.AnchorEnd, scene.VAlignMiddle))
	}
}

type interval struct {
	unit  string
	n     int
	dur   time.Duration // approximate
	label string        // default strftime format
}

var candidates = []interval{
	{"minute", 1, time.Minute, "%H:%M"},
	{"minute", 5, 5 * time.Minute, "%H:%M"},
	{"minute", 15, 15 * time.Minute, "%H:%M"},
	{"minute", 30, 30 * time.Minute, "%H:%M"},
	{"hour", 1, time.Hour, "%H:%M"},
	{"hour", 3, 3 * time.Hour, "%H:%M"},
	{"hour", 6, 6 * time.Hour, "%b %d %H:%M"},
	{"hour", 12, 12 * time.Hour, "%b %d %H:%M"},
	{"day", 1, 24 * time.Hour, "%Y-%m-%d"},
	{"day", 2, 48 * time.Hour, "%Y-%m-%d"},
	{"week", 1, 7 * 24 * time.Hour, "%Y-%m-%d"},
	{"week", 2, 14 * 24 * time.Hour, "%Y-%m-%d"},
	{"month", 1, 30 * 24 * time.Hour, "%Y-%m"},
	{"month", 3, 91 * 24 * time.Hour, "%Y-%m"},
	{"month", 6, 182 * 24 * time.Hour, "%Y-%m"},
	{"year", 1, 365 * 24 * time.Hour, "%Y"},
	{"year", 5, 5 * 365 * 24 * time.Hour, "%Y"},
}

// ticks returns tick times and the strftime format for labels.
func (r *renderer) ticks() ([]time.Time, string) {
	c := r.c
	span := r.max.Sub(r.min)
	var iv interval
	if m := tickRe.FindStringSubmatch(c.TickInterval); m != nil {
		n, _ := strconv.Atoi(m[1])
		iv = interval{unit: m[2], n: n}
		switch m[2] {
		case "millisecond", "second", "minute":
			iv.label = "%H:%M:%S"
		case "hour":
			iv.label = "%H:%M"
		case "month":
			iv.label = "%Y-%m"
		default:
			iv.label = "%Y-%m-%d"
		}
	} else {
		format := c.AxisFormat
		if format == "" {
			format = "%Y-%m-%d"
		}
		labelW := scene.MeasureText(Strftime(format, r.min), r.axisFont) + 18
		maxTicks := math.Max(r.w/labelW, 2)
		iv = candidates[len(candidates)-1]
		for _, cand := range candidates {
			if float64(span)/float64(cand.dur) <= maxTicks {
				iv = cand
				break
			}
		}
	}
	format := c.AxisFormat
	if format == "" {
		format = iv.label
		if c.TickInterval == "" && (iv.unit == "day" || iv.unit == "week") {
			format = "%Y-%m-%d"
		}
	}
	// align the first tick
	t := r.min
	switch iv.unit {
	case "millisecond":
		t = t.Truncate(time.Millisecond)
	case "second":
		t = t.Truncate(time.Second)
	case "minute":
		t = t.Truncate(time.Duration(iv.n) * time.Minute)
	case "hour":
		t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()-t.Hour()%iv.n, 0, 0, 0, time.UTC)
	case "day":
		t = startOfDay(t)
	case "week":
		t = startOfDay(t)
		t = t.AddDate(0, 0, -int(t.Weekday()))
	case "month":
		t = time.Date(t.Year(), t.Month()-time.Month((int(t.Month())-1)%iv.n), 1, 0, 0, 0, 0, time.UTC)
	case "year":
		t = time.Date(t.Year()-t.Year()%iv.n, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	var out []time.Time
	for i := 0; !t.After(r.max) && i < 500; i++ {
		if !t.Before(r.min) {
			out = append(out, t)
		}
		switch iv.unit {
		case "millisecond":
			t = t.Add(time.Duration(iv.n) * time.Millisecond)
		case "second":
			t = t.Add(time.Duration(iv.n) * time.Second)
		case "minute":
			t = t.Add(time.Duration(iv.n) * time.Minute)
		case "hour":
			t = t.Add(time.Duration(iv.n) * time.Hour)
		case "day":
			t = t.AddDate(0, 0, iv.n)
		case "week":
			t = t.AddDate(0, 0, 7*iv.n)
		case "month":
			t = t.AddDate(0, iv.n, 0)
		case "year":
			t = t.AddDate(iv.n, 0, 0)
		}
	}
	return out, format
}
