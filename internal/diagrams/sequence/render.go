// Package sequence implements Mermaid sequence diagrams.
package sequence

import (
	"image/color"
	"math"
	"sort"
	"strconv"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

func init() {
	diagram.Register(diagram.Type{
		Name:   "sequence",
		Detect: diagram.Keyword("sequenceDiagram"),
		Render: Render,
	})
}

// Render renders a sequence diagram.
func Render(src string, cfg *diagram.Config) (*scene.Scene, error) {
	d, err := Parse(src)
	if err != nil {
		return nil, err
	}
	r := newRenderer(d, cfg)
	r.layoutX()
	r.layoutY()
	sc := r.draw()
	title := d.Title
	if title == "" {
		title = cfg.Title
	}
	return diagram.Finish(sc, cfg.Theme, title), nil
}

type pinfo struct {
	w, h     float64 // box size
	textW    float64
	textH    float64
	x        float64 // center
	top      float64 // top y of the (first) box
	lifeTop  float64 // lifeline start
	lifeEnd  float64 // lifeline end (0 = until bottom)
	createdY float64 // center y of the box of created participants
}

type bar struct {
	p          *Participant
	depth      int
	start, end float64
}

type frame struct {
	ev          *Event
	top, bottom float64
	seps        []sepInfo
	minX, maxX  float64
	hasX        bool
	depth       int
	rect        bool
	color       color.RGBA
}

type sepInfo struct {
	y     float64
	label string
}

type msgDraw struct {
	ev     *Event
	x1, x2 float64
	y      float64
	self   bool
}

type noteDraw struct {
	ev         *Event
	x, y, w, h float64
}

type renderer struct {
	d   *Diagram
	cfg *diagram.Config
	th  *theme.Theme

	font, noteFont, labelFont scene.Font
	actorW, actorH, margin    float64
	mirror, rightAngles       bool

	ps map[*Participant]*pinfo

	headerH float64
	bottomY float64

	bars   []*bar
	frames []*frame
	msgs   []msgDraw
	notes  []noteDraw
	xs     []struct{ x, y float64 }
}

func newRenderer(d *Diagram, cfg *diagram.Config) *renderer {
	th := cfg.Theme
	return &renderer{
		d:           d,
		cfg:         cfg,
		th:          th,
		font:        scene.Font{Size: th.FontSize},
		noteFont:    scene.Font{Size: th.FontSize * 0.9},
		labelFont:   scene.Font{Size: th.FontSize * 0.85, Bold: true},
		actorW:      cfg.Float("sequence", "width", 150),
		actorH:      cfg.Float("sequence", "height", 65),
		margin:      cfg.Float("sequence", "actorMargin", 50),
		mirror:      cfg.Bool("sequence", "mirrorActors", true),
		rightAngles: cfg.Bool("sequence", "rightAngles", false),
		ps:          map[*Participant]*pinfo{},
	}
}

func isIcon(k Kind) bool {
	switch k {
	case KindActor, KindBoundary, KindControl, KindEntity, KindDatabase:
		return true
	}
	return false
}

const iconH = 40.0

// ---------------------------------------------------------------------------
// horizontal layout

type constraint struct {
	a, b int
	dist float64
}

func (r *renderer) layoutX() {
	ps := r.d.Participants
	for _, p := range ps {
		tw, tht := scene.MeasureBlock(p.Label, r.font, 0)
		pi := &pinfo{textW: tw, textH: tht}
		pi.w = math.Max(r.actorW, tw+24)
		if isIcon(p.Kind) {
			pi.w = math.Max(math.Min(r.actorW, 110), tw+16)
			pi.h = iconH + 8 + tht
		} else {
			pi.h = math.Max(r.actorH, tht+24)
		}
		r.ps[p] = pi
		r.headerH = math.Max(r.headerH, pi.h)
	}
	var cons []constraint
	add := func(a, b int, d float64) {
		if a > b {
			a, b = b, a
		}
		if a == b || a < 0 || b >= len(ps) {
			return
		}
		cons = append(cons, constraint{a, b, d})
	}
	for _, ev := range r.d.Events {
		switch ev.Kind {
		case EvMessage:
			tw, _ := scene.MeasureBlock(ev.Text, r.font, 0)
			if ev.Number > 0 {
				tw += 22
			}
			a, b := ev.From.index, ev.To.index
			if a == b {
				add(a, a+1, tw+70)
			} else {
				add(a, b, tw+40)
			}
		case EvNote:
			tw, _ := scene.MeasureBlock(ev.Text, r.noteFont, 0)
			nw := tw + 24
			a := ev.Parts[0].index
			switch ev.Pos {
			case NoteRight:
				add(a, a+1, nw+30)
			case NoteLeft:
				add(a-1, a, nw+30)
			case NoteOver:
				if len(ev.Parts) > 1 {
					lo, hi := a, a
					for _, p := range ev.Parts {
						lo, hi = min(lo, p.index), max(hi, p.index)
					}
					add(lo, hi, nw-40)
				}
			}
		}
	}
	// initial positions
	x := 0.0
	for i, p := range ps {
		pi := r.ps[p]
		if i == 0 {
			x = pi.w / 2
		} else {
			prev := r.ps[ps[i-1]]
			x += prev.w/2 + r.margin + pi.w/2
		}
		pi.x = x
	}
	sort.SliceStable(cons, func(i, j int) bool { return cons[i].b < cons[j].b })
	for _, c := range cons {
		cur := r.ps[ps[c.b]].x - r.ps[ps[c.a]].x
		if cur < c.dist {
			delta := c.dist - cur
			for j := c.b; j < len(ps); j++ {
				r.ps[ps[j]].x += delta
			}
		}
	}
}

// ---------------------------------------------------------------------------
// vertical layout

func (r *renderer) activeDepth(p *Participant) int {
	n := 0
	for _, b := range r.bars {
		if b.p == p && b.end == 0 {
			n++
		}
	}
	return n
}

// attachX returns the x where a message attaches to p's lifeline (taking
// activation bars into account) when coming from direction dir (-1 = from
// the left, +1 = from the right).
func (r *renderer) attachX(p *Participant, depth int, dir float64) float64 {
	x := r.ps[p].x
	if depth <= 0 {
		return x
	}
	return x + float64(depth-1)*5 + dir*5
}

func (r *renderer) extend(x0, x1 float64) {
	for _, f := range r.frames {
		if f.bottom != 0 {
			continue
		}
		if !f.hasX {
			f.minX, f.maxX, f.hasX = x0, x1, true
		} else {
			f.minX = math.Min(f.minX, x0)
			f.maxX = math.Max(f.maxX, x1)
		}
	}
}

func (r *renderer) layoutY() {
	for _, p := range r.d.Participants {
		pi := r.ps[p]
		pi.top = r.headerH - pi.h // bottom-align header boxes
		pi.lifeTop = r.headerH
	}
	y := r.headerH + 12
	lastMsgY := y
	var open []*frame
	for _, ev := range r.d.Events {
		switch ev.Kind {
		case EvMessage:
			_, tht := scene.MeasureBlock(ev.Text, r.font, 0)
			if ev.Text == "" {
				tht = 0
			}
			self := ev.From == ev.To
			var createH float64
			if ev.Creates {
				createH = r.ps[ev.To].h / 2
			}
			y += 18 + tht + 4 + createH
			lineY := y
			fromDepth := r.activeDepth(ev.From)
			toDepth := r.activeDepth(ev.To)
			if ev.ActivateTo {
				toDepth++
			}
			fx, tx := r.ps[ev.From].x, r.ps[ev.To].x
			dir := 1.0
			if tx < fx {
				dir = -1
			}
			m := msgDraw{ev: ev, y: lineY, self: self}
			if self {
				m.x1 = r.attachX(ev.From, fromDepth, 1)
				m.x2 = m.x1
				if ev.ActivateTo {
					m.x2 = r.attachX(ev.To, toDepth, 1)
				}
				y += 24
				tw, _ := scene.MeasureBlock(ev.Text, r.font, 0)
				r.extend(m.x1-10, m.x1+math.Max(tw+16, 50))
			} else {
				m.x1 = r.attachX(ev.From, fromDepth, dir)
				m.x2 = r.attachX(ev.To, toDepth, -dir)
				if ev.Creates {
					m.x2 = tx - dir*r.ps[ev.To].w/2
				}
				r.extend(math.Min(fx, tx), math.Max(fx, tx))
			}
			if ev.Creates {
				pi := r.ps[ev.To]
				pi.createdY = lineY
				pi.top = lineY - pi.h/2
				pi.lifeTop = lineY + pi.h/2
				y += createH
			}
			r.msgs = append(r.msgs, m)
			if ev.ActivateTo {
				r.bars = append(r.bars, &bar{p: ev.To, depth: r.activeDepth(ev.To) + 1, start: lineY})
			}
			if ev.DeactivateFrom {
				r.endBar(ev.From, lineY)
			}
			if ev.Destroys != nil {
				pi := r.ps[ev.Destroys]
				pi.lifeEnd = lineY + 8
				r.xs = append(r.xs, struct{ x, y float64 }{pi.x, lineY + 8})
				y += 10
			}
			lastMsgY = lineY
		case EvNote:
			tw, tht := scene.MeasureBlock(ev.Text, r.noteFont, 0)
			w, h := tw+24, tht+16
			y += 14
			var x float64
			a := r.ps[ev.Parts[0]]
			switch ev.Pos {
			case NoteLeft:
				x = a.x - 12 - w
			case NoteRight:
				x = a.x + 12
			default:
				lo, hi := a.x, a.x
				for _, p := range ev.Parts {
					lo = math.Min(lo, r.ps[p].x)
					hi = math.Max(hi, r.ps[p].x)
				}
				if len(ev.Parts) > 1 {
					w = math.Max(w, hi-lo+50)
				}
				x = (lo+hi)/2 - w/2
			}
			r.notes = append(r.notes, noteDraw{ev: ev, x: x, y: y, w: w, h: h})
			r.extend(x, x+w)
			y += h
		case EvActivate:
			start := lastMsgY
			if start < r.headerH+12 {
				start = y
			}
			r.bars = append(r.bars, &bar{p: ev.Target, depth: r.activeDepth(ev.Target) + 1, start: start})
		case EvDeactivate:
			r.endBar(ev.Target, math.Max(lastMsgY, y-4))
		case EvBlockStart, EvRectStart:
			y += 14
			f := &frame{ev: ev, top: y, depth: len(open), rect: ev.Kind == EvRectStart, color: ev.Color}
			if !f.rect {
				_, lh := scene.MeasureBlock(ev.Label, r.labelFont, 0)
				y += math.Max(lh, 18) + 6
			}
			r.frames = append(r.frames, f)
			open = append(open, f)
		case EvBlockSep:
			y += 12
			if len(open) > 0 {
				f := open[len(open)-1]
				f.seps = append(f.seps, sepInfo{y: y, label: ev.Label})
			}
			y += 20
		case EvBlockEnd, EvRectEnd:
			y += 12
			if len(open) > 0 {
				f := open[len(open)-1]
				open = open[:len(open)-1]
				f.bottom = y
			}
		}
	}
	for _, b := range r.bars {
		if b.end == 0 {
			b.end = y + 6
		}
	}
	r.bottomY = y + 24
	// Finalize frame extents: nested frames grow outwards; empty frames
	// span everything.
	allMin, allMax := math.Inf(1), math.Inf(-1)
	for _, p := range r.d.Participants {
		allMin = math.Min(allMin, r.ps[p].x)
		allMax = math.Max(allMax, r.ps[p].x)
	}
	for i := len(r.frames) - 1; i >= 0; i-- {
		f := r.frames[i]
		if !f.hasX {
			f.minX, f.maxX = allMin, allMax
		}
		if f.bottom == 0 {
			f.bottom = y
		}
		pad := 14.0
		f.minX -= pad
		f.maxX += pad
		// room for the label tag and texts
		need := r.frameLabelWidth(f)
		if f.maxX-f.minX < need {
			c := (f.minX + f.maxX) / 2
			f.minX, f.maxX = c-need/2, c+need/2
		}
		// propagate to enclosing frames
		for j := i - 1; j >= 0; j-- {
			g := r.frames[j]
			if g.depth == f.depth-1 && g.top <= f.top && (g.bottom == 0 || g.bottom >= f.bottom) {
				if !g.hasX {
					g.minX, g.maxX, g.hasX = f.minX, f.maxX, true
				} else {
					g.minX = math.Min(g.minX, f.minX)
					g.maxX = math.Max(g.maxX, f.maxX)
				}
				break
			}
		}
	}
}

func (r *renderer) frameLabelWidth(f *frame) float64 {
	if f.rect {
		return 0
	}
	tag := scene.MeasureText(f.ev.Block, r.labelFont) + 26
	w := tag
	if f.ev.Label != "" {
		w += scene.MeasureText("["+f.ev.Label+"]", r.labelFont)*1 + 30
	}
	for _, s := range f.seps {
		if s.label != "" {
			w = math.Max(w, scene.MeasureText("["+s.label+"]", r.labelFont)+30)
		}
	}
	return w
}

func (r *renderer) endBar(p *Participant, y float64) {
	for i := len(r.bars) - 1; i >= 0; i-- {
		b := r.bars[i]
		if b.p == p && b.end == 0 {
			b.end = math.Max(y, b.start+12)
			return
		}
	}
}

// ---------------------------------------------------------------------------
// drawing

func (r *renderer) draw() *scene.Scene {
	th := r.th
	sc := diagram.NewScene(th)
	bottomTop := r.bottomY

	// participant groups (box ... end)
	for _, b := range r.d.Boxes {
		if len(b.Members) == 0 {
			continue
		}
		x0, x1 := math.Inf(1), math.Inf(-1)
		for _, p := range b.Members {
			pi := r.ps[p]
			x0 = math.Min(x0, pi.x-pi.w/2)
			x1 = math.Max(x1, pi.x+pi.w/2)
		}
		_, th2 := scene.MeasureBlock(b.Title, r.labelFont, 0)
		if b.Title == "" {
			th2 = 0
		}
		top := -th2 - 18
		bottom := r.headerH + 10
		if r.mirror {
			bottom = bottomTop + r.headerH + 10
		} else {
			bottom = bottomTop + 10
		}
		fill := b.Color
		if fill.A == 0 {
			fill = theme.WithAlpha(th.TertiaryColor, 0)
		}
		sc.Add(scene.RectPath(x0-12, top, x1-x0+24, bottom-top, 6, scene.Style{
			Fill: fill, Stroke: theme.WithAlpha(th.ActorBorder, 90), StrokeWidth: 1,
		}))
		if b.Title != "" {
			tc := th.TextColor
			if b.Color.A > 100 {
				tc = theme.ContrastText(b.Color)
			}
			sc.Add(scene.NewText((x0+x1)/2, top+8, b.Title, r.labelFont, tc, scene.AnchorMiddle, scene.VAlignTop))
		}
	}

	// rect highlights (behind everything else)
	for _, f := range r.frames {
		if !f.rect {
			continue
		}
		sc.Add(scene.RectPath(f.minX, f.top, f.maxX-f.minX, f.bottom-f.top, 0, scene.Style{Fill: f.color}))
	}

	// lifelines
	for _, p := range r.d.Participants {
		pi := r.ps[p]
		end := bottomTop
		if pi.lifeEnd > 0 {
			end = pi.lifeEnd
		}
		sc.Add(scene.Line(pi.x, pi.lifeTop, pi.x, end, scene.Style{Stroke: th.ActorLine, StrokeWidth: 1}))
	}

	// frames
	for _, f := range r.frames {
		if f.rect {
			continue
		}
		r.drawFrame(sc, f)
	}

	// activation bars
	sort.SliceStable(r.bars, func(i, j int) bool { return r.bars[i].depth < r.bars[j].depth })
	for _, b := range r.bars {
		x := r.ps[b.p].x + float64(b.depth-1)*5 - 5
		sc.Add(scene.RectPath(x, b.start, 10, b.end-b.start, 0, scene.Style{
			Fill: th.ActivationBkg, Stroke: th.ActivationBorder, StrokeWidth: 1,
		}))
	}

	// messages
	for _, m := range r.msgs {
		r.drawMessage(sc, m)
	}

	// notes
	for _, n := range r.notes {
		sc.Add(scene.RectPath(n.x, n.y, n.w, n.h, 2, scene.Style{
			Fill: th.NoteBkg, Stroke: th.NoteBorder, StrokeWidth: 1, Shadow: true,
		}))
		sc.Add(scene.NewText(n.x+n.w/2, n.y+n.h/2, n.ev.Text, r.noteFont, th.NoteText, scene.AnchorMiddle, scene.VAlignMiddle))
	}

	// destroy marks
	for _, x := range r.xs {
		st := scene.Style{Stroke: th.SignalColor, StrokeWidth: 2, RoundCaps: true}
		sc.Add(scene.Line(x.x-8, x.y-8, x.x+8, x.y+8, st), scene.Line(x.x-8, x.y+8, x.x+8, x.y-8, st))
	}

	// actors
	for _, p := range r.d.Participants {
		pi := r.ps[p]
		r.drawActor(sc, p, pi.top)
		if r.mirror && !p.destroyed {
			r.drawActor(sc, p, bottomTop)
		}
	}
	return sc
}

func (r *renderer) drawFrame(sc *scene.Scene, f *frame) {
	th := r.th
	lineSt := scene.Style{Stroke: th.LabelBoxBorder, StrokeWidth: 1.5, Dash: []float64{3, 3}}
	x0, x1 := f.minX, f.maxX
	sc.Add(scene.RectPath(x0, f.top, x1-x0, f.bottom-f.top, 0, lineSt))
	// label tag (pentagon)
	tw := scene.MeasureText(f.ev.Block, r.labelFont)
	_, lh := scene.MeasureBlock(f.ev.Block, r.labelFont, 0)
	tagW, tagH := tw+20, math.Max(lh+6, 20)
	sc.Add(scene.NewPath(scene.Style{Fill: th.LabelBoxBkg, Stroke: th.LabelBoxBorder, StrokeWidth: 1}).Polygon(
		scene.Pt(x0, f.top), scene.Pt(x0+tagW, f.top), scene.Pt(x0+tagW, f.top+tagH-6),
		scene.Pt(x0+tagW-8, f.top+tagH), scene.Pt(x0, f.top+tagH)))
	sc.Add(scene.NewText(x0+tagW/2-3, f.top+tagH/2, f.ev.Block, r.labelFont, th.LabelText, scene.AnchorMiddle, scene.VAlignMiddle))
	if f.ev.Label != "" {
		cx := math.Max((x0+x1)/2, x0+tagW+8+scene.MeasureText("["+f.ev.Label+"]", r.labelFont)/2)
		sc.Add(scene.NewText(cx, f.top+tagH/2, "["+f.ev.Label+"]", r.labelFont, th.LoopText, scene.AnchorMiddle, scene.VAlignMiddle))
	}
	for _, s := range f.seps {
		sc.Add(scene.Line(x0, s.y, x1, s.y, lineSt))
		if s.label != "" {
			sc.Add(scene.NewText((x0+x1)/2, s.y+10, "["+s.label+"]", r.labelFont, th.LoopText, scene.AnchorMiddle, scene.VAlignMiddle))
		}
	}
}

func (r *renderer) drawMessage(sc *scene.Scene, m msgDraw) {
	th := r.th
	ev := m.ev
	st := scene.Style{Stroke: th.SignalColor, StrokeWidth: 1.5}
	if ev.Style == Dotted {
		st.Dash = []float64{4, 3}
	}
	end := scene.MarkerNone
	switch ev.Head {
	case HeadFilled:
		end = scene.MarkerArrow
	case HeadCross:
		end = scene.MarkerCross
	case HeadOpen:
		end = scene.MarkerOpenArrow
	}
	start := scene.MarkerNone
	if ev.Bidirectional {
		start = end
	}
	opts := scene.EdgeOpts{Style: st, Curve: scene.CurveLinear, Start: start, End: end,
		Marker: scene.MarkerOpts{Size: 10, Hollow: th.Background}}
	_, tht := scene.MeasureBlock(ev.Text, r.font, 0)
	if m.self {
		x := m.x1
		w := 36.0
		y0, y1 := m.y, m.y+24
		var pts []scene.Point
		if r.rightAngles {
			pts = []scene.Point{{X: x, Y: y0}, {X: x + w, Y: y0}, {X: x + w, Y: y1}, {X: m.x2, Y: y1}}
			opts.Curve = scene.CurveLinear
		} else {
			pts = []scene.Point{{X: x, Y: y0}, {X: x + w, Y: y0 - 2}, {X: x + w, Y: y1 + 2}, {X: m.x2, Y: y1}}
			opts.Curve = scene.CurveCatmull
		}
		sc.Add(scene.Edge(pts, opts)...)
		if ev.Text != "" {
			sc.Add(scene.NewText(x+8, y0-4-tht/2, ev.Text, r.font, th.SignalText, scene.AnchorStart, scene.VAlignMiddle))
		}
	} else {
		sc.Add(scene.Edge([]scene.Point{{X: m.x1, Y: m.y}, {X: m.x2, Y: m.y}}, opts)...)
		if ev.Text != "" {
			sc.Add(scene.NewText((m.x1+m.x2)/2, m.y-5-tht/2, ev.Text, r.font, th.SignalText, scene.AnchorMiddle, scene.VAlignMiddle))
		}
	}
	if ev.Number > 0 {
		f := scene.Font{Size: 11, Bold: true}
		s := strconv.Itoa(ev.Number)
		rad := math.Max(9, scene.MeasureText(s, f)/2+4)
		sc.Add(scene.Circle(m.x1, m.y, rad, scene.Style{Fill: th.SequenceNumberBg}))
		sc.Add(scene.NewText(m.x1, m.y, s, f, th.SequenceNumberText, scene.AnchorMiddle, scene.VAlignMiddle))
	}
}

// drawActor draws a participant header whose top edge is at y.
func (r *renderer) drawActor(sc *scene.Scene, p *Participant, y float64) {
	th := r.th
	pi := r.ps[p]
	cx := pi.x
	boxSt := scene.Style{Fill: th.ActorBkg, Stroke: th.ActorBorder, StrokeWidth: 1.3, Shadow: true}
	lineSt := scene.Style{Stroke: th.ActorBorder, StrokeWidth: 1.8, RoundCaps: true}
	labelY := y + iconH + 8 + pi.textH/2
	switch p.Kind {
	case KindActor:
		hy := y + 8
		sc.Add(scene.Circle(cx, hy, 7.5, scene.Style{Fill: th.ActorBkg, Stroke: th.ActorBorder, StrokeWidth: 1.8}))
		sc.Add(scene.NewPath(lineSt).
			MoveTo(cx, hy+7.5).LineTo(cx, y+27).
			MoveTo(cx-12, y+19).LineTo(cx+12, y+19).
			MoveTo(cx, y+27).LineTo(cx-10, y+iconH).
			MoveTo(cx, y+27).LineTo(cx+10, y+iconH))
	case KindBoundary:
		c := scene.Pt(cx+6, y+iconH/2)
		sc.Add(scene.Circle(c.X, c.Y, 15, boxSt))
		sc.Add(scene.NewPath(lineSt).MoveTo(cx-22, y+iconH/2-14).LineTo(cx-22, y+iconH/2+14).
			MoveTo(cx-22, y+iconH/2).LineTo(c.X-15, c.Y))
	case KindControl:
		c := scene.Pt(cx, y+iconH/2+2)
		sc.Add(scene.Circle(c.X, c.Y, 16, boxSt))
		sc.Add(scene.NewPath(scene.Style{Fill: th.ActorBorder}).Polygon(
			scene.Pt(c.X+2, c.Y-16), scene.Pt(c.X-6, c.Y-21), scene.Pt(c.X-6, c.Y-11)))
	case KindEntity:
		c := scene.Pt(cx, y+iconH/2-2)
		sc.Add(scene.Circle(c.X, c.Y, 16, boxSt))
		sc.Add(scene.Line(cx-16, c.Y+19, cx+16, c.Y+19, lineSt))
	case KindDatabase:
		w, h := 40.0, iconH
		x0 := cx - w/2
		ry := 6.0
		body := scene.NewPath(boxSt)
		body.MoveTo(x0, y+ry)
		body.Arc(cx, y+ry, w/2, ry, math.Pi, 2*math.Pi, true)
		body.LineTo(x0+w, y+h-ry)
		body.Arc(cx, y+h-ry, w/2, ry, 0, math.Pi, true)
		body.Close()
		sc.Add(body)
		sc.Add(scene.NewPath(scene.Style{Stroke: th.ActorBorder, StrokeWidth: 1.3}).Arc(cx, y+ry, w/2, ry, 0, math.Pi, false))
	case KindCollections:
		sc.Add(scene.RectPath(cx-pi.w/2+6, y-6, pi.w, pi.h, 3, scene.Style{Fill: th.ActorBkg, Stroke: th.ActorBorder, StrokeWidth: 1.3}))
		sc.Add(scene.RectPath(cx-pi.w/2, y, pi.w, pi.h, 3, boxSt))
		labelY = y + pi.h/2
	case KindQueue:
		rx := 10.0
		x0, w, h := cx-pi.w/2, pi.w, pi.h
		body := scene.NewPath(boxSt)
		body.MoveTo(x0+rx, y).LineTo(x0+w-rx, y)
		body.Arc(x0+w-rx, y+h/2, rx, h/2, -math.Pi/2, math.Pi/2, true)
		body.LineTo(x0+rx, y+h)
		body.Arc(x0+rx, y+h/2, rx, h/2, math.Pi/2, 3*math.Pi/2, true)
		body.Close()
		sc.Add(body)
		sc.Add(scene.NewPath(scene.Style{Stroke: th.ActorBorder, StrokeWidth: 1.3}).Arc(x0+w-rx, y+h/2, rx, h/2, math.Pi/2, 3*math.Pi/2, false))
		labelY = y + h/2
	default:
		sc.Add(scene.RectPath(cx-pi.w/2, y, pi.w, pi.h, 3, boxSt))
		labelY = y + pi.h/2
	}
	sc.Add(scene.NewText(cx, labelY, p.Label, r.font, th.ActorText, scene.AnchorMiddle, scene.VAlignMiddle))
}
