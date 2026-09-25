package scene

import (
	"image/color"
	"math"
	"strings"
	"testing"
)

func styled(s string) string {
	return strings.NewReplacer("[b]", string(BoldOn), "[/b]", string(BoldOff),
		"[i]", string(ItalicOn), "[/i]", string(ItalicOff)).Replace(s)
}

func TestStyledMeasure(t *testing.T) {
	f := Font{Size: 16}
	reg := MeasureText("plain bold", f)
	mixed := MeasureText(styled("plain [b]bold[/b]"), f)
	want := MeasureText("plain ", f) + MeasureText("bold", Font{Size: 16, Bold: true})
	if math.Abs(mixed-want) > 0.01 || mixed <= reg {
		t.Errorf("styled width %v, want %v (regular %v)", mixed, want, reg)
	}
	if got := MeasureText(styled("[i][/i]"), f); got != 0 {
		t.Errorf("markers only should measure 0, got %v", got)
	}
	// a bold base font stays bold after a span closes
	if got, want := MeasureText(styled("[b]x[/b]y"), Font{Size: 16, Bold: true}), MeasureText("xy", Font{Size: 16, Bold: true}); math.Abs(got-want) > 0.5 {
		t.Errorf("bold base: %v vs %v", got, want)
	}
}

func TestStyleLines(t *testing.T) {
	for in, want := range map[string]string{
		"a\nb":                   "a\nb",
		"[b]a\nb[/b] c":          "[b]a[/b]\n[b]b[/b] c",
		"[b][i]x\ny[/i][/b]":     "[b][i]x[/i][/b]\n[b][i]y[/i][/b]",
		"[b]x[/b]\n[i][/i]\nz":   "[b]x[/b]\n\nz",
		"[i]a\n[b]b\nc[/b][/i]d": "[i]a[/i]\n[i][b]b[/i][/b]\n[b][i]c[/b][/i]d",
	} {
		if got := StyleLines(styled(in)); got != styled(want) {
			t.Errorf("StyleLines(%q) = %q, want %q", in, got, styled(want))
		}
	}
	if StripStyle(styled("[b]a[/b] [i]b[/i]")) != "a b" {
		t.Error("StripStyle")
	}
}

func TestWrapStyled(t *testing.T) {
	f := Font{Size: 16}
	got := WrapText(styled("[b]one two three four[/b] five"), f, 70)
	lines := strings.Split(got, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected wrapping, got %q", got)
	}
	// every line must be self-contained: bold words stay bold after the wrap
	for _, l := range lines {
		for _, r := range runs(l, f) {
			bold := !strings.Contains(r.s, "five")
			if strings.TrimSpace(r.s) != "" && r.f.Bold != bold {
				t.Errorf("run %q bold=%v in %q", r.s, r.f.Bold, got)
			}
		}
	}
}

func TestRenderStyled(t *testing.T) {
	// bold glyphs cover more pixels than regular ones
	ink := func(s string) int {
		sc := New(color.RGBA{255, 255, 255, 255})
		sc.Add(NewText(0, 0, s, Font{Size: 24}, color.RGBA{0, 0, 0, 255}, AnchorStart, VAlignTop))
		sc.Fit(4)
		img := sc.Render(RenderOptions{Scale: 1, NoShadows: true})
		n := 0
		for i := 0; i < len(img.Pix); i += 4 {
			if img.Pix[i] < 128 {
				n++
			}
		}
		return n
	}
	if reg, bold := ink("Mermaid"), ink(styled("[b]Mermaid[/b]")); bold <= reg {
		t.Errorf("bold span not rendered bold: %d vs %d dark pixels", bold, reg)
	}
}
