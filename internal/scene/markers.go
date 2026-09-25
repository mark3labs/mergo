package scene

import (
	"image/color"
	"math"
)

// MarkerKind is a line-end decoration.
type MarkerKind int

const (
	MarkerNone MarkerKind = iota
	// MarkerArrow is a filled triangular arrow head (-->).
	MarkerArrow
	// MarkerOpenArrow is a "V" shaped arrow made of two strokes (class dependency, async message).
	MarkerOpenArrow
	// MarkerCircle is a filled circle (--o).
	MarkerCircle
	// MarkerCross is an "x" (--x).
	MarkerCross
	// MarkerTriangleOpen is a hollow triangle (UML inheritance / realization).
	MarkerTriangleOpen
	// MarkerDiamondFilled is a filled diamond (UML composition).
	MarkerDiamondFilled
	// MarkerDiamondOpen is a hollow diamond (UML aggregation).
	MarkerDiamondOpen
	// MarkerHalfArrowTop is a half arrow head (sequence "-)" async).
	MarkerHalfArrowTop
	// MarkerCircleOpen is a hollow circle (lollipop interface).
	MarkerCircleOpen
	// Crow's foot notations for ER diagrams.
	MarkerERExactlyOne
	MarkerERZeroOrOne
	MarkerEROneOrMore
	MarkerERZeroOrMore
)

// MarkerInset returns how far (in scene units) a line should be shortened so
// that it ends at the base of the marker instead of the tip.
func MarkerInset(k MarkerKind, size float64) float64 {
	switch k {
	case MarkerArrow:
		return size * 0.9
	case MarkerTriangleOpen:
		return size * 1.3
	case MarkerDiamondFilled, MarkerDiamondOpen:
		return size * 2
	case MarkerCircle, MarkerCircleOpen:
		return size * 0.8
	case MarkerOpenArrow, MarkerHalfArrowTop:
		return 0.5
	}
	return 0
}

// MarkerOpts configures marker drawing.
type MarkerOpts struct {
	Size        float64
	Stroke      color.RGBA
	Fill        color.RGBA // used for filled markers (defaults to Stroke)
	Hollow      color.RGBA // interior of hollow markers (usually the background)
	StrokeWidth float64
}

// Marker returns the items for a marker whose tip is at tip, pointing along
// dir (which points from the line towards the tip).
func Marker(k MarkerKind, tip, dir Point, o MarkerOpts) []Item {
	if k == MarkerNone {
		return nil
	}
	if o.Size == 0 {
		o.Size = 10
	}
	if o.StrokeWidth == 0 {
		o.StrokeWidth = 1.5
	}
	if o.Fill.A == 0 {
		o.Fill = o.Stroke
	}
	d := dir.Norm()
	if d.X == 0 && d.Y == 0 {
		d = Point{1, 0}
	}
	n := Point{-d.Y, d.X}
	s := o.Size
	at := func(back, side float64) Point {
		return tip.Sub(d.Mul(back)).Add(n.Mul(side))
	}
	stroke := Style{Stroke: o.Stroke, StrokeWidth: o.StrokeWidth, RoundCaps: true}
	switch k {
	case MarkerArrow:
		return []Item{NewPath(Style{Fill: o.Fill, Stroke: o.Fill, StrokeWidth: 1}).Polygon(
			tip, at(s, s*0.5), at(s*0.8, 0), at(s, -s*0.5))}
	case MarkerOpenArrow:
		return []Item{NewPath(stroke).Polyline(at(s, s*0.5), tip, at(s, -s*0.5))}
	case MarkerHalfArrowTop:
		return []Item{NewPath(stroke).Polyline(at(s, -s*0.5), tip)}
	case MarkerTriangleOpen:
		return []Item{NewPath(Style{Fill: o.Hollow, Stroke: o.Stroke, StrokeWidth: o.StrokeWidth}).Polygon(
			tip, at(s*1.3, s*0.65), at(s*1.3, -s*0.65))}
	case MarkerDiamondFilled, MarkerDiamondOpen:
		fill := o.Fill
		if k == MarkerDiamondOpen {
			fill = o.Hollow
		}
		return []Item{NewPath(Style{Fill: fill, Stroke: o.Stroke, StrokeWidth: o.StrokeWidth}).Polygon(
			tip, at(s, s*0.5), at(s*2, 0), at(s, -s*0.5))}
	case MarkerCircle:
		c := at(s*0.4, 0)
		return []Item{Circle(c.X, c.Y, s*0.4, Style{Fill: o.Fill, Stroke: o.Fill, StrokeWidth: 1})}
	case MarkerCircleOpen:
		c := at(s*0.45, 0)
		return []Item{Circle(c.X, c.Y, s*0.45, Style{Fill: o.Hollow, Stroke: o.Stroke, StrokeWidth: o.StrokeWidth})}
	case MarkerCross:
		c := at(s*0.45, 0)
		r := s * 0.4
		a, b := c.Add(d.Mul(r)).Add(n.Mul(r)), c.Sub(d.Mul(r)).Sub(n.Mul(r))
		e, f := c.Add(d.Mul(r)).Sub(n.Mul(r)), c.Sub(d.Mul(r)).Add(n.Mul(r))
		st := stroke
		st.StrokeWidth = o.StrokeWidth * 1.4
		return []Item{NewPath(st).MoveTo(a.X, a.Y).LineTo(b.X, b.Y).MoveTo(e.X, e.Y).LineTo(f.X, f.Y)}
	case MarkerERExactlyOne, MarkerERZeroOrOne, MarkerEROneOrMore, MarkerERZeroOrMore:
		return erMarker(k, tip, d, n, o)
	}
	return nil
}

