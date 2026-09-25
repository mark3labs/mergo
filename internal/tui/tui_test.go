package tui

import (
	"image"
	"image/color"
	"math"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"

	_ "github.com/mark3labs/mergo/internal/mermaid" // register diagram types
	"github.com/mark3labs/mergo/internal/theme"
)

func TestExtractMermaidBlocks(t *testing.T) {
	md := "# Title\n\n```mermaid\ngraph LR\n  A-->B\n```\n\ntext\n\n~~~ mermaid\npie\n  \"a\": 1\n~~~\n\n```go\nfmt.Println()\n```\n  ```mermaid\n  sequenceDiagram\n    A->>B: hi\n  ```\n````mermaid\nflowchart TD\n```\nnested fence\n````\n"
	blocks := ExtractMermaidBlocks(md)
	if len(blocks) != 4 {
		t.Fatalf("got %d blocks: %+v", len(blocks), blocks)
	}
	if blocks[0].Line != 4 || blocks[0].Source != "graph LR\n  A-->B" {
		t.Errorf("block 0 = %+v", blocks[0])
	}
	if !strings.HasPrefix(blocks[1].Source, "pie") {
		t.Errorf("block 1 = %+v", blocks[1])
	}
	if blocks[2].Source != "sequenceDiagram\n  A->>B: hi" {
		t.Errorf("indented block = %q", blocks[2].Source)
	}
	if !strings.Contains(blocks[3].Source, "```\nnested fence") {
		t.Errorf("long fence block = %q", blocks[3].Source)
	}
}

func TestParseInput(t *testing.T) {
	ds := parseInput("-", "text\n```mermaid\ngraph LR; A-->B\n```\n")
	if len(ds) != 1 || ds[0].Source != "graph LR; A-->B" || ds[0].Line != 3 {
		t.Errorf("stdin markdown detection: %+v", ds)
	}
	ds = parseInput("x.mmd", "graph LR; A-->B")
	if len(ds) != 1 || ds[0].Line != 1 {
		t.Errorf("mmd: %+v", ds)
	}
	if ds[0].Kind() != "flowchart" {
		t.Errorf("kind = %s", ds[0].Kind())
	}
	if savePath(&Diagram{Path: "docs/a.md", Block: 1}) != "docs/a-2.png" || savePath(&Diagram{Path: "a.mmd"}) != "a.png" {
		t.Error("savePath")
	}
}

func TestPlaceholderGrid(t *testing.T) {
	lines := placeholderGrid(200, 4, 3)
	if len(lines) != 3 {
		t.Fatalf("rows = %d", len(lines))
	}
	for r, l := range lines {
		if w := ansi.StringWidth(l); w != 4 {
			t.Errorf("row %d width = %d", r, w)
		}
		if !strings.HasPrefix(l, "\x1b[38;5;200m") {
			t.Errorf("row %d missing id color: %q", r, l)
		}
		want := string(kitty.Placeholder) + string(kitty.Diacritic(r)) + string(kitty.Diacritic(2))
		if !strings.Contains(l, want) {
			t.Errorf("row %d missing cell (r=%d,c=2)", r, r)
		}
	}
	// the placeholders must survive ultraviolet's cell model with width 1
	buf := uv.NewScreenBuffer(4, 1)
	uv.NewStyledString(lines[1]).Draw(buf, buf.Bounds())
	for x := range 4 {
		c := buf.CellAt(x, 0)
		if c == nil || c.Width != 1 || !strings.HasPrefix(c.Content, string(kitty.Placeholder)) {
			t.Fatalf("cell %d = %+v", x, c)
		}
	}
}

