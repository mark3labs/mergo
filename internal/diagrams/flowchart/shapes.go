package flowchart

import (
	"image/color"
	"math"

	"github.com/mark3labs/mergo/internal/scene"
)

// Padding around node labels.
const (
	padX = 16.0
	padY = 11.0
)

// geom describes the geometry of a node shape.
type geom struct {
	// w, h is the full node box used by the layout.
	w, h float64
	// shape box inside the node box (relative to the node's top-left).
	sx, sy, sw, sh float64
	// text center relative to the node center
	tx, ty float64
	// outline builds the main outline path for a shape box.
	outline func(x, y, w, h float64) *scene.Path
	// deco adds decorations (drawn over the filled outline).
	deco func(x, y, w, h float64, st scene.Style) []scene.Item
	// under adds items drawn below the main outline (stacked shapes).
	under func(x, y, w, h float64, st scene.Style) []scene.Item
	// filledWithStroke paints the shape with the stroke color (junctions).
	filledWithStroke bool
	// noBox: text-only shapes.
	noBox bool
}

func rectPath(r float64) func(x, y, w, h float64) *scene.Path {
	return func(x, y, w, h float64) *scene.Path { return scene.NewPath(scene.Style{}).Rect(x, y, w, h, r) }
}

func polyPath(f func(x, y, w, h float64) []scene.Point) func(x, y, w, h float64) *scene.Path {
	return func(x, y, w, h float64) *scene.Path { return scene.NewPath(scene.Style{}).Polygon(f(x, y, w, h)...) }
}

func ellipsePath(x, y, w, h float64) *scene.Path {
	return scene.NewPath(scene.Style{}).Ellipse(x+w/2, y+h/2, w/2, h/2)
}

func pt(x, y float64) scene.Point { return scene.Point{X: x, Y: y} }

// strokeOnly returns st without fill/shadow.
func strokeOnly(st scene.Style) scene.Style {
	st.Fill = color.RGBA{}
	st.Shadow = false
	return st
}

// wavePath appends a wavy bottom edge from (x+w, y+h) to (x, y+h).
func waveBottom(p *scene.Path, x, y, w, h, amp float64) {
	// two half waves
	p.CubicTo(x+w*0.75, y+h+amp*1.6, x+w*0.5, y+h+amp*1.6, x+w*0.5, y+h)
	p.CubicTo(x+w*0.5, y+h-amp*1.6, x+w*0.25, y+h-amp*1.6, x, y+h)
}

func docPath(x, y, w, h float64) *scene.Path {
	amp := math.Min(h*0.12, 8)
	p := scene.NewPath(scene.Style{})
	p.MoveTo(x, y).LineTo(x+w, y).LineTo(x+w, y+h-amp)
	waveBottom(p, x, y, w, h-amp, amp)
	return p.Close()
}

func cylinderGeom(tw, th float64) geom {
	w := tw + 2*padX
	ry := w / 2 / (2.5 + w/50)
	h := th + 2*padY + 2*ry
	g := geom{w: w, h: h, ty: ry / 2}
	g.outline = func(x, y, w, h float64) *scene.Path {
		p := scene.NewPath(scene.Style{})
		p.MoveTo(x, y+ry)
		p.Arc(x+w/2, y+ry, w/2, ry, math.Pi, 2*math.Pi, true)
		p.LineTo(x+w, y+h-ry)
		p.Arc(x+w/2, y+h-ry, w/2, ry, 0, math.Pi, true)
		return p.Close()
	}
	g.deco = func(x, y, w, h float64, st scene.Style) []scene.Item {
		return []scene.Item{scene.NewPath(strokeOnly(st)).Arc(x+w/2, y+ry, w/2, ry, 0, math.Pi, false)}
	}
	return g
}

