package flowchart

import (
	"strings"
	"testing"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/theme"
)

func TestParseBasic(t *testing.T) {
	src := `flowchart TD
    A[Start]
    B[End]
    A --> B`

	doc, err := parseFlowchart(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if len(doc.nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(doc.nodes))
	}

	if len(doc.edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(doc.edges))
	}
}

func TestParseDirection(t *testing.T) {
	tests := map[string]string{
		"flowchart TB\nA --> B": "TB",
		"flowchart TD\nA --> B": "TB",
		"flowchart BT\nA --> B": "BT",
		"flowchart LR\nA --> B": "LR",
		"flowchart RL\nA --> B": "RL",
	}

	for src, expected := range tests {
		doc, err := parseFlowchart(src)
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if doc.direction != expected {
			t.Errorf("expected %s, got %s", expected, doc.direction)
		}
	}
}

func TestParseShapes(t *testing.T) {
	tests := map[string]int{
		"A[rect]":      0,
		"A(rounded)":   1,
		"A[[subrout]]": 2,
		"A((circle))":  4,
		"A{diamond}":   5,
		"A>asymm]":     6,
	}

	for nodeDef, expectedShape := range tests {
		src := "flowchart TD\n" + nodeDef
		doc, err := parseFlowchart(src)
		if err != nil {
			t.Fatalf("parse error for %s: %v", nodeDef, err)
		}

		// Find the node
		var shape int
		for _, node := range doc.nodes {
			shape = node.shape
			break
		}

		if shape != expectedShape {
			t.Errorf("%s: expected shape %d, got %d", nodeDef, expectedShape, shape)
		}
	}
}

func TestParseEdges(t *testing.T) {
	src := `flowchart LR
    A -->|label| B
    C --- D
    E -.-> F`

	doc, err := parseFlowchart(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if len(doc.edges) != 3 {
		t.Errorf("expected 3 edges, got %d", len(doc.edges))
	}

	if doc.edges[0].label != "label" {
		t.Errorf("expected edge label, got %s", doc.edges[0].label)
	}
}

func TestRenderBasic(t *testing.T) {
	src := `flowchart TD
    A[Start]
    B[End]
    A --> B`

	th := theme.Default()
	sc, err := Render(src, &diagram.Config{Theme: th})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}

	if sc == nil {
		t.Fatal("scene is nil")
	}
}

func TestRenderComplex(t *testing.T) {
	src := `flowchart TD
    Start([Start]) --> Build[Build]
    Build --> Test{Tests?}
    Test -->|Pass| Deploy[Deploy]
    Test -->|Fail| Build
    Deploy --> End([End])`

	th := theme.Default()
	sc, err := Render(src, &diagram.Config{Theme: th})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}

	if sc == nil {
		t.Fatal("scene is nil")
	}
}

func TestRenderChaining(t *testing.T) {
	src := `flowchart LR
    A[1] --> B[2] --> C[3]`

	th := theme.Default()
	sc, err := Render(src, &diagram.Config{Theme: th})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}

	if sc == nil {
		t.Fatal("scene is nil")
	}
}

func TestRenderUnicode(t *testing.T) {
	src := `flowchart LR
    A["Unicode: ♥"]
    B["With #quot;quotes#quot;"]
    A --> B`

	th := theme.Default()
	sc, err := Render(src, &diagram.Config{Theme: th})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}

	if sc == nil {
		t.Fatal("scene is nil")
	}
}

func TestRenderMultiline(t *testing.T) {
	src := `flowchart TD
    A["Line 1<br/>Line 2"]
    B["Multi<br/>Line"]
    A --> B`

	th := theme.Default()
	sc, err := Render(src, &diagram.Config{Theme: th})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}

	if sc == nil {
		t.Fatal("scene is nil")
	}
}

func TestParseComments(t *testing.T) {
	src := `flowchart TD
    %% Comment
    A[Start]
    %% Another comment
    B[End]
    A --> B`

	doc, err := parseFlowchart(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if len(doc.nodes) != 2 {
		t.Errorf("expected 2 nodes (comments skipped), got %d", len(doc.nodes))
	}
}

func TestParseQuoted(t *testing.T) {
	src := `flowchart LR
    A["Text with (parens) [brackets]"]`

	doc, err := parseFlowchart(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if node, ok := doc.nodes["A"]; ok {
		if !strings.Contains(node.label, "parens") {
			t.Errorf("expected label to contain 'parens', got %s", node.label)
		}
	}
}

func TestParseEmpty(t *testing.T) {
	src := "flowchart TD"

	doc, err := parseFlowchart(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if len(doc.nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(doc.nodes))
	}
}
