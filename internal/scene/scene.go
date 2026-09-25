// Package scene implements a small retained-mode vector display list.
//
// Diagram renderers draw into a Scene using logical ("CSS pixel")
// coordinates. The scene can later be rasterized at any scale and for any
// viewport, which lets the TUI zoom and pan while keeping everything crisp.
//
// All colors are straight (non-premultiplied) RGBA values stored in
// color.RGBA. A zero alpha means "none".
package scene

import (
	"image/color"
	"math"
)

// Point is a 2D point.
type Point struct{ X, Y float64 }

// Pt is shorthand for Point{x, y}.
func Pt(x, y float64) Point { return Point{x, y} }

// Add returns p+q.
func (p Point) Add(q Point) Point { return Point{p.X + q.X, p.Y + q.Y} }

// Sub returns p-q.
func (p Point) Sub(q Point) Point { return Point{p.X - q.X, p.Y - q.Y} }

// Mul returns p*s.
func (p Point) Mul(s float64) Point { return Point{p.X * s, p.Y * s} }

// Len returns the vector length.
func (p Point) Len() float64 { return math.Hypot(p.X, p.Y) }

// Norm returns the unit vector (or zero).
func (p Point) Norm() Point {
	l := p.Len()
	if l == 0 {
		return Point{}
	}
	return Point{p.X / l, p.Y / l}
}

// Dist returns the distance between p and q.
func (p Point) Dist(q Point) float64 { return p.Sub(q).Len() }

// Rect is an axis aligned rectangle.
type Rect struct{ X, Y, W, H float64 }

// Empty reports whether r has no area.
func (r Rect) Empty() bool { return r.W <= 0 || r.H <= 0 }

// Union returns the smallest rect containing r and o.
func (r Rect) Union(o Rect) Rect {
	if r.W == 0 && r.H == 0 && r.X == 0 && r.Y == 0 {
		return o
	}
	if o.W == 0 && o.H == 0 && o.X == 0 && o.Y == 0 {
		return r
	}
	x0 := math.Min(r.X, o.X)
	y0 := math.Min(r.Y, o.Y)
	x1 := math.Max(r.X+r.W, o.X+o.W)
	y1 := math.Max(r.Y+r.H, o.Y+o.H)
	return Rect{x0, y0, x1 - x0, y1 - y0}
}

// Center returns the rectangle center.
func (r Rect) Center() Point { return Point{r.X + r.W/2, r.Y + r.H/2} }

// Anchor is horizontal text alignment.
type Anchor int

const (
	AnchorStart Anchor = iota
	AnchorMiddle
	AnchorEnd
)

// VAlign is vertical text alignment relative to Text.Y.
type VAlign int

const (
	// VAlignMiddle centers the whole text block on Y.
	VAlignMiddle VAlign = iota
	// VAlignTop puts the top of the block at Y.
	VAlignTop
	// VAlignBottom puts the bottom of the block at Y.
	VAlignBottom
	// VAlignBaseline puts the baseline of the first line at Y.
	VAlignBaseline
)

// Font describes a font style.
type Font struct {
	Size   float64
	Bold   bool
	Italic bool
	Mono   bool
}

// Style describes how a path is painted.
type Style struct {
	Fill        color.RGBA
	Stroke      color.RGBA
	StrokeWidth float64
	Dash        []float64
	// Shadow draws a soft drop shadow beneath the shape.
	Shadow bool
	// RoundCaps uses round caps and joins (default: butt caps, miter joins).
	RoundCaps bool
}

// Item is anything that can be drawn.
type Item interface {
	bounds() Rect
	translate(dx, dy float64)
}

type opKind uint8

const (
	opMove opKind = iota
	opLine
	opQuad
	opCubic
	opClose
)

type pathOp struct {
	kind opKind
	pts  [3]Point
}

// Path is a vector path.
type Path struct {
	Style Style
	ops   []pathOp
}

