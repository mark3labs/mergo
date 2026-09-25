package theme

import (
	"fmt"
	"image/color"
	"slices"
	"strings"
	"sync"
)

// DefaultName is the theme used when none is configured: mergo's own theme,
// built around the Go brand colors (gopyter's default).
const DefaultName = "mergo"

// UI is a theme's interface palette, the same one gopyter uses. Every
// selectable theme has a light and a dark UI palette; the diagram theme is
// derived from it, and the viewer draws its chrome with it.
type UI struct {
	// Accent colors.
	Primary, Info, Accent, Success, Warning, Error, Orange color.RGBA
	// Selection highlights the selected entry; Raised is a hovered surface.
	Selection, Raised color.RGBA
	// Neutral shades from the foreground (Text) to the background (Ink).
	Text, Dim, Muted, Subtle, Faint, Ink color.RGBA
	// Syntax colors.
	Keyword, Str, Number, Name color.RGBA
}

// themeDef is a named theme with a light and a dark variant.
type themeDef struct {
	name        string
	light, dark UI
}

// mergoTheme is mergo's own theme, identical to gopyter's default.
func mergoTheme() themeDef {
	return themeDef{
		name: DefaultName,
		dark: UI{
			Primary: Hex("#00ADD8"), Info: Hex("#5DC9E2"), Accent: Hex("#8B5CF6"),
			Success: Hex("#4ADE80"), Warning: Hex("#FDDD00"), Error: Hex("#F87171"),
			Orange:    Hex("#FB923C"),
			Selection: Hex("#1F4E6B"), Raised: Hex("#52525B"),
			Text: Hex("#E4E4E7"), Dim: Hex("#A1A1AA"), Muted: Hex("#71717A"),
			Subtle: Hex("#3F3F46"), Faint: Hex("#27272A"), Ink: Hex("#101014"),
			// catppuccin-mocha, gopyter's syntax style for this variant
			Keyword: Hex("#cba6f7"), Str: Hex("#a6e3a1"), Number: Hex("#fab387"), Name: Hex("#89b4fa"),
		},
		light: UI{
			Primary: Hex("#007D9C"), Info: Hex("#0E7490"), Accent: Hex("#7C3AED"),
			Success: Hex("#16A34A"), Warning: Hex("#B45309"), Error: Hex("#DC2626"),
			Orange:    Hex("#EA580C"),
			Selection: Hex("#BAE6FD"), Raised: Hex("#A1A1AA"),
			Text: Hex("#18181B"), Dim: Hex("#52525B"), Muted: Hex("#71717A"),
			Subtle: Hex("#C4C4CC"), Faint: Hex("#E4E4E7"), Ink: Hex("#FAFAFA"),
			// catppuccin-latte
			Keyword: Hex("#8839ef"), Str: Hex("#40a02b"), Number: Hex("#fe640b"), Name: Hex("#1e66f5"),
		},
	}
}

// kitColors mirrors the preset palette format of the kit coding agent
// (github.com/mark3labs/kit), so its themes can be shared verbatim. Each pair
// is {light, dark}; optional pairs left empty are derived.
type kitColors struct {
	primary, secondary, success, warning, error_, info      [2]string
	text, muted, veryMuted, background, border, mutedBorder [2]string
	system, tool, accent, highlight                         [2]string
	mdKeyword, mdString, mdNumber, mdComment, mdLink        [2]string
}

