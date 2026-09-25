package diagram_test

import (
	"strings"
	"testing"

	"github.com/mark3labs/mergo/internal/diagram"
	_ "github.com/mark3labs/mergo/internal/mermaid"
	"github.com/mark3labs/mergo/internal/theme"
)

func TestPreprocess(t *testing.T) {
	src := `---
title: Hello
config:
  theme: forest
  flowchart:
    nodeSpacing: 80
---
%%{init: {'themeVariables': {'primaryColor': '#ff0000'}, 'flowchart': {'curve': 'linear'}}}%%
flowchart LR
  %% a comment
  accTitle: x
  A --> B`
	doc, err := diagram.Preprocess(src)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Type != "flowchart" || doc.Title != "Hello" || doc.Header != "flowchart LR" {
		t.Errorf("doc = %+v", doc)
	}
	cfg := &diagram.Config{Raw: doc.Raw}
	if cfg.Float("flowchart", "nodeSpacing", 0) != 80 || cfg.String("flowchart", "curve", "") != "linear" {
		t.Errorf("config merge: %v", doc.Raw)
	}
	// line numbers are preserved
	lines := strings.Split(doc.Source, "\n")
	if strings.TrimSpace(lines[11]) != "A --> B" {
		t.Errorf("line 12 = %q (lines are not preserved)", lines[11])
	}
	if _, err := diagram.Preprocess("notADiagram\nfoo"); err == nil || !strings.Contains(err.Error(), "notADiagram") {
		t.Errorf("unknown type error = %v", err)
	}
	if _, err := diagram.Preprocess("   \n\n"); err == nil {
		t.Error("empty diagram should fail")
	}
}

func TestThemeVariables(t *testing.T) {
	sc, err := diagram.Render("%%{init: {\"theme\": \"dark\", \"themeVariables\": {\"background\": \"#123456\"}}}%%\ngraph LR; A-->B", theme.Default())
	if err != nil {
		t.Fatal(err)
	}
	if sc.Background != theme.Hex("#123456") {
		t.Errorf("background = %v", sc.Background)
	}
}

func TestCleanLabel(t *testing.T) {
	for in, want := range map[string]string{
		`"quoted"`:             "quoted",
		"a<br>b<br/>c":         "a\nb\nc",
		"#quot;x#quot; #9829;": `"x" ♥`,
		"fa:fa-car Car":        "Car",
		"`**md** text`":        "md text",
		`a\nb`:                 "a\nb",
		"&lt;tag&gt;":          "<tag>",
	} {
		if got := diagram.CleanLabel(in); got != want {
			t.Errorf("CleanLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAllTypesRegistered(t *testing.T) {
	want := []string{"class", "er", "flowchart", "gantt", "gitGraph", "journey", "mindmap", "pie", "quadrantChart", "sequence", "state", "timeline", "xychart"}
	got := strings.Join(diagram.Types(), ",")
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("type %s not registered (have %s)", w, got)
		}
	}
}
