// Package mindmap implements Mermaid mindmaps.
package mindmap

import (
	"fmt"
	"image/color"
	"math"
	"regexp"
	"strings"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

func init() {
	diagram.Register(diagram.Type{
		Name:   "mindmap",
		Detect: diagram.Keyword("mindmap"),
		Render: Render,
	})
}

// Shape of a mindmap node.
type Shape int

const (
	ShapeDefault Shape = iota
	ShapeSquare
	ShapeRounded
	ShapeCircle
	ShapeBang
	ShapeCloud
	ShapeHexagon
)

// Node is a mindmap node.
type Node struct {
	ID       string
	Label    string
	Shape    Shape
	Icon     string
	Classes  []string
	Children []*Node
	indent   int

	// layout
	w, h    float64
	x, y    float64 // center
	subH    float64 // subtree extent across the flow
	branch  int
	depth   int
	textW   float64
	textH   float64
	wrapped string
}

var shapeDelims = []struct {
	open, close string
	shape       Shape
}{
	{"((", "))", ShapeCircle},
	{"))", "((", ShapeBang},
	{"{{", "}}", ShapeHexagon},
	{"(", ")", ShapeRounded},
	{")", "(", ShapeCloud},
	{"[", "]", ShapeSquare},
}

var iconRe = regexp.MustCompile(`^::icon\((.*)\)\s*$`)

// Parse parses mindmap source and returns the root.
func Parse(src string) (*Node, error) {
	var root *Node
	var stack []*Node
	var last *Node
	header := false
	for i, raw := range strings.Split(src, "\n") {
		ln := i + 1
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "%%") {
			continue
		}
		if !header {
			if !strings.HasPrefix(trimmed, "mindmap") {
				return nil, fmt.Errorf("line %d: expected 'mindmap'", ln)
			}
			header = true
			continue
		}
		indent := indentOf(raw)
		if m := iconRe.FindStringSubmatch(trimmed); m != nil {
			if last != nil {
				last.Icon = m[1]
			}
			continue
		}
		if strings.HasPrefix(trimmed, ":::") {
			if last != nil {
				last.Classes = append(last.Classes, strings.Fields(trimmed[3:])...)
			}
			continue
		}
		n := parseNode(trimmed)
		n.indent = indent
		if root == nil {
			root = n
			stack = []*Node{n}
			last = n
			continue
		}
		for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			return nil, fmt.Errorf("line %d: there can be only one root node (check indentation of %q)", ln, trimmed)
		}
		parent := stack[len(stack)-1]
		parent.Children = append(parent.Children, n)
		stack = append(stack, n)
		last = n
	}
	if root == nil {
		return nil, fmt.Errorf("mindmap has no nodes")
	}
	return root, nil
}

func indentOf(s string) int {
	n := 0
	for _, r := range s {
		switch r {
		case ' ':
			n++
		case '\t':
			n += 4
		default:
			return n
		}
	}
	return n
}

func parseNode(s string) *Node {
	n := &Node{}
	for _, d := range shapeDelims {
		i := strings.Index(s, d.open)
		if i < 0 || !strings.HasSuffix(s, d.close) || len(s) < i+len(d.open)+len(d.close) {
			continue
		}
		// the id must not contain other delimiters
		id := strings.TrimSpace(s[:i])
		if strings.ContainsAny(id, "()[]{}") {
			continue
		}
		n.ID = id
		n.Label = diagram.CleanLabel(s[i+len(d.open) : len(s)-len(d.close)])
		n.Shape = d.shape
		if n.ID == "" {
			n.ID = n.Label
		}
		return n
	}
	n.ID = s
	n.Label = diagram.CleanLabel(s)
	return n
}

// ---------------------------------------------------------------------------

const (
	hGap = 70.0
	vGap = 16.0
)