// Text is a (possibly multi-line) text item.
type Text struct {
	X, Y   float64
	S      string // lines separated by '\n'
	Font   Font
	Color  color.RGBA
	Anchor Anchor
	VAlign VAlign
	// Rotate is a rotation in degrees around (X, Y).
	Rotate float64
	// LineHeight multiplier (default 1.25).
	LineHeight float64
}

// Scene is a list of drawable items.
type Scene struct {
	Width, Height float64
	Background    color.RGBA
	ShadowColor   color.RGBA
	Items         []Item
}

// New creates a new empty scene.
func New(bg color.RGBA) *Scene {
	return &Scene{Background: bg, ShadowColor: color.RGBA{0, 0, 0, 30}}
}

// Add appends items to the scene.
func (s *Scene) Add(items ...Item) {
	s.Items = append(s.Items, items...)
}

// Bounds returns the bounding box of all items.
func (s *Scene) Bounds() Rect {
	var r Rect
	first := true
	for _, it := range s.Items {
		b := it.bounds()
		if b.W == 0 && b.H == 0 && b.X == 0 && b.Y == 0 {
			continue
		}
		if first {
			r = b
			first = false
			continue
		}
		r = r.Union(b)
	}
	return r
}

// Translate moves every item.
func (s *Scene) Translate(dx, dy float64) {
	for _, it := range s.Items {
		it.translate(dx, dy)
	}
}

// Fit translates the content so that its bounding box starts at
// (pad, pad) and sets Width/Height accordingly.
func (s *Scene) Fit(pad float64) {
	b := s.Bounds()
	s.Translate(pad-b.X, pad-b.Y)
	s.Width = math.Ceil(b.W + 2*pad)
	s.Height = math.Ceil(b.H + 2*pad)
}

// ---------------------------------------------------------------------------
// Path construction

// NewPath creates an empty path with the given style.
func NewPath(st Style) *Path { return &Path{Style: st} }

// MoveTo starts a new sub path.
func (p *Path) MoveTo(x, y float64) *Path {
	p.ops = append(p.ops, pathOp{kind: opMove, pts: [3]Point{{x, y}}})
	return p
}

// LineTo adds a line segment.
func (p *Path) LineTo(x, y float64) *Path {
	p.ops = append(p.ops, pathOp{kind: opLine, pts: [3]Point{{x, y}}})
	return p
}

// QuadTo adds a quadratic bezier.
func (p *Path) QuadTo(cx, cy, x, y float64) *Path {
	p.ops = append(p.ops, pathOp{kind: opQuad, pts: [3]Point{{cx, cy}, {x, y}}})
	return p
}

// CubicTo adds a cubic bezier.
func (p *Path) CubicTo(c1x, c1y, c2x, c2y, x, y float64) *Path {
	p.ops = append(p.ops, pathOp{kind: opCubic, pts: [3]Point{{c1x, c1y}, {c2x, c2y}, {x, y}}})
	return p
}

// Close closes the current sub path.
func (p *Path) Close() *Path {
	p.ops = append(p.ops, pathOp{kind: opClose})
	return p
}

// Rect adds a rectangle sub path with optional corner radius.
func (p *Path) Rect(x, y, w, h, r float64) *Path {
	if r <= 0 {
		return p.MoveTo(x, y).LineTo(x+w, y).LineTo(x+w, y+h).LineTo(x, y+h).Close()
	}
	r = math.Min(r, math.Min(w, h)/2)
	const k = 0.5522847498
	p.MoveTo(x+r, y)
	p.LineTo(x+w-r, y)
	p.CubicTo(x+w-r+r*k, y, x+w, y+r-r*k, x+w, y+r)
	p.LineTo(x+w, y+h-r)
	p.CubicTo(x+w, y+h-r+r*k, x+w-r+r*k, y+h, x+w-r, y+h)
	p.LineTo(x+r, y+h)
	p.CubicTo(x+r-r*k, y+h, x, y+h-r+r*k, x, y+h-r)
	p.LineTo(x, y+r)
	p.CubicTo(x, y+r-r*k, x+r-r*k, y, x+r, y)
	return p.Close()
}