func TestKittyEscapes(t *testing.T) {
	data := make([]byte, 10000)
	s := kittyTransmit(7, data, false)
	if !strings.HasPrefix(s, "\x1b_Ga=t,f=100,t=d,i=7,q=2,m=1;") {
		t.Errorf("transmit prefix: %q", s[:40])
	}
	chunks := strings.Count(s, "\x1b_G")
	if chunks < 3 {
		t.Errorf("expected chunked transmission, got %d chunks", chunks)
	}
	if !strings.Contains(s, "\x1b_Gm=0;") {
		t.Error("last chunk must have m=0")
	}
	if got := kittyVirtualPlacement(7, 80, 20, false); got != "\x1b_Ga=p,U=1,i=7,c=80,r=20,q=2\x1b\\" {
		t.Errorf("placement = %q", got)
	}
	if got := wrapTmux("\x1b_Gx\x1b\\"); got != "\x1bPtmux;\x1b\x1b_Gx\x1b\x1b\\\x1b\\" {
		t.Errorf("tmux = %q", got)
	}
	if kittyDelete(9, false) != "\x1b_Ga=d,d=I,i=9,q=2\x1b\\" {
		t.Error("delete")
	}
	id := imageIDBase()
	if id < 16 || id+3 > 255 {
		t.Errorf("image id base %d out of 256-color range", id)
	}
}

func TestDownsampleGamma(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{255, 255, 255, 255})
	img.Set(1, 0, color.RGBA{0, 0, 0, 255})
	img.Set(0, 1, color.RGBA{255, 255, 255, 255})
	img.Set(1, 1, color.RGBA{0, 0, 0, 255})
	out := downsample(img, 1, 1)
	// linear average of black and white is ~188 in sRGB (not 128)
	if v := out.RGBAAt(0, 0).R; v < 180 || v > 195 {
		t.Errorf("gamma-correct average = %d", v)
	}
}

func TestHalfBlocks(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 4))
	red := color.RGBA{255, 0, 0, 255}
	blue := color.RGBA{0, 0, 255, 255}
	for x := range 2 {
		img.Set(x, 0, red)
		img.Set(x, 1, blue)
		img.Set(x, 2, blue)
		img.Set(x, 3, blue)
	}
	lines := halfBlocks(img, 2, 2)
	if len(lines) != 2 {
		t.Fatal(len(lines))
	}
	if !strings.Contains(lines[0], "▀") || !strings.Contains(lines[0], "38;2;255;0;0") || !strings.Contains(lines[0], "48;2;0;0;255") {
		t.Errorf("row 0 = %q", lines[0])
	}
	if strings.Contains(lines[1], "▀") {
		t.Errorf("uniform row should use spaces: %q", lines[1])
	}
	for _, l := range lines {
		if ansi.StringWidth(l) != 2 {
			t.Errorf("width %d", ansi.StringWidth(l))
		}
	}
}

func TestCamera(t *testing.T) {
	var c camera
	c.Fit(1000, 500, 800, 800)
	if math.Abs(c.zoom-0.8*0.94) > 1e-9 || c.cx != 500 || c.cy != 250 || !c.fit {
		t.Errorf("fit: %+v", c)
	}
	// small scenes are not blown up beyond maxFitZoom
	var s camera
	s.Fit(50, 50, 2000, 2000)
	if s.zoom != maxFitZoom {
		t.Errorf("fit zoom cap: %v", s.zoom)
	}
	// zooming keeps the anchor fixed
	c2 := camera{zoom: 1, cx: 100, cy: 100}
	before := c2.cx + (10-400)/c2.zoom
	c2.Zoom(2, 10, 300, 800, 600)
	after := c2.cx + (10-400)/c2.zoom
	if math.Abs(before-after) > 1e-9 || c2.fit {
		t.Errorf("anchor moved: %v -> %v", before, after)
	}
	// clamp: a scene smaller than the view is centered
	c3 := camera{zoom: 1, cx: -500, cy: 9999}
	c3.Clamp(100, 100, 800, 600)
	if c3.cx != 50 || c3.cy != 50 {
		t.Errorf("clamp small: %+v", c3)
	}
	// large scene: cannot scroll past the edges
	c4 := camera{zoom: 1, cx: -500, cy: 9999}
	c4.Clamp(2000, 2000, 800, 600)
	if c4.cx != 400 || c4.cy != 1700 {
		t.Errorf("clamp large: %+v", c4)
	}
	vp := c4.Viewport(800, 600, 1)
	if vp.X != 0 || vp.Y != 1400 || vp.W != 800 || vp.H != 600 {
		t.Errorf("viewport %+v", vp)
	}
}

