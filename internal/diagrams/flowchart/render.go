// Package flowchart implements Mermaid flowcharts (`flowchart` / `graph`).
package flowchart

import (
	"image/color"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/layout"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

func init() {
	diagram.Register(diagram.Type{
		Name:   "flowchart",
		Detect: diagram.Keyword("flowchart", "graph", "flowchart-elk"),
		Render: Render,
	})
}

// nodeStyle is the resolved visual style of a node or cluster.
type nodeStyle struct {
	fill, stroke, text color.RGBA
	strokeW          float64
	dash             []float64
	bold, italic     bool
	fontSize         float64
	textSet          bool
	fillSet          bool
}

func applyCSS(ns *nodeStyle, css map[string]string) {
	for k, v := range css {
		switch k {
		case "fill", "background", "background-color":
			if c, err := theme.ParseColor(v); err == nil {
				ns.fill = c
				ns.fillSet = true
			}
		case "stroke", "border-color":
			if c, err := theme.ParseColor(v); err == nil {
				ns.stroke = c
			}
		case "color":
			if c, err := theme.ParseColor(v); err == nil {
				ns.text = c
				ns.textSet = true
			}
		case "stroke-width":
			if f, ok := parsePx(v); ok {
				ns.strokeW = f
			}
		case "stroke-dasharray":
			ns.dash = parseDash(v)
		case "font-weight":
			ns.bold = v == "bold" || v == "bolder" || v == "600" || v == "700" || v == "800" || v == "900"
		case "font-style":
			ns.italic = v == "italic" || v == "oblique"
		case "font-size":
			if f, ok := parsePx(v); ok && f > 4 && f < 100 {
				ns.fontSize = f
			}
		}
	}
}

func parsePx(v string) (float64, bool) {
	v = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(v), "px"))
	f, err := strconv.ParseFloat(v, 64)
	return f, err == nil
}

func parseDash(v string) []float64 {
	var out []float64
	for _, f := range strings.FieldsFunc(v, func(r rune) bool { return r == ' ' || r == ',' }) {
		if x, ok := parsePx(f); ok && x > 0 {
			out = append(out, x)
		}
	}
	if len(out) == 1 {
		out = append(out, out[0])
	}
	return out
}

// Render renders a flowchart.
func Render(src string, cfg *diagram.Config) (*scene.Scene, error) {
	g, err := Parse(src)
	if err != nil {
		return nil, err
	}
	th := cfg.Theme
	r := &renderer{g: g, th: th, cfg: cfg}
	return r.render(), nil
}

type renderer struct {
	g   *Graph
	th  *theme.Theme
	cfg *diagram.Config

	lnodes   map[string]*layout.Node
	clusters map[string]*layout.Node
	geoms    map[string]geom
	styles   map[string]nodeStyle
	labels   map[string]string
	fonts    map[string]scene.Font
}

func (r *renderer) baseStyle() nodeStyle {
	return nodeStyle{
		fill:     r.th.PrimaryColor,
		stroke:   r.th.PrimaryBorderColor,
		text:     r.th.PrimaryTextColor,
		strokeW:  1.3,
		fontSize: r.th.FontSize,
	}
}

func (r *renderer) nodeStyleFor(n *Node) nodeStyle {
	ns := r.baseStyle()
	if d, ok := r.g.ClassDefs["default"]; ok {
		applyCSS(&ns, d)
	}
	for _, c := range n.Classes {
		if d, ok := r.g.ClassDefs[c]; ok {
			applyCSS(&ns, d)
		}
	}
	applyCSS(&ns, n.Style)
	if ns.fillSet && !ns.textSet {
		// keep labels readable on custom fills
		if math.Abs(theme.Luminance(ns.fill)-theme.Luminance(ns.text)) < 0.3 {
			ns.text = theme.ContrastText(ns.fill)
		}
	}
	return ns
}

