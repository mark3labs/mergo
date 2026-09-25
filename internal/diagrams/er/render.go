package er

import (
	"math"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/layout"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

func init() {
	diagram.Register(diagram.Type{
		Name:   "er",
		Detect: diagram.Keyword("erDiagram"),
		Render: Render,
	})
}

const (
	cellPadX = 10.0
	rowPadY  = 6.0
)

type table struct {
	e       *Entity
	w, h    float64
	headerH float64
	rowH    float64
	cols    [4]float64 // type, name, keys, comment widths
	style   diagram.NodeStyle
}

// Render renders an ER diagram.
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
	d      *Diagram
	th     *theme.Theme
	cfg    *diagram.Config
	font   scene.Font
	bold   scene.Font
	small  scene.Font
	tables map[string]*table
	nodes  map[string]*layout.Node
}

func (r *renderer) measure(e *Entity) *table {
	t := &table{e: e}
	ns := diagram.NodeStyle{
		Fill: r.th.PrimaryColor, Stroke: r.th.PrimaryBorderColor, Text: r.th.PrimaryTextColor,
		StrokeWidth: 1.3, FontSize: r.th.FontSize,
	}
	for _, c := range e.Classes {
		if def, ok := r.d.ClassDefs[c]; ok {
			ns.Apply(def)
		}
	}
	ns.Apply(e.Style)
	ns.FixContrast()
	t.style = ns
	_, nh := scene.MeasureBlock(e.Label, r.bold, 0)
	t.headerH = nh + 2*9
	t.rowH = r.font.Size*1.25 + 2*rowPadY
	for _, a := range e.Attrs {
		cells := [4]string{a.Type, a.Name, joinKeys(a.Keys), a.Comment}
		for i, c := range cells {
			f := r.font
			if i == 2 {
				f = r.small
			}
			if c != "" {
				t.cols[i] = math.Max(t.cols[i], scene.MeasureText(c, f)+2*cellPadX)
			}
		}
	}
	tableW := t.cols[0] + t.cols[1] + t.cols[2] + t.cols[3]
	t.w = math.Max(math.Max(scene.MeasureText(e.Label, r.bold)+40, tableW), 120)
	if tableW > 0 && tableW < t.w {
		// distribute extra width to the name column
		t.cols[1] += t.w - tableW
	}
	t.h = t.headerH + float64(len(e.Attrs))*t.rowH
	if len(e.Attrs) == 0 {
		t.h = math.Max(t.headerH+14, 50)
		t.headerH = t.h
	}
	return t
}

func joinKeys(ks []string) string {
	out := ""
	for i, k := range ks {
		if i > 0 {
			out += ", "
		}
		out += k
	}
	return out
}

func cardMarker(c Card) scene.MarkerKind {
	switch c {
	case ZeroOrOne:
		return scene.MarkerERZeroOrOne
	case ZeroOrMore:
		return scene.MarkerERZeroOrMore
	case OneOrMore:
		return scene.MarkerEROneOrMore
	}
	return scene.MarkerERExactlyOne
}

func (r *renderer) render() *scene.Scene {
	d, th := r.d, r.th
	r.font = scene.Font{Size: th.FontSize * 0.9}
	r.bold = scene.Font{Size: th.FontSize, Bold: true}
	r.small = scene.Font{Size: th.FontSize * 0.78, Bold: true}
	r.tables = map[string]*table{}
	r.nodes = map[string]*layout.Node{}
	lg := &layout.Graph{
		Dir:     layout.ParseDirection(d.Dir),
		NodeSep: r.cfg.Float("er", "nodeSpacing", 60),
		RankSep: r.cfg.Float("er", "rankSpacing", 70),
	}
	for _, id := range d.Order {
		t := r.measure(d.Entities[id])
		r.tables[id] = t
		n := &layout.Node{ID: id, W: t.w, H: t.h}
		r.nodes[id] = n
		lg.Nodes = append(lg.Nodes, n)
	}
	labelFont := scene.Font{Size: th.FontSize * 0.85}
	edges := make([]*layout.Edge, len(d.Rels))
	for i, rel := range d.Rels {
		le := &layout.Edge{From: r.nodes[rel.From], To: r.nodes[rel.To]}
		if rel.Label != "" {
			le.LabelW, le.LabelH = diagram.LabelSize(rel.Label, labelFont)
		}
		edges[i] = le
		lg.Edges = append(lg.Edges, le)
	}
	layout.Layout(lg)

	sc := diagram.NewScene(th)
	var labels []scene.Item
	for i, rel := range d.Rels {
		le := edges[i]
		if len(le.Points) < 2 {
			continue
		}
		st := scene.Style{Stroke: th.LineColor, StrokeWidth: 1.4}
		if !rel.Identifying {
			st.Dash = []float64{6, 4}
		}
		sc.Add(scene.Edge(le.Points, scene.EdgeOpts{
			Style: st, Start: cardMarker(rel.FromCard), End: cardMarker(rel.ToCard),
			Marker: scene.MarkerOpts{Size: 11, Hollow: th.Background, StrokeWidth: 1.4},
		})...)
		labels = append(labels, diagram.EdgeLabel(le.Label.X, le.Label.Y, rel.Label, labelFont, th)...)
	}
	sc.Add(labels...)
	for _, id := range d.Order {
		r.drawTable(sc, id)
	}
	return sc
}