// kitPalette maps a kit theme variant (0 light, 1 dark) onto a palette, the
// same way gopyter does. Neutral shades not given by the theme are blended
// between its background and text, so they carry the theme's tint.
func kitPalette(k kitColors, i int) UI {
	pick := func(pairs ...[2]string) color.RGBA {
		for _, p := range pairs {
			if p[i] != "" {
				return Hex(p[i])
			}
		}
		return color.RGBA{}
	}
	bg, text := Hex(k.background[i]), Hex(k.text[i])
	shade := func(t float64) color.RGBA { return Mix(bg, text, t) }
	p := UI{
		Primary: Hex(k.primary[i]),
		Info:    pick(k.info, k.secondary, k.primary),
		Accent:  pick(k.accent, k.secondary, k.primary),
		Success: Hex(k.success[i]),
		Warning: Hex(k.warning[i]),
		Error:   Hex(k.error_[i]),
		Orange:  pick(k.tool),

		Selection: Mix(bg, Hex(k.primary[i]), 0.28),
		Raised:    shade(0.32),

		Text:   text,
		Dim:    shade(0.68),
		Muted:  pick(k.muted),
		Subtle: pick(k.border),
		Faint:  pick(k.mutedBorder),
		Ink:    bg,

		Keyword: pick(k.mdKeyword, k.primary),
		Str:     pick(k.mdString, k.success),
		Number:  pick(k.mdNumber, k.warning),
		Name:    pick(k.mdLink, k.info),
	}
	if p.Orange.A == 0 {
		p.Orange = Mix(p.Warning, p.Error, 0.4)
	}
	if p.Muted.A == 0 {
		p.Muted = shade(0.46)
	}
	if p.Subtle.A == 0 {
		p.Subtle = shade(0.22)
	}
	if p.Faint.A == 0 {
		p.Faint = shade(0.08)
	}
	return p
}

// themeDefs returns every selectable theme: mergo's own first, then the kit
// themes alphabetically (the same list and order as gopyter).
var themeDefs = sync.OnceValue(func() []themeDef {
	var kit []themeDef
	for name, k := range kitThemes {
		kit = append(kit, themeDef{name: name, light: kitPalette(k, 0), dark: kitPalette(k, 1)})
	}
	slices.SortFunc(kit, func(a, b themeDef) int { return strings.Compare(a.name, b.name) })
	return append([]themeDef{mergoTheme()}, kit...)
})

// Palette returns the UI palette of the named theme in its dark or light
// variant, for drawing interface chrome that matches the diagrams.
func Palette(name string, dark bool) (UI, error) {
	d, ok := lookupDef(name)
	if !ok {
		return UI{}, fmt.Errorf("unknown theme %q (available: %s)", name, strings.Join(Names(), ", "))
	}
	if dark {
		return d.dark, nil
	}
	return d.light, nil
}

func lookupDef(name string) (themeDef, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, d := range themeDefs() {
		if d.name == name {
			return d, true
		}
	}
	return themeDef{}, false
}