// shapeGeom computes the geometry of a shape for a text block of tw x th.
// hasLabel reports whether the node shows a label.
func shapeGeom(s Shape, tw, th float64, hasLabel bool) geom {
	bw, bh := tw+2*padX, th+2*padY
	var g geom
	switch s {
	case ShapeRounded:
		g = geom{w: bw, h: bh, outline: rectPath(math.Min(8, bh/3))}
	case ShapeStadium:
		w := bw + bh/2
		g = geom{w: w, h: bh, outline: rectPath(bh / 2)}
	case ShapeSubroutine:
		g = geom{w: bw + 16, h: bh, outline: rectPath(0)}
		g.deco = func(x, y, w, h float64, st scene.Style) []scene.Item {
			so := strokeOnly(st)
			return []scene.Item{
				scene.Line(x+8, y, x+8, y+h, so),
				scene.Line(x+w-8, y, x+w-8, y+h, so),
			}
		}
	case ShapeCylinder:
		g = cylinderGeom(tw, th)
	case "lin-cyl":
		g = cylinderGeom(tw, th)
		base := g.deco
		g.h += 6
		g.ty += 3
		g.deco = func(x, y, w, h float64, st scene.Style) []scene.Item {
			ry := w / 2 / (2.5 + w/50)
			items := base(x, y, w, h, st)
			return append(items, scene.NewPath(strokeOnly(st)).Arc(x+w/2, y+ry+7, w/2, ry, 0, math.Pi, false))
		}
	case "h-cyl":
		rx := bh / 2 / (2.5 + bh/50)
		w := bw + 2*rx
		g = geom{w: w, h: bh, tx: -rx / 2}
		g.outline = func(x, y, w, h float64) *scene.Path {
			p := scene.NewPath(scene.Style{})
			p.MoveTo(x+rx, y)
			p.LineTo(x+w-rx, y)
			p.Arc(x+w-rx, y+h/2, rx, h/2, -math.Pi/2, math.Pi/2, true)
			p.LineTo(x+rx, y+h)
			p.Arc(x+rx, y+h/2, rx, h/2, math.Pi/2, 3*math.Pi/2, true)
			return p.Close()
		}
		g.deco = func(x, y, w, h float64, st scene.Style) []scene.Item {
			return []scene.Item{scene.NewPath(strokeOnly(st)).Arc(x+w-rx, y+h/2, rx, h/2, math.Pi/2, 3*math.Pi/2, false)}
		}
	case ShapeCircle:
		d := math.Max(math.Max(tw, th)+2*padY+4, 40)
		if hasLabel {
			d = math.Max(d, math.Hypot(tw, th)+14)
		}
		g = geom{w: d, h: d, outline: ellipsePath}
	case ShapeDoubleCircle:
		d := math.Max(math.Max(tw, th)+2*padY+4, 40)
		d = math.Max(d, math.Hypot(tw, th)+14) + 10
		g = geom{w: d, h: d, outline: ellipsePath}
		g.deco = func(x, y, w, h float64, st scene.Style) []scene.Item {
			return []scene.Item{scene.NewPath(strokeOnly(st)).Circle(x+w/2, y+h/2, w/2-5)}
		}
	case ShapeDiamond:
		a := tw/2 + th/2*1.4 + 12
		b := th/2 + tw/2*0.4 + 12
		b = math.Max(b, 24)
		g = geom{w: 2 * a, h: 2 * b}
		g.outline = polyPath(func(x, y, w, h float64) []scene.Point {
			return []scene.Point{pt(x+w/2, y), pt(x+w, y+h/2), pt(x+w/2, y+h), pt(x, y+h/2)}
		})
	case ShapeHexagon:
		m := bh / 4
		g = geom{w: bw + 2*m, h: bh}
		g.outline = polyPath(func(x, y, w, h float64) []scene.Point {
			return []scene.Point{pt(x+m, y), pt(x+w-m, y), pt(x+w, y+h/2), pt(x+w-m, y+h), pt(x+m, y+h), pt(x, y+h/2)}
		})
	case ShapeLeanRight, ShapeLeanLeft:
		sk := bh * 0.45
		g = geom{w: bw + sk, h: bh}
		right := s == ShapeLeanRight
		g.outline = polyPath(func(x, y, w, h float64) []scene.Point {
			if right {
				return []scene.Point{pt(x+sk, y), pt(x+w, y), pt(x+w-sk, y+h), pt(x, y+h)}
			}
			return []scene.Point{pt(x, y), pt(x+w-sk, y), pt(x+w, y+h), pt(x+sk, y+h)}
		})
	case ShapeTrapezoid, ShapeTrapezoidAlt:
		sk := bh * 0.45
		g = geom{w: bw + 2*sk, h: bh}
		bottomWide := s == ShapeTrapezoid
		g.outline = polyPath(func(x, y, w, h float64) []scene.Point {
			if bottomWide {
				return []scene.Point{pt(x+sk, y), pt(x+w-sk, y), pt(x+w, y+h), pt(x, y+h)}
			}
			return []scene.Point{pt(x, y), pt(x+w, y), pt(x+w-sk, y+h), pt(x+sk, y+h)}
		})
	case ShapeOdd:
		n := bh / 3
		g = geom{w: bw + n, h: bh, tx: n / 2}
		g.outline = polyPath(func(x, y, w, h float64) []scene.Point {
			return []scene.Point{pt(x, y), pt(x+w, y), pt(x+w, y+h), pt(x, y+h), pt(x+n, y+h/2)}
		})
	case "doc":
		amp := math.Min(bh*0.12, 8)
		g = geom{w: bw, h: bh + amp, ty: -amp / 2, outline: docPath}
	case "lin-doc":
		amp := math.Min(bh*0.12, 8)
		g = geom{w: bw + 10, h: bh + amp, ty: -amp / 2, tx: 5, outline: docPath}
		g.deco = func(x, y, w, h float64, st scene.Style) []scene.Item {
			return []scene.Item{scene.Line(x+10, y, x+10, y+h-amp*0.9, strokeOnly(st))}
		}
	case "docs":
		amp := math.Min(bh*0.12, 8)
		g = geom{w: bw + 10, h: bh + amp + 10, ty: -amp/2 + 5, tx: -5}
		g.sx, g.sy, g.sw, g.sh = 0, 10, bw, bh+amp
		g.outline = docPath
		g.under = func(x, y, w, h float64, st scene.Style) []scene.Item {
			st.Shadow = false
			a := docPath(x+10, y-10, w, h)
			a.Style = st
			b := docPath(x+5, y-5, w, h)
			b.Style = st
			return []scene.Item{a, b}
		}
	case "tag-doc":
		amp := math.Min(bh*0.12, 8)
		g = geom{w: bw, h: bh + amp, ty: -amp / 2, outline: docPath}
		g.deco = func(x, y, w, h float64, st scene.Style) []scene.Item {
			t := math.Min(14, h/3)
			return []scene.Item{scene.NewPath(scene.Style{Fill: st.Stroke}).Polygon(
				pt(x+w, y+h-amp-t), pt(x+w, y+h-amp*0.2), pt(x+w-t, y+h-amp*0.2))}
		}
	case "notch-rect":
		c := math.Min(12, bh/3)
		g = geom{w: bw, h: bh}
		g.outline = polyPath(func(x, y, w, h float64) []scene.Point {
			return []scene.Point{pt(x+c, y), pt(x+w, y), pt(x+w, y+h), pt(x, y+h), pt(x, y+c)}
		})
	case "notch-pent":
		c := math.Min(12, bh/3)
		g = geom{w: bw + 10, h: bh}
		g.outline = polyPath(func(x, y, w, h float64) []scene.Point {
			return []scene.Point{pt(x+c, y), pt(x+w-c, y), pt(x+w, y+c), pt(x+w, y+h), pt(x, y+h), pt(x, y+c)}
		})
	case "delay":
		g = geom{w: bw + bh/2, h: bh, tx: -bh / 4}
		g.outline = func(x, y, w, h float64) *scene.Path {
			p := scene.NewPath(scene.Style{})
			p.MoveTo(x, y).LineTo(x+w-h/2, y)
			p.Arc(x+w-h/2, y+h/2, h/2, h/2, -math.Pi/2, math.Pi/2, true)
			p.LineTo(x, y+h)
			return p.Close()
		}
	case "curv-trap":
		n := bh * 0.45
		g = geom{w: bw + n + bh/3, h: bh, tx: (n - bh/3) / 2}
		g.outline = func(x, y, w, h float64) *scene.Path {
			r := h / 3
			p := scene.NewPath(scene.Style{})
			p.MoveTo(x+n, y).LineTo(x+w-r, y)
			p.CubicTo(x+w+r*0.3, y, x+w+r*0.3, y+h, x+w-r, y+h)
			p.LineTo(x+n, y+h).LineTo(x, y+h/2)
			return p.Close()
		}
	case "div-rect":
		g = geom{w: bw, h: bh + 10, ty: 5, outline: rectPath(0)}
		g.deco = func(x, y, w, h float64, st scene.Style) []scene.Item {
			return []scene.Item{scene.Line(x, y+12, x+w, y+12, strokeOnly(st))}
		}
	case "lin-rect":
		g = geom{w: bw + 10, h: bh, tx: 5, outline: rectPath(0)}
		g.deco = func(x, y, w, h float64, st scene.Style) []scene.Item {
			return []scene.Item{scene.Line(x+10, y, x+10, y+h, strokeOnly(st))}
		}
	case "win-pane":
		g = geom{w: bw + 10, h: bh + 10, tx: 5, ty: 5, outline: rectPath(0)}
		g.deco = func(x, y, w, h float64, st scene.Style) []scene.Item {
			so := strokeOnly(st)
			return []scene.Item{scene.Line(x+10, y, x+10, y+h, so), scene.Line(x, y+10, x+w, y+10, so)}
		}
	case "tag-rect":
		g = geom{w: bw, h: bh, outline: rectPath(0)}
		g.deco = func(x, y, w, h float64, st scene.Style) []scene.Item {
			t := math.Min(14, h/3)
			return []scene.Item{scene.NewPath(scene.Style{Fill: st.Stroke}).Polygon(pt(x+w, y+h-t), pt(x+w, y+h), pt(x+w-t, y+h))}
		}
	case "st-rect":
		g = geom{w: bw + 10, h: bh + 10, tx: -5, ty: 5}
		g.sx, g.sy, g.sw, g.sh = 0, 10, bw, bh
		g.outline = rectPath(0)
		g.under = func(x, y, w, h float64, st scene.Style) []scene.Item {
			st.Shadow = false
			return []scene.Item{
				scene.RectPath(x+10, y-10, w, h, 0, st),
				scene.RectPath(x+5, y-5, w, h, 0, st),
			}
		}
	case "sl-rect":
		s2 := math.Min(bh*0.35, 14)
		g = geom{w: bw, h: bh + s2, ty: s2 / 2}
		g.outline = polyPath(func(x, y, w, h float64) []scene.Point {
			return []scene.Point{pt(x, y+s2), pt(x+w, y), pt(x+w, y+h), pt(x, y+h)}
		})
	case "bow-rect":
		r := bh / 4
		g = geom{w: bw + 2*r, h: bh}
		g.outline = func(x, y, w, h float64) *scene.Path {
			p := scene.NewPath(scene.Style{})
			p.MoveTo(x+r, y).LineTo(x+w, y)
			p.CubicTo(x+w-r*1.3, y+h*0.25, x+w-r*1.3, y+h*0.75, x+w, y+h)
			p.LineTo(x+r, y+h)
			p.CubicTo(x-r*0.3, y+h*0.75, x-r*0.3, y+h*0.25, x+r, y)
			return p.Close()
		}
	case "flag":
		amp := math.Min(bh*0.12, 7)
		g = geom{w: bw, h: bh + 2*amp}
		g.outline = func(x, y, w, h float64) *scene.Path {
			p := scene.NewPath(scene.Style{})
			p.MoveTo(x, y+amp)
			p.CubicTo(x+w*0.25, y-amp, x+w*0.25, y-amp, x+w*0.5, y+amp)
			p.CubicTo(x+w*0.75, y+3*amp, x+w*0.75, y+3*amp, x+w, y+amp)
			p.LineTo(x+w, y+h-amp)
			p.CubicTo(x+w*0.75, y+h+amp, x+w*0.75, y+h+amp, x+w*0.5, y+h-amp)
			p.CubicTo(x+w*0.25, y+h-3*amp, x+w*0.25, y+h-3*amp, x, y+h-amp)
			return p.Close()
		}
	case "tri", "flip-tri":
		up := s == "tri"
		w := math.Max(tw*2+2*padX, (th+2*padY)*2.2)
		h := w * 0.8
		ty := h / 6
		if !up {
			ty = -h / 6
		}
		g = geom{w: w, h: h, ty: ty}
		g.outline = polyPath(func(x, y, w, h float64) []scene.Point {
			if up {
				return []scene.Point{pt(x+w/2, y), pt(x+w, y+h), pt(x, y+h)}
			}
			return []scene.Point{pt(x, y), pt(x+w, y), pt(x+w/2, y+h)}
		})
	case "hourglass", "f-circ", "sm-circ", "fr-circ", "fork", "bolt", "cross-circ":
		g = iconGeom(s, tw, th, hasLabel)
	case "brace", "brace-r", "braces":
		g = geom{w: bw + 8, h: bh, noBox: true}
		left := s != "brace-r"
		right := s != "brace"
		g.deco = func(x, y, w, h float64, st scene.Style) []scene.Item {
			so := strokeOnly(st)
			var items []scene.Item
			if left {
				items = append(items, bracePath(x+8, y, h, -1, so))
			}
			if right {
				items = append(items, bracePath(x+w-8, y, h, 1, so))
			}
			return items
		}
	case "text":
		g = geom{w: tw + 8, h: th + 8, noBox: true}
	default: // rect
		g = geom{w: bw, h: bh, outline: rectPath(0)}
	}
	if g.sw == 0 && g.sh == 0 {
		g.sw, g.sh = g.w, g.h
	}
	return g
}

