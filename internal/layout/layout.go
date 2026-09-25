// Package layout implements a layered (Sugiyama style) graph layout with
// support for nested clusters. It is used by flowcharts, class, state and
// ER diagrams.
//
// The algorithm follows the classic pipeline used by dagre (which Mermaid
// uses): cycle removal, rank assignment, edge normalization with dummy
// nodes (labels become dummy nodes on the middle rank), crossing reduction
// with barycenter sweeps plus transposition, and coordinate assignment. For
// coordinates we use an iterative weighted-median placement where each rank
// is solved exactly with isotonic regression (pool adjacent violators), which
// yields balanced and straight layouts.
package layout

import (
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/mark3labs/mergo/internal/scene"
)

// Direction is the main flow direction.
type Direction int

const (
	TB Direction = iota
	BT
	LR
	RL
)

// ParseDirection parses TB/TD/BT/LR/RL (defaults to TB).
func ParseDirection(s string) Direction {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "BT":
		return BT
	case "LR":
		return LR
	case "RL":
		return RL
	}
	return TB
}

// Horizontal reports whether the direction is LR or RL.
func (d Direction) Horizontal() bool { return d == LR || d == RL }

// Point is a 2D point.
type Point = scene.Point

// Node is a node in the graph. X and Y are the center coordinates after
// layout. A node with Children is a cluster; its size is computed by the
// layout from its children (plus padding).
type Node struct {
	ID   string
	W, H float64
	X, Y float64

	// Children makes this node a cluster.
	Children []*Node
	// Dir is the direction used inside a cluster (nil = inherit).
	Dir *Direction
	// Pad is the inner padding of a cluster.
	Pad float64
	// PadTop is extra top padding of a cluster (room for the title).
	PadTop float64
	// MinW is the minimum width of a cluster (e.g. to fit its title).
	MinW float64

	// Clip returns the point on the node boundary on the segment from the
	// node center towards p. Nil uses the bounding rectangle.
	Clip func(n *Node, p Point) Point

	// Data is free for the caller.
	Data any

	// internal
	parent *Node
}

// IsCluster reports whether the node has children.
func (n *Node) IsCluster() bool { return len(n.Children) > 0 }

// Rect returns the node bounding rectangle.
func (n *Node) Rect() scene.Rect { return scene.Rect{X: n.X - n.W/2, Y: n.Y - n.H/2, W: n.W, H: n.H} }

// Edge connects two nodes.
type Edge struct {
	From, To *Node
	// LabelW/LabelH is the size of the label (0 = no label).
	LabelW, LabelH float64
	// MinLen is the minimum rank distance (default 1).
	MinLen int
	// Weight controls how hard the layout tries to keep the edge short and
	// straight (default 1).
	Weight float64

	// Output: Points is the route from From's boundary to To's boundary.
	Points []Point
	// Output: Label is the center of the label.
	Label Point

	Data any
}

// Graph is the layout input.
type Graph struct {
	Dir     Direction
	NodeSep float64 // horizontal gap between nodes of one rank (default 50)
	RankSep float64 // gap between ranks (default 50)
	EdgeSep float64 // gap between edge dummies (default 20)
	Nodes   []*Node // top level nodes
	Edges   []*Edge

	// Width/Height of the laid out graph (output).
	Width, Height float64
}

