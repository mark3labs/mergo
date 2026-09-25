package quadrant

import (
	"testing"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/theme"
)

func TestParseBasicQuadrant(t *testing.T) {
	src := `quadrantChart
	title Quadrant Chart
	x-axis Low --> High
	y-axis Low --> High
	A: [0.3, 0.7]
	B: [0.7, 0.3]
	`
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if doc.Title != "Quadrant Chart" {
		t.Errorf("title = %q, want 'Quadrant Chart'", doc.Title)
	}
	if len(doc.Points) != 2 {
		t.Errorf("expected 2 points, got %d", len(doc.Points))
	}
	if doc.Points[0].Name != "A" {
		t.Errorf("first point name = %q, want A", doc.Points[0].Name)
	}
	if doc.Points[0].X != 0.3 || doc.Points[0].Y != 0.7 {
		t.Errorf("first point coords wrong: (%f, %f)", doc.Points[0].X, doc.Points[0].Y)
	}
}

func TestParseAxisLabels(t *testing.T) {
	src := `quadrantChart
	x-axis Poor --> Excellent
	y-axis Slow --> Fast
	`
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if doc.XAxis.Min != "Poor" {
		t.Errorf("x-axis min = %q, want Poor", doc.XAxis.Min)
	}
	if doc.XAxis.Max != "Excellent" {
		t.Errorf("x-axis max = %q, want Excellent", doc.XAxis.Max)
	}
	if doc.YAxis.Min != "Slow" {
		t.Errorf("y-axis min = %q, want Slow", doc.YAxis.Min)
	}
	if doc.YAxis.Max != "Fast" {
		t.Errorf("y-axis max = %q, want Fast", doc.YAxis.Max)
	}
}

func TestParseQuadrantLabels(t *testing.T) {
	src := `quadrantChart
	quadrant-1 High Priority
	quadrant-2 Planning
	quadrant-3 Low Priority
	quadrant-4 Backlog
	`
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if doc.Quadrant1 != "High Priority" {
		t.Errorf("quadrant-1 = %q, want 'High Priority'", doc.Quadrant1)
	}
	if doc.Quadrant4 != "Backlog" {
		t.Errorf("quadrant-4 = %q, want 'Backlog'", doc.Quadrant4)
	}
}

func TestParsePointWithStyle(t *testing.T) {
	src := `quadrantChart
	Task1: [0.5, 0.5], radius: 10, color: #ff0000
	`
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(doc.Points) != 1 {
		t.Errorf("expected 1 point, got %d", len(doc.Points))
	}
	if doc.Points[0].Style.Radius != 10 {
		t.Errorf("radius = %f, want 10", doc.Points[0].Style.Radius)
	}
	if doc.Points[0].Style.Color != "#ff0000" {
		t.Errorf("color = %q, want #ff0000", doc.Points[0].Style.Color)
	}
}

func TestParsePointWithClass(t *testing.T) {
	src := `quadrantChart
	classDef urgent fill:#ff0000,stroke:#000
	Task: [0.8, 0.9]:::urgent
	`
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(doc.Classes) == 0 {
		t.Error("class definition not parsed")
	}
	if len(doc.Points) != 1 {
		t.Errorf("expected 1 point, got %d", len(doc.Points))
	}
}

func TestParseMultiplePoints(t *testing.T) {
	src := `quadrantChart
	A: [0.1, 0.1]
	B: [0.9, 0.9]
	C: [0.2, 0.8]
	D: [0.8, 0.2]
	`
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(doc.Points) != 4 {
		t.Errorf("expected 4 points, got %d", len(doc.Points))
	}
}

func TestRenderBasicQuadrant(t *testing.T) {
	src := `quadrantChart
	title Priority Matrix
	x-axis Low --> High
	y-axis Low --> High
	A: [0.3, 0.7]
	B: [0.7, 0.3]
	`
	cfg := &diagram.Config{Theme: theme.Default()}
	sc, err := Render(src, cfg)
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	if sc.Width <= 0 || sc.Height <= 0 {
		t.Errorf("scene dimensions invalid: %fx%f", sc.Width, sc.Height)
	}
	if len(sc.Items) == 0 {
		t.Error("no items rendered")
	}
}

func TestRenderQuadrantWithLabels(t *testing.T) {
	src := `quadrantChart
	title Skills
	quadrant-1 Expert
	quadrant-2 Learning
	quadrant-3 Novice
	quadrant-4 Avoid
	Point1: [0.8, 0.8]
	`
	cfg := &diagram.Config{Theme: theme.Default()}
	sc, err := Render(src, cfg)
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	if sc.Width <= 0 || sc.Height <= 0 {
		t.Errorf("scene dimensions invalid: %fx%f", sc.Width, sc.Height)
	}
}
