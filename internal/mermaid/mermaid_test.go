package mermaid

import (
	"testing"

	"github.com/mark3labs/mergo/internal/theme"
)

func TestGanttRender(t *testing.T) {
	src := `gantt
    title Test Gantt
    dateFormat YYYY-MM-DD
    section Dev
        Task 1 :a1, 2024-01-01, 5d
        Task 2 :after a1, 3d`

	th := theme.Default()
	sc, err := Render(src, th)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	if sc == nil || sc.Width <= 0 || sc.Height <= 0 {
		t.Fatalf("Invalid scene: %v", sc)
	}
}

func TestTimelineRender(t *testing.T) {
	src := `timeline
    title Test Timeline
    2020 : Event A
    2021 : Event B`

	th := theme.Default()
	sc, err := Render(src, th)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	if sc == nil || sc.Width <= 0 || sc.Height <= 0 {
		t.Fatalf("Invalid scene: %v", sc)
	}
}

func TestJourneyRender(t *testing.T) {
	src := `journey
    title Test Journey
    section Sec1
        Task 1 : 4 : Actor1
        Task 2 : 2 : Actor2`

	th := theme.Default()
	sc, err := Render(src, th)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	if sc == nil || sc.Width <= 0 || sc.Height <= 0 {
		t.Fatalf("Invalid scene: %v", sc)
	}
}
