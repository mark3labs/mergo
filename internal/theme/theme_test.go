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
	for _, n := range Names() {
		th, err := Get(n)
		if err != nil {
			t.Fatal(err)
		}
		if len(th.Palette) == 0 || th.FontSize == 0 || len(th.PaletteText) != len(th.Palette) {
			t.Errorf("%s incomplete", n)
		}
		// primary text must be readable on the primary color
		if d := Luminance(th.PrimaryColor) - Luminance(th.PrimaryTextColor); d*d < 0.1 {
			t.Errorf("%s: poor primary contrast", n)
		}
		c := th.Clone()
		c.Palette[0] = color.RGBA{}
		if th.Palette[0] == (color.RGBA{}) {
			t.Error("Clone is shallow")
		}
	}
	if _, err := Get("unknown"); err == nil {
		t.Error("expected error")
	}
}
