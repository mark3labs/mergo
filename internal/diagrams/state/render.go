// Package state implements Mermaid state diagrams.
package state

import (
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/layout"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

func init() {
	diagram.Register(diagram.Type{
		Name:   "state",
		Detect: diagram.Keyword("stateDiagram", "stateDiagram-v2"),
		Render: Render,
	})
}

// Render renders a state diagram.
func Render(src string, cfg *diagram.Config) (*scene.Scene, error) {
	d, err := Parse(src)
	if err != nil {
		return nil, err
	}
	r := newRenderer(d, cfg)
	sc := r.render()
	title := d.Title
	if title == "" {
		title = cfg.Title
	}
	return diagram.Finish(sc, cfg.Theme, title), nil
}

// geometry of a leaf state (and an optional note reserved beside it)
type leaf struct {
	w, h         float64 // state box
	title        string
	desc         string
	titleH       float64
	note         *Note
	noteW, noteH float64
	noteText     string
	offX         float64 // state center x relative to node center
	style        diagram.NodeStyle
}

type renderer struct {
	d   *Diagram
	th  *theme.Theme
	cfg *diagram.Config

	font, bold, noteFont, edgeFont scene.Font

	leaves   map[string]*leaf
	nodes    map[string]*layout.Node
	clusters map[string]*layout.Node
	regions  map[string][]*layout.Node
	postNote []*Note
	placed   []scene.Rect
}

func newRenderer(d *Diagram, cfg *diagram.Config) *renderer {
	th := cfg.Theme
	return &renderer{
		d: d, th: th, cfg: cfg,
		font:     scene.Font{Size: th.FontSize},
		bold:     scene.Font{Size: th.FontSize, Bold: true},
		noteFont: scene.Font{Size: th.FontSize * 0.88},
		edgeFont: scene.Font{Size: th.FontSize * 0.9},
		leaves:   map[string]*leaf{},
		nodes:    map[string]*layout.Node{},
		clusters: map[string]*layout.Node{},
		regions:  map[string][]*layout.Node{},
	}
}

func (r *renderer) styleFor(s *State) diagram.NodeStyle {
	ns := diagram.NodeStyle{
		Fill: r.th.PrimaryColor, Stroke: r.th.PrimaryBorderColor, Text: r.th.PrimaryTextColor,
		StrokeWidth: 1.3, FontSize: r.th.FontSize,
	}
	if def, ok := r.d.ClassDefs["default"]; ok {
		ns.Apply(def)
	}
	for _, c := range s.Classes {
		if def, ok := r.d.ClassDefs[c]; ok {
			ns.Apply(def)
		}
	}
	ns.Apply(s.Style)
	ns.FixContrast()
	return ns
}

// effective direction of a state's scope
func (r *renderer) dirOf(parent string) layout.Direction {
	for p := parent; p != ""; p = r.d.States[p].Parent {
		if dd := r.d.States[p].Dir; dd != "" {
			return layout.ParseDirection(dd)
		}
	}
	return layout.ParseDirection(r.d.Dir)
}

func (r *renderer) buildLeaf(s *State) *layout.Node {
	lf := &leaf{style: r.styleFor(s)}
	switch s.Kind {
	case KindStart, KindEnd:
		lf.w, lf.h = 18, 18
	case KindFork, KindJoin:
		if r.dirOf(s.Parent).Horizontal() {
			lf.w, lf.h = 9, 70
		} else {
			lf.w, lf.h = 70, 9
		}
	case KindChoice:
		lf.w, lf.h = 30, 30
	default:
		lf.title = s.Label
		tw, th := scene.MeasureBlock(lf.title, r.font, 0)
		lf.titleH = th
		if len(s.Descs) > 0 {
			lf.desc = joinLines(s.Descs)
			dw, dh := scene.MeasureBlock(lf.desc, r.font, 0)
			lf.w = math.Max(math.Max(tw, dw)+32, 80)
			lf.h = th + dh + 30
		} else {
			lf.w = math.Max(tw+32, 70)
			lf.h = math.Max(th+20, 40)
		}
	}
	r.leaves[s.ID] = lf
	n := &layout.Node{ID: s.ID, W: lf.w, H: lf.h}
	switch s.Kind {
	case KindChoice:
		n.Clip = layout.ClipDiamond
	case KindStart, KindEnd:
		n.Clip = layout.ClipEllipse
	}
	return n
}

func joinLines(ls []string) string {
	var out strings.Builder
	for i, l := range ls {
		if i > 0 {
			out.WriteString("\n")
		}
		out.WriteString(l)
	}
	return out.String()
}

// reserveNote widens a leaf's layout node to hold a note beside it.
func (r *renderer) reserveNote(n *layout.Node, note *Note) {
	lf := r.leaves[n.ID]
	txt := scene.WrapText(note.Text, r.noteFont, 220)
	w, h := scene.MeasureBlock(txt, r.noteFont, 0)
	lf.note, lf.noteText, lf.noteW, lf.noteH = note, txt, w+22, h+16
	gap := 24.0
	n.W = lf.w + gap + lf.noteW
	n.H = math.Max(lf.h, lf.noteH)
	if note.Left {
		lf.offX = n.W/2 - lf.w/2
	} else {
		lf.offX = -(n.W/2 - lf.w/2)
	}
	w0, h0, off := lf.w, lf.h, lf.offX
	base := n.Clip
	n.Clip = func(nn *layout.Node, p scene.Point) scene.Point {
		v := &layout.Node{X: nn.X + off, Y: nn.Y, W: w0, H: h0}
		if base != nil {
			return base(v, p)
		}
		return layout.ClipRect(v, p)
	}
}

func (r *renderer) render() *scene.Scene {
	d, th := r.d, r.th
	notesBy := map[string]*Note{}
	for _, n := range d.Notes {
		if _, dup := notesBy[n.Target]; !dup {
			notesBy[n.Target] = n
		} else {
			r.postNote = append(r.postNote, n)
		}
	}
	// leaves first
	for _, id := range d.Order {
		s := d.States[id]
		if s.IsComposite() {
			continue
		}
		n := r.buildLeaf(s)
		r.nodes[id] = n
		if note, ok := notesBy[id]; ok {
			r.reserveNote(n, note)
		}
	}
	// composites (build bottom-up by recursion)
	var build func(id string) *layout.Node
	build = func(id string) *layout.Node {
		if n, ok := r.nodes[id]; ok {
			return n
		}
		if c, ok := r.clusters[id]; ok {
			return c
		}
		s := d.States[id]
		tw, tht := scene.MeasureBlock(s.Label, r.bold, 0)
		c := &layout.Node{ID: "composite:" + id, Pad: 16, PadTop: tht + 20, MinW: tw + 36}
		if s.Dir != "" {
			dd := layout.ParseDirection(s.Dir)
			c.Dir = &dd
		}
		var nonEmpty [][]string
		for _, reg := range s.Regions {
			if len(reg) > 0 {
				nonEmpty = append(nonEmpty, reg)
			}
		}
		if len(nonEmpty) > 1 {
			for i, reg := range nonEmpty {
				rn := &layout.Node{ID: id + "#region" + string(rune('0'+i)), Pad: 6}
				for _, m := range reg {
					rn.Children = append(rn.Children, build(m))
				}
				c.Children = append(c.Children, rn)
				r.regions[id] = append(r.regions[id], rn)
			}
		} else {
			for _, m := range nonEmpty[0] {
				c.Children = append(c.Children, build(m))
			}
		}
		r.clusters[id] = c
		if note, ok := notesBy[id]; ok {
			r.postNote = append(r.postNote, note)
		}
		return c
	}
	var top []*layout.Node
	for _, reg := range d.Root {
		for _, id := range reg {
			top = append(top, build(id))
		}
	}
	lg := &layout.Graph{
		Dir:     layout.ParseDirection(d.Dir),
		NodeSep: r.cfg.Float("state", "nodeSpacing", 50),
		RankSep: r.cfg.Float("state", "rankSpacing", 50),
		Nodes:   top,
	}
	type tInfo struct {
		t  *Transition
		le *layout.Edge
	}
	var edges []tInfo
	for _, t := range d.Transitions {
		a, b := r.endpoint(t.From), r.endpoint(t.To)
		if a == nil || b == nil {
			continue
		}
		le := &layout.Edge{From: a, To: b}
		if t.Label != "" {
			le.LabelW, le.LabelH = diagram.LabelSize(t.Label, r.edgeFont)
		}
		lg.Edges = append(lg.Edges, le)
		edges = append(edges, tInfo{t, le})
	}
	layout.Layout(lg)

	sc := diagram.NewScene(th)
	// composites outer first
	ids := make([]string, 0, len(r.clusters))
	for id := range r.clusters {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		di, dj := r.depth(ids[i]), r.depth(ids[j])
		if di != dj {
			return di < dj
		}
		return d.States[ids[i]].order < d.States[ids[j]].order
	})
	for _, id := range ids {
		r.drawComposite(sc, id)
	}
	// edges
	var labels []scene.Item
	for _, e := range edges {
		if len(e.le.Points) < 2 {
			continue
		}
		sc.Add(scene.Edge(e.le.Points, scene.EdgeOpts{
			Style:  scene.Style{Stroke: th.LineColor, StrokeWidth: 1.5},
			End:    scene.MarkerArrow,
			Marker: scene.MarkerOpts{Size: 10},
		})...)
		if e.t.Label != "" {
			labels = append(labels, diagram.EdgeLabel(e.le.Label.X, e.le.Label.Y, e.t.Label, r.edgeFont, th)...)
		}
	}
	sc.Add(labels...)
	// states
	for _, id := range d.Order {
		if n, ok := r.nodes[id]; ok {
			r.drawLeaf(sc, d.States[id], n)
		}
	}
	// notes that could not be reserved in the layout
	for _, note := range r.postNote {
		r.drawFloatingNote(sc, note)
	}
	return sc
}