func erMarker(k MarkerKind, tip, d, n Point, o MarkerOpts) []Item {
	s := o.Size
	at := func(back, side float64) Point { return tip.Sub(d.Mul(back)).Add(n.Mul(side)) }
	st := Style{Stroke: o.Stroke, StrokeWidth: o.StrokeWidth, RoundCaps: true}
	p := NewPath(st)
	bar := func(back float64) {
		a, b := at(back, s*0.6), at(back, -s*0.6)
		p.MoveTo(a.X, a.Y).LineTo(b.X, b.Y)
	}
	var items []Item
	switch k {
	case MarkerERExactlyOne:
		bar(s * 0.6)
		bar(s * 1.1)
	case MarkerERZeroOrOne:
		bar(s * 0.6)
		c := at(s*1.5, 0)
		items = append(items, Circle(c.X, c.Y, s*0.4, Style{Fill: o.Hollow, Stroke: o.Stroke, StrokeWidth: o.StrokeWidth}))
	case MarkerEROneOrMore, MarkerERZeroOrMore:
		// crow's foot
		base := at(s*1.1, 0)
		a, b := at(0, s*0.6), at(0, -s*0.6)
		p.MoveTo(a.X, a.Y).LineTo(base.X, base.Y).LineTo(b.X, b.Y)
		c := at(0, 0)
		p.MoveTo(c.X, c.Y).LineTo(base.X, base.Y)
		if k == MarkerEROneOrMore {
			bar(s * 1.4)
		} else {
			cc := at(s*1.8, 0)
			items = append(items, Circle(cc.X, cc.Y, s*0.4, Style{Fill: o.Hollow, Stroke: o.Stroke, StrokeWidth: o.StrokeWidth}))
		}
	}
	return append([]Item{p}, items...)
}

// CurveKind selects how an edge polyline is drawn.
type CurveKind int

const (
	// CurveBasis draws a smooth B-spline (Mermaid's default "basis").
	CurveBasis CurveKind = iota
	// CurveLinear draws straight segments.
	CurveLinear
	// CurveRounded draws straight segments with rounded corners.
	CurveRounded
	// CurveCatmull draws a smooth interpolating curve through every point.
	CurveCatmull
)

// EdgeOpts configures Edge.
type EdgeOpts struct {
	Style      Style
	Curve      CurveKind
	Start, End MarkerKind
	Marker     MarkerOpts
}

// Edge draws a polyline edge with optional markers at either end. The
// polyline is shortened so that the line doesn't poke through markers.
func Edge(pts []Point, o EdgeOpts) []Item {
	if len(pts) < 2 {
		return nil
	}
	pts = append([]Point(nil), pts...)
	if o.Marker.Stroke.A == 0 {
		o.Marker.Stroke = o.Style.Stroke
	}
	if o.Marker.StrokeWidth == 0 {
		o.Marker.StrokeWidth = math.Max(1, o.Style.StrokeWidth*0.9)
	}
	if o.Marker.Size == 0 {
		o.Marker.Size = 9 + o.Style.StrokeWidth
	}
	var items []Item
	n := len(pts)
	endTip, endDir := pts[n-1], pts[n-1].Sub(pts[n-2])
	startTip, startDir := pts[0], pts[0].Sub(pts[1])
	if o.End != MarkerNone {
		pts[n-1] = shorten(pts[n-2], pts[n-1], MarkerInset(o.End, o.Marker.Size))
	}
	if o.Start != MarkerNone {
		pts[0] = shorten(pts[1], pts[0], MarkerInset(o.Start, o.Marker.Size))
	}
	p := NewPath(o.Style)
	switch o.Curve {
	case CurveLinear:
		p.Polyline(pts...)
	case CurveRounded:
		p.RoundedPolyline(pts, 8)
	case CurveCatmull:
		p.MonotoneSpline(pts)
	default:
		p.BasisSpline(pts)
	}
	items = append(items, p)
	items = append(items, Marker(o.End, endTip, endDir, o.Marker)...)
	items = append(items, Marker(o.Start, startTip, startDir, o.Marker)...)
	return items
}

// shorten moves b towards a by d (but never past a).
func shorten(a, b Point, d float64) Point {
	v := b.Sub(a)
	l := v.Len()
	if l == 0 || d <= 0 {
		return b
	}
	if d > l*0.9 {
		d = l * 0.9
	}
	return b.Sub(v.Mul(d / l))
}
