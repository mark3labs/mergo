package tui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mark3labs/mergo/internal/theme"
)

func typeText(e *editor, s string) {
	for _, r := range s {
		if r == '\n' {
			e.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
			continue
		}
		e.handleKey(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func press(e *editor, code rune, mod tea.KeyMod) {
	e.handleKey(tea.KeyPressMsg{Code: code, Mod: mod})
}

func TestEditorEditing(t *testing.T) {
	e := newEditor("graph LR")
	e.setSize(40, 10)
	press(e, tea.KeyEnd, 0)
	typeText(e, "\nsubgraph one\nA --> B")
	if got := e.value(); got != "graph LR\nsubgraph one\n    A --> B" {
		t.Fatalf("auto-indent: %q", got)
	}
	// backspace in the indentation removes a whole level
	press(e, tea.KeyHome, 0)
	press(e, tea.KeyBackspace, 0)
	if got := string(e.line()); got != "A --> B" {
		t.Errorf("dedent by backspace: %q", got)
	}
	// typing coalesces into one undo step per word
	e.setValue("")
	typeText(e, "hello world")
	e.handleKey(tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl})
	if got := e.value(); got != "hello " {
		t.Errorf("undo: %q", got)
	}
	e.handleKey(tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl})
	if got := e.value(); got != "hello world" {
		t.Errorf("redo: %q", got)
	}
	// word movement and deletion
	press(e, tea.KeyLeft, tea.ModCtrl)
	if e.col != 6 {
		t.Errorf("word left: col %d", e.col)
	}
	e.handleKey(tea.KeyPressMsg{Code: 'w', Mod: tea.ModCtrl})
	if got := e.value(); got != "world" {
		t.Errorf("delete word: %q", got)
	}
	// joining lines
	e.setValue("a\nb")
	e.setCursor(1, 0)
	press(e, tea.KeyBackspace, 0)
	if e.value() != "ab" || e.col != 1 {
		t.Errorf("join: %q col %d", e.value(), e.col)
	}
	// multi-line paste is one undo step
	e.paste("x\ny\tz")
	if e.value() != "ax\ny    zb" {
		t.Errorf("paste: %q", e.value())
	}
	e.undoOp()
	if e.value() != "ab" {
		t.Errorf("undo paste: %q", e.value())
	}
}

func TestEditorView(t *testing.T) {
	e := newEditor("graph LR\n  A -->|label| B[\"quoted\"] %% not a comment\n%% comment\n" + strings.Repeat("x", 100))
	e.keywords = keywordSet("flowchart")
	e.setSize(30, 3)
	e.errLine = 2
	st := newEditorStyles(testPalette(t))
	lines := e.view(st, true)
	if len(lines) != 3 {
		t.Fatalf("rows = %d", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != e.gutterWidth()+30 {
			t.Errorf("line %d width %d", i, w)
		}
	}
	// long lines scroll horizontally to keep the cursor visible
	e.setCursor(3, 100)
	e.scrollIntoView()
	if x, y, ok := e.cursorPos(); !ok || x >= 30 || y != 2 {
		t.Errorf("cursor at %d,%d visible=%v (top %d left %d)", x, y, ok, e.top, e.left)
	}

	k := highlight([]rune(`A -->|x| B["graph"]`), keywordSet("flowchart"))
	if k[2] != tokArrow || k[5] != tokPunct || k[11] != tokString {
		t.Errorf("highlight: %v", k)
	}
	if k := highlight([]rune("subgraph x"), keywordSet("flowchart")); k[0] != tokKeyword || k[9] != tokText {
		t.Errorf("keyword: %v", k)
	}
	if k := highlight([]rune("  %% hi"), nil); k[4] != tokComment {
		t.Errorf("comment: %v", k)
	}
}

func completions(t *testing.T, src string) []string {
	t.Helper()
	e := newEditor(strings.ReplaceAll(src, "‸", ""))
	before0, _, _ := strings.Cut(src, "‸")
	before := before0
	e.setCursor(strings.Count(before, "\n"), len([]rune(before[strings.LastIndex(before, "\n")+1:])))
	var out []string
	for _, it := range complete(e, false) {
		out = append(out, it.text)
	}
	return out
}

func TestCompletion(t *testing.T) {
	if got := completions(t, "flow‸"); !slices.Contains(got, "flowchart") {
		t.Errorf("header: %v", got)
	}
	if got := completions(t, "%% comment\ngraph ‸"); len(got) != 0 {
		t.Errorf("no prefix, no popup: %v", got)
	}
	if got := completions(t, "graph L‸"); !slices.Equal(got, []string{"LR"}) {
		t.Errorf("direction: %v", got)
	}
	if got := completions(t, "sequenceDiagram\n  parti‸"); !slices.Equal(got, []string{"participant"}) {
		t.Errorf("sequence keyword: %v", got)
	}
	src := "flowchart LR\n  Alice[Hello there] -->|Help me| Bob\n  Bo‸"
	if got := completions(t, src); !slices.Equal(got, []string{"Bob"}) {
		t.Errorf("node ids: %v", got)
	}
	if got := completions(t, "flowchart LR\n  A[Hello] --> B\n  He‸"); len(got) != 0 {
		t.Errorf("labels must not be completed: %v", got)
	}
	if got := completions(t, "flowchart LR\n  A --> B\n  A-‸"); len(got) != 0 {
		t.Errorf("arrows: %v", got)
	}
	if got := completions(t, "stateDiagram-v2\n  state f <<fo‸"); !slices.Equal(got, []string{"<<fork>>"}) {
		t.Errorf("symbol: %v", got)
	}
	if got := completions(t, `sequenceDiagram
  A->>B: "part‸`); len(got) != 0 {
		t.Errorf("in string: %v", got)
	}
	// names rank first after the start of a line, keywords at the start
	if got := completions(t, "flowchart LR\n  Car --> Bob\n  C‸"); got[0] != "class" {
		t.Errorf("line start: %v", got)
	}
	if got := completions(t, "flowchart LR\n  Car --> Bob\n  A --> C‸"); got[0] != "Car" {
		t.Errorf("mid-line: %v", got)
	}
	// the exact word alone is not offered
	if got := completions(t, "gantt\n  section‸"); len(got) != 0 {
		t.Errorf("exact: %v", got)
	}
	// accepting replaces the prefix
	e := newEditor("stateDiagram-v2\n  state f <<fo")
	e.setCursor(1, 14)
	items := complete(e, false)
	e.replaceBefore(items[0].n, items[0].text)
	if got := string(e.line()); got != "  state f <<fork>>" {
		t.Errorf("accept: %q", got)
	}
}

func TestWriteSource(t *testing.T) {
	dir := t.TempDir()

	mmd := filepath.Join(dir, "a.mmd")
	writeFile(t, mmd, []byte("graph LR\r\nA-->B\r\n"), 0o600)
	_, stored, err := writeSource(mmd, 0, "graph LR\nA-->B\n", "graph TD\nA-->C\n", true)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(mmd)
	if string(data) != "graph TD\r\nA-->C\r\n" || stored != string(data) {
		t.Errorf("mmd: %q", data)
	}
	if st, _ := os.Stat(mmd); st.Mode().Perm() != 0o600 {
		t.Errorf("mode changed: %v", st.Mode())
	}
	if _, _, err := writeSource(mmd, 0, "graph LR\nA-->B\n", "x", false); err == nil || !strings.Contains(err.Error(), "changed on disk") {
		t.Errorf("conflict not detected: %v", err)
	}

	md := filepath.Join(dir, "doc.md")
	doc := "# Doc\n\n- item\n  ```mermaid\n  graph LR\n\n    A-->B\n  ```\n\n```mermaid\npie\n  \"a\": 1\n```\n"
	writeFile(t, md, []byte(doc), 0o644)
	blocks := ExtractMermaidBlocks(doc)
	delta, _, err := writeSource(md, 0, blocks[0].Source, "graph TD\n  A-->B\n  B-->C\n\n  C-->A", false)
	if err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(md)
	want := "# Doc\n\n- item\n  ```mermaid\n  graph TD\n    A-->B\n    B-->C\n\n    C-->A\n  ```\n\n```mermaid\npie\n  \"a\": 1\n```\n"
	if string(data) != want || delta != 2 {
		t.Errorf("md (delta %d):\n%s", delta, data)
	}
	nb := ExtractMermaidBlocks(string(data))
	if nb[1].Line != blocks[1].Line+delta {
		t.Errorf("following block moved to line %d", nb[1].Line)
	}
	if _, _, err := writeSource(md, 1, nb[1].Source, "pie\n```\n", false); err == nil {
		t.Error("a fence inside the source must be refused")
	}
	if _, _, err := writeSource(md, 1, "pie\n  \"b\": 1", "pie", false); err == nil {
		t.Error("block conflict not detected")
	}
}

func TestEditModeFlow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.mmd")
	writeFile(t, path, []byte("graph LR\nA --> B"), 0o644)
	ds, err := LoadInputs([]string{path}, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel([]string{path}, ds, Options{Theme: "nord", Renderer: RendererHalfBlock})
	defer func(a, b time.Duration) { editParseDelay, toastDuration = a, b }(editParseDelay, toastDuration)
	editParseDelay, toastDuration = time.Millisecond, time.Millisecond
	m.cell = CellSize{W: 10, H: 20}
	m.cellFromTerm = true

	var feed func(msg tea.Msg)
	var drain func(cmd tea.Cmd)
	drain = func(cmd tea.Cmd) {
		if cmd == nil {
			return
		}
		switch msg := cmd().(type) {
		case tea.BatchMsg:
			for _, c := range msg {
				drain(c)
			}
		case nil:
		default:
			feed(msg)
		}
	}
	feed = func(msg tea.Msg) {
		switch msg.(type) {
		case toastExpireMsg, pollMsg:
			return
		}
		_, cmd := m.Update(msg)
		drain(cmd)
	}
	key := func(k string) {
		switch k {
		case "esc":
			feed(tea.KeyPressMsg{Code: tea.KeyEscape})
		case "enter":
			feed(tea.KeyPressMsg{Code: tea.KeyEnter})
		case "tab":
			feed(tea.KeyPressMsg{Code: tea.KeyTab})
		case "ctrl+s":
			feed(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
		case "ctrl+space":
			feed(tea.KeyPressMsg{Code: tea.KeySpace, Mod: tea.ModCtrl})
		default:
			for _, r := range k {
				feed(tea.KeyPressMsg{Code: r, Text: string(r)})
			}
		}
	}
	checkView := func() string {
		t.Helper()
		v := m.render()
		lines := strings.Split(v, "\n")
		if len(lines) != 24 {
			t.Errorf("view height %d", len(lines))
		}
		for i, l := range lines {
			if w := ansi.StringWidth(l); w != 100 {
				t.Errorf("view line %d width %d", i, w)
			}
		}
		return ansi.Strip(v)
	}

	drain(m.ensureScene())
	feed(tea.WindowSizeMsg{Width: 100, Height: 24})
	key("e")
	if m.edit == nil {
		t.Fatal("e should open the editor")
	}
	if m.bodyLeft() != 45 {
		t.Errorf("pane width %d", m.bodyLeft())
	}
	if v := checkView(); !strings.Contains(v, "✓ valid flowchart") || !strings.Contains(v, "EDIT") {
		t.Errorf("editor view:\n%s", v)
	}
	if m.editCursor() == nil {
		t.Error("cursor should be shown")
	}

	// break the diagram: the error is reported and saving is refused
	m.edit.ed.setCursor(1, 99)
	key("enter")
	key("A -->")
	if m.edit.err == nil || m.edit.ed.errLine != 3 {
		t.Fatalf("err %v line %d", m.edit.err, m.edit.ed.errLine)
	}
	if v := checkView(); !strings.Contains(v, "✗") {
		t.Errorf("error not shown:\n%s", v)
	}
	key("ctrl+s")
	if data, _ := os.ReadFile(path); string(data) != "graph LR\nA --> B" {
		t.Errorf("invalid source was saved: %q", data)
	}
	if !m.toastErr {
		t.Error("refusal should be reported")
	}

	// fix it with completion of a node id, then save
	key(" ")
	feed(tea.KeyPressMsg{Code: 'B', Text: "B"})
	if m.edit.comp.open && m.edit.comp.engaged {
		t.Error("typing opens completion without engaging enter")
	}
	feed(tea.KeyPressMsg{Code: tea.KeyBackspace})
	key("ctrl+space")
	if !m.edit.comp.open || len(m.edit.comp.items) == 0 {
		t.Fatal("manual completion should open")
	}
	i := slices.IndexFunc(m.edit.comp.items, func(c compItem) bool { return c.text == "B" })
	if i < 0 {
		t.Fatalf("B not offered: %+v", m.edit.comp.items)
	}
	m.edit.comp.sel = i
	key("enter") // engaged by ctrl+space: enter accepts
	if got := string(m.edit.ed.line()); got != "A --> B" {
		t.Fatalf("completed line %q", got)
	}
	if m.edit.err != nil {
		t.Fatalf("still invalid: %v", m.edit.err)
	}
	key("ctrl+s")
	if data, _ := os.ReadFile(path); string(data) != "graph LR\nA --> B\nA --> B" {
		t.Errorf("saved %q", data)
	}
	if m.edit.dirty() || m.diagrams[0].Source != "graph LR\nA --> B\nA --> B" {
		t.Error("state not updated after save")
	}

	// unsaved changes need a second esc
	key("x")
	key("esc")
	if m.edit == nil {
		t.Fatal("first esc should only warn")
	}
	key("esc")
	if m.edit != nil {
		t.Fatal("second esc should discard")
	}
	if m.bodyLeft() != 0 {
		t.Error("body should take the full width again")
	}
	checkView()

	// external changes replace an unmodified buffer
	key("e")
	writeFile(t, path, []byte("graph TD\nA --> C"), 0o644)
	ref := m.editRef()
	msg := m.loadCmd(modTimes(m.paths))()
	m.applyReload(msg.(reloadMsg))
	m.reconcileEdit(ref)
	if got := m.edit.ed.value(); got != "graph TD\nA --> C" {
		t.Errorf("buffer not reloaded: %q", got)
	}

	// narrow terminals give the whole width to the editor
	feed(tea.WindowSizeMsg{Width: 50, Height: 20})
	if cols, _ := m.bodySize(); cols != 0 {
		t.Errorf("preview cols %d", cols)
	}
	v := m.render()
	for i, l := range strings.Split(v, "\n") {
		if w := ansi.StringWidth(l); w != 50 {
			t.Errorf("narrow line %d width %d", i, w)
		}
	}
}

func TestEditStdinCannotSave(t *testing.T) {
	ds := parseInput("-", "graph LR\nA-->B")
	m := NewModel([]string{"-"}, ds, Options{Theme: "nord", Renderer: RendererHalfBlock})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if !m.toastErr || !strings.Contains(m.toast, "stdin") {
		t.Errorf("toast %q", m.toast)
	}
}

func testPalette(t *testing.T) theme.UI {
	t.Helper()
	p, err := theme.Palette("nord", true)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func writeFile(t *testing.T, path string, data []byte, perm os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, perm); err != nil {
		t.Fatal(err)
	}
}
