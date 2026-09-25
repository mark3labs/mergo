package journey

import (
	"testing"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/theme"
)

func TestParseBasicJourney(t *testing.T) {
	src := `journey
    title Test Journey
    section Section1
        Task1 : 4 : Actor1
        Task2 : 2 : Actor1, Actor2
`

	cfg := &diagram.Config{Theme: theme.Default()}
	jrn, err := Parse(src, cfg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if jrn.Title != "Test Journey" {
		t.Errorf("Title = %q, want %q", jrn.Title, "Test Journey")
	}

	if len(jrn.Sections) != 1 {
		t.Errorf("Number of sections = %d, want 1", len(jrn.Sections))
	}

	if len(jrn.AllActors) != 2 {
		t.Errorf("Number of unique actors = %d, want 2", len(jrn.AllActors))
	}
}

func TestJourneyScoreValidation(t *testing.T) {
	src := `journey
    title Invalid Score
    section Test
        Task : 6 : Actor1
`

	cfg := &diagram.Config{Theme: theme.Default()}
	_, err := Parse(src, cfg)
	if err == nil {
		t.Error("Expected error for score 6, got nil")
	}
}

func TestJourneyValidScores(t *testing.T) {
	for score := 1; score <= 5; score++ {
		src := `journey
    title Test Score ` + string(rune('0'+score)) + `
    section Test
        Task : ` + string(rune('0'+score)) + ` : Actor1
`

		cfg := &diagram.Config{Theme: theme.Default()}
		jrn, err := Parse(src, cfg)
		if err != nil {
			t.Errorf("Score %d: Parse failed: %v", score, err)
			continue
		}

		if len(jrn.Sections[0].Tasks) == 0 || jrn.Sections[0].Tasks[0].Score != score {
			t.Errorf("Score %d: Got %d", score, jrn.Sections[0].Tasks[0].Score)
		}
	}
}

func TestRenderJourney(t *testing.T) {
	src := `journey
    title Test Journey
    section Section1
        Task1 : 4 : Actor1
        Task2 : 2 : Actor1, Actor2
    section Section2
        Task3 : 3 : Actor2
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