// Ellipse adds an ellipse sub path centered at (cx, cy).
func (p *Path) Ellipse(cx, cy, rx, ry float64) *Path {
	const k = 0.5522847498
	p.MoveTo(cx+rx, cy)
	p.CubicTo(cx+rx, cy+ry*k, cx+rx*k, cy+ry, cx, cy+ry)
	p.CubicTo(cx-rx*k, cy+ry, cx-rx, cy+ry*k, cx-rx, cy)
	p.CubicTo(cx-rx, cy-ry*k, cx-rx*k, cy-ry, cx, cy-ry)
	p.CubicTo(cx+rx*k, cy-ry, cx+rx, cy-ry*k, cx+rx, cy)
	return p.Close()
}

// Circle adds a circle sub path.
func (p *Path) Circle(cx, cy, r float64) *Path { return p.Ellipse(cx, cy, r, r) }

// Arc adds an elliptical arc from angle a0 to a1 (radians, clockwise in
// screen space as angles increase). If the path is empty or connect is false
// a MoveTo is emitted to the start point, otherwise a LineTo.
func (p *Path) Arc(cx, cy, rx, ry, a0, a1 float64, connect bool) *Path {
	start := Point{cx + rx*math.Cos(a0), cy + ry*math.Sin(a0)}
	if connect && len(p.ops) > 0 {
		p.LineTo(start.X, start.Y)
	} else {
		p.MoveTo(start.X, start.Y)
	}
	n := max(int(math.Ceil(math.Abs(a1-a0)/(math.Pi/2))), 1)
	step := (a1 - a0) / float64(n)
	k := 4.0 / 3.0 * math.Tan(step/4)
	a := a0
	for i := 0; i < n; i++ {
		b := a + step
		c0, s0 := math.Cos(a), math.Sin(a)
		c1, s1 := math.Cos(b), math.Sin(b)
		p.CubicTo(
			cx+rx*(c0-k*s0), cy+ry*(s0+k*c0),
			cx+rx*(c1+k*s1), cy+ry*(s1-k*c1),
			cx+rx*c1, cy+ry*s1,
		)
		a = b
	}
	return p
}

// Polygon adds a closed polygon.
func (p *Path) Polygon(pts ...Point) *Path {
	if len(pts) == 0 {
		return p
	}
	p.MoveTo(pts[0].X, pts[0].Y)
	for _, q := range pts[1:] {
		p.LineTo(q.X, q.Y)
	}
	return p.Close()
}

// Polyline adds an open polyline.
func (p *Path) Polyline(pts ...Point) *Path {
	if len(pts) == 0 {
		return p
	}
	p.MoveTo(pts[0].X, pts[0].Y)
	for _, q := range pts[1:] {
		p.LineTo(q.X, q.Y)
	}
	return p
}

// RoundedPolyline adds an open polyline whose corners are rounded with the
// given radius (orthogonal-style edges).
func (p *Path) RoundedPolyline(pts []Point, radius float64) *Path {
	if len(pts) < 3 || radius <= 0 {
		return p.Polyline(pts...)
	}
	p.MoveTo(pts[0].X, pts[0].Y)
	for i := 1; i < len(pts)-1; i++ {
		prev, cur, next := pts[i-1], pts[i], pts[i+1]
		d1 := cur.Sub(prev)
		d2 := next.Sub(cur)
		r := math.Min(radius, math.Min(d1.Len()/2, d2.Len()/2))
		a := cur.Sub(d1.Norm().Mul(r))
		b := cur.Add(d2.Norm().Mul(r))
		p.LineTo(a.X, a.Y)
		p.QuadTo(cur.X, cur.Y, b.X, b.Y)
	}
	last := pts[len(pts)-1]
	return p.LineTo(last.X, last.Y)
}

