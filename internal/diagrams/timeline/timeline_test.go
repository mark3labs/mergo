package timeline

import (
	"testing"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/theme"
)

func TestParseBasicTimeline(t *testing.T) {
	src := `timeline
    title History Test
    2020 : Event A
    2021 : Event B : Event C
`

	cfg := &diagram.Config{Theme: theme.Default()}
	tl, err := Parse(src, cfg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if tl.Title != "History Test" {
		t.Errorf("Title = %q, want %q", tl.Title, "History Test")
	}

	if len(tl.AllPeriods) != 2 {
		t.Errorf("Number of periods = %d, want 2", len(tl.AllPeriods))
	}

	if len(tl.AllPeriods[1].Events) != 2 {
		t.Errorf("Events in period 2 = %d, want 2", len(tl.AllPeriods[1].Events))
	}
}

func TestParseTimelineWithSections(t *testing.T) {
	src := `timeline
    title Test with sections
    section Era1
        Period1 : Event1
    section Era2
        Period2 : Event2
`

	cfg := &diagram.Config{Theme: theme.Default()}
	tl, err := Parse(src, cfg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(tl.Sections) != 2 {
		t.Errorf("Number of sections = %d, want 2", len(tl.Sections))
	}

	if tl.Sections[0].Name != "Era1" {
		t.Errorf("Section 0 name = %q, want %q", tl.Sections[0].Name, "Era1")
	}
}

func TestRenderTimeline(t *testing.T) {
	src := `timeline
    title Test Timeline
    2020 : Event A
    2021 : Event B
`

	cfg := &diagram.Config{Theme: theme.Default()}
	sc, err := Render(src, cfg)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	if sc == nil {
		t.Fatal("Scene is nil")
	}

	if sc.Width <= 0 || sc.Height <= 0 {
		t.Errorf("Scene size = %gx%g, want > 0", sc.Width, sc.Height)
	}

	if len(sc.Items) == 0 {
		t.Error("Scene has no items")
	}
}