// Layout computes positions for all nodes and routes for all edges. The
// resulting drawing has its top-left corner at (0, 0).
func Layout(g *Graph) {
	if g.NodeSep == 0 {
		g.NodeSep = 50
	}
	if g.RankSep == 0 {
		g.RankSep = 50
	}
	if g.EdgeSep == 0 {
		g.EdgeSep = 20
	}
	root := &Node{ID: "\x00root", Children: g.Nodes, Dir: &g.Dir}
	setParents(root)

	// Assign every edge to the level (cluster) of its endpoints' lowest
	// common ancestor.
	levelEdges := map[*Node][]*levelEdge{}
	var unrouted []*Edge
	for _, e := range g.Edges {
		if e.From == nil || e.To == nil {
			continue
		}
		if e.MinLen <= 0 {
			e.MinLen = 1
		}
		if e.Weight <= 0 {
			e.Weight = 1
		}
		lca, u, v := liftEdge(root, e.From, e.To)
		if lca == nil {
			unrouted = append(unrouted, e)
			continue
		}
		levelEdges[lca] = append(levelEdges[lca], &levelEdge{e: e, u: u, v: v})
	}

	lay := &layouter{g: g, levelEdges: levelEdges}
	lay.layoutCluster(root, g.Dir)

	// Translate everything so that the root content starts at 0,0.
	lay.translateTree(root, -(root.X - root.W/2), -(root.Y - root.H/2))
	g.Width, g.Height = root.W, root.H

	// Finalize edge endpoints now that absolute positions are known.
	for _, les := range levelEdges {
		for _, le := range les {
			finishEdge(le)
		}
	}
	for _, e := range unrouted {
		a := clipNode(e.From, Point{X: e.To.X, Y: e.To.Y})
		b := clipNode(e.To, Point{X: e.From.X, Y: e.From.Y})
		e.Points = []Point{a, b}
		e.Label = a.Add(b).Mul(0.5)
	}
	for _, n := range g.Nodes {
		n.parent = nil
	}
}

type levelEdge struct {
	e    *Edge
	u, v *Node // endpoints lifted to the level
	// route through the level in level coordinates (dummy points)
	mid      []Point
	label    Point
	hasLabel bool
}

func setParents(n *Node) {
	for _, c := range n.Children {
		c.parent = n
		setParents(c)
	}
}

func ancestors(n *Node) []*Node {
	var out []*Node
	for p := n; p != nil; p = p.parent {
		out = append(out, p)
	}
	return out
}

// liftEdge finds the lowest common ancestor cluster of a and b, and the
// children of that cluster containing a and b.
func liftEdge(root, a, b *Node) (lca, u, v *Node) {
	if a == b {
		return a.parent, a, a
	}
	aa := ancestors(a)
	bb := ancestors(b)
	inB := map[*Node]int{}
	for i, n := range bb {
		inB[n] = i
	}
	for i, n := range aa {
		if j, ok := inB[n]; ok {
			if i == 0 || j == 0 {
				// One endpoint is an ancestor of the other; such edges are
				// drawn as straight lines after layout.
				return nil, nil, nil
			}
			return n, aa[i-1], bb[j-1]
		}
	}
	return nil, nil, nil
}

type layouter struct {
	g          *Graph
	levelEdges map[*Node][]*levelEdge
}

// layoutCluster lays out the children of c (recursively) and sets c.W/c.H.
// Children positions are relative to c's own center being at c.X,c.Y after
// the call (initially the content is placed with top-left at 0,0 and c's
// center is set accordingly).
func (l *layouter) layoutCluster(c *Node, inherited Direction) {
	dir := inherited
	if c.Dir != nil {
		dir = *c.Dir
	}
	for _, ch := range c.Children {
		if ch.IsCluster() {
			l.layoutCluster(ch, dir)
		}
	}
	w, h := l.layoutLevel(c.Children, l.levelEdges[c], dir)
	pad := c.Pad
	isRoot := c.parent == nil
	if isRoot {
		pad = 0
	}
	cw := w + 2*pad
	chh := h + 2*pad + c.PadTop
	if cw < c.MinW {
		// center content horizontally
		l.translateLevel(c, (c.MinW-cw)/2, 0)
		cw = c.MinW
	}
	// content currently spans [0,w]x[0,h]; shift by padding
	l.translateLevel(c, pad, pad+c.PadTop)
	c.W, c.H = cw, chh
	c.X, c.Y = cw/2, chh/2
}

// translateLevel translates the children subtree of c and the routes of
// the edges owned by c (and its descendants).
func (l *layouter) translateLevel(c *Node, dx, dy float64) {
	for _, ch := range c.Children {
		l.translateTree(ch, dx, dy)
	}
	for _, le := range l.levelEdges[c] {
		for i := range le.mid {
			le.mid[i].X += dx
			le.mid[i].Y += dy
		}
		le.label.X += dx
		le.label.Y += dy
	}
}

