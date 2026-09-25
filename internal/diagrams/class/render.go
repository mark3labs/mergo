// Package class implements Mermaid class diagrams.
package class

import (
	"math"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/layout"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

func init() {
	diagram.Register(diagram.Type{
		Name:   "class",
		Detect: diagram.Keyword("classDiagram", "classDiagram-v2"),
		Render: Render,
	})
}

const (
	padX     = 12.0
	padY     = 7.0
	emptySec = 12.0
)

type box struct {
	c              *Class
	w, h           float64
	headerH        float64
	attrH, methodH float64
	style          diagram.NodeStyle
}

// Render renders a class diagram.
func Render(src string, cfg *diagram.Config) (*scene.Scene, error) {
	d, err := Parse(src)
	if err != nil {
		return nil, err
	}
	r := &renderer{d: d, th: cfg.Theme, cfg: cfg}
	sc := r.render()
	title := d.Title
	if title == "" {
		title = cfg.Title
	}
	return diagram.Finish(sc, cfg.Theme, title), nil
}

type renderer struct {
	d   *Diagram
	th  *theme.Theme
	cfg *diagram.Config

	font, bold, small scene.Font
	lh                float64
	boxes             map[string]*box
	nodes             map[string]*layout.Node
	clusters          map[string]*layout.Node
	noteNodes         []*layout.Node
}

func (r *renderer) measure(c *Class) *box {
	b := &box{c: c}
	ns := diagram.NodeStyle{
		Fill: r.th.PrimaryColor, Stroke: r.th.PrimaryBorderColor, Text: r.th.PrimaryTextColor,
		StrokeWidth: 1.3, FontSize: r.th.FontSize,
	}
	for _, cl := range c.Classes {
		if def, ok := r.d.ClassDefs[cl]; ok {
			ns.Apply(def)
		}
	}
	ns.Apply(c.Style)
	ns.FixContrast()
	b.style = ns
	w := scene.MeasureText(c.Label, r.bold)
	for _, a := range c.Annotations {
		w = math.Max(w, scene.MeasureText("«"+a+"»", r.small))
	}
	for _, m := range c.Attributes {
		w = math.Max(w, scene.MeasureText(m.Text, r.memberFont(m)))
	}
	for _, m := range c.Methods {
		w = math.Max(w, scene.MeasureText(m.Text, r.memberFont(m)))
	}
	b.w = math.Max(w+2*padX, 100)
	b.headerH = 2*padY + r.lh + float64(len(c.Annotations))*r.small.Size*1.3
	b.attrH = emptySec
	if len(c.Attributes) > 0 {
		b.attrH = 2*padY + float64(len(c.Attributes))*r.lh
	}
	b.methodH = emptySec
	if len(c.Methods) > 0 {
		b.methodH = 2*padY + float64(len(c.Methods))*r.lh
	}
	b.h = b.headerH + b.attrH + b.methodH
	return b
}

func (r *renderer) memberFont(m Member) scene.Font {
	f := r.font
	f.Italic = m.Abstract
	return f
}

func endMarker(e End) scene.MarkerKind {
	switch e {
	case EndInheritance:
		return scene.MarkerTriangleOpen
	case EndComposition:
		return scene.MarkerDiamondFilled
	case EndAggregation:
		return scene.MarkerDiamondOpen
	case EndArrow:
		return scene.MarkerArrow
	case EndLollipop:
		return scene.MarkerCircleOpen
	}
	return scene.MarkerNone
}

func (r *renderer) render() *scene.Scene {
	th := r.th
	d := r.d
	r.font = scene.Font{Size: th.FontSize * 0.95}
	r.bold = scene.Font{Size: th.FontSize, Bold: true}
	r.small = scene.Font{Size: th.FontSize * 0.8, Italic: true}
	r.lh = r.font.Size * 1.35
	r.boxes = map[string]*box{}
	r.nodes = map[string]*layout.Node{}
	r.clusters = map[string]*layout.Node{}

	for _, id := range d.Order {
		b := r.measure(d.Classes[id])
		r.boxes[id] = b
		r.nodes[id] = &layout.Node{ID: id, W: b.w, H: b.h}
	}
	// namespaces
	titleFont := scene.Font{Size: th.FontSize, Bold: true}
	for _, id := range d.NSOrder {
		tw, tht := scene.MeasureBlock(id, titleFont, 0)
		r.clusters[id] = &layout.Node{ID: "ns:" + id, Pad: 18, PadTop: tht + 14, MinW: tw + 30}
	}
	for _, id := range d.NSOrder {
		ns := d.Namespaces[id]
		cl := r.clusters[id]
		for _, s := range ns.Subs {
			cl.Children = append(cl.Children, r.clusters[s])
		}
		for _, c := range ns.Classes {
			cl.Children = append(cl.Children, r.nodes[c])
		}
		if len(cl.Children) == 0 {
			cl.W, cl.H = cl.MinW, cl.PadTop+20
		}
	}
	var top []*layout.Node
	for _, id := range d.Order {
		if d.Classes[id].Namespace == "" {
			top = append(top, r.nodes[id])
		}
	}
	for _, id := range d.NSOrder {
		if d.Namespaces[id].Parent == "" {
			top = append(top, r.clusters[id])
		}
	}
	// notes
	noteFont := scene.Font{Size: th.FontSize * 0.9}
	type noteInfo struct {
		n   *Note
		ln  *layout.Node
		le  *layout.Edge
		w   float64
		txt string
	}
	var notes []noteInfo
	lg := &layout.Graph{
		Dir:     layout.ParseDirection(d.Dir),
		NodeSep: r.cfg.Float("class", "nodeSpacing", 60),
		RankSep: r.cfg.Float("class", "rankSpacing", 60),
	}
	for i, n := range d.Notes {
		txt := scene.WrapText(n.Text, noteFont, 220)
		w, h := scene.MeasureBlock(txt, noteFont, 0)
		ln := &layout.Node{ID: "note" + string(rune('a'+i)), W: w + 24, H: h + 18}
		top = append(top, ln)
		ni := noteInfo{n: n, ln: ln, txt: txt}
		if n.For != "" {
			if target, ok := r.nodes[n.For]; ok {
				ni.le = &layout.Edge{From: ln, To: target, Weight: 0.5}
				lg.Edges = append(lg.Edges, ni.le)
			}
		}
		notes = append(notes, ni)
	}
	lg.Nodes = top

	// relations
	edgeFont := scene.Font{Size: th.FontSize * 0.9}
	type relInfo struct {
		rel *Relation
		le  *layout.Edge
	}
	var rels []relInfo
	for _, rel := range d.Relations {
		a, b := r.nodes[rel.From], r.nodes[rel.To]
		if a == nil || b == nil {
			continue
		}
		le := &layout.Edge{From: a, To: b}
		if rel.Label != "" {
			le.LabelW, le.LabelH = diagram.LabelSize(rel.Label, edgeFont)
		}
		lg.Edges = append(lg.Edges, le)
		rels = append(rels, relInfo{rel, le})
	}
	layout.Layout(lg)

	sc := diagram.NewScene(th)
	// namespaces (outer first)
	for _, id := range d.NSOrder {
		cl := r.clusters[id]
		rc := cl.Rect()
		sc.Add(scene.RectPath(rc.X, rc.Y, rc.W, rc.H, 6, scene.Style{
			Fill: th.ClusterBkg, Stroke: th.ClusterBorder, StrokeWidth: 1,
		}))
		sc.Add(scene.NewText(rc.X+rc.W/2, rc.Y+9, id, titleFont, th.TextColor, scene.AnchorMiddle, scene.VAlignTop))
	}
	// relations
	var labels []scene.Item
	cardFont := scene.Font{Size: th.FontSize * 0.85}
	for _, ri := range rels {
		rel, le := ri.rel, ri.le
		if len(le.Points) < 2 {
			continue
		}
		st := scene.Style{Stroke: th.LineColor, StrokeWidth: 1.5}
		if rel.Dashed {
			st.Dash = []float64{5, 4}
		}
		sc.Add(scene.Edge(le.Points, scene.EdgeOpts{
			Style: st, Start: endMarker(rel.FromEnd), End: endMarker(rel.ToEnd),
			Marker: scene.MarkerOpts{Size: 11, Hollow: th.Background, StrokeWidth: 1.4},
		})...)
		if rel.Label != "" {
			labels = append(labels, diagram.EdgeLabel(le.Label.X, le.Label.Y, rel.Label, edgeFont, th)...)
		}
		if rel.FromCard != "" {
			p := diagram.EndLabelPos(le.Points, true, 26, 12)
			labels = append(labels, scene.NewText(p.X, p.Y, rel.FromCard, cardFont, th.TextColor, scene.AnchorMiddle, scene.VAlignMiddle))
		}
		if rel.ToCard != "" {
			p := diagram.EndLabelPos(le.Points, false, 26, -12)
			labels = append(labels, scene.NewText(p.X, p.Y, rel.ToCard, cardFont, th.TextColor, scene.AnchorMiddle, scene.VAlignMiddle))
		}
	}
	// note connectors
	for _, ni := range notes {
		if ni.le != nil && len(ni.le.Points) >= 2 {
			sc.Add(scene.NewPath(scene.Style{Stroke: th.NoteBorder, StrokeWidth: 1.2, Dash: []float64{4, 3}}).Polyline(ni.le.Points...))
		}
	}
	sc.Add(labels...)
	// classes
	for _, id := range d.Order {
		r.drawClass(sc, id)
	}
	// notes
	for _, ni := range notes {
		rc := ni.ln.Rect()
		sc.Add(scene.RectPath(rc.X, rc.Y, rc.W, rc.H, 2, scene.Style{Fill: th.NoteBkg, Stroke: th.NoteBorder, StrokeWidth: 1, Shadow: true}))
		sc.Add(scene.NewText(rc.X+12, rc.Y+rc.H/2, ni.txt, noteFont, th.NoteText, scene.AnchorStart, scene.VAlignMiddle))
	}
	return sc
}

func (r *renderer) drawClass(sc *scene.Scene, id string) {
	b := r.boxes[id]
	n := r.nodes[id]
	x, y := n.X-b.w/2, n.Y-b.h/2
	st := b.style
	sc.Add(scene.RectPath(x, y, b.w, b.h, 3, st.Box(true)))
	div := scene.Style{Stroke: st.Stroke, StrokeWidth: 1}
	// header
	cy := y + padY
	for _, a := range b.c.Annotations {
		h := r.small.Size * 1.3
		sc.Add(scene.NewText(n.X, cy+h/2, "«"+a+"»", r.small, st.Text, scene.AnchorMiddle, scene.VAlignMiddle))
		cy += h
	}
	nameFont := r.bold
	if st.Italic {
		nameFont.Italic = true
	}
	for _, a := range b.c.Annotations {
		if a == "abstract" {
			nameFont.Italic = true
		}
	}
	sc.Add(scene.NewText(n.X, cy+r.lh/2, b.c.Label, nameFont, st.Text, scene.AnchorMiddle, scene.VAlignMiddle))
	yy := y + b.headerH
	sc.Add(scene.Line(x, yy, x+b.w, yy, div))
	r.drawMembers(sc, b.c.Attributes, x, yy, st)
	yy += b.attrH
	sc.Add(scene.Line(x, yy, x+b.w, yy, div))
	r.drawMembers(sc, b.c.Methods, x, yy, st)
}

func (r *renderer) drawMembers(sc *scene.Scene, ms []Member, x, y float64, st diagram.NodeStyle) {
	cy := y + padY
	for _, m := range ms {
		f := r.memberFont(m)
		ty := cy + r.lh/2
		sc.Add(scene.NewText(x+padX, ty, m.Text, f, st.Text, scene.AnchorStart, scene.VAlignMiddle))
		if m.Static {
			w := scene.MeasureText(m.Text, f)
			uy := ty + f.Size*0.45
			sc.Add(scene.Line(x+padX, uy, x+padX+w, uy, scene.Style{Stroke: st.Text, StrokeWidth: 1}))
		}
		cy += r.lh
	}
}
