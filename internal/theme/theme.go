// Package theme defines the color themes used to render Mermaid diagrams.
//
// The variable names mirror Mermaid's theme variables
// (https://mermaid.js.org/config/theming.html) so that `%%{init: {"themeVariables": {...}}}%%`
// overrides can be mapped onto them.
package theme

import (
	"fmt"
	"image/color"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Theme holds all colors and typography used by diagram renderers.
type Theme struct {
	Name     string
	Dark     bool
	FontSize float64

	Background color.RGBA

	PrimaryColor       color.RGBA // main node fill
	PrimaryTextColor   color.RGBA
	PrimaryBorderColor color.RGBA
	SecondaryColor     color.RGBA
	SecondaryTextColor color.RGBA
	SecondaryBorder    color.RGBA
	TertiaryColor      color.RGBA
	TertiaryTextColor  color.RGBA
	TertiaryBorder     color.RGBA

	TextColor          color.RGBA // general text (titles, labels outside shapes)
	LineColor          color.RGBA // edges
	EdgeLabelBg        color.RGBA
	ClusterBkg         color.RGBA
	ClusterBorder      color.RGBA
	TitleColor         color.RGBA
	NoteBkg            color.RGBA
	NoteBorder         color.RGBA
	NoteText           color.RGBA
	ActorBkg           color.RGBA
	ActorBorder        color.RGBA
	ActorText          color.RGBA
	ActorLine          color.RGBA
	SignalColor        color.RGBA
	SignalText         color.RGBA
	LabelBoxBkg        color.RGBA
	LabelBoxBorder     color.RGBA
	LabelText          color.RGBA
	LoopText           color.RGBA
	ActivationBkg      color.RGBA
	ActivationBorder   color.RGBA
	SequenceNumberBg   color.RGBA
	SequenceNumberText color.RGBA

	// Palette used by pie, journey, mindmap, timeline, gantt sections, gitGraph...
	Palette []color.RGBA
	// ChartPalette holds saturated colors for data series (xychart, gitGraph
	// branches, quadrant points). Falls back to a Tableau-like palette.
	ChartPalette []color.RGBA
	// PaletteText is a readable text color for each palette entry.
	PaletteText []color.RGBA

	// Gantt specific.
	TaskBkg       color.RGBA
	TaskBorder    color.RGBA
	TaskText      color.RGBA
	ActiveTask    color.RGBA
	ActiveBorder  color.RGBA
	DoneTask      color.RGBA
	DoneBorder    color.RGBA
	CritTask      color.RGBA
	CritBorder    color.RGBA
	GridColor     color.RGBA
	TodayLine     color.RGBA
	SectionBkg    []color.RGBA
	ExcludeBkg    color.RGBA
	MilestoneFill color.RGBA

	// Misc
	ShadowColor color.RGBA
}

// Names lists the built-in theme names.
func Names() []string {
	n := make([]string, 0, len(builtins))
	for k := range builtins {
		n = append(n, k)
	}
	sort.Strings(n)
	return n
}

var builtins = map[string]func() *Theme{
	"default": Default,
	"dark":    DarkTheme,
	"forest":  Forest,
	"neutral": Neutral,
	"charm":   Charm,
}

// Get returns a fresh copy of the theme with the given name.
func Get(name string) (*Theme, error) {
	f, ok := builtins[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return nil, fmt.Errorf("unknown theme %q (available: %s)", name, strings.Join(Names(), ", "))
	}
	return f(), nil
}

// MustGet is like Get but falls back to the default theme.
func MustGet(name string) *Theme {
	t, err := Get(name)
	if err != nil {
		return Default()
	}
	return t
}

// Clone returns a deep copy of the theme.
func (t *Theme) Clone() *Theme {
	c := *t
	c.Palette = append([]color.RGBA(nil), t.Palette...)
	c.PaletteText = append([]color.RGBA(nil), t.PaletteText...)
	c.SectionBkg = append([]color.RGBA(nil), t.SectionBkg...)
	c.ChartPalette = append([]color.RGBA(nil), t.ChartPalette...)
	return &c
}

// PaletteColor returns palette color i (wrapping around).
func (t *Theme) PaletteColor(i int) color.RGBA {
	if len(t.Palette) == 0 {
		return t.PrimaryColor
	}
	if i < 0 {
		i = -i
	}
	return t.Palette[i%len(t.Palette)]
}

// ChartColor returns data series color i (wrapping around).
func (t *Theme) ChartColor(i int) color.RGBA {
	p := t.ChartPalette
	if len(p) == 0 {
		if t.Dark {
			p = chartDark
		} else {
			p = chartLight
		}
	}
	if i < 0 {
		i = -i
	}
	return p[i%len(p)]
}

var (
	chartLight = HexList("#4e79a7", "#f28e2b", "#e15759", "#59a14f", "#76b7b2", "#edc948", "#b07aa1", "#ff9da7", "#9c755f", "#7f7f7f")
	chartDark  = HexList("#6ea8fe", "#ffa94d", "#ff6b6b", "#69db7c", "#63e6be", "#ffd43b", "#da77f2", "#f783ac", "#c0a080", "#adb5bd")
)

// PaletteTextColor returns a readable text color for palette entry i.
func (t *Theme) PaletteTextColor(i int) color.RGBA {
	if len(t.PaletteText) == 0 {
		return ContrastText(t.PaletteColor(i))
	}
	if i < 0 {
		i = -i
	}
	return t.PaletteText[i%len(t.PaletteText)]
}

func finish(t *Theme) *Theme {
	if t.FontSize == 0 {
		t.FontSize = 16
	}
	if len(t.PaletteText) == 0 {
		for _, c := range t.Palette {
			t.PaletteText = append(t.PaletteText, ContrastText(c))
		}
	}
	return t
}

// Default mirrors Mermaid's "default" theme.
func Default() *Theme {
	return finish(&Theme{
		Name:               "default",
		Background:         Hex("#ffffff"),
		PrimaryColor:       Hex("#ECECFF"),
		PrimaryTextColor:   Hex("#131300"),
		PrimaryBorderColor: Hex("#9370DB"),
		SecondaryColor:     Hex("#ffffde"),
		SecondaryTextColor: Hex("#000021"),
		SecondaryBorder:    Hex("#aaaa33"),
		TertiaryColor:      Hex("#f4f4ff"),
		TertiaryTextColor:  Hex("#131300"),
		TertiaryBorder:     Hex("#c9c9ff"),
		TextColor:          Hex("#333333"),
		LineColor:          Hex("#333333"),
		EdgeLabelBg:        Hex("#e8e8e8"),
		ClusterBkg:         Hex("#ffffde"),
		ClusterBorder:      Hex("#aaaa33"),
		TitleColor:         Hex("#333333"),
		NoteBkg:            Hex("#fff5ad"),
		NoteBorder:         Hex("#aaaa33"),
		NoteText:           Hex("#333333"),
		ActorBkg:           Hex("#ECECFF"),
		ActorBorder:        Hex("#9370DB"),
		ActorText:          Hex("#000000"),
		ActorLine:          Hex("#999999"),
		SignalColor:        Hex("#333333"),
		SignalText:         Hex("#333333"),
		LabelBoxBkg:        Hex("#ECECFF"),
		LabelBoxBorder:     Hex("#9370DB"),
		LabelText:          Hex("#000000"),
		LoopText:           Hex("#000000"),
		ActivationBkg:      Hex("#f4f4f4"),
		ActivationBorder:   Hex("#666666"),
		SequenceNumberBg:   Hex("#333333"),
		SequenceNumberText: Hex("#ffffff"),
		Palette: HexList("#ECECFF", "#ffffde", "#b9b9ff", "#b3b3ff", "#ffff99", "#d6d6ff",
			"#c4c4ff", "#ffe6a8", "#a7a7ff", "#ffd3ab", "#dcdcff", "#ffffb3"),
		TaskBkg:       Hex("#8a90dd"),
		TaskBorder:    Hex("#534fbc"),
		TaskText:      Hex("#ffffff"),
		ActiveTask:    Hex("#bfc7ff"),
		ActiveBorder:  Hex("#534fbc"),
		DoneTask:      Hex("#d3d3d3"),
		DoneBorder:    Hex("#808080"),
		CritTask:      Hex("#ff8888"),
		CritBorder:    Hex("#ff0000"),
		GridColor:     Hex("#d3d3d3"),
		TodayLine:     Hex("#ff0000"),
		SectionBkg:    HexList("#6666ff33", "#ffffff00", "#fff40033", "#ffffff00"),
		ExcludeBkg:    Hex("#eeeeee"),
		MilestoneFill: Hex("#534fbc"),
		ShadowColor:   RGBA(0, 0, 0, 28),
	})
}

// DarkTheme mirrors Mermaid's "dark" theme.
func DarkTheme() *Theme {
	return finish(&Theme{
		Name:               "dark",
		Dark:               true,
		Background:         Hex("#1e1e2e"),
		PrimaryColor:       Hex("#1f2020"),
		PrimaryTextColor:   Hex("#e0dfdf"),
		PrimaryBorderColor: Hex("#cccccc"),
		SecondaryColor:     Hex("#3a3a45"),
		SecondaryTextColor: Hex("#e0dfdf"),
		SecondaryBorder:    Hex("#8a8a99"),
		TertiaryColor:      Hex("#2b2b35"),
		TertiaryTextColor:  Hex("#e0dfdf"),
		TertiaryBorder:     Hex("#6d6d7d"),
		TextColor:          Hex("#cccccc"),
		LineColor:          Hex("#d3d3d3"),
		EdgeLabelBg:        Hex("#585858"),
		ClusterBkg:         Hex("#2b2b3a"),
		ClusterBorder:      Hex("#7c7c96"),
		TitleColor:         Hex("#f9fffe"),
		NoteBkg:            Hex("#5d5a3c"),
		NoteBorder:         Hex("#a3a07a"),
		NoteText:           Hex("#f0f0e0"),
		ActorBkg:           Hex("#1f2020"),
		ActorBorder:        Hex("#cccccc"),
		ActorText:          Hex("#e0dfdf"),
		ActorLine:          Hex("#8a8a8a"),
		SignalColor:        Hex("#d3d3d3"),
		SignalText:         Hex("#e0dfdf"),
		LabelBoxBkg:        Hex("#1f2020"),
		LabelBoxBorder:     Hex("#cccccc"),
		LabelText:          Hex("#e0dfdf"),
		LoopText:           Hex("#e0dfdf"),
		ActivationBkg:      Hex("#3a3a45"),
		ActivationBorder:   Hex("#cccccc"),
		SequenceNumberBg:   Hex("#d3d3d3"),
		SequenceNumberText: Hex("#1e1e2e"),
		Palette: HexList("#4c6ef5", "#f76707", "#37b24d", "#ae3ec9", "#1098ad", "#f59f00",
			"#e64980", "#74b816", "#7048e8", "#d9480f", "#0ca678", "#495057"),
		TaskBkg:       Hex("#4c6ef5"),
		TaskBorder:    Hex("#91a7ff"),
		TaskText:      Hex("#ffffff"),
		ActiveTask:    Hex("#748ffc"),
		ActiveBorder:  Hex("#bac8ff"),
		DoneTask:      Hex("#6c6c7c"),
		DoneBorder:    Hex("#a0a0b0"),
		CritTask:      Hex("#e03131"),
		CritBorder:    Hex("#ff8787"),
		GridColor:     Hex("#44445a"),
		TodayLine:     Hex("#ff6b6b"),
		SectionBkg:    HexList("#4c6ef522", "#ffffff00", "#f59f0022", "#ffffff00"),
		ExcludeBkg:    Hex("#2a2a36"),
		MilestoneFill: Hex("#91a7ff"),
		ShadowColor:   RGBA(0, 0, 0, 70),
	})
}

// Forest mirrors Mermaid's "forest" theme.
func Forest() *Theme {
	return finish(&Theme{
		Name:               "forest",
		ChartPalette:       HexList("#2e7d32", "#9e9d24", "#00897b", "#558b2f", "#f9a825", "#6d4c41", "#43a047", "#827717", "#26a69a", "#795548"),
		Background:         Hex("#ffffff"),
		PrimaryColor:       Hex("#cde498"),
		PrimaryTextColor:   Hex("#000000"),
		PrimaryBorderColor: Hex("#13540c"),
		SecondaryColor:     Hex("#cdffb2"),
		SecondaryTextColor: Hex("#000000"),
		SecondaryBorder:    Hex("#6eaa49"),
		TertiaryColor:      Hex("#eeffe0"),
		TertiaryTextColor:  Hex("#000000"),
		TertiaryBorder:     Hex("#6eaa49"),
		TextColor:          Hex("#333333"),
		LineColor:          Hex("#008000"),
		EdgeLabelBg:        Hex("#e8e8e8"),
		ClusterBkg:         Hex("#cdffb2"),
		ClusterBorder:      Hex("#6eaa49"),
		TitleColor:         Hex("#333333"),
		NoteBkg:            Hex("#fff5ad"),
		NoteBorder:         Hex("#6eaa49"),
		NoteText:           Hex("#333333"),
		ActorBkg:           Hex("#cde498"),
		ActorBorder:        Hex("#13540c"),
		ActorText:          Hex("#000000"),
		ActorLine:          Hex("#6eaa49"),
		SignalColor:        Hex("#333333"),
		SignalText:         Hex("#333333"),
		LabelBoxBkg:        Hex("#cde498"),
		LabelBoxBorder:     Hex("#13540c"),
		LabelText:          Hex("#000000"),
		LoopText:           Hex("#000000"),
		ActivationBkg:      Hex("#e8f5d0"),
		ActivationBorder:   Hex("#13540c"),
		SequenceNumberBg:   Hex("#13540c"),
		SequenceNumberText: Hex("#ffffff"),
		Palette: HexList("#cde498", "#cdffb2", "#9ccc65", "#dce775", "#aed581", "#81c784",
			"#c5e1a5", "#fff59d", "#a5d6a7", "#e6ee9c", "#b2dfdb", "#f0f4c3"),
		TaskBkg:       Hex("#487e3a"),
		TaskBorder:    Hex("#13540c"),
		TaskText:      Hex("#ffffff"),
		ActiveTask:    Hex("#cde498"),
		ActiveBorder:  Hex("#13540c"),
		DoneTask:      Hex("#d3d3d3"),
		DoneBorder:    Hex("#808080"),
		CritTask:      Hex("#ff8888"),
		CritBorder:    Hex("#ff0000"),
		GridColor:     Hex("#d3d3d3"),
		TodayLine:     Hex("#ff0000"),
		SectionBkg:    HexList("#6eaa4933", "#ffffff00", "#cde49855", "#ffffff00"),
		ExcludeBkg:    Hex("#eeeeee"),
		MilestoneFill: Hex("#13540c"),
		ShadowColor:   RGBA(0, 0, 0, 28),
	})
}

// Neutral mirrors Mermaid's "neutral" theme (great for printing).
func Neutral() *Theme {
	return finish(&Theme{
		Name:               "neutral",
		ChartPalette:       HexList("#333333", "#777777", "#aaaaaa", "#555555", "#999999", "#444444", "#888888", "#666666", "#bbbbbb", "#222222"),
		Background:         Hex("#ffffff"),
		PrimaryColor:       Hex("#eeeeee"),
		PrimaryTextColor:   Hex("#111111"),
		PrimaryBorderColor: Hex("#999999"),
		SecondaryColor:     Hex("#f4f4f4"),
		SecondaryTextColor: Hex("#111111"),
		SecondaryBorder:    Hex("#bbbbbb"),
		TertiaryColor:      Hex("#fafafa"),
		TertiaryTextColor:  Hex("#111111"),
		TertiaryBorder:     Hex("#cccccc"),
		TextColor:          Hex("#333333"),
		LineColor:          Hex("#666666"),
		EdgeLabelBg:        Hex("#ffffff"),
		ClusterBkg:         Hex("#f7f7f7"),
		ClusterBorder:      Hex("#bbbbbb"),
		TitleColor:         Hex("#333333"),
		NoteBkg:            Hex("#ffffff"),
		NoteBorder:         Hex("#999999"),
		NoteText:           Hex("#333333"),
		ActorBkg:           Hex("#eeeeee"),
		ActorBorder:        Hex("#999999"),
		ActorText:          Hex("#111111"),
		ActorLine:          Hex("#666666"),
		SignalColor:        Hex("#333333"),
		SignalText:         Hex("#333333"),
		LabelBoxBkg:        Hex("#eeeeee"),
		LabelBoxBorder:     Hex("#999999"),
		LabelText:          Hex("#111111"),
		LoopText:           Hex("#111111"),
		ActivationBkg:      Hex("#f4f4f4"),
		ActivationBorder:   Hex("#666666"),
		SequenceNumberBg:   Hex("#333333"),
		SequenceNumberText: Hex("#ffffff"),
		Palette: HexList("#eeeeee", "#dddddd", "#cccccc", "#bbbbbb", "#aaaaaa", "#e6e6e6",
			"#d5d5d5", "#c4c4c4", "#b3b3b3", "#f2f2f2", "#e0e0e0", "#cfcfcf"),
		TaskBkg:       Hex("#707070"),
		TaskBorder:    Hex("#444444"),
		TaskText:      Hex("#ffffff"),
		ActiveTask:    Hex("#cccccc"),
		ActiveBorder:  Hex("#444444"),
		DoneTask:      Hex("#eeeeee"),
		DoneBorder:    Hex("#999999"),
		CritTask:      Hex("#d95757"),
		CritBorder:    Hex("#8b0000"),
		GridColor:     Hex("#dddddd"),
		TodayLine:     Hex("#d42"),
		SectionBkg:    HexList("#00000011", "#ffffff00", "#00000008", "#ffffff00"),
		ExcludeBkg:    Hex("#f4f4f4"),
		MilestoneFill: Hex("#444444"),
		ShadowColor:   RGBA(0, 0, 0, 22),
	})
}

// Charm is a dark theme inspired by the Charm color palette.
func Charm() *Theme {
	return finish(&Theme{
		Name:               "charm",
		ChartPalette:       HexList("#6b50ff", "#ff60ff", "#12c78f", "#00a4ff", "#fe8e66", "#e8fe96", "#ff577d", "#68ffd6", "#ffd65b", "#8b75ff"),
		Dark:               true,
		Background:         Hex("#171721"),
		PrimaryColor:       Hex("#2b2146"),
		PrimaryTextColor:   Hex("#f1efef"),
		PrimaryBorderColor: Hex("#9c7cf8"),
		SecondaryColor:     Hex("#123c3a"),
		SecondaryTextColor: Hex("#f1efef"),
		SecondaryBorder:    Hex("#12c78f"),
		TertiaryColor:      Hex("#3a1e33"),
		TertiaryTextColor:  Hex("#f1efef"),
		TertiaryBorder:     Hex("#ff60ff"),
		TextColor:          Hex("#dfdbdd"),
		LineColor:          Hex("#bfbcc8"),
		EdgeLabelBg:        Hex("#2d2c35"),
		ClusterBkg:         Hex("#201f2b"),
		ClusterBorder:      Hex("#6b50ff"),
		TitleColor:         Hex("#ff60ff"),
		NoteBkg:            Hex("#3b3419"),
		NoteBorder:         Hex("#e8fe96"),
		NoteText:           Hex("#fffbe0"),
		ActorBkg:           Hex("#2b2146"),
		ActorBorder:        Hex("#9c7cf8"),
		ActorText:          Hex("#f1efef"),
		ActorLine:          Hex("#605f6b"),
		SignalColor:        Hex("#bfbcc8"),
		SignalText:         Hex("#dfdbdd"),
		LabelBoxBkg:        Hex("#2b2146"),
		LabelBoxBorder:     Hex("#9c7cf8"),
		LabelText:          Hex("#f1efef"),
		LoopText:           Hex("#dfdbdd"),
		ActivationBkg:      Hex("#3f3160"),
		ActivationBorder:   Hex("#9c7cf8"),
		SequenceNumberBg:   Hex("#ff60ff"),
		SequenceNumberText: Hex("#171721"),
		Palette: HexList("#6b50ff", "#ff60ff", "#12c78f", "#00a4ff", "#fe8e66", "#e8fe96",
			"#ff577d", "#68ffd6", "#ffd65b", "#8b75ff", "#0adcd9", "#ff985a"),
		TaskBkg:       Hex("#6b50ff"),
		TaskBorder:    Hex("#9c7cf8"),
		TaskText:      Hex("#ffffff"),
		ActiveTask:    Hex("#00a4ff"),
		ActiveBorder:  Hex("#68ffd6"),
		DoneTask:      Hex("#4d4c57"),
		DoneBorder:    Hex("#858392"),
		CritTask:      Hex("#ff577d"),
		CritBorder:    Hex("#ffa5b8"),
		GridColor:     Hex("#3a3943"),
		TodayLine:     Hex("#ff60ff"),
		SectionBkg:    HexList("#6b50ff22", "#ffffff00", "#12c78f1c", "#ffffff00"),
		ExcludeBkg:    Hex("#24232d"),
		MilestoneFill: Hex("#ff60ff"),
		ShadowColor:   RGBA(0, 0, 0, 80),
	})
}

// ---------------------------------------------------------------------------
// Color helpers

// RGBA builds a non-premultiplied color (stored as color.RGBA for convenience;
// the renderer treats these as straight alpha).
func RGBA(r, g, b, a uint8) color.RGBA { return color.RGBA{r, g, b, a} }

// Hex parses "#rgb", "#rgba", "#rrggbb" or "#rrggbbaa". Invalid input yields magenta.
func Hex(s string) color.RGBA {
	c, err := ParseColor(s)
	if err != nil {
		return color.RGBA{255, 0, 255, 255}
	}
	return c
}

// HexList parses a list of hex colors.
func HexList(s ...string) []color.RGBA {
	out := make([]color.RGBA, len(s))
	for i, v := range s {
		out[i] = Hex(v)
	}
	return out
}

// ParseColor parses CSS-ish colors: hex, rgb(), rgba(), hsl(), hsla() and named colors.
func ParseColor(s string) (color.RGBA, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.TrimSuffix(s, "!important")
	s = strings.TrimSpace(s)
	if s == "" {
		return color.RGBA{}, fmt.Errorf("empty color")
	}
	if s == "none" || s == "transparent" {
		return color.RGBA{}, nil
	}
	if strings.HasPrefix(s, "#") {
		h := s[1:]
		expand := func(c byte) string { return string([]byte{c, c}) }
		switch len(h) {
		case 3:
			h = expand(h[0]) + expand(h[1]) + expand(h[2]) + "ff"
		case 4:
			h = expand(h[0]) + expand(h[1]) + expand(h[2]) + expand(h[3])
		case 6:
			h += "ff"
		case 8:
		default:
			return color.RGBA{}, fmt.Errorf("invalid hex color %q", s)
		}
		v, err := strconv.ParseUint(h, 16, 32)
		if err != nil {
			return color.RGBA{}, fmt.Errorf("invalid hex color %q", s)
		}
		return color.RGBA{uint8(v >> 24), uint8(v >> 16), uint8(v >> 8), uint8(v)}, nil
	}
	if i := strings.IndexByte(s, '('); i > 0 && strings.HasSuffix(s, ")") {
		fn := s[:i]
		args := strings.FieldsFunc(s[i+1:len(s)-1], func(r rune) bool { return r == ',' || r == ' ' || r == '/' })
		num := func(a string, scale float64) float64 {
			if before, ok := strings.CutSuffix(a, "%"); ok {
				v, _ := strconv.ParseFloat(before, 64)
				return v / 100 * scale
			}
			a = strings.TrimSuffix(a, "deg")
			v, _ := strconv.ParseFloat(a, 64)
			return v
		}
		alpha := 1.0
		if len(args) >= 4 {
			alpha = num(args[3], 1)
			if strings.HasSuffix(args[3], "%") {
				alpha = num(args[3], 1)
			}
		}
		if len(args) < 3 {
			return color.RGBA{}, fmt.Errorf("invalid color %q", s)
		}
		switch fn {
		case "rgb", "rgba":
			r, g, b := num(args[0], 255), num(args[1], 255), num(args[2], 255)
			return color.RGBA{clamp8(r), clamp8(g), clamp8(b), clamp8(alpha * 255)}, nil
		case "hsl", "hsla":
			h, sat, l := num(args[0], 360), num(args[1], 1), num(args[2], 1)
			if !strings.HasSuffix(args[1], "%") {
				sat /= 100
			}
			if !strings.HasSuffix(args[2], "%") {
				l /= 100
			}
			r, g, b := hslToRGB(h, sat, l)
			return color.RGBA{clamp8(r * 255), clamp8(g * 255), clamp8(b * 255), clamp8(alpha * 255)}, nil
		}
		return color.RGBA{}, fmt.Errorf("unsupported color function %q", fn)
	}
	if v, ok := namedColors[s]; ok {
		return Hex(v), nil
	}
	return color.RGBA{}, fmt.Errorf("unknown color %q", s)
}

func clamp8(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(math.Round(v))
}

func hslToRGB(h, s, l float64) (float64, float64, float64) {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return r + m, g + m, b + m
}

// Luminance returns relative luminance (0..1).
func Luminance(c color.RGBA) float64 {
	f := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*f(c.R) + 0.7152*f(c.G) + 0.0722*f(c.B)
}

// ContrastText returns black-ish or white-ish text depending on the background.
func ContrastText(bg color.RGBA) color.RGBA {
	if Luminance(bg) > 0.4 {
		return Hex("#1a1a1a")
	}
	return Hex("#f5f5f5")
}

// Mix blends a and b; t=0 gives a, t=1 gives b.
func Mix(a, b color.RGBA, t float64) color.RGBA {
	l := func(x, y uint8) uint8 { return clamp8(float64(x)*(1-t) + float64(y)*t) }
	return color.RGBA{l(a.R, b.R), l(a.G, b.G), l(a.B, b.B), l(a.A, b.A)}
}

// WithAlpha returns c with alpha a (0..255).
func WithAlpha(c color.RGBA, a uint8) color.RGBA { c.A = a; return c }

// Lighten moves a color towards white.
func Lighten(c color.RGBA, t float64) color.RGBA { return Mix(c, color.RGBA{255, 255, 255, c.A}, t) }

// Darken moves a color towards black.
func Darken(c color.RGBA, t float64) color.RGBA { return Mix(c, color.RGBA{0, 0, 0, c.A}, t) }

var namedColors = map[string]string{
	"black": "#000000", "white": "#ffffff", "red": "#ff0000", "green": "#008000", "blue": "#0000ff",
	"yellow": "#ffff00", "orange": "#ffa500", "purple": "#800080", "pink": "#ffc0cb", "gray": "#808080",
	"grey": "#808080", "lightgray": "#d3d3d3", "lightgrey": "#d3d3d3", "darkgray": "#a9a9a9",
	"darkgrey": "#a9a9a9", "silver": "#c0c0c0", "maroon": "#800000", "olive": "#808000",
	"lime": "#00ff00", "aqua": "#00ffff", "cyan": "#00ffff", "teal": "#008080", "navy": "#000080",
	"fuchsia": "#ff00ff", "magenta": "#ff00ff", "brown": "#a52a2a", "gold": "#ffd700",
	"coral": "#ff7f50", "salmon": "#fa8072", "tomato": "#ff6347", "crimson": "#dc143c",
	"indigo": "#4b0082", "violet": "#ee82ee", "orchid": "#da70d6", "plum": "#dda0dd",
	"khaki": "#f0e68c", "beige": "#f5f5dc", "ivory": "#fffff0", "lavender": "#e6e6fa",
	"skyblue": "#87ceeb", "lightblue": "#add8e6", "steelblue": "#4682b4", "royalblue": "#4169e1",
	"dodgerblue": "#1e90ff", "deepskyblue": "#00bfff", "darkblue": "#00008b", "lightgreen": "#90ee90",
	"darkgreen": "#006400", "forestgreen": "#228b22", "seagreen": "#2e8b57", "limegreen": "#32cd32",
	"yellowgreen": "#9acd32", "darkorange": "#ff8c00", "orangered": "#ff4500", "darkred": "#8b0000",
	"firebrick": "#b22222", "hotpink": "#ff69b4", "deeppink": "#ff1493", "lightpink": "#ffb6c1",
	"chocolate": "#d2691e", "tan": "#d2b48c", "wheat": "#f5deb3", "linen": "#faf0e6",
	"lightyellow": "#ffffe0", "lightcyan": "#e0ffff", "turquoise": "#40e0d0", "slategray": "#708090",
	"slategrey": "#708090", "darkslategray": "#2f4f4f", "dimgray": "#696969", "gainsboro": "#dcdcdc",
	"whitesmoke": "#f5f5f5", "aliceblue": "#f0f8ff", "honeydew": "#f0fff0", "mintcream": "#f5fffa",
	"azure": "#f0ffff", "snow": "#fffafa", "seashell": "#fff5ee", "oldlace": "#fdf5e6",
	"mistyrose": "#ffe4e1", "lemonchiffon": "#fffacd", "peachpuff": "#ffdab9", "rebeccapurple": "#663399",
	"darkviolet": "#9400d3", "mediumpurple": "#9370db", "slateblue": "#6a5acd", "cornflowerblue": "#6495ed",
	"cadetblue": "#5f9ea0", "darkcyan": "#008b8b", "aquamarine": "#7fffd4", "chartreuse": "#7fff00",
	"lawngreen": "#7cfc00", "olivedrab": "#6b8e23", "goldenrod": "#daa520", "darkgoldenrod": "#b8860b",
	"sienna": "#a0522d", "peru": "#cd853f", "rosybrown": "#bc8f8f", "indianred": "#cd5c5c",
	"lightcoral": "#f08080", "darksalmon": "#e9967a", "lightsalmon": "#ffa07a", "palegreen": "#98fb98",
	"paleturquoise": "#afeeee", "powderblue": "#b0e0e6", "thistle": "#d8bfd8", "moccasin": "#ffe4b5",
}