func (l *layouter) translateTree(n *Node, dx, dy float64) {
	n.X += dx
	n.Y += dy
	l.translateLevel(n, dx, dy)
}

// ---------------------------------------------------------------------------
// Single level layered layout

type lnode struct {
	n       *Node // nil for dummies
	w, h    float64
	rank    int
	order   int
	x, y    float64
	dummy   bool
	isLabel bool
	in, out []*ledge
	idx     int
}

type ledge struct {
	from, to *lnode
	weight   float64
}

type chain struct {
	le       *levelEdge
	nodes    []*lnode // dummy nodes from source rank+1 to target rank-1
	reversed bool
	labelAt  int // index into nodes of label dummy (-1 if none)
}

func (l *layouter) layoutLevel(nodes []*Node, edges []*levelEdge, dir Direction) (float64, float64) {
	if len(nodes) == 0 {
		return 0, 0
	}
	horiz := dir.Horizontal()
	nodeSep, rankSep, edgeSep := l.g.NodeSep, l.g.RankSep, l.g.EdgeSep

	hasLabels := false
	for _, e := range edges {
		if e.e.LabelW > 0 && e.u != e.v {
			hasLabels = true
			break
		}
	}
	if hasLabels {
		rankSep /= 2
	}

	ln := make([]*lnode, len(nodes))
	index := map[*Node]*lnode{}
	for i, n := range nodes {
		w, h := n.W, n.H
		if horiz {
			w, h = h, w
		}
		ln[i] = &lnode{n: n, w: w, h: h, idx: i}
		index[n] = ln[i]
	}

	// Separate self loops.
	type redge struct {
		le       *levelEdge
		u, v     *lnode
		minlen   int
		weight   float64
		reversed bool
	}
	var real []*redge
	var loops []*levelEdge
	for _, e := range edges {
		u, v := index[e.u], index[e.v]
		if u == nil || v == nil {
			continue
		}
		if u == v {
			loops = append(loops, e)
			continue
		}
		ml := e.e.MinLen
		if hasLabels {
			ml *= 2
		}
		real = append(real, &redge{le: e, u: u, v: v, minlen: ml, weight: e.e.Weight})
	}

	// 1. Cycle removal via DFS.
	adj := map[*lnode][]*redge{}
	for _, e := range real {
		adj[e.u] = append(adj[e.u], e)
	}
	state := map[*lnode]int{}
	var dfs func(n *lnode)
	dfs = func(n *lnode) {
		state[n] = 1
		for _, e := range adj[n] {
			if e.reversed {
				continue
			}
			switch state[e.v] {
			case 0:
				dfs(e.v)
			case 1:
				e.reversed = true
			}
		}
		state[n] = 2
	}
	for _, n := range ln {
		if state[n] == 0 {
			dfs(n)
		}
	}
	src := func(e *redge) *lnode {
		if e.reversed {
			return e.v
		}
		return e.u
	}
	dst := func(e *redge) *lnode {
		if e.reversed {
			return e.u
		}
		return e.v
	}

	// 2. Ranking: longest path from sources, then pull nodes towards their
	// successors when that shortens edges.
	outE := map[*lnode][]*redge{}
	inE := map[*lnode][]*redge{}
	for _, e := range real {
		outE[src(e)] = append(outE[src(e)], e)
		inE[dst(e)] = append(inE[dst(e)], e)
	}
	// topological order (Kahn)
	indeg := map[*lnode]int{}
	for _, e := range real {
		indeg[dst(e)]++
	}
	var topo []*lnode
	var queue []*lnode
	for _, n := range ln {
		if indeg[n] == 0 {
			queue = append(queue, n)
		}
	}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		topo = append(topo, n)
		for _, e := range outE[n] {
			d := dst(e)
			indeg[d]--
			if indeg[d] == 0 {
				queue = append(queue, d)
			}
		}
	}
	if len(topo) != len(ln) { // should not happen; fall back
		topo = ln
	}
	for _, n := range topo {
		r := 0
		for _, e := range inE[n] {
			if v := src(e).rank + e.minlen; v > r {
				r = v
			}
		}
		n.rank = r
	}
	// Pull nodes down toward successors if they have more outgoing than
	// incoming weight (reduces total edge length; sources get tight).
	for range 4 {
		changed := false
		for _, n := range slices.Backward(topo) {

			if len(outE[n]) == 0 {
				continue
			}
			var win, wout float64
			for _, e := range inE[n] {
				win += e.weight
			}
			for _, e := range outE[n] {
				wout += e.weight
			}
			if wout <= win && len(inE[n]) > 0 {
				continue
			}
			maxR := math.MaxInt32
			for _, e := range outE[n] {
				if v := dst(e).rank - e.minlen; v < maxR {
					maxR = v
				}
			}
			if maxR > n.rank {
				n.rank = maxR
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	// normalize ranks
	minR := math.MaxInt32
	for _, n := range ln {
		minR = min(minR, n.rank)
	}
	maxRank := 0
	for _, n := range ln {
		n.rank -= minR
		maxRank = max(maxRank, n.rank)
	}

	// 3. Normalize long edges with dummy nodes.
	all := append([]*lnode(nil), ln...)
	var chains []*chain
	addEdge := func(a, b *lnode, w float64) {
		le := &ledge{from: a, to: b, weight: w}
		a.out = append(a.out, le)
		b.in = append(b.in, le)
	}
	for _, e := range real {
		s, t := src(e), dst(e)
		ch := &chain{le: e.le, reversed: e.reversed, labelAt: -1}
		chains = append(chains, ch)
		prev := s
		labelRank := -1
		if e.le.e.LabelW > 0 && hasLabels {
			labelRank = s.rank + (t.rank-s.rank)/2
			if labelRank == s.rank {
				labelRank = s.rank + 1
			}
		}
		for r := s.rank + 1; r < t.rank; r++ {
			d := &lnode{dummy: true, rank: r, w: edgeSep / 2, h: 0, idx: len(all)}
			if r == labelRank {
				lw, lh := e.le.e.LabelW, e.le.e.LabelH
				if horiz {
					lw, lh = lh, lw
				}
				d.w, d.h = lw, lh
				d.isLabel = true
				ch.labelAt = len(ch.nodes)
			}
			all = append(all, d)
			ch.nodes = append(ch.nodes, d)
			w := 2.0
			if prev.dummy {
				w = 8
			}
			addEdge(prev, d, w*e.weight)
			prev = d
		}
		w := 1.0
		if prev.dummy {
			w = 2
		}
		addEdge(prev, t, w*e.weight)
	}

	// 4. Ordering.
	layers := make([][]*lnode, maxRank+1)
	// initial order: DFS from real nodes in input order
	visited := map[*lnode]bool{}
	var visit func(n *lnode)
	visit = func(n *lnode) {
		if visited[n] {
			return
		}
		visited[n] = true
		layers[n.rank] = append(layers[n.rank], n)
		for _, e := range n.out {
			visit(e.to)
		}
	}
	// start with sources (in input order), then everything else
	for _, n := range ln {
		if len(n.in) == 0 {
			visit(n)
		}
	}
	for _, n := range all {
		visit(n)
	}
	for _, layer := range layers {
		for i, n := range layer {
			n.order = i
		}
	}
	orderLayers(layers)

	// 5. Coordinates.
	sepFn := func(a, b *lnode) float64 {
		switch {
		case a.dummy && b.dummy:
			return edgeSep
		case a.dummy || b.dummy:
			return (nodeSep + edgeSep) / 2
		}
		return nodeSep
	}
	assignX(layers, sepFn)

	// y by rank
	y := 0.0
	for _, layer := range layers {
		maxH := 0.0
		for _, n := range layer {
			maxH = math.Max(maxH, n.h)
		}
		for _, n := range layer {
			n.y = y + maxH/2
		}
		y += maxH + rankSep
	}

	// bbox
	minX, maxX := math.Inf(1), math.Inf(-1)
	minY, maxY := math.Inf(1), math.Inf(-1)
	for _, n := range all {
		if n.dummy && !n.isLabel {
			minX = math.Min(minX, n.x)
			maxX = math.Max(maxX, n.x)
			continue
		}
		minX = math.Min(minX, n.x-n.w/2)
		maxX = math.Max(maxX, n.x+n.w/2)
		minY = math.Min(minY, n.y-n.h/2)
		maxY = math.Max(maxY, n.y+n.h/2)
	}
	// self loops stick out to the right (TB) - reserve room
	loopExtra := map[*lnode]float64{}
	for _, e := range loops {
		n := index[e.u]
		extra := 30.0
		if e.e.LabelW > 0 {
			lw := e.e.LabelW
			if horiz {
				lw = e.e.LabelH
			}
			extra += lw + 6
		}
		loopExtra[n] = math.Max(loopExtra[n], extra)
		maxX = math.Max(maxX, n.x+n.w/2+loopExtra[n])
	}
	W, H := maxX-minX, maxY-minY

	// map back to real orientation
	tf := func(x, y float64) Point {
		x -= minX
		y -= minY
		switch dir {
		case BT:
			return Point{X: x, Y: H - y}
		case LR:
			return Point{X: y, Y: x}
		case RL:
			return Point{X: H - y, Y: x}
		}
		return Point{X: x, Y: y}
	}
	for _, n := range ln {
		p := tf(n.x, n.y)
		dx, dy := p.X-n.n.X, p.Y-n.n.Y
		if n.n.IsCluster() {
			l.translateTree(n.n, dx, dy)
		} else {
			n.n.X, n.n.Y = p.X, p.Y
		}
	}
	for _, ch := range chains {
		ch.le.mid = ch.le.mid[:0]
		for _, d := range ch.nodes {
			ch.le.mid = append(ch.le.mid, tf(d.x, d.y))
		}
		if ch.labelAt >= 0 {
			d := ch.nodes[ch.labelAt]
			ch.le.label = tf(d.x, d.y)
			ch.le.hasLabel = true
		}
		if ch.reversed {
			for i, j := 0, len(ch.le.mid)-1; i < j; i, j = i+1, j-1 {
				ch.le.mid[i], ch.le.mid[j] = ch.le.mid[j], ch.le.mid[i]
			}
		}
	}
	// self loops: route around the right side (or bottom for LR).
	for _, e := range loops {
		n := index[e.u]
		c := tf(n.x, n.y)
		nw, nh := n.n.W, n.n.H
		var pts []Point
		if horiz {
			yb := c.Y + nh/2
			pts = []Point{{X: c.X - nw/4, Y: yb + 25}, {X: c.X, Y: yb + 35}, {X: c.X + nw/4, Y: yb + 25}}
			e.label = Point{X: c.X, Y: yb + 35 + e.e.LabelH/2 + 4}
		} else {
			xr := c.X + nw/2
			pts = []Point{{X: xr + 25, Y: c.Y - nh/4}, {X: xr + 35, Y: c.Y}, {X: xr + 25, Y: c.Y + nh/4}}
			e.label = Point{X: xr + 38 + e.e.LabelW/2, Y: c.Y}
		}
		e.mid = pts
		e.hasLabel = e.e.LabelW > 0
	}
	if horiz {
		W, H = H, W
	}
	// Spread parallel edges between the same pair that have no dummies.
	spreadParallel(edges, horiz)
	return W, H
}

func spreadParallel(edges []*levelEdge, horiz bool) {
	type key struct{ a, b *Node }
	groups := map[key][]*levelEdge{}
	for _, e := range edges {
		if e.u == e.v || len(e.mid) > 0 {
			continue
		}
		k := key{e.u, e.v}
		if ptrLess(e.v, e.u) {
			k = key{e.v, e.u}
		}
		groups[k] = append(groups[k], e)
	}
	for _, g := range groups {
		if len(g) < 2 {
			continue
		}
		for i, e := range g {
			off := (float64(i) - float64(len(g)-1)/2) * 22
			a := Point{X: e.u.X, Y: e.u.Y}
			b := Point{X: e.v.X, Y: e.v.Y}
			m := a.Add(b).Mul(0.5)
			d := b.Sub(a).Norm()
			nrm := Point{X: -d.Y, Y: d.X}
			if ptrLess(e.v, e.u) {
				nrm = nrm.Mul(-1)
			}
			e.mid = []Point{m.Add(nrm.Mul(off))}
		}
	}
}

func ptrLess(a, b *Node) bool { return strings.Compare(a.ID, b.ID) < 0 }

// finishEdge computes the final route of an edge.
func finishEdge(le *levelEdge) {
	e := le.e
	from, to := e.From, e.To
	fc := Point{X: from.X, Y: from.Y}
	tc := Point{X: to.X, Y: to.Y}
	pts := make([]Point, 0, len(le.mid)+2)
	first := tc
	if len(le.mid) > 0 {
		first = le.mid[0]
	}
	last := fc
	if len(le.mid) > 0 {
		last = le.mid[len(le.mid)-1]
	}
	// When an endpoint is inside a cluster, route to the cluster first.
	start := clipNode(from, first)
	end := clipNode(to, last)
	if from == to {
		// self loop: start and end on the node boundary near first/last mid points
		start = clipNode(from, le.mid[0])
		end = clipNode(to, le.mid[len(le.mid)-1])
	}
	pts = append(pts, start)
	pts = append(pts, le.mid...)
	pts = append(pts, end)
	e.Points = dedupe(pts)
	if le.hasLabel {
		e.Label = le.label
	} else {
		e.Label = midpoint(e.Points)
	}
}

func dedupe(pts []Point) []Point {
	out := pts[:0]
	for i, p := range pts {
		if i > 0 && p.Dist(out[len(out)-1]) < 0.5 {
			continue
		}
		out = append(out, p)
	}
	return out
}

// midpoint returns the point halfway along a polyline.
func midpoint(pts []Point) Point {
	if len(pts) == 0 {
		return Point{}
	}
	total := 0.0
	for i := 1; i < len(pts); i++ {
		total += pts[i].Dist(pts[i-1])
	}
	half := total / 2
	for i := 1; i < len(pts); i++ {
		d := pts[i].Dist(pts[i-1])
		if half <= d && d > 0 {
			t := half / d
			return pts[i-1].Add(pts[i].Sub(pts[i-1]).Mul(t))
		}
		half -= d
	}
	return pts[len(pts)-1]
}

// Midpoint returns the point halfway along a polyline.
func Midpoint(pts []Point) Point { return midpoint(pts) }

func clipNode(n *Node, toward Point) Point {
	if n.Clip != nil {
		return n.Clip(n, toward)
	}
	return ClipRect(n, toward)
}

// ClipRect intersects the segment from the node center towards p with the
// node's bounding rectangle.
func ClipRect(n *Node, p Point) Point {
	cx, cy := n.X, n.Y
	dx, dy := p.X-cx, p.Y-cy
	if dx == 0 && dy == 0 {
		return Point{X: cx, Y: cy}
	}
	hw, hh := n.W/2, n.H/2
	var sx, sy float64
	if math.Abs(dy)*hw > math.Abs(dx)*hh {
		// top/bottom
		if dy < 0 {
			hh = -hh
		}
		sx = hh * dx / dy
		sy = hh
	} else {
		if dx < 0 {
			hw = -hw
		}
		sx = hw
		sy = hw * dy / dx
	}
	return Point{X: cx + sx, Y: cy + sy}
}

// ClipEllipse clips to the ellipse inscribed in the node box.
func ClipEllipse(n *Node, p Point) Point {
	dx, dy := p.X-n.X, p.Y-n.Y
	if dx == 0 && dy == 0 {
		return Point{X: n.X, Y: n.Y}
	}
	rx, ry := n.W/2, n.H/2
	t := 1 / math.Sqrt(dx*dx/(rx*rx)+dy*dy/(ry*ry))
	return Point{X: n.X + dx*t, Y: n.Y + dy*t}
}

// ClipDiamond clips to a rhombus inscribed in the node box.
func ClipDiamond(n *Node, p Point) Point {
	dx, dy := p.X-n.X, p.Y-n.Y
	if dx == 0 && dy == 0 {
		return Point{X: n.X, Y: n.Y}
	}
	rx, ry := n.W/2, n.H/2
	t := 1 / (math.Abs(dx)/rx + math.Abs(dy)/ry)
	return Point{X: n.X + dx*t, Y: n.Y + dy*t}
}

// ClipPolygon returns a clip function for a convex polygon given in
// coordinates relative to the node center.
func ClipPolygon(poly []Point) func(n *Node, p Point) Point {
	return func(n *Node, p Point) Point {
		c := Point{X: n.X, Y: n.Y}
		best := -1.0
		var res Point
		for i := range poly {
			a := poly[i].Add(c)
			b := poly[(i+1)%len(poly)].Add(c)
			if t, ok := segIntersect(c, p, a, b); ok {
				if best < 0 || t < best {
					best = t
					res = c.Add(p.Sub(c).Mul(t))
				}
			}
		}
		if best < 0 {
			return ClipRect(n, p)
		}
		return res
	}
}

// segIntersect returns t along p0->p1 where it hits segment a-b.
func segIntersect(p0, p1, a, b Point) (float64, bool) {
	r := p1.Sub(p0)
	s := b.Sub(a)
	den := r.X*s.Y - r.Y*s.X
	if math.Abs(den) < 1e-9 {
		return 0, false
	}
	q := a.Sub(p0)
	t := (q.X*s.Y - q.Y*s.X) / den
	u := (q.X*r.Y - q.Y*r.X) / den
	if u < -1e-9 || u > 1+1e-9 || t < 0 {
		return 0, false
	}
	// the segment p0->p1 may end inside the polygon; allow t>1 so we still
	// hit the boundary when p is inside the node (rare)
	return t, true
}

// ---------------------------------------------------------------------------
// Crossing reduction

func orderLayers(layers [][]*lnode) {
	best := snapshot(layers)
	bestC := totalCrossings(layers)
	if bestC == 0 {
		return
	}
	for iter := range 24 {
		if iter%2 == 0 {
			for r := 1; r < len(layers); r++ {
				sortByBary(layers[r], true)
			}
		} else {
			for r := len(layers) - 2; r >= 0; r-- {
				sortByBary(layers[r], false)
			}
		}
		transpose(layers)
		c := totalCrossings(layers)
		if c < bestC {
			bestC = c
			best = snapshot(layers)
			if c == 0 {
				break
			}
		}
	}
	for r := range layers {
		layers[r] = best[r]
		for i, n := range layers[r] {
			n.order = i
		}
	}
}

func snapshot(layers [][]*lnode) [][]*lnode {
	out := make([][]*lnode, len(layers))
	for i, l := range layers {
		out[i] = append([]*lnode(nil), l...)
	}
	return out
}

func sortByBary(layer []*lnode, useIn bool) {
	type kv struct {
		n *lnode
		b float64
	}
	vals := make([]kv, len(layer))
	for i, n := range layer {
		var sum, cnt float64
		es := n.out
		if useIn {
			es = n.in
		}
		for _, e := range es {
			o := e.to
			if useIn {
				o = e.from
			}
			sum += float64(o.order)
			cnt++
		}
		if cnt == 0 {
			vals[i] = kv{n, -1}
		} else {
			vals[i] = kv{n, sum / cnt}
		}
	}
	// nodes without neighbors keep their position: fill in with current order
	for i := range vals {
		if vals[i].b < 0 {
			vals[i].b = float64(vals[i].n.order)
		}
	}
	sort.SliceStable(vals, func(i, j int) bool { return vals[i].b < vals[j].b })
	for i, v := range vals {
		layer[i] = v.n
		v.n.order = i
	}
}

func transpose(layers [][]*lnode) {
	improved := true
	for pass := 0; improved && pass < 4; pass++ {
		improved = false
		for r := range layers {
			layer := layers[r]
			for i := 0; i < len(layer)-1; i++ {
				a, b := layer[i], layer[i+1]
				before := pairCrossings(a, b)
				after := pairCrossings(b, a)
				if after < before {
					layer[i], layer[i+1] = b, a
					a.order, b.order = i+1, i
					improved = true
				}
			}
		}
	}
}

// pairCrossings counts crossings between the edges of a and b (with a left of b).
func pairCrossings(a, b *lnode) int {
	c := 0
	for _, ea := range a.in {
		for _, eb := range b.in {
			if ea.from.order > eb.from.order {
				c++
			}
		}
	}
	for _, ea := range a.out {
		for _, eb := range b.out {
			if ea.to.order > eb.to.order {
				c++
			}
		}
	}
	return c
}

func totalCrossings(layers [][]*lnode) int {
	c := 0
	for r := 0; r < len(layers)-1; r++ {
		var es []*ledge
		for _, n := range layers[r] {
			es = append(es, n.out...)
		}
		for i := 0; i < len(es); i++ {
			for j := i + 1; j < len(es); j++ {
				a, b := es[i], es[j]
				if (a.from.order-b.from.order)*(a.to.order-b.to.order) < 0 {
					c++
				}
			}
		}
	}
	return c
}

// ---------------------------------------------------------------------------
// Coordinate assignment

func assignX(layers [][]*lnode, sep func(a, b *lnode) float64) {
	// initial packing
	for _, layer := range layers {
		x := 0.0
		for i, n := range layer {
			if i > 0 {
				x += layer[i-1].w/2 + sep(layer[i-1], n) + n.w/2
			}
			n.x = x
		}
		// center layer around 0
		if len(layer) > 0 {
			off := (layer[0].x + layer[len(layer)-1].x) / 2
			for _, n := range layer {
				n.x -= off
			}
		}
	}
	for iter := range 12 {
		down := iter%2 == 0
		useBoth := iter >= 2
		if down {
			for r := 1; r < len(layers); r++ {
				placeLayer(layers[r], sep, true, useBoth)
			}
		} else {
			for r := len(layers) - 2; r >= 0; r-- {
				placeLayer(layers[r], sep, false, useBoth)
			}
		}
	}
	// a final pass for the first layer
	if len(layers) > 1 {
		placeLayer(layers[0], sep, false, true)
	}
}

func placeLayer(layer []*lnode, sep func(a, b *lnode) float64, fromAbove, both bool) {
	n := len(layer)
	if n == 0 {
		return
	}
	desired := make([]float64, n)
	weights := make([]float64, n)
	for i, v := range layer {
		var sum, wsum float64
		addE := func(es []*ledge, useFrom bool) {
			for _, e := range es {
				o := e.to
				if useFrom {
					o = e.from
				}
				sum += o.x * e.weight
				wsum += e.weight
			}
		}
		if fromAbove || both {
			addE(v.in, true)
		}
		if !fromAbove || both {
			addE(v.out, false)
		}
		if wsum == 0 {
			desired[i] = v.x
			weights[i] = 0.01
		} else {
			desired[i] = sum / wsum
			weights[i] = wsum
		}
	}
	// offsets such that x_i = y_i + off_i and constraints become y_i <= y_{i+1}
	off := make([]float64, n)
	for i := 1; i < n; i++ {
		off[i] = off[i-1] + layer[i-1].w/2 + sep(layer[i-1], layer[i]) + layer[i].w/2
	}
	target := make([]float64, n)
	for i := range target {
		target[i] = desired[i] - off[i]
	}
	y := isotonic(target, weights)
	for i, v := range layer {
		v.x = y[i] + off[i]
	}
}

// isotonic solves weighted least squares with a non-decreasing constraint
// using the pool adjacent violators algorithm.
func isotonic(v, w []float64) []float64 {
	type block struct {
		sum, wsum float64
		n         int
	}
	var blocks []block
	for i := range v {
		blocks = append(blocks, block{v[i] * w[i], w[i], 1})
		for len(blocks) > 1 {
			a := blocks[len(blocks)-2]
			b := blocks[len(blocks)-1]
			if a.sum/a.wsum <= b.sum/b.wsum {
				break
			}
			blocks = blocks[:len(blocks)-2]
			blocks = append(blocks, block{a.sum + b.sum, a.wsum + b.wsum, a.n + b.n})
		}
	}
	out := make([]float64, 0, len(v))
	for _, b := range blocks {
		m := b.sum / b.wsum
		for i := 0; i < b.n; i++ {
			out = append(out, m)
		}
	}
	return out
}