func (r *renderer) drawTable(sc *scene.Scene, id string) {
	th := r.th
	t := r.tables[id]
	n := r.nodes[id]
	x, y := n.X-t.w/2, n.Y-t.h/2
	st := t.style
	// shadow + outline
	sc.Add(scene.RectPath(x, y, t.w, t.h, 4, scene.Style{Fill: st.Fill, Shadow: true}))
	// header
	f := r.bold
	f.Italic = st.Italic
	sc.Add(scene.NewText(n.X, y+t.headerH/2, t.e.Label, f, st.Text, scene.AnchorMiddle, scene.VAlignMiddle))
	// rows
	odd := theme.Mix(th.Background, st.Fill, 0.12)
	even := theme.Mix(th.Background, st.Fill, 0.35)
	textCol := th.TextColor
	if theme.Luminance(odd) < 0.4 != (theme.Luminance(textCol) < 0.4) {
		// ok: contrasting already
	} else {
		textCol = theme.ContrastText(odd)
	}
	ry := y + t.headerH
	for i, a := range t.e.Attrs {
		fill := odd
		if i%2 == 1 {
			fill = even
		}
		last := i == len(t.e.Attrs)-1
		if last {
			p := roundedBottom(x, ry, t.w, t.rowH, 4)
			p.Style = scene.Style{Fill: fill}
			sc.Add(p)
		} else {
			sc.Add(scene.RectPath(x, ry, t.w, t.rowH, 0, scene.Style{Fill: fill}))
		}
		cx := x
		cells := [4]string{a.Type, a.Name, joinKeys(a.Keys), a.Comment}
		for ci, c := range cells {
			if c != "" {
				ff := r.font
				col := textCol
				if ci == 2 {
					ff = r.small
					col = theme.Mix(textCol, st.Stroke, 0.35)
				}
				if ci == 3 {
					ff.Italic = true
				}
				sc.Add(scene.NewText(cx+cellPadX, ry+t.rowH/2, c, ff, col, scene.AnchorStart, scene.VAlignMiddle))
			}
			cx += t.cols[ci]
		}
		ry += t.rowH
	}
	// grid lines
	grid := scene.Style{Stroke: theme.WithAlpha(st.Stroke, 110), StrokeWidth: 1}
	if len(t.e.Attrs) > 0 {
		cx := x
		for ci := 0; ci < 3; ci++ {
			cx += t.cols[ci]
			if t.cols[ci] > 0 && cx < x+t.w-1 {
				sc.Add(scene.Line(cx, y+t.headerH, cx, y+t.h, grid))
			}
		}
		sc.Add(scene.Line(x, y+t.headerH, x+t.w, y+t.headerH, scene.Style{Stroke: st.Stroke, StrokeWidth: 1.2}))
	}
	sc.Add(scene.RectPath(x, y, t.w, t.h, 4, scene.Style{Stroke: st.Stroke, StrokeWidth: st.StrokeWidth, Dash: st.Dash}))
}

func roundedBottom(x, y, w, h, rad float64) *scene.Path {
	const k = 0.5522847498
	p := scene.NewPath(scene.Style{})
	p.MoveTo(x, y).LineTo(x+w, y).LineTo(x+w, y+h-rad)
	p.CubicTo(x+w, y+h-rad+rad*k, x+w-rad+rad*k, y+h, x+w-rad, y+h)
	p.LineTo(x+rad, y+h)
	p.CubicTo(x+rad-rad*k, y+h, x, y+h-rad+rad*k, x, y+h-rad)
	return p.Close()
}
