package tui

import (
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// decodeSixel decodes a DCS sequence produced by encodeSixel (a minimal
// decoder: raster attributes, RGB palette, repeats, CR and LF). Unpainted
// pixels have zero alpha.
func decodeSixel(t *testing.T, seq string) *image.RGBA {
	t.Helper()
	const prefix = "\x1bP0;1;0q\""
	if !strings.HasPrefix(seq, prefix) || !strings.HasSuffix(seq, "\x1b\\") {
		t.Fatalf("not a sixel sequence: %q…", seq[:min(len(seq), 24)])
	}
	d := strings.TrimSuffix(strings.TrimPrefix(seq, prefix), "\x1b\\")
	num := func() int {
		n := 0
		for len(d) > 0 && d[0] >= '0' && d[0] <= '9' {
			n = n*10 + int(d[0]-'0')
			d = d[1:]
		}
		return n
	}
	args := func() []int {
		v := []int{num()}
		for len(d) > 0 && d[0] == ';' {
			d = d[1:]
			v = append(v, num())
		}
		return v
	}
	ra := args()
	if len(ra) != 4 {
		t.Fatalf("raster %v", ra)
	}
	img := image.NewRGBA(image.Rect(0, 0, ra[2], ra[3]))
	pal := map[int]color.RGBA{}
	var cur color.RGBA
	x, band := 0, 0
	for len(d) > 0 {
		c := d[0]
		d = d[1:]
		rep := 1
		switch c {
		case '#':
			a := args()
			if len(a) == 5 {
				pc := func(v int) uint8 { return uint8((v*255 + 50) / 100) }
				pal[a[0]] = color.RGBA{pc(a[2]), pc(a[3]), pc(a[4]), 255}
			}
			cur = pal[a[0]]
			continue
		case '$':
			x = 0
			continue
		case '-':
			x, band = 0, band+1
			continue
		case '!':
			rep = num()
			c = d[0]
			d = d[1:]
		}
		if c < '?' || c > '~' {
			t.Fatalf("bad sixel byte %q", c)
		}
		for range rep {
			for dy := range 6 {
				if (c-'?')&(1<<dy) != 0 {
					img.SetRGBA(x, band*6+dy, cur)
				}
			}
			x++
		}
	}
	return img
}

func near(a, b uint8) bool { return int(a)-int(b) <= 3 && int(b)-int(a) <= 3 }

func TestEncodeSixelRoundTrip(t *testing.T) {
	// a few flat colors, a size that is not a multiple of 6, and a mask
	img := image.NewRGBA(image.Rect(0, 0, 37, 23))
	cols := []color.RGBA{{30, 30, 46, 255}, {137, 180, 250, 255}, {243, 139, 168, 255}}
	for y := range 23 {
		for x := range 37 {
			img.SetRGBA(x, y, cols[(x/5+y/4)%3])
		}
	}
	mask := []image.Rectangle{image.Rect(10, 6, 20, 12)}
	out := decodeSixel(t, encodeSixel(img, mask))
	if out.Bounds().Dx() != 37 || out.Bounds().Dy() != 23 {
		t.Fatalf("size %v", out.Bounds())
	}
	for y := range 23 {
		for x := range 37 {
			r, g, b, a := out.At(x, y).RGBA()
			if image.Pt(x, y).In(mask[0]) {
				if a != 0 {
					t.Fatalf("masked pixel %d,%d painted", x, y)
				}
				continue
			}
			want := img.RGBAAt(x, y)
			if a == 0 || !near(uint8(r>>8), want.R) || !near(uint8(g>>8), want.G) || !near(uint8(b>>8), want.B) {
				t.Fatalf("pixel %d,%d = %d,%d,%d,%d want %v", x, y, r>>8, g>>8, b>>8, a>>8, want)
			}
		}
	}
}

func TestEncodeSixelManyColors(t *testing.T) {
	// a gradient with far more colors than palette registers
	img := image.NewRGBA(image.Rect(0, 0, 256, 64))
	for y := range 64 {
		for x := range 256 {
			img.SetRGBA(x, y, color.RGBA{uint8(x), uint8(y * 4), uint8(255 - x), 255})
		}
	}
	pal, idx := quantize(img, nil)
	if len(pal) > sixelMaxColors || len(pal) < 200 {
		t.Fatalf("palette size %d", len(pal))
	}
	for i, c := range idx {
		if int(c) >= len(pal) {
			t.Fatalf("pixel %d has index %d", i, c)
		}
	}
	out := decodeSixel(t, encodeSixel(img, nil))
	var worst int
	for y := range 64 {
		for x := range 256 {
			r, g, b, _ := out.At(x, y).RGBA()
			w := img.RGBAAt(x, y)
			for _, d := range []int{int(r>>8) - int(w.R), int(g>>8) - int(w.G), int(b>>8) - int(w.B)} {
				worst = max(worst, d, -d)
			}
		}
	}
	if worst > 24 {
		t.Errorf("quantization error %d", worst)
	}
}

func TestWriteSixelRowRLE(t *testing.T) {
	var sb strings.Builder
	row := []byte{1, 1, 1, 1, 1, 2, 2, 0, 0}
	writeSixelRow(&sb, row)
	if got := sb.String(); got != "!5@AA" {
		t.Errorf("got %q", got)
	}
}

func TestEraseCells(t *testing.T) {
	got := eraseCells(1, 10, 2, []image.Rectangle{image.Rect(3, 1, 6, 2)})
	want := "\x1b7\x1b[0m" +
		"\x1b[2;1H\x1b[3X\x1b[2;7H\x1b[4X" + // row 1: around the kept cells
		"\x1b[3;1H\x1b[10X" + // row 2: whole row
		"\x1b8"
	if got != want {
		t.Errorf("\n got %q\nwant %q", got, want)
	}
}

func TestSixelDetection(t *testing.T) {
	if !hasSixel([]int{62, 4, 22}) || hasSixel([]int{62, 22}) {
		t.Error("hasSixel")
	}
	if !sixelHint(envOf(map[string]string{"TERM": "foot"})) || sixelHint(envOf(map[string]string{"TERM": "xterm-256color"})) {
		t.Error("sixelHint")
	}
}

// TestModelSixel drives the viewer through sixel detection and rendering.
func TestModelSixel(t *testing.T) {
	ds := parseInput("x.mmd", "graph LR\nA[Hello] --> B{World}")
	m := NewModel([]string{"x.mmd"}, ds, Options{Theme: "nord"})
	m.cell = CellSize{W: 10, H: 20}
	m.cellFromTerm = true

	var raws []string
	var feed func(msg tea.Msg)
	var exec func(c tea.Cmd)
	exec = func(c tea.Cmd) {
		if c == nil {
			return
		}
		// skip slow timers (toasts); the render ticks are short
		ch := make(chan tea.Msg, 1)
		go func() { ch <- c() }()
		var msg tea.Msg
		select {
		case msg = <-ch:
		case <-time.After(300 * time.Millisecond):
			return
		}
		switch v := msg.(type) {
		case tea.BatchMsg:
			for _, cc := range v {
				exec(cc)
			}
		case tea.RawMsg:
			raws = append(raws, v.Msg.(string))
		case renderTickMsg, sceneMsg, renderDoneMsg, sixelPaintMsg:
			feed(v)
		}
	}
	feed = func(msg tea.Msg) {
		_, cmd := m.Update(msg)
		exec(cmd)
	}
	exec(m.ensureScene())
	feed(tea.WindowSizeMsg{Width: 60, Height: 20})
	raws = nil

	// DA1 with attribute 4 and no kitty reply -> sixel
	feed(uv.PrimaryDeviceAttributesEvent{62, 4, 22})
	if m.probing || m.mode != RendererSixel || !m.sixelOK {
		t.Fatalf("mode = %v probing=%v", m.mode, m.probing)
	}
	if len(raws) != 1 || !strings.Contains(raws[0], "\x1bP0;1;0q") || !strings.HasPrefix(raws[0], "\x1b7\x1b[2;1H") {
		t.Fatalf("expected one sixel paint at the body origin, got %d", len(raws))
	}
	img := decodeSixel(t, strings.TrimSuffix(strings.TrimPrefix(raws[0], "\x1b7\x1b[2;1H"), "\x1b8"))
	if img.Bounds().Dx() != 600 || img.Bounds().Dy() != 18*20 {
		t.Errorf("image size %v", img.Bounds())
	}
	if len(m.body) != 18 || strings.TrimSpace(m.body[0]) != "" {
		t.Fatal("sixel body should be blank cells")
	}
	if !strings.Contains(m.render(), "sixel") {
		t.Error("status bar should show the sixel renderer")
	}

	// opening help repaints the image around the help panel
	raws = nil
	feed(tea.KeyPressMsg{Code: '?', Text: "?"})
	if len(raws) != 1 {
		t.Fatalf("help: %d paints", len(raws))
	}
	rects := m.overlayRects()
	if len(rects) != 1 {
		t.Fatalf("overlays %v", rects)
	}
	img = decodeSixel(t, strings.TrimSuffix(strings.TrimPrefix(raws[0], "\x1b7\x1b[2;1H"), "\x1b8"))
	r := rects[0]
	cx, cy := (r.Min.X+r.Max.X)/2*10, ((r.Min.Y+r.Max.Y)/2-1)*20
	if _, _, _, a := img.At(cx, cy).RGBA(); a != 0 {
		t.Error("help panel area should be transparent")
	}
	if _, _, _, a := img.At(0, 0).RGBA(); a == 0 {
		t.Error("image outside the panel should be painted")
	}

	// closing it paints the full image again
	raws = nil
	feed(tea.KeyPressMsg{Code: '?', Text: "?"})
	if len(raws) != 1 {
		t.Fatalf("help closed: %d paints", len(raws))
	}
	img = decodeSixel(t, strings.TrimSuffix(strings.TrimPrefix(raws[0], "\x1b7\x1b[2;1H"), "\x1b8"))
	if _, _, _, a := img.At(cx, cy).RGBA(); a == 0 {
		t.Error("image should be complete after closing help")
	}

	// r cycles sixel -> half-block (kitty isn't available): the image is erased
	raws = nil
	feed(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if m.mode != RendererHalfBlock {
		t.Fatalf("toggle: mode %v", m.mode)
	}
	if len(raws) == 0 || !strings.Contains(raws[0], "\x1b[2;1H\x1b[60X") {
		t.Error("switching away from sixel should erase the image")
	}
	feed(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if m.mode != RendererSixel {
		t.Fatalf("toggle back: mode %v", m.mode)
	}
}
