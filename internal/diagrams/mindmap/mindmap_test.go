package mindmap

import (
	"testing"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

func TestMindmapBasic(t *testing.T) {
	src := `root((Root))
  A
    B
    C
  D`

	th := theme.Default()
	_, err := Render(src, &diagram.Config{Theme: th})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
}

func TestMindmapShapes(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"Square", `root[Square]
  child1
  child2`},
		{"Rounded", `root(Rounded)
  child`},
		{"Circle", `root((Circle))
  child`},
		{"Hexagon", `root{{Hexagon}}
  child`},
		{"Bang", `root))Bang((
  child`},
		{"Cloud", `root)Cloud(
  child`},
	}

	th := theme.Default()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Render(tt.src, &diagram.Config{Theme: th})
			if err != nil {
				t.Fatalf("Render failed: %v", err)
			}
		})
	}
}

func TestParseNodeShape(t *testing.T) {
	tests := []struct {
		input       string
		expectText  string
		expectShape ShapeType
	}{
		{"[Square Text]", "Square Text", ShapeSquare},
		{"(Rounded Text)", "Rounded Text", ShapeRounded},
		{"((Circle Text))", "Circle Text", ShapeCircle},
		{"{{Hexagon Text}}", "Hexagon Text", ShapeHexagon},
		{"))Bang Text((", "Bang Text", ShapeBang},
		{")Cloud Text(", "Cloud Text", ShapeCloud},
		{"Plain Text", "Plain Text", ShapeDefault},
	}

	for _, tt := range tests {
		text, shape := parseNodeShape(tt.input)
		if text != tt.expectText || shape != tt.expectShape {
			t.Errorf("parseNodeShape(%q) = (%q, %v), want (%q, %v)",
				tt.input, text, shape, tt.expectText, tt.expectShape)
		}
	}
}

func TestParseNodes(t *testing.T) {
	lines := []string{
		"root",
		"  child1",
		"    grandchild",
		"  child2",
	}

	root, err := parseNodes(lines)
	if err != nil {
		t.Fatalf("parseNodes failed: %v", err)
	}

	if root == nil {
		t.Fatal("root is nil")
	}
	if root.Text != "root" {
		t.Errorf("root.Text = %q, want %q", root.Text, "root")
	}
	if len(root.Children) != 2 {
		t.Errorf("root has %d children, want 2", len(root.Children))
	}
}

func TestMindmapRender(t *testing.T) {
	src := `root((Mindmap))
  Origins
    Long history
    Popularisation
  Research
    On effectiveness
  Tools
    Pen and paper
    Mermaid`

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

	// Render to image to check no panics
	img := sc.Render(scene.RenderOptions{Scale: 1})
	if img == nil {
		t.Fatal("rendered image is nil")
	}
}

func TestMindmapEmptySource(t *testing.T) {
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
