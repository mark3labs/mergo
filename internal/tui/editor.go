package tui

import (
	"image/color"
	"strconv"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// editor is a small multi-line text editor for Mermaid source: no soft
// wrapping (long lines scroll horizontally), undo/redo, auto-indent and
// light syntax highlighting. It knows nothing about the viewer; the model
// wires it to validation, completion and saving.
type editor struct {
	lines    [][]rune
	row, col int // cursor (col is a rune index)
	goal     int // display column kept by vertical moves (-1 = none)

	// viewport: first visible line and display column, and the text area
	// size (excluding the gutter)
	top, left     int
	width, height int

	undo, redo []edSnap
	lastOp     string

	// errLine is the 1-based line of the current syntax error (0 = none).
	errLine int
	// keywords are highlighted (set from the detected diagram type).
	keywords map[string]bool

	cache      string
	cacheValid bool
}

type edSnap struct {
	lines    [][]rune
	row, col int
}

const (
	editTabWidth = 4
	editIndent   = "    "
	maxUndo      = 500
)

func newEditor(src string) *editor {
	e := &editor{goal: -1}
	e.setValue(src)
	return e
}

func (e *editor) setValue(src string) {
	e.lines = nil
	for l := range strings.SplitSeq(src, "\n") {
		e.lines = append(e.lines, []rune(l))
	}
	e.row, e.col, e.goal = 0, 0, -1
	e.top, e.left = 0, 0
	e.undo, e.redo, e.lastOp = nil, nil, ""
	e.changed()
}

// replaceAll replaces the whole buffer (an undoable step), keeping the
// cursor and the scroll position where possible.
func (e *editor) replaceAll(src string) {
	e.snapshot("")
	row, col, top, left := e.row, e.col, e.top, e.left
	undo := e.undo
	e.setValue(src)
	e.undo = undo
	e.setCursor(row, col)
	e.top, e.left = top, left
	e.scrollIntoView()
}

// value returns the buffer content.
func (e *editor) value() string {
	if !e.cacheValid {
		var sb strings.Builder
		for i, l := range e.lines {
			if i > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString(string(l))
		}
		e.cache, e.cacheValid = sb.String(), true
	}
	return e.cache
}

func (e *editor) changed() { e.cacheValid = false }

func (e *editor) line() []rune { return e.lines[e.row] }

// ---------------------------------------------------------------------------
// undo

func (e *editor) snapshot(op string) {
	if op != "" && op == e.lastOp {
		return // coalesce runs of typing / deleting
	}
	e.lastOp = op
	e.undo = append(e.undo, e.snap())
	if len(e.undo) > maxUndo {
		e.undo = e.undo[1:]
	}
	e.redo = nil
}

func (e *editor) snap() edSnap {
	lines := make([][]rune, len(e.lines))
	for i, l := range e.lines {
		lines[i] = append([]rune(nil), l...)
	}
	return edSnap{lines: lines, row: e.row, col: e.col}
}

func (e *editor) restore(s edSnap) {
	e.lines, e.row, e.col = s.lines, s.row, s.col
	e.goal, e.lastOp = -1, ""
	e.changed()
}

func (e *editor) undoOp() bool {
	if len(e.undo) == 0 {
		return false
	}
	e.redo = append(e.redo, e.snap())
	e.restore(e.undo[len(e.undo)-1])
	e.undo = e.undo[:len(e.undo)-1]
	return true
}

func (e *editor) redoOp() bool {
	if len(e.redo) == 0 {
		return false
	}
	e.undo = append(e.undo, e.snap())
	e.restore(e.redo[len(e.redo)-1])
	e.redo = e.redo[:len(e.redo)-1]
	return true
}

// ---------------------------------------------------------------------------
// editing

// insert inserts text (which may contain newlines) at the cursor.
func (e *editor) insert(s string) {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	if s == "" {
		return
	}
	parts := strings.Split(s, "\n")
	cur := e.line()
	head := append([]rune(nil), cur[:e.col]...)
	tail := append([]rune(nil), cur[e.col:]...)
	if len(parts) == 1 {
		r := []rune(parts[0])
		e.lines[e.row] = append(append(head, r...), tail...)
		e.col += len(r)
	} else {
		nl := make([][]rune, 0, len(parts))
		nl = append(nl, append(head, []rune(parts[0])...))
		for _, p := range parts[1 : len(parts)-1] {
			nl = append(nl, []rune(p))
		}
		last := []rune(parts[len(parts)-1])
		nl = append(nl, append(last, tail...))
		e.lines = append(e.lines[:e.row], append(nl, e.lines[e.row+1:]...)...)
		e.row += len(parts) - 1
		e.col = len(last)
	}
	e.goal = -1
	e.changed()
}

// replaceBefore replaces the n runes before the cursor with s (completion).
func (e *editor) replaceBefore(n int, s string) {
	e.snapshot("")
	l := e.line()
	n = min(n, e.col)
	e.lines[e.row] = append(l[:e.col-n:e.col-n], l[e.col:]...)
	e.col -= n
	e.insert(s)
	e.lastOp = ""
}

// blockOpeners start an indented block in some diagram type.
var blockOpeners = map[string]bool{
	"subgraph": true, "loop": true, "alt": true, "opt": true, "par": true,
	"critical": true, "break": true, "rect": true, "box": true, "else": true,
	"and": true, "option": true, "namespace": true, "section": true,
}

// newline splits the line, keeping the indentation (one level more after a
// block opener or an opening brace).
func (e *editor) newline() {
	cur := e.line()
	indent := leadingSpace(cur)
	before := strings.TrimSpace(string(cur[:e.col]))
	if strings.HasSuffix(before, "{") {
		indent += editIndent
	} else if f := strings.Fields(before); len(f) > 0 && blockOpeners[f[0]] {
		indent += editIndent
	}
	// drop trailing spaces left on the split line
	head := []rune(strings.TrimRight(string(cur[:e.col]), " \t"))
	tail := []rune(strings.TrimLeft(string(cur[e.col:]), " \t"))
	e.lines[e.row] = head
	nl := append([]rune(indent), tail...)
	e.lines = append(e.lines[:e.row+1], append([][]rune{nl}, e.lines[e.row+1:]...)...)
	e.row++
	e.col = len([]rune(indent))
	e.goal = -1
	e.changed()
}

func leadingSpace(l []rune) string {
	i := 0
	for i < len(l) && (l[i] == ' ' || l[i] == '\t') {
		i++
	}
	return string(l[:i])
}

func (e *editor) backspace() bool {
	if e.col == 0 && e.row == 0 {
		return false
	}
	if e.col == 0 {
		prev := e.lines[e.row-1]
		e.col = len(prev)
		e.lines[e.row-1] = append(prev, e.line()...)
		e.lines = append(e.lines[:e.row], e.lines[e.row+1:]...)
		e.row--
	} else {
		l := e.line()
		n := 1
		// in leading indentation, remove back to the previous indent stop
		if strings.TrimSpace(string(l[:e.col])) == "" && e.col%editTabWidth == 0 {
			n = 0
			for n < editTabWidth && e.col-n > 0 && l[e.col-n-1] == ' ' {
				n++
			}
			n = max(n, 1)
		}
		e.lines[e.row] = append(l[:e.col-n:e.col-n], l[e.col:]...)
		e.col -= n
	}
	e.goal = -1
	e.changed()
	return true
}

func (e *editor) deleteForward() bool {
	l := e.line()
	if e.col < len(l) {
		e.lines[e.row] = append(l[:e.col:e.col], l[e.col+1:]...)
	} else if e.row < len(e.lines)-1 {
		e.lines[e.row] = append(l, e.lines[e.row+1]...)
		e.lines = append(e.lines[:e.row+1], e.lines[e.row+2:]...)
	} else {
		return false
	}
	e.changed()
	return true
}

func (e *editor) deleteWordBack() bool {
	if e.col == 0 {
		return e.backspace()
	}
	to := e.wordLeftCol()
	l := e.line()
	e.lines[e.row] = append(l[:to:to], l[e.col:]...)
	e.col = to
	e.changed()
	return true
}

func (e *editor) killToEnd() bool {
	l := e.line()
	if e.col == len(l) {
		return e.deleteForward()
	}
	e.lines[e.row] = l[:e.col:e.col]
	e.changed()
	return true
}

func (e *editor) killToStart() bool {
	if e.col == 0 {
		return false
	}
	e.lines[e.row] = append([]rune(nil), e.line()[e.col:]...)
	e.col = 0
	e.changed()
	return true
}

// dedent removes one indentation level from the cursor line.
func (e *editor) dedent() bool {
	l := e.line()
	n := 0
	for n < editTabWidth && n < len(l) && l[n] == ' ' {
		n++
	}
	if n == 0 && len(l) > 0 && l[0] == '\t' {
		n = 1
	}
	if n == 0 {
		return false
	}
	e.lines[e.row] = l[n:]
	e.col = max(e.col-n, 0)
	e.changed()
	return true
}

// ---------------------------------------------------------------------------
// movement

func isWordRune(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }

func (e *editor) wordLeftCol() int {
	l, c := e.line(), e.col
	for c > 0 && !isWordRune(l[c-1]) {
		c--
	}
	for c > 0 && isWordRune(l[c-1]) {
		c--
	}
	return c
}

func (e *editor) wordRightCol() int {
	l, c := e.line(), e.col
	for c < len(l) && !isWordRune(l[c]) {
		c++
	}
	for c < len(l) && isWordRune(l[c]) {
		c++
	}
	return c
}

func (e *editor) moveVert(d int) {
	if e.goal < 0 {
		e.goal = dispCol(e.line(), e.col)
	}
	e.row = min(max(e.row+d, 0), len(e.lines)-1)
	e.col = colAt(e.line(), e.goal)
}

func (e *editor) setCursor(row, col int) {
	e.row = min(max(row, 0), len(e.lines)-1)
	e.col = min(max(col, 0), len(e.line()))
	e.goal = -1
}

// handleKey applies an editing key. It reports whether the key was handled
// and whether the text changed.
func (e *editor) handleKey(msg tea.KeyPressMsg) (handled, changed bool) {
	k := msg.String()
	move := func(row, col int) { e.setCursor(row, col); e.lastOp = "" }
	edit := func(op string, f func() bool) {
		snap := e.snap()
		lastOp := e.lastOp
		if f() {
			if op == "" || op != lastOp {
				e.undo = append(e.undo, snap)
				if len(e.undo) > maxUndo {
					e.undo = e.undo[1:]
				}
				e.redo = nil
			}
			e.lastOp = op
			changed = true
		}
	}
	handled = true
	switch k {
	case "left", "ctrl+b":
		if e.col > 0 {
			move(e.row, e.col-1)
		} else if e.row > 0 {
			move(e.row-1, len(e.lines[e.row-1]))
		}
	case "right", "ctrl+f":
		if e.col < len(e.line()) {
			move(e.row, e.col+1)
		} else if e.row < len(e.lines)-1 {
			move(e.row+1, 0)
		}
	case "up":
		e.moveVert(-1)
		e.lastOp = ""
	case "down":
		e.moveVert(1)
		e.lastOp = ""
	case "pgup":
		e.moveVert(-max(e.height-1, 1))
		e.lastOp = ""
	case "pgdown":
		e.moveVert(max(e.height-1, 1))
		e.lastOp = ""
	case "home", "ctrl+a":
		// toggle between the first non-blank character and column 0
		ind := len([]rune(leadingSpace(e.line())))
		if e.col == ind {
			ind = 0
		}
		move(e.row, ind)
	case "end", "ctrl+e":
		move(e.row, len(e.line()))
	case "ctrl+home":
		move(0, 0)
	case "ctrl+end":
		move(len(e.lines)-1, len(e.lines[len(e.lines)-1]))
	case "ctrl+left", "alt+left", "alt+b":
		if e.col == 0 && e.row > 0 {
			move(e.row-1, len(e.lines[e.row-1]))
		} else {
			move(e.row, e.wordLeftCol())
		}
	case "ctrl+right", "alt+right", "alt+f":
		if e.col == len(e.line()) && e.row < len(e.lines)-1 {
			move(e.row+1, 0)
		} else {
			move(e.row, e.wordRightCol())
		}
	case "enter", "ctrl+j", "ctrl+m":
		edit("", func() bool { e.newline(); return true })
	case "backspace", "ctrl+h":
		edit("del", e.backspace)
	case "delete", "ctrl+d":
		edit("delf", e.deleteForward)
	case "ctrl+w", "alt+backspace", "ctrl+backspace":
		edit("", e.deleteWordBack)
	case "ctrl+k":
		edit("", e.killToEnd)
	case "ctrl+u":
		edit("", e.killToStart)
	case "tab":
		edit("", func() bool {
			n := editTabWidth - dispCol(e.line(), e.col)%editTabWidth
			e.insert(strings.Repeat(" ", n))
			return true
		})
	case "shift+tab":
		edit("", e.dedent)
	case "ctrl+z":
		changed = e.undoOp()
	case "ctrl+y", "ctrl+shift+z":
		changed = e.redoOp()
	default:
		if msg.Text == "" || msg.Mod.Contains(tea.ModCtrl) || msg.Mod.Contains(tea.ModAlt) {
			return false, false
		}
		op := "ins"
		if strings.TrimSpace(msg.Text) == "" {
			op = "" // a new undo step per word
		}
		edit(op, func() bool { e.insert(msg.Text); return true })
	}
	e.scrollIntoView()
	return handled, changed
}

// paste inserts pasted text as a single undo step.
func (e *editor) paste(s string) bool {
	if s == "" {
		return false
	}
	e.snapshot("")
	e.insert(strings.ReplaceAll(s, "\t", editIndent))
	e.lastOp = ""
	e.scrollIntoView()
	return true
}

// ---------------------------------------------------------------------------
// display

// runeWidth is the display width of r in the editor (tabs are expanded to
// a fixed width).
func runeWidth(r rune) int {
	if r == '\t' {
		return editTabWidth
	}
	return max(ansi.StringWidth(string(r)), 0)
}

// dispCol is the display column of rune index col in l.
func dispCol(l []rune, col int) int {
	w := 0
	for _, r := range l[:min(col, len(l))] {
		w += runeWidth(r)
	}
	return w
}

// colAt returns the rune index at display column x of l.
func colAt(l []rune, x int) int {
	w := 0
	for i, r := range l {
		rw := runeWidth(r)
		if w+rw > x {
			return i
		}
		w += rw
	}
	return len(l)
}

func (e *editor) setSize(w, h int) {
	e.width, e.height = max(w, 1), max(h, 1)
	e.scrollIntoView()
}

func (e *editor) scrollIntoView() {
	if e.height <= 0 {
		return
	}
	if e.row < e.top {
		e.top = e.row
	}
	if e.row >= e.top+e.height {
		e.top = e.row - e.height + 1
	}
	e.top = max(min(e.top, len(e.lines)-1), 0)
	x := dispCol(e.line(), e.col)
	margin := min(4, e.width/4)
	if x < e.left+margin {
		e.left = max(x-margin, 0)
	}
	if x >= e.left+e.width-1 {
		e.left = x - e.width + 2
	}
}

// scroll moves the viewport by d lines without moving the cursor.
func (e *editor) scroll(d int) {
	e.top = min(max(e.top+d, 0), max(len(e.lines)-e.height, 0))
}

// cursorPos returns the cursor position relative to the text area, and
// whether it is visible.
func (e *editor) cursorPos() (x, y int, ok bool) {
	x = dispCol(e.line(), e.col) - e.left
	y = e.row - e.top
	return x, y, x >= 0 && x < e.width && y >= 0 && y < e.height
}

func (e *editor) gutterWidth() int {
	return len(strconv.Itoa(max(len(e.lines), 99))) + 2
}

// editorStyles are the colors of the editor pane.
type editorStyles struct {
	bg, curBg       color.Color
	text            lipgloss.Style
	lineNo, lineCur lipgloss.Style
	lineErr         lipgloss.Style
	tok             [tokCount]lipgloss.Style
}

// view renders the visible lines (gutter included), each exactly
// gutterWidth()+width cells wide.
func (e *editor) view(st editorStyles, focused bool) []string {
	gw := e.gutterWidth()
	out := make([]string, 0, e.height)
	for y := range e.height {
		i := e.top + y
		if i >= len(e.lines) {
			out = append(out, st.text.Background(st.bg).Render(strings.Repeat(" ", gw+e.width)))
			continue
		}
		bg := st.bg
		if focused && i == e.row {
			bg = st.curBg
		}
		ns := st.lineNo
		switch {
		case e.errLine == i+1:
			ns = st.lineErr
		case i == e.row:
			ns = st.lineCur
		}
		num := ns.Render(padLeft(strconv.Itoa(i+1), gw-1) + " ")
		out = append(out, num+e.renderLine(e.lines[i], st, bg))
	}
	return out
}

func padLeft(s string, w int) string {
	if n := w - len(s); n > 0 {
		return strings.Repeat(" ", n) + s
	}
	return s
}

// renderLine renders the visible part of l with syntax highlighting.
func (e *editor) renderLine(l []rune, st editorStyles, bg color.Color) string {
	kinds := highlight(l, e.keywords)
	var sb, run strings.Builder
	cur := tokKind(-1)
	flush := func() {
		if run.Len() > 0 {
			sb.WriteString(st.tok[cur].Background(bg).Render(run.String()))
			run.Reset()
		}
	}
	x, used := 0, 0
	for i, r := range l {
		rw := runeWidth(r)
		if x+rw <= e.left {
			x += rw
			continue
		}
		if used+rw > e.width {
			break
		}
		s := string(r)
		switch {
		case r == '\t':
			s = strings.Repeat(" ", rw)
		case x < e.left: // wide rune cut by the left edge
			s, rw = strings.Repeat(" ", x+rw-e.left), x+rw-e.left
		case rw == 0 || unicode.IsControl(r):
			s, rw = "?", 1
		}
		if kinds[i] != cur {
			flush()
			cur = kinds[i]
		}
		run.WriteString(s)
		x += rw
		used += rw
	}
	flush()
	if used < e.width {
		sb.WriteString(st.text.Background(bg).Render(strings.Repeat(" ", e.width-used)))
	}
	return sb.String()
}

// ---------------------------------------------------------------------------
// highlighting

type tokKind int

const (
	tokText tokKind = iota
	tokKeyword
	tokString
	tokNumber
	tokComment
	tokArrow
	tokPunct
	tokCount
)

func isArrowRune(r rune) bool { return strings.ContainsRune("-=.<>~", r) }

// highlight classifies each rune of a line.
func highlight(l []rune, keywords map[string]bool) []tokKind {
	kinds := make([]tokKind, len(l))
	s := strings.TrimSpace(string(l))
	if strings.HasPrefix(s, "%%") {
		for i := range kinds {
			kinds[i] = tokComment
		}
		return kinds
	}
	fill := func(a, b int, k tokKind) {
		for j := a; j < b; j++ {
			kinds[j] = k
		}
	}
	for i := 0; i < len(l); {
		r := l[i]
		switch {
		case r == '"':
			j := i + 1
			for j < len(l) && l[j] != '"' {
				j++
			}
			j = min(j+1, len(l))
			fill(i, j, tokString)
			i = j
		case unicode.IsLetter(r) || r == '_':
			j := i
			for j < len(l) && (isWordRune(l[j]) || l[j] == '-' && j+1 < len(l) && isWordRune(l[j+1]) && j > i) {
				j++
			}
			if keywords[string(l[i:j])] {
				fill(i, j, tokKeyword)
			}
			i = j
		case unicode.IsDigit(r):
			j := i
			for j < len(l) && (unicode.IsDigit(l[j]) || l[j] == '.') {
				j++
			}
			if j == len(l) || !isWordRune(l[j]) {
				fill(i, j, tokNumber)
			}
			for j < len(l) && isWordRune(l[j]) {
				j++
			}
			i = j
		case isArrowRune(r):
			j := i
			for j < len(l) && isArrowRune(l[j]) {
				j++
			}
			if j-i >= 2 {
				fill(i, j, tokArrow)
			}
			i = j
		case strings.ContainsRune("[](){}|:;,&", r):
			kinds[i] = tokPunct
			i++
		default:
			i++
		}
	}
	return kinds
}
