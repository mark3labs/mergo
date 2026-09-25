package tui

import (
	"image/color"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mark3labs/mergo/internal/theme"
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
	Edit     key.Binding
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
		Theme:    key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "pick theme")),
		Renderer: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "renderer")),
		Save:     key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "save png")),
		Edit:     key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit source")),
		Reload:   key.NewBinding(key.WithKeys("R", "ctrl+r"), key.WithHelp("R", "reload")),
		Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c", "esc"), key.WithHelp("q", "quit")),
	}
}

// ShortHelp implements help.KeyMap.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.ZoomIn, k.ZoomOut, k.Fit, k.Next, k.Theme, k.Edit, k.Help, k.Quit}
}

// FullHelp implements help.KeyMap.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.ZoomIn, k.ZoomOut, k.Fit, k.Actual},
		{k.Up, k.Down, k.Left, k.Right},
		{k.Next, k.Prev, k.Theme, k.Renderer},
		{k.Edit, k.Save, k.Reload, k.Help, k.Quit},
	}
}

// styles draw the viewer's chrome. They are derived from the UI palette of
// the active theme, so the interface matches the diagram (as in gopyter).
type styles struct {
	bar         lipgloss.Style
	logo        lipgloss.Style
	tab         lipgloss.Style
	tabActive   lipgloss.Style
	statusKey   lipgloss.Style
	statusVal   lipgloss.Style
	statusDim   lipgloss.Style
	kindPill    lipgloss.Style
	kittyPill   lipgloss.Style
	blockPill   lipgloss.Style
	errBox      lipgloss.Style
	errTitle    lipgloss.Style
	errMsg      lipgloss.Style
	errCode     lipgloss.Style
	errLineNo   lipgloss.Style
	toast       lipgloss.Style
	toastErr    lipgloss.Style
	helpBox     lipgloss.Style
	help        help.Styles
	placeholder lipgloss.Style
	// panelBg is the background of boxes (the theme background).
	panelBg color.Color

	// theme picker
	pickBox   lipgloss.Style
	pickTitle lipgloss.Style
	pickItem  lipgloss.Style
	pickSel   lipgloss.Style
	pickCur   lipgloss.Style
	pickKey   lipgloss.Style
	pickDim   lipgloss.Style

	// source editor
	ed       editorStyles
	edSep    lipgloss.Style
	edOK     lipgloss.Style
	edErr    lipgloss.Style
	edPill   lipgloss.Style
	edDirty  lipgloss.Style
	compItem lipgloss.Style
	compSel  lipgloss.Style
	compKind lipgloss.Style
	compSelK lipgloss.Style
}

func newStyles(p theme.UI) styles {
	s := lipgloss.NewStyle
	// Panels sit on the theme background; the bars on a slightly raised
	// surface, like gopyter's.
	panel := s().Background(p.Ink)
	bar := s().Background(p.Faint).Foreground(p.Dim)
	pill := func(bg color.Color) lipgloss.Style {
		return s().Background(bg).Foreground(p.Ink).Bold(true).Padding(0, 1)
	}
	box := func(border color.Color) lipgloss.Style {
		return panel.
			Border(lipgloss.RoundedBorder()).
			BorderForeground(border).
			BorderBackground(p.Ink)
	}
	helpKey := panel.Foreground(p.Dim).Bold(true)
	helpDesc := panel.Foreground(p.Muted)
	helpSep := panel.Foreground(p.Subtle)
	return styles{
		bar:       bar,
		logo:      pill(p.Primary),
		tab:       bar.Foreground(p.Muted).Padding(0, 1),
		tabActive: s().Background(p.Selection).Foreground(p.Text).Bold(true).Padding(0, 1),
		statusKey: bar.Foreground(p.Muted),
		statusVal: bar.Foreground(p.Text),
		statusDim: bar.Foreground(p.Subtle),
		kindPill:  pill(p.Info),
		kittyPill: pill(p.Success),
		blockPill: pill(p.Warning),

		errBox:    box(p.Error).Foreground(p.Text).Padding(1, 2),
		errTitle:  panel.Foreground(p.Error).Bold(true),
		errMsg:    panel.Foreground(p.Text),
		errCode:   panel.Foreground(p.Warning),
		errLineNo: panel.Foreground(p.Muted),

		toast:    pill(p.Success),
		toastErr: pill(p.Error),

		helpBox: box(p.Primary).Padding(0, 2),
		help: help.Styles{
			Ellipsis:       helpSep,
			ShortKey:       helpKey,
			ShortDesc:      helpDesc,
			ShortSeparator: helpSep,
			FullKey:        helpKey,
			FullDesc:       helpDesc,
			FullSeparator:  helpSep,
		},
		placeholder: s().Foreground(p.Muted),
		panelBg:     p.Ink,

		pickBox:   box(p.Primary),
		pickTitle: panel.Foreground(p.Primary).Bold(true),
		pickItem:  panel.Foreground(p.Text),
		pickSel:   s().Background(p.Primary).Foreground(p.Ink).Bold(true),
		pickCur:   panel.Foreground(p.Primary),
		pickKey:   helpKey,
		pickDim:   helpDesc,

		ed:       newEditorStyles(p),
		edSep:    panel.Foreground(p.Subtle),
		edOK:     panel.Foreground(p.Success),
		edErr:    panel.Foreground(p.Error),
		edPill:   pill(p.Accent),
		edDirty:  bar.Foreground(p.Warning).Bold(true),
		compItem: s().Background(p.Faint).Foreground(p.Text),
		compSel:  s().Background(p.Selection).Foreground(p.Text).Bold(true),
		compKind: s().Background(p.Faint).Foreground(p.Muted),
		compSelK: s().Background(p.Selection).Foreground(p.Dim),
	}
}

func newEditorStyles(p theme.UI) editorStyles {
	s := lipgloss.NewStyle
	fg := func(c color.Color) lipgloss.Style { return s().Foreground(c) }
	st := editorStyles{
		bg:      p.Ink,
		curBg:   p.Faint,
		text:    fg(p.Text),
		lineNo:  fg(p.Subtle).Background(p.Ink),
		lineCur: fg(p.Dim).Background(p.Faint),
		lineErr: s().Foreground(p.Ink).Background(p.Error).Bold(true),
	}
	st.tok[tokText] = fg(p.Text)
	st.tok[tokKeyword] = fg(p.Keyword).Bold(true)
	st.tok[tokString] = fg(p.Str)
	st.tok[tokNumber] = fg(p.Number)
	st.tok[tokComment] = fg(p.Muted).Italic(true)
	st.tok[tokArrow] = fg(p.Name)
	st.tok[tokPunct] = fg(p.Dim)
	return st
}

// fillPanel paints content on bg: gaps left by resets inside styled text
// and short lines (which lipgloss pads with unstyled spaces) would otherwise
// show the terminal's background instead of the theme's.
func fillPanel(content string, bg color.Color) string {
	on := ansi.Style{}.BackgroundColor(bg).String()
	lines := strings.Split(content, "\n")
	w := 0
	for _, l := range lines {
		w = max(w, ansi.StringWidth(l))
	}
	for i, l := range lines {
		l = strings.ReplaceAll(l, "\x1b[m", "\x1b[m"+on)
		l = strings.ReplaceAll(l, "\x1b[0m", "\x1b[0m"+on)
		lines[i] = on + l + strings.Repeat(" ", w-ansi.StringWidth(l)) + "\x1b[m"
	}
	return strings.Join(lines, "\n")
}