func TestParseRenderer(t *testing.T) {
	for in, want := range map[string]Renderer{"": RendererAuto, "kitty": RendererKitty, "halfblock": RendererHalfBlock, "half-block": RendererHalfBlock, "sixel": RendererSixel} {
		if got, ok := ParseRenderer(in); !ok || got != want {
			t.Errorf("%q -> %v %v", in, got, ok)
		}
	}
	if _, ok := ParseRenderer("ascii-art"); ok {
		t.Error("unknown renderer accepted")
	}
	env := map[string]string{"TERM": "xterm-kitty"}
	if !kittyHint(func(k string) string { return env[k] }) {
		t.Error("kitty hint")
	}
	env = map[string]string{"TERM_PROGRAM": "ghostty"}
	if !kittyHint(func(k string) string { return env[k] }) {
		t.Error("ghostty hint")
	}
	env = map[string]string{"TERM": "xterm-256color"}
	if kittyHint(func(k string) string { return env[k] }) {
		t.Error("xterm hint")
	}
}

// TestModelFlow drives the model through detection, parsing and rendering
// without a terminal.
func TestModelFlow(t *testing.T) {
	ds := parseInput("x.mmd", "graph LR\nA[Hello] --> B{World}")
	m := NewModel([]string{"x.mmd"}, ds, Options{Theme: "nord", Placement: PlacementUnicode})
	m.cell = CellSize{W: 10, H: 20}
	m.cellFromTerm = true

	run := func(cmd tea.Cmd) []tea.Msg {
		var out []tea.Msg
		var exec func(c tea.Cmd)
		exec = func(c tea.Cmd) {
			if c == nil {
				return
			}
			msg := c()
			switch v := msg.(type) {
			case tea.BatchMsg:
				for _, cc := range v {
					exec(cc)
				}
			case nil:
			default:
				out = append(out, msg)
			}
		}
		exec(cmd)
		return out
	}
	var feed func(msg tea.Msg)
	feed = func(msg tea.Msg) {
		_, cmd := m.Update(msg)
		for _, out := range run(cmd) {
			switch out.(type) {
			case renderTickMsg, sceneMsg, renderDoneMsg:
				feed(out)
			}
		}
	}

	// parse
	for _, msg := range run(m.ensureScene()) {
		feed(msg)
	}
	feed(tea.WindowSizeMsg{Width: 60, Height: 20})
	if !m.probing {
		t.Fatal("should be probing")
	}
	// DA1 arrives without a kitty reply -> half blocks
	feed(uv.PrimaryDeviceAttributesEvent{62, 22})
	if m.probing || m.mode != RendererHalfBlock {
		t.Fatalf("mode = %v probing=%v", m.mode, m.probing)
	}
	if len(m.body) != 18 {
		t.Fatalf("body rows = %d", len(m.body))
	}
	view := m.render()
	if h := strings.Count(view, "\n") + 1; h != 20 {
		t.Errorf("view height = %d", h)
	}
	for i, l := range strings.Split(view, "\n") {
		if w := ansi.StringWidth(l); w != 60 {
			t.Errorf("view line %d width = %d", i, w)
		}
	}
	if !strings.Contains(view, "flowchart") || !strings.Contains(view, "half-block") {
		t.Error("status bar missing info")
	}

	// kitty reply switches to placeholders when renderer is toggled
	feed(uv.KittyGraphicsEvent{Options: kitty.Options{ID: probeImageID}, Payload: []byte("OK")})
	if !m.kittyOK {
		t.Fatal("kitty not detected")
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	var raw bool
	for _, out := range run(cmd) {
		if _, ok := out.(renderTickMsg); ok {
			_, c2 := m.Update(out)
			for _, o2 := range run(c2) {
				if rd, ok := o2.(renderDoneMsg); ok {
					raw = rd.raw != ""
					m.Update(rd)
				}
			}
		}
	}
	if m.mode != RendererKitty || !raw {
		t.Fatalf("kitty render: mode=%v raw=%v", m.mode, raw)
	}
	if !strings.Contains(m.body[0], string(kitty.Placeholder)) {
		t.Error("kitty body should contain placeholders")
	}

	// zooming changes the camera and marks it as non-fit
	z := m.cam().zoom
	m.Update(tea.KeyPressMsg{Code: '+', Text: "+"})
	if m.cam().zoom <= z || m.cam().fit {
		t.Errorf("zoom in: %v -> %v", z, m.cam().zoom)
	}

	// errors are shown with the offending line
	bad := parseInput("y.mmd", "graph LR\nA --> B\nA -->")
	m2 := NewModel([]string{"y.mmd"}, bad, Options{Theme: "nord", Renderer: RendererHalfBlock})
	m2.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	for _, msg := range run(m2.ensureScene()) {
		m2.Update(msg)
	}
	v := m2.render()
	if !strings.Contains(v, "line 3") || !strings.Contains(v, "A -->") {
		t.Errorf("error view missing details:\n%s", ansi.Strip(v))
	}
}

func TestThemePicker(t *testing.T) {
	ds := parseInput("x.mmd", "graph LR\nA --> B")
	var saved []string
	m := NewModel([]string{"x.mmd"}, ds, Options{
		Theme: "nord", Renderer: RendererHalfBlock,
		SaveTheme: func(name string) error { saved = append(saved, name); return nil },
	})
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	key := func(code rune, text string) { m.Update(tea.KeyPressMsg{Code: code, Text: text}) }

	// t opens the picker on the active theme; moving previews without saving
	key('t', "t")
	if !m.picker.open || m.themes[m.picker.idx] != "nord" {
		t.Fatalf("picker open=%v idx=%d", m.picker.open, m.picker.idx)
	}
	key(tea.KeyDown, "")
	preview := m.themeName()
	if preview == "nord" || len(saved) != 0 {
		t.Fatalf("preview = %q, saved = %v", preview, saved)
	}
	view := m.render()
	if !strings.Contains(view, "Theme") || !strings.Contains(view, preview) {
		t.Error("picker not rendered")
	}
	for i, l := range strings.Split(view, "\n") {
		if w := ansi.StringWidth(l); w > 80 {
			t.Errorf("view line %d overflows: width %d", i, w)
		}
	}

	// esc restores the original theme and keeps the viewer running
	key(tea.KeyEscape, "")
	if m.picker.open || m.themeName() != "nord" || len(saved) != 0 {
		t.Fatalf("after cancel: open=%v theme=%q saved=%v", m.picker.open, m.themeName(), saved)
	}

	// enter keeps and persists the selection
	key('t', "t")
	key(tea.KeyDown, "")
	key(tea.KeyEnter, "")
	if m.picker.open || m.themeName() != preview || len(saved) != 1 || saved[0] != preview {
		t.Fatalf("after confirm: open=%v theme=%q saved=%v", m.picker.open, m.themeName(), saved)
	}

	// clicking an entry picks it
	key('t', "t")
	m.render()
	_, _ = m.Update(tea.MouseClickMsg{X: m.picker.box.Min.X + 3, Y: m.picker.first, Button: tea.MouseLeft})
	if m.picker.open || m.themeName() != m.themes[m.picker.top] || len(saved) != 2 {
		t.Fatalf("after click: open=%v theme=%q saved=%v", m.picker.open, m.themeName(), saved)
	}
}

func TestChromeFollowsTheme(t *testing.T) {
	ds := parseInput("x.mmd", "graph LR\nA --> B")
	m := NewModel([]string{"x.mmd"}, ds, Options{Theme: "nord", Dark: true, Renderer: RendererHalfBlock})
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	bg := func() color.Color { return m.st.bar.GetBackground() }
	nord := bg()
	want, _ := theme.Palette("nord", true)
	if !sameColor(nord, want.Faint) {
		t.Fatalf("bar background = %v, want nord's %v", nord, want.Faint)
	}
	// previewing a theme in the picker restyles the chrome; cancel restores it
	m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if sameColor(bg(), nord) {
		t.Error("chrome not restyled for the previewed theme")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !sameColor(bg(), nord) {
		t.Error("chrome not restored after cancel")
	}
	// the light variant uses the light palette
	light := NewModel([]string{"x.mmd"}, ds, Options{Theme: "nord", Renderer: RendererHalfBlock})
	lp, _ := theme.Palette("nord", false)
	if !sameColor(light.st.bar.GetBackground(), lp.Faint) {
		t.Error("light variant not used")
	}
}

func sameColor(a, b color.Color) bool {
	r1, g1, b1, a1 := a.RGBA()
	r2, g2, b2, a2 := b.RGBA()
	return r1 == r2 && g1 == g2 && b1 == b2 && a1 == a2
}