func (r *renderer) clusterStyleFor(sg *Subgraph) nodeStyle {
	ns := nodeStyle{
		fill:     r.th.ClusterBkg,
		stroke:   r.th.ClusterBorder,
		text:     r.th.TextColor,
		strokeW:  1,
		fontSize: r.th.FontSize,
	}
	for _, c := range sg.Classes {
		if d, ok := r.g.ClassDefs[c]; ok {
			applyCSS(&ns, d)
		}
	}
	applyCSS(&ns, sg.Style)
	if ns.fillSet && !ns.textSet && math.Abs(theme.Luminance(ns.fill)-theme.Luminance(ns.text)) < 0.3 {
		ns.text = theme.ContrastText(ns.fill)
	}
	return ns
}

func (r *renderer) render() *scene.Scene {
	g, th := r.g, r.th
	wrapW := r.cfg.Float("flowchart", "wrappingWidth", 200)
	r.lnodes = map[string]*layout.Node{}
	r.clusters = map[string]*layout.Node{}
	r.geoms = map[string]geom{}
	r.styles = map[string]nodeStyle{}
	r.labels = map[string]string{}
	r.fonts = map[string]scene.Font{}

	// Nodes
	for _, id := range g.NodeOrder {
		n := g.Nodes[id]
		ns := r.nodeStyleFor(n)
		f := scene.Font{Size: ns.fontSize, Bold: ns.bold, Italic: ns.italic}
		label := n.Label
		hasLabel := label != ""
		if isIcon(n.Shape) && !n.labelSetText() {
			label, hasLabel = "", false
		}
		if hasLabel && wrapW > 0 {
			label = scene.WrapText(label, f, wrapW)
		}
		tw, tht := 0.0, 0.0
		if hasLabel {
			tw, tht = scene.MeasureBlock(label, f, 0)
		}
		gm := shapeGeom(n.Shape, tw, tht, hasLabel)
		ln := &layout.Node{ID: id, W: gm.w, H: gm.h, Data: n}
		ln.Clip = clipFor(gm)
		r.lnodes[id] = ln
		r.geoms[id] = gm
		r.styles[id] = ns
		r.labels[id] = label
		r.fonts[id] = f
	}

	// Clusters
	titleFont := scene.Font{Size: th.FontSize}
	for _, id := range g.SubOrder {
		sg := g.Subgraphs[id]
		tw, tht := scene.MeasureBlock(sg.Title, titleFont, 0)
		ln := &layout.Node{ID: "cluster:" + id, Pad: 16, PadTop: tht + 12, MinW: tw + 32, Data: sg}
		if sg.Dir != "" {
			d := layout.ParseDirection(sg.Dir)
			ln.Dir = &d
		}
		r.clusters[id] = ln
	}
	for _, id := range g.SubOrder {
		sg := g.Subgraphs[id]
		ln := r.clusters[id]
		for _, sid := range sg.Subs {
			ln.Children = append(ln.Children, r.clusters[sid])
		}
		for _, nid := range sg.Children {
			if n, ok := r.lnodes[nid]; ok {
				ln.Children = append(ln.Children, n)
			}
		}
		if len(ln.Children) == 0 {
			// empty subgraph: a box just big enough for the title
			ln.W = ln.MinW
			ln.H = ln.PadTop + 30
		}
	}
	var top []*layout.Node
	for _, id := range g.NodeOrder {
		if g.Nodes[id].Subgraph == "" {
			top = append(top, r.lnodes[id])
		}
	}
	for _, id := range g.SubOrder {
		if g.Subgraphs[id].Parent == "" {
			top = append(top, r.clusters[id])
		}
	}
	// Keep a stable, source-ordered top-level list (nodes and clusters
	// interleaved by first appearance works best for crossing reduction).
	sort.SliceStable(top, func(i, j int) bool { return r.firstSeen(top[i]) < r.firstSeen(top[j]) })

	// Edges
	edgeFont := scene.Font{Size: th.FontSize * 0.95}
	lg := &layout.Graph{
		Dir:     layout.ParseDirection(g.Dir),
		NodeSep: r.cfg.Float("flowchart", "nodeSpacing", 50),
		RankSep: r.cfg.Float("flowchart", "rankSpacing", 50),
		Nodes:   top,
	}
	type edgeInfo struct {
		e     *Edge
		le    *layout.Edge
		label string
	}
	var edges []edgeInfo
	for _, e := range g.Edges {
		from, to := r.endpoint(e.From), r.endpoint(e.To)
		if from == nil || to == nil {
			continue
		}
		le := &layout.Edge{From: from, To: to, MinLen: e.Length, Data: e}
		label := e.Label
		if label != "" {
			if wrapW > 0 {
				label = scene.WrapText(label, edgeFont, wrapW)
			}
			w, h := scene.MeasureBlock(label, edgeFont, 0)
			le.LabelW, le.LabelH = w+12, h+6
		}
		if e.Kind == LinkInvisible {
			le.Weight = 0.5
		}
		lg.Edges = append(lg.Edges, le)
		edges = append(edges, edgeInfo{e, le, label})
	}
	layout.Layout(lg)

	sc := diagram.NewScene(th)

	// Clusters, outermost first.
	order := append([]string(nil), g.SubOrder...)
	sort.SliceStable(order, func(i, j int) bool { return r.depth(order[i]) < r.depth(order[j]) })
	for _, id := range order {
		sg := g.Subgraphs[id]
		ln := r.clusters[id]
		cs := r.clusterStyleFor(sg)
		rc := ln.Rect()
		st := scene.Style{Fill: cs.fill, Stroke: cs.stroke, StrokeWidth: cs.strokeW, Dash: cs.dash}
		sc.Add(scene.RectPath(rc.X, rc.Y, rc.W, rc.H, 4, st))
		if sg.Title != "" {
			f := scene.Font{Size: cs.fontSize, Bold: cs.bold, Italic: cs.italic}
			sc.Add(scene.NewText(rc.X+rc.W/2, rc.Y+8, sg.Title, f, cs.text, scene.AnchorMiddle, scene.VAlignTop))
		}
	}

	// Edges
	curve := curveKind(r.cfg.String("flowchart", "curve", "basis"))
	var labelItems []scene.Item
	for _, ei := range edges {
		e, le := ei.e, ei.le
		if e.Kind == LinkInvisible || len(le.Points) < 2 {
			continue
		}
		st := scene.Style{Stroke: th.LineColor, StrokeWidth: 1.8, RoundCaps: false}
		textCol := th.TextColor
		switch e.Kind {
		case LinkThick:
			st.StrokeWidth = 3.5
		case LinkDotted:
			st.Dash = []float64{3, 4}
		}
		css := map[string]string{}
		for k, v := range g.LinkStyles[-1] {
			css[k] = v
		}
		for k, v := range g.LinkStyles[e.Index] {
			css[k] = v
		}
		for k, v := range css {
			switch k {
			case "stroke":
				if c, err := theme.ParseColor(v); err == nil {
					st.Stroke = c
				}
			case "stroke-width":
				if f, ok := parsePx(v); ok {
					st.StrokeWidth = f
				}
			case "stroke-dasharray":
				st.Dash = parseDash(v)
			case "color":
				if c, err := theme.ParseColor(v); err == nil {
					textCol = c
				}
			}
		}
		opts := scene.EdgeOpts{
			Style:  st,
			Curve:  curve,
			Start:  markerKind(e.Start),
			End:    markerKind(e.End),
			Marker: scene.MarkerOpts{Hollow: th.Background, Size: 8 + st.StrokeWidth*1.2},
		}
		sc.Add(scene.Edge(le.Points, opts)...)
		if ei.label != "" {
			w, h := le.LabelW, le.LabelH
			bg := th.EdgeLabelBg
			if bg.A == 255 {
				bg.A = 235
			}
			labelItems = append(labelItems,
				scene.RectPath(le.Label.X-w/2, le.Label.Y-h/2, w, h, 3, scene.Style{Fill: bg}),
				scene.NewText(le.Label.X, le.Label.Y, ei.label, edgeFont, textCol, scene.AnchorMiddle, scene.VAlignMiddle),
			)
		}
	}
	sc.Add(labelItems...)

	// Nodes
	for _, id := range g.NodeOrder {
		sc.Add(r.drawNode(id)...)
	}
	return diagram.Finish(sc, th, r.cfg.Title)
}

