package tui

import (
	"image/color"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
)

// keyMap holds all key bindings.
type keyMap struct {
	ZoomIn   key.Binding
	ZoomOut  key.Binding
	Fit      key.Binding
	Actual   key.Binding
	Left     key.Binding
	Right    key.Binding
	Up       key.Binding
	Down     key.Binding
	Next     key.Binding
	Prev     key.Binding
	Theme    key.Binding
	Renderer key.Binding
	Save     key.Binding
	Reload   key.Binding
	Help     key.Binding
	Quit     key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		ZoomIn:   key.NewBinding(key.WithKeys("+", "=", "i"), key.WithHelp("+/i", "zoom in")),
		ZoomOut:  key.NewBinding(key.WithKeys("-", "_", "o"), key.WithHelp("-/o", "zoom out")),
		Fit:      key.NewBinding(key.WithKeys("0", "f"), key.WithHelp("0/f", "fit")),
		Actual:   key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "100%")),
		Left:     key.NewBinding(key.WithKeys("h", "left"), key.WithHelp("←/h", "pan left")),
		Right:    key.NewBinding(key.WithKeys("l", "right"), key.WithHelp("→/l", "pan right")),
		Up:       key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("↑/k", "pan up")),
		Down:     key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("↓/j", "pan down")),
		Next:     key.NewBinding(key.WithKeys("tab", "n", "]"), key.WithHelp("tab/n", "next diagram")),
		Prev:     key.NewBinding(key.WithKeys("shift+tab", "p", "["), key.WithHelp("⇧tab/p", "prev diagram")),
		Theme:    key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
		Renderer: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "renderer")),
		Save:     key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "save png")),
		Reload:   key.NewBinding(key.WithKeys("R", "ctrl+r"), key.WithHelp("R", "reload")),
		Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c", "esc"), key.WithHelp("q", "quit")),
	}
}

// ShortHelp implements help.KeyMap.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.ZoomIn, k.ZoomOut, k.Fit, k.Next, k.Theme, k.Help, k.Quit}
}

// FullHelp implements help.KeyMap.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.ZoomIn, k.ZoomOut, k.Fit, k.Actual},
		{k.Up, k.Down, k.Left, k.Right},
		{k.Next, k.Prev, k.Theme, k.Renderer},
		{k.Save, k.Reload, k.Help, k.Quit},
	}
}

// Palette used for the application chrome (Charm-ish).
var (
	colCharple  = lipgloss.Color("#6B50FF")
	colDolly    = lipgloss.Color("#FF60FF")
	colJulep    = lipgloss.Color("#00FFB2")
	colCherry   = lipgloss.Color("#FF388B")
	colSquid    = lipgloss.Color("#858392")
	colSmoke    = lipgloss.Color("#BFBCC8")
	colAsh      = lipgloss.Color("#DFDBDD")
	colPepper   = lipgloss.Color("#201F26")
	colIron     = lipgloss.Color("#4D4C57")
	colCharcoal = lipgloss.Color("#3A3943")
	colButter   = lipgloss.Color("#FFFAF1")
	colMalibu   = lipgloss.Color("#00A4FF")
	colZest     = lipgloss.Color("#E8FE96")
)

type styles struct {
	bar         lipgloss.Style
	logo        lipgloss.Style
	tab         lipgloss.Style
	tabActive   lipgloss.Style
	statusKey   lipgloss.Style
	statusVal   lipgloss.Style
	statusDim   lipgloss.Style
	pill        func(bg color.Color) lipgloss.Style
	errBox      lipgloss.Style
	errTitle    lipgloss.Style
	errCode     lipgloss.Style
	errLineNo   lipgloss.Style
	toast       lipgloss.Style
	helpBox     lipgloss.Style
	placeholder lipgloss.Style
}

func newStyles() styles {
	bar := lipgloss.NewStyle().Background(colPepper).Foreground(colSmoke)
	return styles{
		bar:       bar,
		logo:      lipgloss.NewStyle().Background(colCharple).Foreground(colButter).Bold(true).Padding(0, 1),
		tab:       bar.Foreground(colSquid).Padding(0, 1),
		tabActive: lipgloss.NewStyle().Background(colCharcoal).Foreground(colDolly).Bold(true).Padding(0, 1),
		statusKey: bar.Foreground(colSquid),
		statusVal: bar.Foreground(colAsh),
		statusDim: bar.Foreground(colIron),
		pill: func(bg color.Color) lipgloss.Style {
			return lipgloss.NewStyle().Background(bg).Foreground(colPepper).Bold(true).Padding(0, 1)
		},
		errBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colCherry).
			Background(colPepper).
			Foreground(colAsh).
			Padding(1, 2),
		errTitle:  lipgloss.NewStyle().Foreground(colCherry).Background(colPepper).Bold(true),
		errCode:   lipgloss.NewStyle().Foreground(colZest).Background(colPepper),
		errLineNo: lipgloss.NewStyle().Foreground(colSquid).Background(colPepper),
		toast: lipgloss.NewStyle().
			Background(colJulep).Foreground(colPepper).Bold(true).Padding(0, 1),
		helpBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colCharple).
			Background(colPepper).
			Padding(0, 2),
		placeholder: lipgloss.NewStyle().Foreground(colSquid),
	}
}