// BasisSpline adds a curve through pts using the same algorithm as
// d3.curveBasis (which Mermaid uses for flowchart edges).
func (p *Path) BasisSpline(pts []Point) *Path {
	switch len(pts) {
	case 0:
		return p
	case 1:
		return p.MoveTo(pts[0].X, pts[0].Y)
	case 2:
		return p.MoveTo(pts[0].X, pts[0].Y).LineTo(pts[1].X, pts[1].Y)
	}
	x0, y0 := pts[0].X, pts[0].Y
	x1, y1 := pts[1].X, pts[1].Y
	p.MoveTo(x0, y0)
	p.LineTo((5*x0+x1)/6, (5*y0+y1)/6)
	bez := func(x, y float64) {
		p.CubicTo((2*x0+x1)/3, (2*y0+y1)/3, (x0+2*x1)/3, (y0+2*y1)/3, (x0+4*x1+x)/6, (y0+4*y1+y)/6)
	}
	for _, q := range pts[2:] {
		bez(q.X, q.Y)
		x0, y0, x1, y1 = x1, y1, q.X, q.Y
	}
	bez(x1, y1)
	p.LineTo(x1, y1)
	return p
}

// MonotoneSpline draws a smooth curve through all points (Catmull-Rom).
func (p *Path) MonotoneSpline(pts []Point) *Path {
	if len(pts) < 3 {
		return p.Polyline(pts...)
	}
	p.MoveTo(pts[0].X, pts[0].Y)
	for i := 0; i < len(pts)-1; i++ {
		p0 := pts[max(i-1, 0)]
		p1 := pts[i]
		p2 := pts[i+1]
		p3 := pts[min(i+2, len(pts)-1)]
		c1 := p1.Add(p2.Sub(p0).Mul(1.0 / 6))
		c2 := p2.Sub(p3.Sub(p1).Mul(1.0 / 6))
		p.CubicTo(c1.X, c1.Y, c2.X, c2.Y, p2.X, p2.Y)
	}
	return p
}

// Empty reports whether the path has no segments.
func (p *Path) Empty() bool { return len(p.ops) == 0 }

func (p *Path) bounds() Rect {
	first := true
	var x0, y0, x1, y1 float64
	add := func(q Point) {
		if first {
			x0, y0, x1, y1 = q.X, q.Y, q.X, q.Y
			first = false
			return
		}
		x0 = math.Min(x0, q.X)
		y0 = math.Min(y0, q.Y)
		x1 = math.Max(x1, q.X)
		y1 = math.Max(y1, q.Y)
	}
	for _, op := range p.ops {
		switch op.kind {
		case opMove, opLine:
			add(op.pts[0])
		case opQuad:
			add(op.pts[0])
			add(op.pts[1])
		case opCubic:
			add(op.pts[0])
			add(op.pts[1])
			add(op.pts[2])
		}
	}
	if first {
		return Rect{}
	}
	hw := p.Style.StrokeWidth / 2
	if p.Style.Stroke.A == 0 {
		hw = 0
	}
	return Rect{x0 - hw, y0 - hw, x1 - x0 + 2*hw, y1 - y0 + 2*hw}
}

func (p *Path) translate(dx, dy float64) {
	for i := range p.ops {
		for j := range p.ops[i].pts {
			p.ops[i].pts[j].X += dx
			p.ops[i].pts[j].Y += dy
		}
	}
}

// ---------------------------------------------------------------------------
// Text

func (t *Text) lineHeight() float64 {
	lh := t.LineHeight
	if lh == 0 {
		lh = 1.25
	}
	return lh * t.Font.Size
}

// blockRect returns the unrotated block rectangle of the text.
func (t *Text) blockRect() Rect {
	w, h := MeasureBlock(t.S, t.Font, t.LineHeight)
	x := t.X
	switch t.Anchor {
	case AnchorMiddle:
		x -= w / 2
	case AnchorEnd:
		x -= w
	}
	y := t.Y
	switch t.VAlign {
	case VAlignMiddle:
		y -= h / 2
	case VAlignBottom:
		y -= h
	case VAlignBaseline:
		y -= baselineOffset(t.Font, t.lineHeight())
	}
	return Rect{x, y, w, h}
}