// fromPalette derives a full diagram theme from a UI palette: tinted fills
// with saturated borders on the theme background.
func fromPalette(name string, p UI, dark bool) *Theme {
	bg, text := p.Ink, p.Text
	tint := func(c color.RGBA, t float64) color.RGBA { return Mix(bg, c, t) }
	fill, strong := 0.16, 0.62 // node fills; saturated fills (bars, tasks)
	shadow := RGBA(0, 0, 0, 28)
	if dark {
		fill, strong = 0.22, 0.72
		shadow = RGBA(0, 0, 0, 70)
	}
	line := Mix(text, bg, 0.25)

	// Hues in the order gopyter's swatch shows them, then the syntax colors.
	var hues []color.RGBA
	for _, c := range []color.RGBA{p.Primary, p.Accent, p.Success, p.Warning, p.Info, p.Error, p.Orange, p.Keyword, p.Str, p.Name, p.Number} {
		if !slices.Contains(hues, c) {
			hues = append(hues, c)
		}
	}
	var pal []color.RGBA
	for _, t := range []float64{fill + 0.2, fill + 0.05} {
		for _, c := range hues {
			pal = append(pal, tint(c, t))
		}
	}

	return finish(&Theme{
		Name:       name,
		Dark:       dark,
		Background: bg,

		PrimaryColor:       tint(p.Primary, fill),
		PrimaryTextColor:   text,
		PrimaryBorderColor: p.Primary,
		SecondaryColor:     tint(p.Accent, fill),
		SecondaryTextColor: text,
		SecondaryBorder:    p.Accent,
		TertiaryColor:      tint(p.Info, fill*0.7),
		TertiaryTextColor:  text,
		TertiaryBorder:     p.Info,

		TextColor:     text,
		LineColor:     line,
		EdgeLabelBg:   tint(text, 0.1),
		ClusterBkg:    tint(text, 0.04),
		ClusterBorder: p.Subtle,
		TitleColor:    text,
		NoteBkg:       tint(p.Warning, fill),
		NoteBorder:    p.Warning,
		NoteText:      text,

		ActorBkg:           tint(p.Primary, fill),
		ActorBorder:        p.Primary,
		ActorText:          text,
		ActorLine:          p.Muted,
		SignalColor:        line,
		SignalText:         text,
		LabelBoxBkg:        tint(p.Accent, fill),
		LabelBoxBorder:     p.Accent,
		LabelText:          text,
		LoopText:           text,
		ActivationBkg:      tint(text, 0.12),
		ActivationBorder:   p.Muted,
		SequenceNumberBg:   p.Primary,
		SequenceNumberText: ContrastText(p.Primary),

		Palette:      pal,
		ChartPalette: hues,

		TaskBkg:       tint(p.Primary, strong),
		TaskBorder:    p.Primary,
		TaskText:      ContrastText(tint(p.Primary, strong)),
		ActiveTask:    tint(p.Primary, fill+0.1),
		ActiveBorder:  p.Primary,
		DoneTask:      tint(text, 0.2),
		DoneBorder:    p.Muted,
		CritTask:      tint(p.Error, strong),
		CritBorder:    p.Error,
		GridColor:     tint(text, 0.15),
		TodayLine:     p.Error,
		SectionBkg:    []color.RGBA{WithAlpha(p.Primary, 0x22), {}, WithAlpha(p.Warning, 0x22), {}},
		ExcludeBkg:    tint(text, 0.06),
		MilestoneFill: p.Primary,
		ShadowColor:   shadow,
	})
}

