package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// editSession is the state of the source editor. The buffer replaces the
// diagram's source for parsing and preview while it is open; the file is
// only written by an explicit save, and only when the source parses.
type editSession struct {
	diag int
	ed   *editor
	// orig is the source as on disk (loaded or last saved), with LF line
	// endings; crlf records whether the file used CRLF.
	orig string
	crlf bool
	comp completion
	gen  int

	// validation: the result of the latest parse of the buffer
	err     error
	checked uint64 // hash of the source err belongs to (0 = none yet)

	discard bool // esc was pressed once with unsaved changes
	saving  bool
}

func (s *editSession) dirty() bool { return s.ed.value() != s.orig }

// editParseMsg triggers validation once typing pauses.
type editParseMsg struct{ gen int }

// sourceSavedMsg reports the result of writing the source back.
type sourceSavedMsg struct {
	diag   int
	path   string
	src    string // the saved buffer
	stored string // the diagram's Source as the loader would read it
	delta  int    // change in line count (markdown blocks below shift)
	mtime  time.Time
	err    error
}

// editParseDelay is how long typing must pause before the buffer is
// validated.
var editParseDelay = 150 * time.Millisecond

// editPaneWidth is the width of the editor pane (0 when not editing). On
// narrow terminals the editor takes the whole width.
func (m *Model) editPaneWidth() int {
	if m.edit == nil {
		return 0
	}
	if m.width < 60 {
		return m.width
	}
	return min(max(m.width*9/20, 40), m.width-20)
}

// layoutEditor sizes the editor to its pane: the last row shows the
// validation result and the last column separates it from the preview.
func (m *Model) layoutEditor() {
	if m.edit == nil {
		return
	}
	e := m.edit.ed
	_, rows := m.bodySize()
	e.setSize(m.editPaneWidth()-1-e.gutterWidth(), rows-1)
}

func (m *Model) startEdit() tea.Cmd {
	d := m.currentDiagram()
	if d == nil {
		return nil
	}
	m.showHelp = false
	erase := m.eraseSixel() // the layout changes under the image
	src := d.Source
	crlf := strings.Contains(src, "\r\n")
	src = strings.ReplaceAll(src, "\r\n", "\n")
	m.edit = &editSession{diag: m.current, ed: newEditor(src), orig: src, crlf: crlf}
	m.updateKeywords()
	m.layoutEditor()
	m.refreshEditCheck()
	if cam := m.cam(); cam != nil {
		cam.fit = true
	}
	hint := "editing"
	if d.Path == "-" {
		hint = "editing (stdin: can't be saved)"
	}
	return tea.Batch(erase, m.ensureScene(), m.requestRender(), m.showToast(hint))
}

// exitEdit closes the editor; with unsaved changes it asks for
// confirmation (a second esc) first.
func (m *Model) exitEdit() tea.Cmd {
	s := m.edit
	if s.dirty() && !s.discard {
		s.discard = true
		return m.showError("unsaved changes: ctrl+s saves, esc again discards")
	}
	erase := m.eraseSixel()
	m.edit = nil
	if cam := m.cam(); cam != nil {
		cam.fit = true
	}
	return tea.Batch(erase, m.ensureScene(), m.requestRender())
}

// editRef identifies the edited diagram across reloads.
type editRef struct {
	path  string
	block int
	ok    bool
}

func (m *Model) editRef() editRef {
	if m.edit == nil || m.edit.diag >= len(m.diagrams) {
		return editRef{}
	}
	d := m.diagrams[m.edit.diag]
	return editRef{d.Path, d.Block, true}
}

// reconcileEdit re-attaches the editor after a reload. Changes made on
// disk replace an unmodified buffer; a modified one is kept (saving will
// overwrite the file).
func (m *Model) reconcileEdit(ref editRef) tea.Cmd {
	s := m.edit
	if s == nil || !ref.ok {
		return nil
	}
	idx := slices.IndexFunc(m.diagrams, func(d *Diagram) bool { return d.Path == ref.path && d.Block == ref.block })
	if idx < 0 {
		if len(m.diagrams) == 0 {
			m.edit = nil
			return m.showError("the edited diagram is gone")
		}
		s.diag = min(s.diag, len(m.diagrams)-1)
		m.current = s.diag
		return m.showError("the edited diagram is gone from " + filepath.Base(ref.path))
	}
	s.diag, m.current = idx, idx
	d := m.diagrams[idx]
	src := strings.ReplaceAll(d.Source, "\r\n", "\n")
	s.crlf = strings.Contains(d.Source, "\r\n")
	if src == s.orig {
		return nil
	}
	cur := s.ed.value()
	dirty := cur != s.orig
	s.orig = src
	switch {
	case src == cur: // our own save, or the same edit made elsewhere
		return nil
	case !dirty:
		s.ed.replaceAll(src)
		s.comp.close()
		m.updateKeywords()
		m.refreshEditCheck()
		return nil
	}
	return m.showError("file changed on disk: saving will overwrite it")
}

