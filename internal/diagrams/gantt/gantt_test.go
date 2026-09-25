package gantt

import (
	"testing"
	"time"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/theme"
)

func TestParseDateFormats(t *testing.T) {
	tests := []struct {
		name      string
		dateStr   string
		format    string
		expectErr bool
	}{
		{"YYYY-MM-DD", "2024-01-15", "YYYY-MM-DD", false},
		{"YY-MM-DD", "24-01-15", "YY-MM-DD", false},
		{"MMM DD YYYY", "Jan 15 2024", "MMM DD YYYY", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseDate(tt.dateStr, tt.format)
			if (err != nil) != tt.expectErr {
				t.Errorf("parseDate(%q, %q) error = %v, expectErr %v", tt.dateStr, tt.format, err, tt.expectErr)
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name      string
		durStr    string
		expectErr bool
		expectMin time.Duration
		expectMax time.Duration
	}{
		{"days", "3d", false, 72 * time.Hour, 72 * time.Hour},
		{"hours", "4h", false, 4 * time.Hour, 4 * time.Hour},
		{"weeks", "2w", false, 14 * 24 * time.Hour, 14 * 24 * time.Hour},
		{"decimal", "1.5d", false, 35 * time.Hour, 37 * time.Hour},
		{"invalid", "3x", true, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dur, err := parseDuration(tt.durStr)
			if (err != nil) != tt.expectErr {
				t.Errorf("parseDuration(%q) error = %v, expectErr %v", tt.durStr, err, tt.expectErr)
			}
			if !tt.expectErr && (dur < tt.expectMin || dur > tt.expectMax) {
				t.Errorf("parseDuration(%q) = %v, want between %v and %v", tt.durStr, dur, tt.expectMin, tt.expectMax)
			}
		})
	}
}

func TestParseBasicGantt(t *testing.T) {
	src := `gantt
    title Test Chart
    dateFormat YYYY-MM-DD
    section Section1
        Task A :a1, 2024-01-01, 5d
        Task B :after a1, 3d
`

	cfg := &diagram.Config{Theme: theme.Default()}
	gantt, err := Parse(src, cfg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if gantt.Title != "Test Chart" {
		t.Errorf("Title = %q, want %q", gantt.Title, "Test Chart")
	}

	if len(gantt.AllTasks) < 2 {
		t.Errorf("Number of tasks = %d, want >= 2", len(gantt.AllTasks))
	}
}

func TestExcludedDays(t *testing.T) {
	tests := []struct {
		name          string
		day           time.Time
		excludes      []string
		weekendStart  string
		expectExclude bool
	}{
		{"Regular weekday", time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC), []string{}, "saturday", false},
		{"Saturday with weekends", time.Date(2024, 1, 6, 0, 0, 0, 0, time.UTC), []string{"weekends"}, "saturday", true},
		{"Sunday with weekends", time.Date(2024, 1, 7, 0, 0, 0, 0, time.UTC), []string{"weekends"}, "saturday", true},
		{"Specific date", time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC), []string{"2024-01-08"}, "saturday", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isExcludedDay(tt.day, tt.excludes, tt.weekendStart)
			if result != tt.expectExclude {
				t.Errorf("isExcludedDay = %v, want %v", result, tt.expectExclude)
			}
		})
	}
}

func TestRenderGanttExample(t *testing.T) {
	src := `gantt
    title Test Gantt
    dateFormat YYYY-MM-DD
    section Dev
        Task 1 :a1, 2024-01-01, 5d
        Task 2 :after a1, 3d
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

func TestGanttWithMilestones(t *testing.T) {
	// Test milestone positioning and rendering
	src := `gantt
    title Gantt with Milestones
    dateFormat YYYY-MM-DD
    section Plan
        Milestone 1 :milestone, m1, 2024-01-15, 0d
        Task :a1, 2024-01-01, 14d
        Milestone 2 :milestone, m2, 2024-01-20, 0d
`
	cfg := &diagram.Config{Theme: theme.Default()}
	sc, err := Render(src, cfg)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if sc.Width <= 0 || sc.Height <= 0 {
		t.Errorf("Scene size invalid: %gx%g", sc.Width, sc.Height)
	}
}

func TestGanttWithDifferentTaskTypes(t *testing.T) {
	// Test different task states: done, active, crit
	src := `gantt
    title Task Types
    dateFormat YYYY-MM-DD
    section Work
        Done Task :done, d1, 2024-01-01, 3d
        Active Task :active, a1, 2024-01-04, 3d
        Critical Task :crit, c1, 2024-01-07, 3d
        Normal Task :n1, 2024-01-10, 3d
`
	cfg := &diagram.Config{Theme: theme.Default()}
	sc, err := Render(src, cfg)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if len(sc.Items) == 0 {
		t.Error("No items rendered")
	}
}

func TestGanttParsingRobustness(t *testing.T) {
	// Test parser doesn't panic on various prefixes
	tests := []struct {
		name string
		src  string
	}{
		{"Just gantt", "gantt"},
		{"With title", "gantt\ntitle Test"},
		{"With section", "gantt\nsection S1"},
		{"Empty lines", "gantt\n\n\ntitle Test\n\n"},
	}

	cfg := &diagram.Config{Theme: theme.Default()}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Render(tt.src, cfg)
			if err != nil {
				t.Fatalf("Render failed: %v", err)
			}
		})
	}
}