func (r *renderer) depth(id string) int {
	n := 0
	for p := r.d.States[id].Parent; p != ""; p = r.d.States[p].Parent {
		n++
	}
	return n
}

func (r *renderer) endpoint(id string) *layout.Node {
	if n, ok := r.nodes[id]; ok {
		return n
	}
	return r.clusters[id]
}

// roundedBottom builds a rect path whose bottom corners are rounded.
func roundedBottom(x, y, w, h, rad float64) *scene.Path {
	const k = 0.5522847498
	p := scene.NewPath(scene.Style{})
	p.MoveTo(x, y).LineTo(x+w, y).LineTo(x+w, y+h-rad)
	p.CubicTo(x+w, y+h-rad+rad*k, x+w-rad+rad*k, y+h, x+w-rad, y+h)
	p.LineTo(x+rad, y+h)
	p.CubicTo(x+rad-rad*k, y+h, x, y+h-rad+rad*k, x, y+h-rad)
	return p.Close()
}

func (r *renderer) drawComposite(sc *scene.Scene, id string) {
	th := r.th
	s := r.d.States[id]
	c := r.clusters[id]
	ns := r.styleFor(s)
	rc := c.Rect()
	rad := 8.0
	band := c.PadTop - 8
	sc.Add(scene.RectPath(rc.X, rc.Y, rc.W, rc.H, rad, scene.Style{Fill: ns.Fill, Stroke: ns.Stroke, StrokeWidth: ns.StrokeWidth, Shadow: true}))
	body := theme.Mix(th.Background, th.PrimaryColor, 0.10+0.12*float64(r.depth(id)%2))
	inner := roundedBottom(rc.X+1, rc.Y+band, rc.W-2, rc.H-band-1, rad-1)
	inner.Style = scene.Style{Fill: body}
	sc.Add(inner)
	sc.Add(scene.Line(rc.X, rc.Y+band, rc.X+rc.W, rc.Y+band, scene.Style{Stroke: ns.Stroke, StrokeWidth: 1}))
	sc.Add(scene.NewText(rc.X+rc.W/2, rc.Y+band/2, s.Label, r.bold, ns.Text, scene.AnchorMiddle, scene.VAlignMiddle))
	// concurrency separators
	regs := r.regions[id]
	for i := 0; i+1 < len(regs); i++ {
		a, b := regs[i].Rect(), regs[i+1].Rect()
		st := scene.Style{Stroke: ns.Stroke, StrokeWidth: 1.2, Dash: []float64{6, 4}}
		if math.Abs(a.Y-b.Y) < math.Abs(a.X-b.X) {
			// side by side
			x := (a.X + a.W + b.X) / 2
			if b.X < a.X {
				x = (b.X + b.W + a.X) / 2
			}
			sc.Add(scene.Line(x, rc.Y+band+6, x, rc.Y+rc.H-6, st))
		} else {
			y := (a.Y + a.H + b.Y) / 2
			if b.Y < a.Y {
				y = (b.Y + b.H + a.Y) / 2
			}
			sc.Add(scene.Line(rc.X+6, y, rc.X+rc.W-6, y, st))
		}
	}
}