// handleEditKey routes keys while editing.
func (m *Model) handleEditKey(msg tea.KeyPressMsg) tea.Cmd {
	s := m.edit
	c := &s.comp
	k := msg.String()
	if k != "esc" && k != "ctrl+c" {
		s.discard = false
	}
	switch k {
	case "ctrl+s":
		c.close()
		return m.saveSource()
	case "esc":
		if c.open {
			c.close()
			return nil
		}
		return m.exitEdit()
	case "ctrl+c", "ctrl+q":
		c.close()
		return m.exitEdit()
	case "ctrl+r":
		return m.forceReload()
	case "ctrl+space", "ctrl+@":
		m.openCompletion(true)
		return nil
	}
	if c.open {
		switch k {
		case "up", "ctrl+p":
			c.move(-1)
			return nil
		case "down", "ctrl+n":
			c.move(1)
			return nil
		case "tab":
			return m.acceptCompletion()
		case "enter":
			if c.engaged {
				return m.acceptCompletion()
			}
		}
	}
	handled, changed := s.ed.handleKey(msg)
	if !handled {
		return nil
	}
	switch {
	case !changed:
		c.close() // the cursor moved
	case msg.Text != "" && isCompletionTrigger(msg.Text):
		m.openCompletion(false)
	case c.open && (k == "backspace" || k == "ctrl+h"):
		m.openCompletion(false)
	default:
		c.close()
	}
	if !changed {
		return nil
	}
	return m.sourceChanged()
}

func (m *Model) editPaste(text string) tea.Cmd {
	m.edit.comp.close()
	if !m.edit.ed.paste(text) {
		return nil
	}
	return m.sourceChanged()
}

// handleEditMouse handles clicks and the wheel over the editor pane.
func (m *Model) handleEditMouse(msg tea.Msg) {
	e := m.edit.ed
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		mo := msg.Mouse()
		if mo.Button != tea.MouseLeft {
			return
		}
		row := e.top + mo.Y - m.bodyTop()
		if row < e.top || row >= e.top+e.height {
			return
		}
		m.edit.comp.close()
		row = min(row, len(e.lines)-1)
		e.setCursor(row, colAt(e.lines[row], e.left+mo.X-e.gutterWidth()))
		e.lastOp = ""
	case tea.MouseWheelMsg:
		switch msg.Mouse().Button {
		case tea.MouseWheelUp:
			e.scroll(-3)
		case tea.MouseWheelDown:
			e.scroll(3)
		}
	}
}

// sourceChanged schedules validation (and the preview) of the buffer.
func (m *Model) sourceChanged() tea.Cmd {
	s := m.edit
	s.gen++
	g := s.gen
	return tea.Tick(editParseDelay, func(time.Time) tea.Msg { return editParseMsg{gen: g} })
}

func (m *Model) editParse(msg editParseMsg) tea.Cmd {
	s := m.edit
	if s == nil || msg.gen != s.gen {
		return nil
	}
	m.updateKeywords()
	// drop the scenes of intermediate versions of the buffer
	cur := m.currentKey()
	for k := range m.scenes {
		if k.diag == s.diag && k != cur {
			delete(m.scenes, k)
		}
	}
	m.refreshEditCheck()
	return m.ensureScene()
}

// refreshEditCheck picks up an already cached parse of the buffer.
func (m *Model) refreshEditCheck() {
	if m.edit == nil {
		return
	}
	k := m.currentKey()
	if e, ok := m.scenes[k]; ok {
		m.editChecked(sceneMsg{key: k, entry: e})
	}
}

// editChecked records the validation result of a parse of the buffer.
func (m *Model) editChecked(msg sceneMsg) {
	s := m.edit
	if s == nil || msg.key.diag != s.diag || msg.key.hash != hashSource(s.ed.value()) {
		return
	}
	s.err, s.checked = msg.entry.err, msg.key.hash
	s.ed.errLine = 0
	if msg.entry.err != nil {
		if mm := lineRe.FindStringSubmatch(msg.entry.err.Error()); mm != nil {
			s.ed.errLine, _ = strconv.Atoi(mm[1])
		}
	}
}

func (m *Model) updateKeywords() {
	_, words := headerInfo(m.edit.ed.lines)
	m.edit.ed.keywords = keywordSet(diagramTypeOf(words))
}

// ---------------------------------------------------------------------------
// completion

func (m *Model) openCompletion(manual bool) {
	c := &m.edit.comp
	items := complete(m.edit.ed, manual)
	if len(items) == 0 {
		c.close()
		return
	}
	engaged := manual || c.open && c.engaged
	*c = completion{open: true, items: items, engaged: engaged}
}

