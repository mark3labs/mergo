package xychart

import (
	"testing"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

func TestXYChartBasic(t *testing.T) {
	src := `title "Sales"
  x-axis [Jan, Feb, Mar]
  y-axis "Revenue" 0 --> 1000
  bar [100, 200, 300]`

	th := theme.Default()
	_, err := Render(src, &diagram.Config{Theme: th})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
}

func TestXYChartBeta(t *testing.T) {
	src := `title "Data"
  x-axis [A, B, C]
  y-axis "Value" 0 --> 100
  line [10, 20, 30]`

	th := theme.Default()
	_, err := Render(src, &diagram.Config{Theme: th})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
}

func TestXYChartMultipleSeries(t *testing.T) {
	src := `title "Comparison"
  x-axis [Q1, Q2, Q3, Q4]
  y-axis "Value" 0 --> 100
  bar "Series A" [20, 30, 40, 50]
  bar "Series B" [10, 20, 30, 40]
  line "Average" [15, 25, 35, 45]`

	th := theme.Default()
	_, err := Render(src, &diagram.Config{Theme: th})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
}

func TestXYChartWithoutAxis(t *testing.T) {
	src := `bar [10, 20, 30, 40, 50]
  line [5, 15, 25, 35, 45]`

	th := theme.Default()
	_, err := Render(src, &diagram.Config{Theme: th})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
}

func TestParseAxis(t *testing.T) {
	chart := &XYChart{
		DataSeries: []*Series{},
	}

	err := parseAxis(chart, "x-axis [cat1, cat2, cat3]", true)
	if err != nil {
		t.Fatalf("parseAxis failed: %v", err)
	}

	if chart.XAxis == nil {
		t.Fatal("XAxis is nil")
	}

	if len(chart.XAxis.Categories) != 3 {
		t.Errorf("expected 3 categories, got %d", len(chart.XAxis.Categories))
	}

	if chart.XAxis.Categories[0] != "cat1" {
		t.Errorf("first category = %q, want %q", chart.XAxis.Categories[0], "cat1")
	}
}

func TestParseAxisNumeric(t *testing.T) {
	chart := &XYChart{
		DataSeries: []*Series{},
	}

	err := parseAxis(chart, "y-axis \"Revenue\" 0 --> 1000", false)
	if err != nil {
		t.Fatalf("parseAxis failed: %v", err)
	}

	if chart.YAxis == nil {
		t.Fatal("YAxis is nil")
	}

	if chart.YAxis.Min != 0 {
		t.Errorf("YAxis.Min = %v, want 0", chart.YAxis.Min)
	}

	if chart.YAxis.Max != 1000 {
		t.Errorf("YAxis.Max = %v, want 1000", chart.YAxis.Max)
	}

	if !chart.YAxis.HasRange {
		t.Fatal("YAxis.HasRange should be true")
	}
}

func TestParseSeries(t *testing.T) {
	chart := &XYChart{
		DataSeries: []*Series{},
	}

	err := parseSeries(chart, `bar "Sales" [100, 200, 300]`, "bar")
	if err != nil {
		t.Fatalf("parseSeries failed: %v", err)
	}

	if len(chart.DataSeries) != 1 {
		t.Errorf("expected 1 series, got %d", len(chart.DataSeries))
	}

	series := chart.DataSeries[0]
	if series.Name != "Sales" {
		t.Errorf("series.Name = %q, want %q", series.Name, "Sales")
	}

	if series.Type != "bar" {
		t.Errorf("series.Type = %q, want %q", series.Type, "bar")
	}

	if len(series.Values) != 3 {
		t.Errorf("expected 3 values, got %d", len(series.Values))
	}

	if series.Values[0] != 100 {
		t.Errorf("first value = %v, want 100", series.Values[0])
	}
}

func TestXYChartRender(t *testing.T) {
	src := `title "Performance"
  x-axis [Jan, Feb, Mar, Apr, May]
  y-axis "Score" 0 --> 100
  bar "Test A" [60, 70, 75, 80, 85]
  line "Test B" [50, 65, 70, 75, 90]`

	th := theme.Default()
	sc, err := Render(src, &diagram.Config{Theme: th})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	if sc == nil {
		t.Fatal("scene is nil")
	}

	if sc.Width <= 0 || sc.Height <= 0 {
		t.Errorf("scene dimensions invalid: %v x %v", sc.Width, sc.Height)
	}

	// Render to image
	img := sc.Render(scene.RenderOptions{Scale: 1})
	if img == nil {
		t.Fatal("rendered image is nil")
	}
}

func TestXYChartEmptySource(t *testing.T) {
	src := ""
	th := theme.Default()
	sc, err := Render(src, &diagram.Config{Theme: th})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if sc == nil {
		t.Fatal("scene is nil")
	}
}

func TestFindYRange(t *testing.T) {
	chart := &XYChart{
		DataSeries: []*Series{
			{
				Values: []float64{10, 20, 30, 40},
			},
			{
				Values: []float64{5, 15, 25},
			},
		},
	}

	minY, maxY := findYRange(chart)

	if minY != 5 {
		t.Errorf("minY = %v, want 5", minY)
	}

	if maxY != 40 {
		t.Errorf("maxY = %v, want 40", maxY)
	}
}

func TestGetMaxDataPoints(t *testing.T) {
	chart := &XYChart{
		DataSeries: []*Series{
			{Values: []float64{1, 2, 3}},
			{Values: []float64{1, 2, 3, 4, 5}},
			{Values: []float64{1, 2}},
		},
	}

	max := getMaxDataPoints(chart)
	if max != 5 {
		t.Errorf("max = %v, want 5", max)
	}
}