// Render renders a mindmap.
func Render(src string, cfg *diagram.Config) (*scene.Scene, error) {
	root, err := Parse(src)
	if err != nil {
		return nil, err
	}
	th := cfg.Theme
	sc := diagram.NewScene(th)
	measure(root, 0, th)

	// split first level children into right and left groups balancing
	// their subtree extents
	var right, left []*Node
	var rh, lh float64
	for i, c := range root.Children {
		assignBranch(c, i)
		if rh <= lh {
			right = append(right, c)
			rh += c.subH + vGap
		} else {
			left = append(left, c)
			lh += c.subH + vGap
		}
	}
	root.x, root.y = 0, 0
	place(right, root, 1, th)
	place(left, root, -1, th)

	drawEdges(sc, root, th)
	drawNodes(sc, root, th)
	return diagram.Finish(sc, th, cfg.Title), nil
}

func fontFor(depth int, th *theme.Theme) scene.Font {
	switch depth {
	case 0:
		return scene.Font{Size: th.FontSize * 1.25, Bold: true}
	case 1:
		return scene.Font{Size: th.FontSize * 1.05, Bold: true}
	}
	return scene.Font{Size: th.FontSize * 0.95}
}

func measure(n *Node, depth int, th *theme.Theme) {
	n.depth = depth
	f := fontFor(depth, th)
	maxW := 180.0
	if depth == 0 {
		maxW = 220
	}
	n.wrapped = scene.WrapText(n.Label, f, maxW)
	n.textW, n.textH = scene.MeasureBlock(n.wrapped, f, 0)
	padX, padY := 18.0, 10.0
	if depth == 0 {
		padX, padY = 26, 18
	}
	n.w, n.h = n.textW+2*padX, n.textH+2*padY
	switch n.Shape {
	case ShapeCircle:
		d := math.Max(n.w, n.h) * 0.95
		if depth == 0 {
			d = math.Max(d, 90)
		}
		n.w, n.h = d, d
	case ShapeHexagon:
		n.w += n.h / 2
	case ShapeCloud, ShapeBang:
		n.w += 24
		n.h += 18
	}
	if depth == 0 && n.Shape == ShapeDefault {
		// the root defaults to a pill
		n.w += n.h / 2
	}
	childSum := 0.0
	for i, c := range n.Children {
		measure(c, depth+1, th)
		if i > 0 {
			childSum += vGap
		}
		childSum += c.subH
	}
	n.subH = math.Max(n.h, childSum)
}

func assignBranch(n *Node, b int) {
	n.branch = b
	for _, c := range n.Children {
		assignBranch(c, b)
	}
}

// place lays out a list of subtrees to one side (dir=+1 right, -1 left) of
// parent, stacked vertically and centered on the parent.
func place(nodes []*Node, parent *Node, dir float64, th *theme.Theme) {
	if len(nodes) == 0 {
		return
	}
	total := 0.0
	maxW := 0.0
	for i, n := range nodes {
		if i > 0 {
			total += vGap
		}
		total += n.subH
		maxW = math.Max(maxW, n.w)
	}
	gap := hGap
	if parent.depth == 0 {
		gap = hGap + 20
	}
	y := parent.y - total/2
	for _, n := range nodes {
		n.x = parent.x + dir*(parent.w/2+gap+n.w/2)
		n.y = y + n.subH/2
		y += n.subH + vGap
		place(n.Children, n, dir, th)
	}
}

func colors(n *Node, th *theme.Theme) (fillC, textC, lineC color.RGBA) {
	if n.depth == 0 {
		return th.PrimaryColor, th.PrimaryTextColor, th.PrimaryBorderColor
	}
	c := th.ChartColor(n.branch)
	fill := c
	if n.depth >= 2 {
		fill = theme.Mix(c, th.Background, 0.55)
	}
	return fill, theme.ContrastText(fill), c
}

