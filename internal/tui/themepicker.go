package tui

import (
	"image"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mark3labs/mergo/internal/theme"
)

// themePicker is the theme selection overlay. Moving the selection previews
// the theme on the current diagram; enter keeps (and persists) it, esc
// restores the theme that was active when the picker opened.
type themePicker struct {
	open bool
	idx  int    // selected entry
	top  int    // first visible row
	rows int    // visible rows (set while rendering)
	orig string // theme to restore on cancel
	// box is the picker's screen area and first the screen row of its
	// first entry (set while rendering), for mouse hit testing.
	box   image.Rectangle
	first int
}

// Picker box layout: border (1) + vertical padding (1), then title and a
// spacer before the entries; a spacer and the hint after them.
const (
	pickerPadX  = 2
	pickerPadY  = 1
	pickerChrom = 2 + 2*pickerPadY + 4
)

// setTheme makes the named theme active and schedules a re-render. Unknown
// names are ignored.
func (m *Model) setTheme(name string) tea.Cmd {
	for i, n := range m.themes {
		if n == name {
			if i == m.themeIdx {
				return nil
			}
			m.themeIdx = i
			m.restyle()
			return tea.Batch(m.ensureScene(), m.requestRender())
		}
	}
	return nil
}

// openThemePicker shows the picker with the active theme selected.
func (m *Model) openThemePicker() tea.Cmd {
	m.showHelp = false
	p := &m.picker
	p.open = true
	p.orig = m.themeName()
	p.idx, p.top = m.themeIdx, 0
	return nil
}

// previewTheme selects picker entry i and applies it without saving.
func (m *Model) previewTheme(i int) tea.Cmd {
	p := &m.picker
	if i < 0 || i >= len(m.themes) {
		return nil
	}
	p.idx = i
	if p.rows > 0 {
		p.top = min(max(p.top, i-p.rows+1), i)
	}
	return m.setTheme(m.themes[i])
}

// confirmTheme keeps entry i and persists it.
func (m *Model) confirmTheme(i int) tea.Cmd {
	cmd := m.previewTheme(i)
	m.picker.open = false
	name := m.themeName()
	if m.opts.SaveTheme != nil {
		if err := m.opts.SaveTheme(name); err != nil {
			return tea.Batch(cmd, m.showError("theme "+name+" applied but not saved: "+err.Error()))
		}
	}
	return tea.Batch(cmd, m.showToast("theme: "+name))
}

// cancelTheme closes the picker and restores the previous theme.
func (m *Model) cancelTheme() tea.Cmd {
	m.picker.open = false
	return m.setTheme(m.picker.orig)
}

// handleThemeKey handles keys while the picker is open.
func (m *Model) handleThemeKey(msg tea.KeyPressMsg) tea.Cmd {
	p := &m.picker
	last := len(m.themes) - 1
	page := max(p.rows-1, 1)
	switch msg.String() {
	case "up", "k", "shift+tab":
		return m.previewTheme(max(p.idx-1, 0))
	case "down", "j", "tab":
		return m.previewTheme(min(p.idx+1, last))
	case "pgup", "ctrl+u":
		return m.previewTheme(max(p.idx-page, 0))
	case "pgdown", "ctrl+d":
		return m.previewTheme(min(p.idx+page, last))
	case "home", "g":
		return m.previewTheme(0)
	case "end", "G":
		return m.previewTheme(last)
	case "enter", "space":
		return m.confirmTheme(p.idx)
	case "esc", "q", "t", "ctrl+c":
		return m.cancelTheme()
	}
	return nil
}

// handleThemeMouse handles mouse input while the picker is open: clicking an
// entry picks it, clicking outside cancels and the wheel moves the selection.
func (m *Model) handleThemeMouse(msg tea.Msg) tea.Cmd {
	p := &m.picker
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		mo := msg.Mouse()
		if mo.Button != tea.MouseLeft {
			return nil
		}
		if !image.Pt(mo.X, mo.Y).In(p.box) {
			return m.cancelTheme()
		}
		if row := mo.Y - p.first; row >= 0 && row < p.rows && p.top+row < len(m.themes) {
			return m.confirmTheme(p.top + row)
		}
	case tea.MouseWheelMsg:
		switch msg.Mouse().Button {
		case tea.MouseWheelUp:
			return m.previewTheme(max(p.idx-1, 0))
		case tea.MouseWheelDown:
			return m.previewTheme(min(p.idx+1, len(m.themes)-1))
		}
	}
	return nil
}

// themePickerView renders the picker box to fit in rows screen rows.
func (m *Model) themePickerView(rows int) string {
	st := m.st
	p := &m.picker
	names := m.themes

	p.rows = min(max(rows-pickerChrom, 1), len(names))
	p.top = min(max(p.top, p.idx-p.rows+1), p.idx)
	p.top = min(max(p.top, 0), max(len(names)-p.rows, 0))

	nameW := 0
	for _, n := range names {
		nameW = max(nameW, lipgloss.Width(n))
	}

	var lines []string
	end := min(p.top+p.rows, len(names))
	for i := p.top; i < end; i++ {
		n := names[i]
		mark := "  "
		if n == p.orig {
			mark = "● "
		}
		label := " " + mark + n + strings.Repeat(" ", nameW-lipgloss.Width(n)) + " "
		switch {
		case i == p.idx:
			label = st.pickSel.Render(label)
		case n == p.orig:
			label = st.pickCur.Render(label)
		default:
			label = st.pickItem.Render(label)
		}
		lines = append(lines, label+st.pickItem.Render(" ")+themeSwatch(n, m.opts.Dark))
	}

	title := st.pickTitle.Render("◆ Theme")
	if len(names) > p.rows {
		title += st.pickDim.Render("    " + strconv.Itoa(p.idx+1) + "/" + strconv.Itoa(len(names)))
	}
	hint := st.pickKey.Render("↑↓") + st.pickDim.Render(" preview  ") +
		st.pickKey.Render("enter") + st.pickDim.Render(" apply  ") +
		st.pickKey.Render("esc") + st.pickDim.Render(" cancel")
	content := lipgloss.JoinVertical(lipgloss.Left, append(append([]string{title, ""}, lines...), "", hint)...)
	return st.pickBox.Padding(pickerPadY, pickerPadX).Render(fillPanel(content, st.panelBg))
}

// placeThemePicker records where the picker box was drawn.
func (m *Model) placeThemePicker(x, y, w, h int) {
	m.picker.box = image.Rect(x, y, x+w, y+h)
	m.picker.first = y + 1 + pickerPadY + 2
}

// themeSwatch previews a theme's main colors on its background.
func themeSwatch(name string, dark bool) string {
	t, err := theme.Get(name, dark)
	if err != nil {
		return ""
	}
	bg := lipgloss.NewStyle().Background(t.Background)
	var b strings.Builder
	b.WriteString(bg.Render(" "))
	// the theme's hues, like gopyter's swatch
	for i := range 6 {
		b.WriteString(bg.Foreground(t.ChartColor(i)).Render("■"))
	}
	b.WriteString(bg.Render(" "))
	return b.String()
}