func (r *renderer) drawLeaf(sc *scene.Scene, s *State, n *layout.Node) {
	th := r.th
	lf := r.leaves[s.ID]
	cx, cy := n.X+lf.offX, n.Y
	x, y := cx-lf.w/2, cy-lf.h/2
	ns := lf.style
	dark := th.LineColor
	switch s.Kind {
	case KindStart:
		sc.Add(scene.Circle(cx, cy, 8, scene.Style{Fill: dark}))
	case KindEnd:
		sc.Add(scene.Circle(cx, cy, 8.5, scene.Style{Fill: th.Background, Stroke: dark, StrokeWidth: 1.6}))
		sc.Add(scene.Circle(cx, cy, 5, scene.Style{Fill: dark}))
	case KindFork, KindJoin:
		sc.Add(scene.RectPath(x, y, lf.w, lf.h, 2, scene.Style{Fill: dark}))
	case KindChoice:
		sc.Add(scene.NewPath(ns.Box(true)).Polygon(
			scene.Pt(cx, y), scene.Pt(x+lf.w, cy), scene.Pt(cx, y+lf.h), scene.Pt(x, cy)))
	default:
		sc.Add(scene.RectPath(x, y, lf.w, lf.h, 8, ns.Box(true)))
		f := r.font
		f.Bold, f.Italic = ns.Bold, ns.Italic
		if lf.desc != "" {
			ty := y + 10 + lf.titleH/2
			sc.Add(scene.NewText(cx, ty, lf.title, f, ns.Text, scene.AnchorMiddle, scene.VAlignMiddle))
			ly := y + 10 + lf.titleH + 5
			sc.Add(scene.Line(x, ly, x+lf.w, ly, scene.Style{Stroke: ns.Stroke, StrokeWidth: 1}))
			sc.Add(scene.NewText(cx, (ly+y+lf.h)/2, lf.desc, r.font, ns.Text, scene.AnchorMiddle, scene.VAlignMiddle))
		} else {
			sc.Add(scene.NewText(cx, cy, lf.title, f, ns.Text, scene.AnchorMiddle, scene.VAlignMiddle))
		}
	}
	if lf.note != nil {
		var nx float64
		if lf.note.Left {
			nx = x - 24 - lf.noteW
		} else {
			nx = x + lf.w + 24
		}
		ny := cy - lf.noteH/2
		r.drawNoteBox(sc, nx, ny, lf.noteW, lf.noteH, lf.noteText)
		x1, x2 := x+lf.w, nx
		if lf.note.Left {
			x1, x2 = x, nx+lf.noteW
		}
		sc.Add(scene.Line(x1, cy, x2, cy, scene.Style{Stroke: th.NoteBorder, StrokeWidth: 1.2, Dash: []float64{4, 3}}))
	}
}