func (m *Model) acceptCompletion() tea.Cmd {
	c := &m.edit.comp
	it := c.items[c.sel]
	c.close()
	m.edit.ed.replaceBefore(it.n, it.text)
	m.edit.ed.scrollIntoView()
	return m.sourceChanged()
}

// completionOverlay renders the completion popup below (or above) the
// cursor.
func (m *Model) completionOverlay() (overlay, bool) {
	if m.edit == nil || !m.edit.comp.open {
		return overlay{}, false
	}
	c, e := &m.edit.comp, m.edit.ed
	x, y, ok := e.cursorPos()
	if !ok {
		return overlay{}, false
	}
	end := min(c.top+compRows, len(c.items))
	w := 0
	for _, it := range c.items[c.top:end] {
		w = max(w, ansi.StringWidth(it.text)+len(it.kind)+3)
	}
	w = min(w, max(m.width, 1))
	var lines []string
	for i := c.top; i < end; i++ {
		it := c.items[i]
		item, kind := m.st.compItem, m.st.compKind
		if i == c.sel {
			item, kind = m.st.compSel, m.st.compSelK
		}
		kw := ansi.StringWidth(it.kind) + 1
		text := ansi.Truncate(" "+it.text, max(w-kw, 1), "…")
		pad := max(w-ansi.StringWidth(text)-kw, 0)
		lines = append(lines, item.Render(text+strings.Repeat(" ", pad))+kind.Render(it.kind+" "))
	}
	if len(c.items) > compRows {
		more := fmt.Sprintf(" %d/%d", c.sel+1, len(c.items))
		lines = append(lines, m.st.compKind.Render(more+strings.Repeat(" ", max(w-len(more), 0))))
	}
	box := strings.Join(lines, "\n")
	h := len(lines)
	// align the popup with the start of the word being completed
	px := e.gutterWidth() + x - dispCol(e.line()[e.col-c.items[c.sel].n:], c.items[c.sel].n)
	px = min(max(px-1, 0), max(m.width-w, 0))
	py := m.bodyTop() + y + 1
	if py+h > m.height-1 && m.bodyTop()+y-h >= m.bodyTop() {
		py = m.bodyTop() + y - h
	}
	return overlay{box, px, py, 6}, true
}

// ---------------------------------------------------------------------------
// saving

// saveSource writes the buffer back to its file: the whole file for a
// diagram file, the fenced block for markdown. The source must parse.
func (m *Model) saveSource() tea.Cmd {
	s := m.edit
	if s.saving || s.diag >= len(m.diagrams) {
		return nil
	}
	d := m.diagrams[s.diag]
	if d.Path == "-" {
		return m.showError("can't save: the diagram was read from stdin")
	}
	src := s.ed.value()
	if src == s.orig {
		return m.showToast("no changes")
	}
	if s.err != nil && s.checked == hashSource(src) {
		return m.showError("not saved: fix the syntax error first")
	}
	s.saving = true
	th, dark := m.themeName(), m.opts.Dark
	diag, path, block, orig, crlf := s.diag, d.Path, d.Block, s.orig, s.crlf
	return func() tea.Msg {
		msg := sourceSavedMsg{diag: diag, path: path, src: src}
		// always validated right before writing, whatever the state of
		// the background check
		if _, err := parseScene(src, th, dark); err != nil {
			msg.err = fmt.Errorf("not saved: %w", err)
			return msg
		}
		msg.delta, msg.stored, msg.err = writeSource(path, block, orig, src, crlf)
		if msg.err == nil {
			if st, err := os.Stat(path); err == nil {
				msg.mtime = st.ModTime()
			}
		}
		return msg
	}
}

func (m *Model) sourceSaved(msg sourceSavedMsg) tea.Cmd {
	s := m.edit
	if s != nil {
		s.saving = false
	}
	if msg.err != nil {
		return m.showError(msg.err.Error())
	}
	if msg.diag < len(m.diagrams) && m.diagrams[msg.diag].Path == msg.path {
		d := m.diagrams[msg.diag]
		d.Source = msg.stored
		for _, o := range m.diagrams {
			if o.Path == d.Path && o.Block > d.Block {
				o.Line += msg.delta
			}
		}
	}
	if !msg.mtime.IsZero() {
		m.mtimes[msg.path] = msg.mtime
	}
	if s != nil && s.diag == msg.diag {
		s.orig = msg.src
		s.discard = false
	}
	return m.showToast("saved " + filepath.Base(msg.path))
}

