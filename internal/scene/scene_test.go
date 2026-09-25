package scene

import (
	"image/color"
	"math"
	"testing"
)

func TestTextMetrics(t *testing.T) {
	f := Font{Size: 16}
	w1 := MeasureText("Hello", f)
	w2 := MeasureText("Hello", Font{Size: 32})
	if w1 <= 0 || math.Abs(w2-2*w1) > 0.5 {
		t.Errorf("text width not proportional: %v %v", w1, w2)
	}
	if MeasureText("Hello", Font{Size: 16, Bold: true}) <= w1 {
		t.Error("bold should be wider")
	}
	w, h := MeasureBlock("a\nlonger line", f, 0)
	if h != 2*16*1.25 || w != MeasureText("longer line", f) {
		t.Errorf("block %v x %v", w, h)
	}
	wrapped := WrapText("the quick brown fox jumps over the lazy dog", f, 100)
	for _, l := range splitLines(wrapped) {
		if MeasureText(l, f) > 100 && len(l) > 6 {
			t.Errorf("line too long: %q", l)
		}
	}
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

func TestFitAndRender(t *testing.T) {
	sc := New(color.RGBA{255, 255, 255, 255})
	sc.Add(RectPath(100, 50, 40, 20, 3, Style{Fill: color.RGBA{255, 0, 0, 255}, Shadow: true}))
	sc.Add(NewText(120, 100, "hi", Font{Size: 12}, color.RGBA{0, 0, 0, 255}, AnchorMiddle, VAlignMiddle))
	sc.Fit(10)
	b := sc.Bounds()
	if math.Abs(b.X-10) > 0.01 || math.Abs(b.Y-10) > 0.01 {
		t.Errorf("fit bounds %+v", b)
	}
	img := sc.Render(RenderOptions{Scale: 2})
	if img.Bounds().Dx() != int(math.Ceil(sc.Width*2)) {
		t.Errorf("render size %v for width %v", img.Bounds(), sc.Width)
	}
	// center of the rect is red
	p := img.RGBAAt(int((10+20)*2), int((10+10)*2))
	if p.R < 200 || p.G > 60 {
		t.Errorf("expected red pixel, got %v", p)
	}
	// viewport rendering
	vp := sc.Render(RenderOptions{Scale: 1, Viewport: Rect{X: 10, Y: 10, W: 40, H: 20}})
	if vp.Bounds().Dx() != 40 || vp.RGBAAt(20, 10).R < 200 {
		t.Errorf("viewport render wrong: %v %v", vp.Bounds(), vp.RGBAAt(20, 10))
	}
}

func TestRotatedTextBounds(t *testing.T) {
	tx := &Text{X: 0, Y: 0, S: "long label here", Font: Font{Size: 16}, Anchor: AnchorStart, VAlign: VAlignMiddle, Rotate: -90}
	b := tx.bounds()
	if b.H < b.W {
		t.Errorf("rotated text should be taller than wide: %+v", b)
	}
}