func (n *Node) labelSetText() bool { return n.labelSet && n.Label != "" && n.Label != n.ID }

func isIcon(s Shape) bool {
	switch s {
	case "f-circ", "sm-circ", "fr-circ", "fork", "hourglass", "bolt", "cross-circ":
		return true
	}
	return false
}

func (r *renderer) firstSeen(n *layout.Node) int {
	switch d := n.Data.(type) {
	case *Node:
		return d.order * 2
	case *Subgraph:
		// place a cluster where its first member appears
		best := math.MaxInt32
		var walk func(sg *Subgraph)
		walk = func(sg *Subgraph) {
			for _, c := range sg.Children {
				if nn, ok := r.g.Nodes[c]; ok && nn.order*2 < best {
					best = nn.order * 2
				}
			}
			for _, s := range sg.Subs {
				walk(r.g.Subgraphs[s])
			}
		}
		walk(d)
		if best == math.MaxInt32 {
			return d.order*2 + 1
		}
		return best + 1
	}
	return 0
}

func (r *renderer) depth(id string) int {
	d := 0
	for sg := r.g.Subgraphs[id]; sg != nil && sg.Parent != ""; sg = r.g.Subgraphs[sg.Parent] {
		d++
		if d > 100 {
			break
		}
	}
	return d
}