// writeSource replaces the diagram source orig with src in the file at
// path (block is the mermaid block index in markdown files). It fails if
// the file no longer contains orig. It returns the change in line count
// and the new source as the loader reads it.
func writeSource(path string, block int, orig, src string, crlf bool) (delta int, stored string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, "", err
	}
	content := string(data)
	conflict := fmt.Errorf("%s changed on disk: reload with ctrl+r (your edits are kept) and save again", filepath.Base(path))
	if !isMarkdown(path) {
		if strings.ReplaceAll(content, "\r\n", "\n") != orig {
			return 0, "", conflict
		}
		out := src
		if crlf {
			out = strings.ReplaceAll(src, "\n", "\r\n")
		}
		return 0, out, os.WriteFile(path, []byte(out), 0o644)
	}
	blocks := ExtractMermaidBlocks(content)
	if block >= len(blocks) || blocks[block].Source != orig {
		return 0, "", conflict
	}
	b := blocks[block]
	fileCRLF := strings.Contains(content, "\r\n")
	pad := strings.Repeat(" ", b.Indent)
	var repl []string
	for l := range strings.SplitSeq(src, "\n") {
		if l != "" {
			l = pad + l
		}
		if fileCRLF {
			l += "\r"
		}
		repl = append(repl, l)
	}
	lines := strings.Split(content, "\n")
	start := b.Line - 1
	out := strings.Join(slices.Concat(lines[:start], repl, lines[start+b.Lines:]), "\n")
	// the new source must come back as the same block (a fence line
	// inside it would end the block early)
	if nb := ExtractMermaidBlocks(out); block >= len(nb) || nb[block].Source != src || len(nb) != len(blocks) {
		return 0, "", errors.New("not saved: the source would break the markdown code fence")
	}
	return len(repl) - b.Lines, src, os.WriteFile(path, []byte(out), 0o644)
}

// ---------------------------------------------------------------------------
// view

// editorView renders the editor pane.
func (m *Model) editorView() string {
	s := m.edit
	w := m.editPaneWidth()
	sep := m.st.edSep.Render("│")
	var out []string
	for _, l := range s.ed.view(m.st.ed, true) {
		out = append(out, l+sep)
	}
	out = append(out, m.editDiagnostics(w-1)+sep)
	return strings.Join(out, "\n")
}

// editDiagnostics is the validation line at the bottom of the editor.
func (m *Model) editDiagnostics(w int) string {
	s := m.edit
	var line string
	switch {
	case s.checked == 0:
		line = m.st.placeholder.Background(m.st.panelBg).Render(" checking…")
	case s.err != nil:
		msg := strings.ReplaceAll(s.err.Error(), "\n", " ")
		line = m.st.edErr.Render(ansi.Truncate(" ✗ "+msg, w, "…"))
	default:
		kind := diagramTypeOf(func() []string { _, ws := headerInfo(s.ed.lines); return ws }())
		line = m.st.edOK.Render(ansi.Truncate(" ✓ valid "+kind, w, "…"))
	}
	if pad := w - ansi.StringWidth(line); pad > 0 {
		line += m.st.edOK.Render(strings.Repeat(" ", pad))
	}
	return line
}

// editCursor places the terminal cursor in the editor.
func (m *Model) editCursor() *tea.Cursor {
	if m.edit == nil || m.picker.open || m.width <= 0 {
		return nil
	}
	e := m.edit.ed
	x, y, ok := e.cursorPos()
	if !ok {
		return nil
	}
	c := tea.NewCursor(e.gutterWidth()+x, m.bodyTop()+y)
	c.Shape = tea.CursorBar
	return c
}

// editStatusView is the status bar while editing.
func (m *Model) editStatusView() string {
	st := m.st
	s := m.edit
	sep := st.statusDim.Render(" │ ")
	left := st.edPill.Render("EDIT")
	if s.diag < len(m.diagrams) {
		left += st.statusVal.Render(" " + m.diagrams[s.diag].Name())
	}
	if s.dirty() {
		left += st.edDirty.Render(" ●")
	}
	pos := st.statusKey.Render(fmt.Sprintf("ln %d, col %d", s.ed.row+1, dispCol(s.ed.line(), s.ed.col)+1))
	keys := []string{
		st.statusVal.Render("ctrl+s") + st.statusKey.Render(" save"),
		st.statusVal.Render("ctrl+space") + st.statusKey.Render(" complete"),
		st.statusVal.Render("esc") + st.statusKey.Render(" done"),
	}
	r := pos + sep + strings.Join(keys, sep) + st.bar.Render(" ")
	for len(keys) > 0 && lipgloss.Width(left)+lipgloss.Width(r) >= m.width {
		keys = keys[:len(keys)-1]
		r = pos + sep + strings.Join(keys, sep) + st.bar.Render(" ")
		if len(keys) == 0 {
			r = pos + st.bar.Render(" ")
		}
	}
	gap := max(m.width-lipgloss.Width(left)-lipgloss.Width(r), 0)
	return st.bar.Width(m.width).MaxWidth(m.width).Render(left + st.bar.Render(strings.Repeat(" ", gap)) + r)
}
