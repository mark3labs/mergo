package scene

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"strings"

	"github.com/fogleman/gg"
	"golang.org/x/image/font"
)

// RenderOptions controls rasterization.
type RenderOptions struct {
	// Scale is the number of device pixels per scene unit (default 1).
	Scale float64
	// Viewport selects the region of the scene (in scene units) to render.
	// A zero viewport renders the whole scene.
	Viewport Rect
	// Background overrides the scene background when non-nil.
	Background *color.RGBA
	// NoShadows disables drop shadows.
	NoShadows bool
}

func nrgba(c color.RGBA) color.NRGBA { return color.NRGBA(c) }

// Render rasterizes the scene.
func (s *Scene) Render(o RenderOptions) *image.RGBA {
	if o.Scale <= 0 {
		o.Scale = 1
	}
	vp := o.Viewport
	if vp.Empty() {
		vp = Rect{0, 0, s.Width, s.Height}
	}
	w := int(math.Ceil(vp.W * o.Scale))
	h := int(math.Ceil(vp.H * o.Scale))
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	bg := s.Background
	if o.Background != nil {
		bg = *o.Background
	}
	dc := gg.NewContext(w, h)
	if bg.A > 0 {
		dc.SetColor(nrgba(bg))
		dc.Clear()
	}
	tr := transform{scale: o.Scale, dx: -vp.X, dy: -vp.Y}

	// Shadow layer: all shadowed shapes are drawn into one alpha layer that is
	// blurred and composited beneath the content they belong to. To keep the
	// z-order sensible we composite the shadow of a run of items right before
	// the run is drawn.
	if !o.NoShadows && s.ShadowColor.A > 0 {
		s.drawShadows(dc, tr, w, h)
	}

	for _, it := range s.Items {
		drawItem(dc, it, tr)
	}
	return dc.Image().(*image.RGBA)
}

type transform struct {
	scale, dx, dy float64
}

func (t transform) pt(p Point) (float64, float64) {
	return (p.X + t.dx) * t.scale, (p.Y + t.dy) * t.scale
}

func drawItem(dc *gg.Context, it Item, tr transform) {
	switch v := it.(type) {
	case *Path:
		drawPath(dc, v, tr)
	case *Text:
		drawText(dc, v, tr)
	case *Group:
		for _, c := range v.Items {
			drawItem(dc, c, tr)
		}
	}
}

func tracePath(dc *gg.Context, p *Path, tr transform) {
	dc.ClearPath()
	for _, op := range p.ops {
		switch op.kind {
		case opMove:
			x, y := tr.pt(op.pts[0])
			dc.MoveTo(x, y)
		case opLine:
			x, y := tr.pt(op.pts[0])
			dc.LineTo(x, y)
		case opQuad:
			cx, cy := tr.pt(op.pts[0])
			x, y := tr.pt(op.pts[1])
			dc.QuadraticTo(cx, cy, x, y)
		case opCubic:
			c1x, c1y := tr.pt(op.pts[0])
			c2x, c2y := tr.pt(op.pts[1])
			x, y := tr.pt(op.pts[2])
			dc.CubicTo(c1x, c1y, c2x, c2y, x, y)
		case opClose:
			dc.ClosePath()
		}
	}
}

func drawPath(dc *gg.Context, p *Path, tr transform) {
	if len(p.ops) == 0 {
		return
	}
	st := p.Style
	if st.Fill.A > 0 {
		tracePath(dc, p, tr)
		dc.SetFillRuleWinding()
		dc.SetColor(nrgba(st.Fill))
		dc.Fill()
	}
	if st.Stroke.A > 0 && st.StrokeWidth > 0 {
		tracePath(dc, p, tr)
		dc.SetColor(nrgba(st.Stroke))
		dc.SetLineWidth(st.StrokeWidth * tr.scale)
		if st.RoundCaps {
			dc.SetLineCapRound()
			dc.SetLineJoinRound()
		} else {
			dc.SetLineCapButt()
			dc.SetLineJoinRound()
		}
		if len(st.Dash) > 0 {
			d := make([]float64, len(st.Dash))
			for i, v := range st.Dash {
				d[i] = v * tr.scale
			}
			dc.SetDash(d...)
		} else {
			dc.SetDash()
		}
		dc.Stroke()
		dc.SetDash()
	}
	dc.ClearPath()
}

