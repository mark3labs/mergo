package tui

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestResolvePlacement(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want Placement
	}{
		{"kitty", map[string]string{"TERM": "xterm-kitty", "KITTY_WINDOW_ID": "1"}, PlacementUnicode},
		{"ghostty", map[string]string{"TERM_PROGRAM": "ghostty"}, PlacementUnicode},
		// zellij inherits the outer terminal's variables; it must win
		{"zellij in kitty", map[string]string{"ZELLIJ": "0", "KITTY_WINDOW_ID": "1", "TERM": "xterm-kitty"}, PlacementDirect},
		{"zellij session name", map[string]string{"ZELLIJ_SESSION_NAME": "main"}, PlacementDirect},
		{"tmux", map[string]string{"TMUX": "/tmp/tmux-1000/default,1,0"}, PlacementUnicode},
		{"wezterm", map[string]string{"TERM_PROGRAM": "WezTerm"}, PlacementDirect},
		{"unknown", map[string]string{"TERM": "xterm-256color"}, PlacementDirect},
	}
	for _, c := range cases {
		if got := resolvePlacement(PlacementAuto, envOf(c.env)); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
	// explicit choice always wins
	if resolvePlacement(PlacementUnicode, envOf(map[string]string{"ZELLIJ": "0"})) != PlacementUnicode {
		t.Error("explicit placement overridden")
	}
	for in, want := range map[string]Placement{"auto": PlacementAuto, "unicode": PlacementUnicode, "placeholder": PlacementUnicode, "direct": PlacementDirect} {
		if got, ok := ParsePlacement(in); !ok || got != want {
			t.Errorf("ParsePlacement(%q) = %v %v", in, got, ok)
		}
	}
	if _, ok := ParsePlacement("sideways"); ok {
		t.Error("bad placement accepted")
	}
	if !graphicsHint(envOf(map[string]string{"TERM_PROGRAM": "WezTerm"})) || graphicsHint(envOf(map[string]string{"TERM": "xterm"})) {
		t.Error("graphicsHint")
	}
}

func TestKittyPlaceAt(t *testing.T) {
	got := kittyPlaceAt(70, 1, 0, 80, 20, false)
	want := "\x1b7\x1b[2;1H\x1b_Ga=p,i=70,p=1,c=80,r=20,C=1,z=-1073741825,q=2\x1b\\\x1b8"
	if got != want {
		t.Errorf("placeAt\n got %q\nwant %q", got, want)
	}
	for _, l := range blankGrid(5, 2) {
		if l != "     " {
			t.Errorf("blank line %q", l)
		}
	}
}

// TestDirectPlacementFlow simulates zellij: the kitty query is answered,
// images are placed directly at the body origin, the body is blank, and
// replaced/hidden images are deleted.
func TestDirectPlacementFlow(t *testing.T) {
	ds := append(parseInput("a.mmd", "graph LR\nA-->B"), parseInput("b.mmd", "graph LR\nA -->")...)
	m := NewModel([]string{"a.mmd", "b.mmd"}, ds, Options{Theme: "nord", Placement: PlacementDirect})
	m.cell = CellSize{W: 10, H: 20}
	m.cellFromTerm = true
	m.tmux = false

	var raws []string
	var feed func(msg tea.Msg)
	var run func(c tea.Cmd)
	run = func(c tea.Cmd) {
		if c == nil {
			return
		}
		switch v := c().(type) {
		case tea.BatchMsg:
			for _, cc := range v {
				run(cc)
			}
		case tea.RawMsg:
			raws = append(raws, v.Msg.(string))
		case renderTickMsg, sceneMsg, renderDoneMsg:
			feed(v)
		}
	}
	feed = func(msg tea.Msg) {
		_, cmd := m.Update(msg)
		run(cmd)
	}
	run(m.ensureScene())
	feed(tea.WindowSizeMsg{Width: 60, Height: 20})
	feed(uv.KittyGraphicsEvent{Options: kitty.Options{ID: probeImageID}, Payload: []byte("OK")})
	if m.mode != RendererKitty || m.shownID == 0 {
		t.Fatalf("mode=%v shown=%d", m.mode, m.shownID)
	}
	all := strings.Join(raws, "")
	if !strings.Contains(all, "\x1b[2;1H\x1b_Ga=p,i=") || !strings.Contains(all, ",c=60,r=18,C=1,") {
		t.Fatalf("no direct placement at the body origin: %q", all)
	}
	if strings.Contains(all, "U=1") {
		t.Error("virtual placement used in direct mode")
	}
	if strings.Contains(strings.Join(m.body, ""), string(kitty.Placeholder)) {
		t.Error("placeholders in direct mode body")
	}
	view := m.render()
	if !strings.Contains(ansi.Strip(view), "kitty·direct") {
		t.Error("status bar should show kitty·direct")
	}

	// re-render (zoom): the previous image is deleted after the new one is placed
	first := m.shownID
	raws = nil
	feed(tea.KeyPressMsg{Code: '+', Text: "+"})
	all = strings.Join(raws, "")
	place := strings.Index(all, "a=p,i=")
	del := strings.Index(all, "a=d,d=I,i="+itoa(first)+",")
	if m.shownID == first || place < 0 || del < place {
		t.Errorf("old image %d not deleted after new placement: %q", first, all)
	}

	// switching to a broken diagram (no scene) hides the image
	raws = nil
	feed(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.shownID != 0 || !strings.Contains(strings.Join(raws, ""), "a=d,d=I") {
		t.Errorf("image not hidden for broken diagram: shown=%d raws=%q", m.shownID, raws)
	}
}

func itoa(i int) string { return strconv.Itoa(i) }