func drawEdges(sc *scene.Scene, n *Node, th *theme.Theme) {
	for _, c := range n.Children {
		_, _, line := colors(c, th)
		dir := 1.0
		if c.x < n.x {
			dir = -1
		}
		x1 := n.x + dir*n.w/2*0.85
		if n.depth == 0 {
			x1 = n.x + dir*n.w/2*0.6
		}
		y1 := n.y
		x2 := c.x - dir*c.w/2
		y2 := c.y
		width := math.Max(4.5-float64(c.depth-1)*1.3, 1.5)
		mx := (x1 + x2) / 2
		p := scene.NewPath(scene.Style{Stroke: line, StrokeWidth: width, RoundCaps: true})
		p.MoveTo(x1, y1).CubicTo(mx, y1, mx, y2, x2, y2)
		sc.Add(p)
		drawEdges(sc, c, th)
	}
}

func drawNodes(sc *scene.Scene, n *Node, th *theme.Theme) {
	fill, text, line := colors(n, th)
	x, y, w, h := n.x-n.w/2, n.y-n.h/2, n.w, n.h
	st := scene.Style{Fill: fill, Stroke: line, StrokeWidth: 1.5, Shadow: true}
	if n.depth > 0 && n.Shape == ShapeDefault {
		st.Stroke = theme.Darken(fill, 0.12)
	}
	switch n.Shape {
	case ShapeSquare:
		sc.Add(scene.RectPath(x, y, w, h, 0, st))
	case ShapeRounded:
		sc.Add(scene.RectPath(x, y, w, h, 10, st))
	case ShapeCircle:
		sc.Add(scene.Circle(n.x, n.y, w/2, st))
	case ShapeHexagon:
		m := h / 4
		sc.Add(scene.NewPath(st).Polygon(scene.Pt(x+m, y), scene.Pt(x+w-m, y), scene.Pt(x+w, n.y),
			scene.Pt(x+w-m, y+h), scene.Pt(x+m, y+h), scene.Pt(x, n.y)))
	case ShapeCloud:
		sc.Add(cloud(x, y, w, h, st))
	case ShapeBang:
		sc.Add(bang(x, y, w, h, st))
	default:
		sc.Add(scene.RectPath(x, y, w, h, math.Min(h/2, 14), st))
	}
	sc.Add(scene.NewText(n.x, n.y, n.wrapped, fontFor(n.depth, th), text, scene.AnchorMiddle, scene.VAlignMiddle))
	for _, c := range n.Children {
		drawNodes(sc, c, th)
	}
}

// cloud draws a cloud-like outline made of arcs.
func cloud(x, y, w, h float64, st scene.Style) *scene.Path {
	p := scene.NewPath(st)
	cx, cy := x+w/2, y+h/2
	n := 10
	rx, ry := w/2, h/2
	pts := make([]scene.Point, n)
	for i := range n {
		a := 2 * math.Pi * float64(i) / float64(n)
		pts[i] = scene.Pt(cx+rx*0.92*math.Cos(a), cy+ry*0.92*math.Sin(a))
	}
	p.MoveTo(pts[0].X, pts[0].Y)
	for i := range n {
		a, b := pts[i], pts[(i+1)%n]
		mid := a.Add(b).Mul(0.5)
		out := mid.Sub(scene.Pt(cx, cy)).Norm().Mul(math.Min(rx, ry) * 0.35)
		c := mid.Add(out)
		p.QuadTo(c.X, c.Y, b.X, b.Y)
	}
	return p.Close()
}

// bang draws an explosion-like star outline.
func bang(x, y, w, h float64, st scene.Style) *scene.Path {
	cx, cy := x+w/2, y+h/2
	n := 12
	var pts []scene.Point
	for i := 0; i < 2*n; i++ {
		a := math.Pi * float64(i) / float64(n)
		f := 1.0
		if i%2 == 1 {
			f = 0.78
		}
		pts = append(pts, scene.Pt(cx+w/2*f*math.Cos(a), cy+h/2*f*math.Sin(a)))
	}
	return scene.NewPath(st).Polygon(pts...)
}
