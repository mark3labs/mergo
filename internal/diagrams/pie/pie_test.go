package pie

import (
	"testing"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/theme"
)

func TestParseSimplePie(t *testing.T) {
	src := `pie
	"Apples" : 30
	"Bananas" : 20
	"Oranges" : 50
	`
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(doc.Slices) != 3 {
		t.Errorf("expected 3 slices, got %d", len(doc.Slices))
	}
	if doc.Slices[0].Label != "Apples" || doc.Slices[0].Value != 30 {
		t.Errorf("first slice wrong: %v", doc.Slices[0])
	}
}

func TestParseShowData(t *testing.T) {
	src := `pie showData
	"A" : 10
	"B" : 20
	`
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if !doc.ShowData {
		t.Error("showData flag not set")
	}
}

func TestParseTitle(t *testing.T) {
	src := `pie title "Fruit Distribution"
	"Apples" : 30
	`
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if doc.Title != "Fruit Distribution" {
		t.Errorf("title = %q, want 'Fruit Distribution'", doc.Title)
	}
}

func TestParseSingleSlice(t *testing.T) {
	src := `pie
	"Only Item" : 100
	`
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(doc.Slices) != 1 {
		t.Errorf("expected 1 slice, got %d", len(doc.Slices))
	}
	if doc.Slices[0].Value != 100 {
		t.Errorf("value = %f, want 100", doc.Slices[0].Value)
	}
}

func TestParseZeroValue(t *testing.T) {
	src := `pie
	"A" : 0
	"B" : 100
	`
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	// Should accept zero values
	if len(doc.Slices) != 2 {
		t.Errorf("expected 2 slices, got %d", len(doc.Slices))
	}
}

func TestParseFloatValue(t *testing.T) {
	src := `pie
	"A" : 12.5
	"B" : 37.5
	`
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if doc.Slices[0].Value != 12.5 {
		t.Errorf("value = %f, want 12.5", doc.Slices[0].Value)
	}
}

func TestRenderBasicPie(t *testing.T) {
	src := `pie
	"Apples" : 30
	"Bananas" : 20
	"Oranges" : 50
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

func TestRenderPieWithTitle(t *testing.T) {
	src := `pie showData
	title Fruit Distribution
	"Apples" : 30
	"Bananas" : 70
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

func TestRenderPieEmptyOrAllZeros(t *testing.T) {
	// All zero values should result in empty chart
	src := `pie
	"A" : 0
	"B" : 0
	`
	cfg := &diagram.Config{Theme: theme.Default()}
	sc, err := Render(src, cfg)
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	if sc == nil {
		t.Fatal("scene is nil")
	}
}

func TestRenderPieSingleSlice(t *testing.T) {
	// Single slice pie (full circle)
	src := `pie
	"Entire" : 100
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

func TestRenderPieVerySmallSlice(t *testing.T) {
	// Test with one very small slice
	src := `pie
	"Large" : 99.9
	"Tiny" : 0.1
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
