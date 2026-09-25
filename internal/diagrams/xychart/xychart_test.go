package xychart

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mergo/internal/devutil"
	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

func TestParse(t *testing.T) {
	c, err := Parse(`xychart-beta horizontal
    title "Sales, 2024"
    x-axis Months [jan, "feb, early", mar]
    y-axis "Revenue (in $)" 4000 --> 11000
    bar [5000, 6000, 7500]
    line "Target" [5500, 6500, 8000]`)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Horizontal || c.Title != "Sales, 2024" {
		t.Errorf("header/title %+v", c)
	}
	if c.X.Title != "Months" || len(c.X.Categories) != 3 || c.X.Categories[1] != "feb, early" {
		t.Errorf("x axis %+v", c.X)
	}
	if c.Y.Title != "Revenue (in $)" || !c.Y.HasRange || c.Y.Min != 4000 || c.Y.Max != 11000 {
		t.Errorf("y axis %+v", c.Y)
	}
	if len(c.Series) != 2 || c.Series[1].Title != "Target" || c.Series[1].Values[2] != 8000 {
		t.Errorf("series %+v", c.Series)
	}
	c, err = Parse("xychart-beta\nx-axis 1 --> 5\nline [1,2,3]")
	if err != nil || !c.X.HasRange || c.X.Max != 5 {
		t.Errorf("numeric x axis: %v %+v", err, c.X)
	}
	for _, bad := range []string{"bar [1, x]", "line 1,2", "pie [1]"} {
		if _, err := Parse("xychart-beta\n" + bad); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}

func TestNiceTicks(t *testing.T) {
	lo, hi, ticks := niceTicks(0, 97, 6)
	if lo != 0 || hi != 100 || len(ticks) < 4 || ticks[1] != 20 {
		t.Errorf("ticks %v %v %v", lo, hi, ticks)
	}
}

func TestExamples(t *testing.T) {
	files, _ := filepath.Glob("../../../examples/xychart/*.mmd")
	for _, f := range files {
		b, _ := os.ReadFile(f)
		s := string(b)
		for i := 0; i <= len(s); i += 3 {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("%s prefix %d: %v", f, i, r)
					}
				}()
				_, _ = diagram.Render(s[:i], theme.Default())
			}()
		}
		sc, err := diagram.Render(s, theme.MustGet("dark"))
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if testing.Verbose() {
			t.Logf("%s\n%s", f, devutil.ASCII(sc.Render(scene.RenderOptions{Scale: 1}), 120))
		}
	}
}