func (r *renderer) drawNoteBox(sc *scene.Scene, x, y, w, h float64, text string) {
	th := r.th
	sc.Add(scene.RectPath(x, y, w, h, 2, scene.Style{Fill: th.NoteBkg, Stroke: th.NoteBorder, StrokeWidth: 1, Shadow: true}))
	sc.Add(scene.NewText(x+11, y+h/2, text, r.noteFont, th.NoteText, scene.AnchorStart, scene.VAlignMiddle))
}

// drawFloatingNote places a note beside a composite (or second note).
func (r *renderer) drawFloatingNote(sc *scene.Scene, note *Note) {
	target := r.endpoint(note.Target)
	if target == nil {
		return
	}
	txt := scene.WrapText(note.Text, r.noteFont, 220)
	w, h := scene.MeasureBlock(txt, r.noteFont, 0)
	w += 22
	h += 16
	rc := target.Rect()
	// candidate positions: requested side, other side, below, above
	cands := []scene.Rect{
		{X: rc.X + rc.W + 24, Y: rc.Y + 8, W: w, H: h},
		{X: rc.X - 24 - w, Y: rc.Y + 8, W: w, H: h},
		{X: rc.X + rc.W/2 - w/2, Y: rc.Y + rc.H + 24, W: w, H: h},
		{X: rc.X + rc.W/2 - w/2, Y: rc.Y - 24 - h, W: w, H: h},
	}
	if note.Left {
		cands[0], cands[1] = cands[1], cands[0]
	}
	best := cands[0]
	for _, c := range cands {
		if !r.collides(c, note.Target) {
			best = c
			break
		}
	}
	x, y := best.X, best.Y
	r.placed = append(r.placed, best)
	r.drawNoteBox(sc, x, y, w, h, txt)
	// dashed connector from the target boundary to the note
	nc := scene.Pt(x+w/2, y+h/2)
	a := layout.ClipRect(target, nc)
	b := layout.ClipRect(&layout.Node{X: nc.X, Y: nc.Y, W: w, H: h}, scene.Pt(target.X, target.Y))
	sc.Add(scene.Line(a.X, a.Y, b.X, b.Y, scene.Style{Stroke: r.th.NoteBorder, StrokeWidth: 1.2, Dash: []float64{4, 3}}))
}

// collides reports whether rect c overlaps any state, composite (other than
// the note target and its ancestors) or previously placed note.
func (r *renderer) collides(c scene.Rect, target string) bool {
	hit := func(o scene.Rect) bool {
		return c.X < o.X+o.W+8 && o.X < c.X+c.W+8 && c.Y < o.Y+o.H+8 && o.Y < c.Y+c.H+8
	}
	anc := map[string]bool{}
	for p := target; p != ""; p = r.d.States[p].Parent {
		anc[p] = true
	}
	for id, n := range r.nodes {
		if !anc[id] && hit(n.Rect()) {
			return true
		}
	}
	for id, n := range r.clusters {
		if anc[id] {
			continue
		}
		// only composites that don't contain the target matter
		if hit(n.Rect()) {
			return true
		}
	}
	return slices.ContainsFunc(r.placed, hit)
}