// bracePath draws a curly brace along a vertical line at x.
func bracePath(x, y, h, dir float64, st scene.Style) *scene.Path {
	d := 8 * dir
	p := scene.NewPath(st)
	p.MoveTo(x-d, y)
	p.QuadTo(x, y, x, y+8)
	p.LineTo(x, y+h/2-8)
	p.QuadTo(x, y+h/2, x+d, y+h/2)
	p.QuadTo(x, y+h/2, x, y+h/2+8)
	p.LineTo(x, y+h-8)
	p.QuadTo(x, y+h, x-d, y+h)
	return p
}

// iconGeom handles small symbol shapes whose label (if any) goes below.
func iconGeom(s Shape, tw, th float64, hasLabel bool) geom {
	var sw, sh float64
	switch s {
	case "f-circ":
		sw, sh = 16, 16
	case "sm-circ":
		sw, sh = 16, 16
	case "fr-circ":
		sw, sh = 22, 22
	case "fork":
		sw, sh = 70, 10
	case "hourglass":
		sw, sh = 40, 40
	case "bolt":
		sw, sh = 26, 44
	case "cross-circ":
		sw, sh = 34, 34
	}
	g := geom{w: sw, h: sh, sw: sw, sh: sh}
	if hasLabel {
		g.w = math.Max(sw, tw+8)
		g.h = sh + 6 + th
		g.sx = (g.w - sw) / 2
		g.ty = (g.h / 2) - th/2
	}
	switch s {
	case "f-circ", "sm-circ":
		g.outline = ellipsePath
		g.filledWithStroke = s == "f-circ"
	case "fr-circ":
		g.outline = ellipsePath
		g.deco = func(x, y, w, h float64, st scene.Style) []scene.Item {
			return []scene.Item{scene.Circle(x+w/2, y+h/2, w/2-5, scene.Style{Fill: st.Stroke})}
		}
	case "fork":
		g.outline = rectPath(2)
		g.filledWithStroke = true
	case "hourglass":
		g.outline = polyPath(func(x, y, w, h float64) []scene.Point {
			return []scene.Point{pt(x, y), pt(x+w, y), pt(x, y+h), pt(x+w, y+h)}
		})
	case "bolt":
		g.outline = polyPath(func(x, y, w, h float64) []scene.Point {
			return []scene.Point{pt(x+w*0.55, y), pt(x+w*0.1, y+h*0.55), pt(x+w*0.5, y+h*0.5), pt(x+w*0.35, y+h), pt(x+w*0.9, y+h*0.4), pt(x+w*0.5, y+h*0.45)}
		})
		g.filledWithStroke = true
	case "cross-circ":
		g.outline = ellipsePath
		g.deco = func(x, y, w, h float64, st scene.Style) []scene.Item {
			so := strokeOnly(st)
			r := w / 2 * 0.7071
			cx, cy := x+w/2, y+h/2
			return []scene.Item{scene.Line(cx-r, cy-r, cx+r, cy+r, so), scene.Line(cx-r, cy+r, cx+r, cy-r, so)}
		}
	}
	return g
}