// kitThemes are the built-in themes of kit (github.com/mark3labs/kit), shared
// verbatim with gopyter so all three tools look alike.
var kitThemes = map[string]kitColors{
	"kitt": {
		primary: [2]string{"#CC1100", "#FF2200"}, secondary: [2]string{"#CC6600", "#FF8800"},
		success: [2]string{"#5F7A1F", "#A3BE4C"}, warning: [2]string{"#CC8800", "#FFB800"},
		error_: [2]string{"#C21038", "#FF4466"}, info: [2]string{"#BB6600", "#DD8833"},
		text: [2]string{"#1A1A1A", "#E0E0E0"}, muted: [2]string{"#707070", "#808080"},
		veryMuted: [2]string{"#A0A0A0", "#505050"}, background: [2]string{"#F0F0F0", "#0D0D0D"},
		border: [2]string{"#B0B0B0", "#3A3A3A"}, mutedBorder: [2]string{"#D0D0D0", "#222222"},
		system: [2]string{"#CC6600", "#FF8800"}, tool: [2]string{"#CC6600", "#FF8800"},
		accent: [2]string{"#DD2222", "#FF4444"}, highlight: [2]string{"#FFF0F0", "#1A1010"},
		mdKeyword: [2]string{"#CC3300", "#FF6644"}, mdString: [2]string{"#BB7700", "#DDAA33"},
		mdNumber: [2]string{"#CC8800", "#FFB800"}, mdComment: [2]string{"#909090", "#606060"},
		mdLink: [2]string{"#CC4400", "#FF7744"},
	},

	"catppuccin": {
		primary: [2]string{"#8839ef", "#cba6f7"}, secondary: [2]string{"#04a5e5", "#89dceb"},
		success: [2]string{"#40a02b", "#a6e3a1"}, warning: [2]string{"#df8e1d", "#f9e2af"},
		error_: [2]string{"#d20f39", "#f38ba8"}, info: [2]string{"#1e66f5", "#89b4fa"},
		text: [2]string{"#4c4f69", "#cdd6f4"}, muted: [2]string{"#6c6f85", "#a6adc8"},
		veryMuted: [2]string{"#9ca0b0", "#6c7086"}, background: [2]string{"#eff1f5", "#1e1e2e"},
		border: [2]string{"#acb0be", "#585b70"}, mutedBorder: [2]string{"#ccd0da", "#313244"},
		system: [2]string{"#179299", "#94e2d5"}, tool: [2]string{"#fe640b", "#fab387"},
		accent: [2]string{"#ea76cb", "#f5c2e7"}, highlight: [2]string{"#e6e9ef", "#181825"},
		mdKeyword: [2]string{"#8839ef", "#cba6f7"}, mdString: [2]string{"#40a02b", "#a6e3a1"},
		mdNumber: [2]string{"#fe640b", "#fab387"}, mdComment: [2]string{"#9ca0b0", "#6c7086"},
	},

	"dracula": {
		primary: [2]string{"#7c6bf5", "#bd93f9"}, secondary: [2]string{"#d16090", "#ff79c6"},
		success: [2]string{"#2fbf71", "#50fa7b"}, warning: [2]string{"#f7a14d", "#ffb86c"},
		error_: [2]string{"#d9536f", "#ff5555"}, info: [2]string{"#1d7fc5", "#8be9fd"},
		text: [2]string{"#1f1f2f", "#f8f8f2"}, background: [2]string{"#f8f8f2", "#1d1e28"},
		accent:    [2]string{"#d16090", "#ff79c6"},
		mdKeyword: [2]string{"#7c6bf5", "#bd93f9"}, mdString: [2]string{"#2fbf71", "#50fa7b"},
		mdComment: [2]string{"#6272a4", "#6272a4"},
	},

	"tokyonight": {
		primary: [2]string{"#2e7de9", "#7aa2f7"}, secondary: [2]string{"#b15c00", "#ff9e64"},
		success: [2]string{"#587539", "#9ece6a"}, warning: [2]string{"#8c6c3e", "#e0af68"},
		error_: [2]string{"#c94060", "#f7768e"}, info: [2]string{"#007197", "#7dcfff"},
		text: [2]string{"#273153", "#c0caf5"}, background: [2]string{"#e1e2e7", "#1a1b26"},
		mdKeyword: [2]string{"#2e7de9", "#7aa2f7"}, mdString: [2]string{"#587539", "#9ece6a"},
		mdComment: [2]string{"#848cb5", "#565f89"},
	},

	"nord": {
		primary: [2]string{"#5e81ac", "#88c0d0"}, secondary: [2]string{"#bf616a", "#d57780"},
		success: [2]string{"#8fbcbb", "#a3be8c"}, warning: [2]string{"#d08770", "#d08770"},
		error_: [2]string{"#bf616a", "#bf616a"}, info: [2]string{"#81a1c1", "#81a1c1"},
		text: [2]string{"#2e3440", "#e5e9f0"}, background: [2]string{"#eceff4", "#2e3440"},
		mdKeyword: [2]string{"#5e81ac", "#81a1c1"}, mdString: [2]string{"#8fbcbb", "#a3be8c"},
		mdComment: [2]string{"#616e88", "#616e88"},
	},

	"gruvbox": {
		primary: [2]string{"#076678", "#83a598"}, secondary: [2]string{"#9d0006", "#fb4934"},
		success: [2]string{"#79740e", "#b8bb26"}, warning: [2]string{"#b57614", "#fabd2f"},
		error_: [2]string{"#9d0006", "#fb4934"}, info: [2]string{"#8f3f71", "#d3869b"},
		text: [2]string{"#3c3836", "#ebdbb2"}, background: [2]string{"#fbf1c7", "#282828"},
		mdKeyword: [2]string{"#9d0006", "#fb4934"}, mdString: [2]string{"#79740e", "#b8bb26"},
		mdComment: [2]string{"#928374", "#928374"},
	},

	"monokai": {
		primary: [2]string{"#bf7bff", "#ae81ff"}, secondary: [2]string{"#d9487c", "#f92672"},
		success: [2]string{"#4fb54b", "#a6e22e"}, warning: [2]string{"#f1a948", "#fd971f"},
		error_: [2]string{"#e54b4b", "#f92672"}, info: [2]string{"#2d9ad7", "#66d9ef"},
		text: [2]string{"#292318", "#f8f8f2"}, background: [2]string{"#fdf8ec", "#272822"},
		mdKeyword: [2]string{"#d9487c", "#f92672"}, mdString: [2]string{"#4fb54b", "#a6e22e"},
		mdComment: [2]string{"#888888", "#75715e"},
	},

	"solarized": {
		primary: [2]string{"#268bd2", "#6c71c4"}, secondary: [2]string{"#d33682", "#d33682"},
		success: [2]string{"#859900", "#859900"}, warning: [2]string{"#b58900", "#b58900"},
		error_: [2]string{"#dc322f", "#dc322f"}, info: [2]string{"#2aa198", "#2aa198"},
		text: [2]string{"#586e75", "#93a1a1"}, background: [2]string{"#fdf6e3", "#002b36"},
		mdKeyword: [2]string{"#268bd2", "#6c71c4"}, mdString: [2]string{"#859900", "#859900"},
		mdComment: [2]string{"#93a1a1", "#586e75"},
	},

	"github": {
		primary: [2]string{"#0969da", "#58a6ff"}, secondary: [2]string{"#1b7c83", "#39c5cf"},
		success: [2]string{"#1a7f37", "#3fb950"}, warning: [2]string{"#9a6700", "#e3b341"},
		error_: [2]string{"#cf222e", "#f85149"}, info: [2]string{"#bc4c00", "#d29922"},
		text: [2]string{"#24292f", "#c9d1d9"}, background: [2]string{"#ffffff", "#0d1117"},
		mdKeyword: [2]string{"#0969da", "#58a6ff"}, mdString: [2]string{"#1a7f37", "#3fb950"},
		mdComment: [2]string{"#6e7781", "#8b949e"},
	},

	"one-dark": {
		primary: [2]string{"#4078f2", "#61afef"}, secondary: [2]string{"#0184bc", "#56b6c2"},
		success: [2]string{"#50a14f", "#98c379"}, warning: [2]string{"#c18401", "#e5c07b"},
		error_: [2]string{"#e45649", "#e06c75"}, info: [2]string{"#986801", "#d19a66"},
		text: [2]string{"#383a42", "#abb2bf"}, background: [2]string{"#fafafa", "#282c34"},
		mdKeyword: [2]string{"#a626a4", "#c678dd"}, mdString: [2]string{"#50a14f", "#98c379"},
		mdComment: [2]string{"#a0a1a7", "#5c6370"},
	},

	"rose-pine": {
		primary: [2]string{"#31748f", "#9ccfd8"}, secondary: [2]string{"#d7827e", "#ebbcba"},
		success: [2]string{"#286983", "#31748f"}, warning: [2]string{"#ea9d34", "#f6c177"},
		error_: [2]string{"#b4637a", "#eb6f92"}, info: [2]string{"#56949f", "#9ccfd8"},
		text: [2]string{"#575279", "#e0def4"}, background: [2]string{"#faf4ed", "#191724"},
		mdKeyword: [2]string{"#31748f", "#9ccfd8"}, mdString: [2]string{"#ea9d34", "#f6c177"},
		mdComment: [2]string{"#9893a5", "#6e6a86"},
	},

	"ayu": {
		primary: [2]string{"#4aa8c8", "#3fb7e3"}, secondary: [2]string{"#ef7d71", "#f2856f"},
		success: [2]string{"#5fb978", "#78d05c"}, warning: [2]string{"#ea9f41", "#e4a75c"},
		error_: [2]string{"#e6656a", "#f58572"}, info: [2]string{"#2f9bce", "#66c6f1"},
		text: [2]string{"#4f5964", "#d6dae0"}, background: [2]string{"#fdfaf4", "#0f1419"},
		mdKeyword: [2]string{"#4aa8c8", "#3fb7e3"}, mdString: [2]string{"#5fb978", "#78d05c"},
		mdComment: [2]string{"#abb0b6", "#5c6773"},
	},

	"material": {
		primary: [2]string{"#6182b8", "#82aaff"}, secondary: [2]string{"#39adb5", "#89ddff"},
		success: [2]string{"#91b859", "#c3e88d"}, warning: [2]string{"#ffb300", "#ffcb6b"},
		error_: [2]string{"#e53935", "#f07178"}, info: [2]string{"#f4511e", "#ffcb6b"},
		text: [2]string{"#263238", "#eeffff"}, background: [2]string{"#fafafa", "#263238"},
		mdKeyword: [2]string{"#6182b8", "#82aaff"}, mdString: [2]string{"#91b859", "#c3e88d"},
		mdComment: [2]string{"#aabfc5", "#546e7a"},
	},

	"everforest": {
		primary: [2]string{"#8da101", "#a7c080"}, secondary: [2]string{"#df69ba", "#d699b6"},
		success: [2]string{"#8da101", "#a7c080"}, warning: [2]string{"#f57d26", "#e69875"},
		error_: [2]string{"#f85552", "#e67e80"}, info: [2]string{"#35a77c", "#83c092"},
		text: [2]string{"#5c6a72", "#d3c6aa"}, background: [2]string{"#fdf6e3", "#2d353b"},
		mdKeyword: [2]string{"#8da101", "#a7c080"}, mdString: [2]string{"#35a77c", "#83c092"},
		mdComment: [2]string{"#939b84", "#859289"},
	},

	"kanagawa": {
		primary: [2]string{"#2D4F67", "#7E9CD8"}, secondary: [2]string{"#D27E99", "#D27E99"},
		success: [2]string{"#98BB6C", "#98BB6C"}, warning: [2]string{"#D7A657", "#D7A657"},
		error_: [2]string{"#E82424", "#E82424"}, info: [2]string{"#76946A", "#76946A"},
		text: [2]string{"#54433A", "#DCD7BA"}, background: [2]string{"#F2E9DE", "#1F1F28"},
		mdKeyword: [2]string{"#2D4F67", "#7E9CD8"}, mdString: [2]string{"#98BB6C", "#98BB6C"},
		mdComment: [2]string{"#A09D98", "#727169"},
	},

	"amoled": {
		primary: [2]string{"#6200ff", "#b388ff"}, secondary: [2]string{"#ff0080", "#ff4081"},
		success: [2]string{"#00e676", "#00ff88"}, warning: [2]string{"#ffab00", "#ffea00"},
		error_: [2]string{"#ff1744", "#ff1744"}, info: [2]string{"#00b0ff", "#18ffff"},
		text: [2]string{"#0a0a0a", "#ffffff"}, background: [2]string{"#f0f0f0", "#000000"},
		mdKeyword: [2]string{"#6200ff", "#b388ff"}, mdString: [2]string{"#00e676", "#00ff88"},
		mdComment: [2]string{"#757575", "#424242"},
	},

	"synthwave": {
		primary: [2]string{"#00bcd4", "#36f9f6"}, secondary: [2]string{"#9c27b0", "#b084eb"},
		success: [2]string{"#4caf50", "#72f1b8"}, warning: [2]string{"#ff9800", "#fede5d"},
		error_: [2]string{"#f44336", "#fe4450"}, info: [2]string{"#ff5722", "#ff8b39"},
		text: [2]string{"#262335", "#ffffff"}, background: [2]string{"#fafafa", "#262335"},
		mdKeyword: [2]string{"#9c27b0", "#b084eb"}, mdString: [2]string{"#4caf50", "#72f1b8"},
		mdComment: [2]string{"#848bbd", "#848bbd"},
	},

	"vesper": {
		primary: [2]string{"#FFC799", "#FFC799"}, secondary: [2]string{"#B30000", "#FF8080"},
		success: [2]string{"#99FFE4", "#99FFE4"}, warning: [2]string{"#FFC799", "#FFC799"},
		error_: [2]string{"#FF8080", "#FF8080"}, info: [2]string{"#FFC799", "#FFC799"},
		text: [2]string{"#1a1a1a", "#FFF"}, background: [2]string{"#F0F0F0", "#101010"},
		mdKeyword: [2]string{"#FFC799", "#FFC799"}, mdString: [2]string{"#99FFE4", "#99FFE4"},
		mdComment: [2]string{"#7a7a7a", "#505050"},
	},

	"flexoki": {
		primary: [2]string{"#205EA6", "#DA702C"}, secondary: [2]string{"#BC5215", "#8B7EC8"},
		success: [2]string{"#66800B", "#879A39"}, warning: [2]string{"#BC5215", "#DA702C"},
		error_: [2]string{"#AF3029", "#D14D41"}, info: [2]string{"#24837B", "#3AA99F"},
		text: [2]string{"#100F0F", "#CECDC3"}, background: [2]string{"#FFFCF0", "#100F0F"},
		mdKeyword: [2]string{"#205EA6", "#DA702C"}, mdString: [2]string{"#66800B", "#879A39"},
		mdComment: [2]string{"#878580", "#878580"},
	},

	"matrix": {
		primary: [2]string{"#1cc24b", "#2eff6a"}, secondary: [2]string{"#c770ff", "#c770ff"},
		success: [2]string{"#1cc24b", "#62ff94"}, warning: [2]string{"#e6ff57", "#e6ff57"},
		error_: [2]string{"#ff4b4b", "#ff4b4b"}, info: [2]string{"#30b3ff", "#30b3ff"},
		text: [2]string{"#203022", "#62ff94"}, background: [2]string{"#eef3ea", "#0a0e0a"},
		mdKeyword: [2]string{"#1cc24b", "#2eff6a"}, mdString: [2]string{"#1cc24b", "#62ff94"},
		mdComment: [2]string{"#5a7a5e", "#3a5a3e"},
	},

	"vercel": {
		primary: [2]string{"#0070F3", "#0070F3"}, secondary: [2]string{"#8E4EC6", "#8E4EC6"},
		success: [2]string{"#388E3C", "#46A758"}, warning: [2]string{"#FF9500", "#FFB224"},
		error_: [2]string{"#DC3545", "#E5484D"}, info: [2]string{"#0070F3", "#52A8FF"},
		text: [2]string{"#171717", "#EDEDED"}, background: [2]string{"#FFFFFF", "#000000"},
		mdKeyword: [2]string{"#0070F3", "#0070F3"}, mdString: [2]string{"#388E3C", "#46A758"},
		mdComment: [2]string{"#6B6B6B", "#666666"},
	},

	"zenburn": {
		primary: [2]string{"#5f7f8f", "#8cd0d3"}, secondary: [2]string{"#5f8f8f", "#93e0e3"},
		success: [2]string{"#5f8f5f", "#7f9f7f"}, warning: [2]string{"#8f8f5f", "#f0dfaf"},
		error_: [2]string{"#8f5f5f", "#cc9393"}, info: [2]string{"#8f7f5f", "#dfaf8f"},
		text: [2]string{"#3f3f3f", "#dcdccc"}, background: [2]string{"#ffffef", "#3f3f3f"},
		mdKeyword: [2]string{"#5f7f8f", "#8cd0d3"}, mdString: [2]string{"#5f8f5f", "#cc9393"},
		mdComment: [2]string{"#7f7f7f", "#7f9f7f"},
	},
}