func (t *Text) bounds() Rect {
	r := t.blockRect()
	if t.Rotate == 0 {
		return r
	}
	a := t.Rotate * math.Pi / 180
	cs, sn := math.Cos(a), math.Sin(a)
	pts := []Point{{r.X, r.Y}, {r.X + r.W, r.Y}, {r.X, r.Y + r.H}, {r.X + r.W, r.Y + r.H}}
	var out Rect
	for i, q := range pts {
		dx, dy := q.X-t.X, q.Y-t.Y
		p := Point{t.X + dx*cs - dy*sn, t.Y + dx*sn + dy*cs}
		pr := Rect{p.X, p.Y, 0.001, 0.001}
		if i == 0 {
			out = pr
		} else {
			out = out.Union(pr)
		}
	}
	return out
}

func (t *Text) translate(dx, dy float64) { t.X += dx; t.Y += dy }

// ---------------------------------------------------------------------------
// Convenience constructors

// Rect returns a rectangle path.
func RectPath(x, y, w, h, r float64, st Style) *Path {
	return NewPath(st).Rect(x, y, w, h, r)
}

// Line returns a straight line path.
func Line(x1, y1, x2, y2 float64, st Style) *Path {
	return NewPath(st).MoveTo(x1, y1).LineTo(x2, y2)
}

// Circle returns a circle path.
func Circle(cx, cy, r float64, st Style) *Path {
	return NewPath(st).Circle(cx, cy, r)
}

// NewText returns a text item.
func NewText(x, y float64, s string, f Font, c color.RGBA, a Anchor, v VAlign) *Text {
	return &Text{X: x, Y: y, S: s, Font: f, Color: c, Anchor: a, VAlign: v}
}

// Group is a set of items that is translated as a unit. It is useful to
// lay out a sub diagram in local coordinates and then place it.
type Group struct {
	Items []Item
}

// Add appends items.
func (g *Group) Add(items ...Item) { g.Items = append(g.Items, items...) }

func (g *Group) bounds() Rect {
	var r Rect
	first := true
	for _, it := range g.Items {
		b := it.bounds()
		if first {
			r = b
			first = false
		} else {
			r = r.Union(b)
		}
	}
	return r
}

func (g *Group) translate(dx, dy float64) {
	for _, it := range g.Items {
		it.translate(dx, dy)
	}
}

// Bounds returns the bounds of an arbitrary item.
func Bounds(it Item) Rect { return it.bounds() }

// Translate moves an arbitrary item.
func Translate(it Item, dx, dy float64) { it.translate(dx, dy) }

// baselineOffset returns the distance from the top of a line box of height lh
// to the baseline, such that capital letters appear vertically centered.
func baselineOffset(f Font, lh float64) float64 {
	capH := 0.72 * f.Size
	return lh/2 + capH/2
}

// Flatten approximates the first sub path of p by a polygon (curves are
// sampled with n segments each).
func (p *Path) Flatten(n int) []Point {
	if n < 1 {
		n = 8
	}
	var out []Point
	var cur Point
	started := false
	for _, op := range p.ops {
		switch op.kind {
		case opMove:
			if started {
				return out
			}
			started = true
			cur = op.pts[0]
			out = append(out, cur)
		case opLine:
			cur = op.pts[0]
			out = append(out, cur)
		case opQuad:
			c, e := op.pts[0], op.pts[1]
			for i := 1; i <= n; i++ {
				t := float64(i) / float64(n)
				mt := 1 - t
				out = append(out, Point{
					mt*mt*cur.X + 2*mt*t*c.X + t*t*e.X,
					mt*mt*cur.Y + 2*mt*t*c.Y + t*t*e.Y,
				})
			}
			cur = e
		case opCubic:
			c1, c2, e := op.pts[0], op.pts[1], op.pts[2]
			for i := 1; i <= n; i++ {
				t := float64(i) / float64(n)
				mt := 1 - t
				a, b, c, d := mt*mt*mt, 3*mt*mt*t, 3*mt*t*t, t*t*t
				out = append(out, Point{
					a*cur.X + b*c1.X + c*c2.X + d*e.X,
					a*cur.Y + b*c1.Y + c*c2.Y + d*e.Y,
				})
			}
			cur = e
		case opClose:
			return out
		}
	}
	return out
}