func (r *renderer) endpoint(id string) *layout.Node {
	if n, ok := r.lnodes[id]; ok {
		return n
	}
	if c, ok := r.clusters[id]; ok {
		return c
	}
	return nil
}

func curveKind(s string) scene.CurveKind {
	switch strings.ToLower(s) {
	case "linear":
		return scene.CurveLinear
	case "step", "stepbefore", "stepafter":
		return scene.CurveRounded
	case "cardinal", "catmullrom", "monotonex", "monotoney", "natural", "bumpx", "bumpy":
		return scene.CurveCatmull
	}
	return scene.CurveBasis
}

func markerKind(m Marker) scene.MarkerKind {
	switch m {
	case MarkerArrow:
		return scene.MarkerArrow
	case MarkerCircle:
		return scene.MarkerCircle
	case MarkerCross:
		return scene.MarkerCross
	}
	return scene.MarkerNone
}

// clipFor builds a clip function from the shape outline.
func clipFor(gm geom) func(n *layout.Node, p scene.Point) scene.Point {
	if gm.outline == nil || gm.noBox {
		return nil
	}
	// outline in coordinates relative to the node center
	x := gm.sx - gm.w/2
	y := gm.sy - gm.h/2
	poly := gm.outline(x, y, gm.sw, gm.sh).Flatten(8)
	if len(poly) < 3 {
		return nil
	}
	return layout.ClipPolygon(poly)
}

func (r *renderer) drawNode(id string) []scene.Item {
	ln := r.lnodes[id]
	gm := r.geoms[id]
	ns := r.styles[id]
	x0, y0 := ln.X-ln.W/2, ln.Y-ln.H/2
	sx, sy := x0+gm.sx, y0+gm.sy
	st := scene.Style{Fill: ns.fill, Stroke: ns.stroke, StrokeWidth: ns.strokeW, Dash: ns.dash, Shadow: true}
	if gm.filledWithStroke {
		st.Fill = ns.stroke
		if ns.fillSet {
			st.Fill = ns.fill
		}
	}
	var items []scene.Item
	if gm.under != nil {
		items = append(items, gm.under(sx, sy, gm.sw, gm.sh, st)...)
	}
	if gm.outline != nil && !gm.noBox {
		p := gm.outline(sx, sy, gm.sw, gm.sh)
		p.Style = st
		items = append(items, p)
	}
	if gm.deco != nil {
		items = append(items, gm.deco(sx, sy, gm.sw, gm.sh, st)...)
	}
	if label := r.labels[id]; label != "" {
		items = append(items, scene.NewText(ln.X+gm.tx, ln.Y+gm.ty, label, r.fonts[id], ns.text, scene.AnchorMiddle, scene.VAlignMiddle))
	}
	return items
}
