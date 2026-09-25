package theme

import (
	"image/color"
	"testing"
)

func TestParseColor(t *testing.T) {
	for in, want := range map[string]color.RGBA{
		"#fff":               {255, 255, 255, 255},
		"#FF000080":          {255, 0, 0, 128},
		"rgb(1, 2, 3)":       {1, 2, 3, 255},
		"rgba(10,20,30,0.5)": {10, 20, 30, 128},
		"hsl(0, 100%, 50%)":  {255, 0, 0, 255},
		"RebeccaPurple":      {0x66, 0x33, 0x99, 255},
		"#abc !important":    {0xaa, 0xbb, 0xcc, 255},
		"transparent":        {},
	} {
		got, err := ParseColor(in)
		if err != nil || got != want {
			t.Errorf("ParseColor(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ParseColor("nope"); err == nil {
		t.Error("expected error")
	}
}

func TestThemes(t *testing.T) {
	names := Names()
	if names[0] != DefaultName || len(names) != 23 {
		t.Fatalf("names = %v", names)
	}
	for _, n := range names {
		for _, dark := range []bool{false, true} {
			th, err := Get(n, dark)
			if err != nil {
				t.Fatal(err)
			}
			if th.Name != n || th.Dark != dark {
				t.Errorf("%s/%v: got name %q dark %v", n, dark, th.Name, th.Dark)
			}
			if len(th.Palette) == 0 || th.FontSize == 0 || len(th.PaletteText) != len(th.Palette) {
				t.Errorf("%s/%v incomplete", n, dark)
			}
			// text must be readable on node fills and the background
			for what, bg := range map[string]color.RGBA{"primary": th.PrimaryColor, "background": th.Background} {
				if r := contrast(bg, th.PrimaryTextColor); r < 4 {
					t.Errorf("%s/%v: poor contrast on %s: %.2f", n, dark, what, r)
				}
			}
			if dark != (Luminance(th.Background) < 0.3) {
				t.Errorf("%s/%v: background %v doesn't match the variant", n, dark, th.Background)
			}
			c := th.Clone()
			c.Palette[0] = color.RGBA{}
			if th.Palette[0] == (color.RGBA{}) {
				t.Error("Clone is shallow")
			}
		}
	}
	if _, err := Get("unknown", true); err == nil {
		t.Error("expected error")
	}
	if MustGet("unknown", true).Name != DefaultName {
		t.Error("MustGet should fall back to the default theme")
	}
}

func TestMermaidThemes(t *testing.T) {
	for _, n := range []string{"default", "dark", "forest", "neutral"} {
		if th, err := Mermaid(n); err != nil || th.Name != n {
			t.Errorf("Mermaid(%q) = %v, %v", n, th, err)
		}
	}
	if _, err := Mermaid("nord"); err == nil {
		t.Error("nord is not a mermaid theme")
	}
}

// contrast is the WCAG contrast ratio of two colors.
func contrast(a, b color.RGBA) float64 {
	la, lb := Luminance(a)+0.05, Luminance(b)+0.05
	return max(la, lb) / min(la, lb)
}