func drawText(dc *gg.Context, t *Text, tr transform) {
	if t.S == "" || t.Color.A == 0 {
		return
	}
	px := t.Font.Size * tr.scale
	r := t.blockRect()
	lh := t.lineHeight()
	lines := strings.Split(t.S, "\n")
	widths := make([]float64, len(lines))
	lineRuns := make([][]textRun, len(lines))
	faces := map[Font]font.Face{}
	for i, l := range lines {
		widths[i] = MeasureText(l, t.Font)
		lineRuns[i] = runs(l, t.Font)
		for _, rn := range lineRuns[i] {
			if _, ok := faces[rn.f]; !ok {
				faces[rn.f] = face(rn.f, px)
			}
		}
		for j := range lineRuns[i] {
			lineRuns[i][j].w = MeasureText(lineRuns[i][j].s, lineRuns[i][j].f)
		}
	}

	fontMu.Lock()
	defer fontMu.Unlock()
	dc.SetColor(nrgba(t.Color))
	dc.Push()
	if t.Rotate != 0 {
		ox, oy := tr.pt(Point{t.X, t.Y})
		dc.RotateAbout(gg.Radians(t.Rotate), ox, oy)
	}
	for i := range lines {
		if widths[i] == 0 {
			continue
		}
		lw := widths[i]
		var x float64
		switch t.Anchor {
		case AnchorStart:
			x = r.X
		case AnchorMiddle:
			x = r.X + (r.W-lw)/2
		case AnchorEnd:
			x = r.X + r.W - lw
		}
		baseline := r.Y + float64(i)*lh + baselineOffset(t.Font, lh)
		for _, rn := range lineRuns[i] {
			if rn.s != "" {
				dc.SetFontFace(faces[rn.f])
				bx, by := tr.pt(Point{x, baseline})
				dc.DrawString(rn.s, bx, by)
			}
			x += rn.w
		}
	}
	dc.Pop()
}

// drawShadows renders a blurred shadow for all items with Style.Shadow.
func (s *Scene) drawShadows(dc *gg.Context, tr transform, w, h int) {
	var shadowed []*Path
	var collect func(items []Item)
	collect = func(items []Item) {
		for _, it := range items {
			switch v := it.(type) {
			case *Path:
				if v.Style.Shadow && v.Style.Fill.A > 0 {
					shadowed = append(shadowed, v)
				}
			case *Group:
				collect(v.Items)
			}
		}
	}
	collect(s.Items)
	if len(shadowed) == 0 {
		return
	}
	off := 2.5 * tr.scale
	blur := int(math.Max(1, math.Round(3*tr.scale)))
	sdc := gg.NewContext(w, h)
	sdc.SetColor(color.White)
	str := transform{scale: tr.scale, dx: tr.dx + off/tr.scale*0.6, dy: tr.dy + off/tr.scale}
	for _, p := range shadowed {
		tracePath(sdc, p, str)
		sdc.Fill()
	}
	mask := image.NewAlpha(image.Rect(0, 0, w, h))
	src := sdc.Image().(*image.RGBA)
	for i := 0; i < w*h; i++ {
		mask.Pix[i] = src.Pix[i*4+3]
	}
	boxBlur(mask, blur)
	boxBlur(mask, blur)
	boxBlur(mask, blur)
	sc := nrgba(s.ShadowColor)
	draw.DrawMask(dc.Image().(*image.RGBA), image.Rect(0, 0, w, h), image.NewUniform(sc), image.Point{}, mask, image.Point{}, draw.Over)
}

// boxBlur applies a separable box blur of radius r in place.
func boxBlur(m *image.Alpha, r int) {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	if r <= 0 || w == 0 || h == 0 {
		return
	}
	tmp := make([]uint8, len(m.Pix))
	div := 2*r + 1
	// horizontal
	for y := range h {
		row := m.Pix[y*m.Stride : y*m.Stride+w]
		sum := 0
		for x := -r; x <= r; x++ {
			sum += int(row[clampi(x, 0, w-1)])
		}
		for x := range w {
			tmp[y*m.Stride+x] = uint8(sum / div)
			sum += int(row[clampi(x+r+1, 0, w-1)]) - int(row[clampi(x-r, 0, w-1)])
		}
	}
	// vertical
	for x := range w {
		sum := 0
		for y := -r; y <= r; y++ {
			sum += int(tmp[clampi(y, 0, h-1)*m.Stride+x])
		}
		for y := range h {
			m.Pix[y*m.Stride+x] = uint8(sum / div)
			sum += int(tmp[clampi(y+r+1, 0, h-1)*m.Stride+x]) - int(tmp[clampi(y-r, 0, h-1)*m.Stride+x])
		}
	}
}

func clampi(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// RenderToPNG encodes an image as PNG bytes.
func RenderToPNG(img *image.RGBA) []byte {
	if img == nil {
		return nil
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil
	}
	return buf.Bytes()
}
