package diagram_test

import (
	"strings"
	"testing"

	"github.com/mark3labs/mergo/internal/diagram"
	_ "github.com/mark3labs/mergo/internal/mermaid"
	"github.com/mark3labs/mergo/internal/scene"
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
	// [b] [/b] [i] [/i] stand for the scene style markers
	mk := strings.NewReplacer("[b]", string(scene.BoldOn), "[/b]", string(scene.BoldOff),
		"[i]", string(scene.ItalicOn), "[/i]", string(scene.ItalicOff))
	for in, want := range map[string]string{
		`"quoted"`:                     "quoted",
		"a<br>b<br/>c":                 "a\nb\nc",
		"#quot;x#quot; #9829;":         `"x" ♥`,
		"fa:fa-car Car":                "Car",
		"`**md** text`":                "[b]md[/b] text",
		`a\nb`:                         "a\nb",
		"&lt;tag&gt;":                  "<tag>",
		"a<BR >b<br />c":               "a\nb\nc",
		"<b>bold</b> <I>it</I>":        "[b]bold[/b] [i]it[/i]",
		"<strong>s</strong><em>e</em>": "[b]s[/b][i]e[/i]",
		"<span>plain</span>":           "plain",
		"`*em* and _em_`":              "[i]em[/i] and [i]em[/i]",
		"`__strong__ x`":               "[b]strong[/b] x",
		"`***both***`":                 "[b][i]both[/b][/i]",
		"`snake_case_name`":            "snake_case_name",
		"`2 * 3 * 4`":                  "2 * 3 * 4",
		"a * b":                        "a * b",
		"**not markdown**":             "**not markdown**",
		"<b> a<br>b </b>":              "[b]a[/b]\n[b]b[/b]",
		"<b></b>":                      "",
	} {
		if got, w := diagram.CleanLabel(in), mk.Replace(want); got != w {
			t.Errorf("CleanLabel(%q) = %q, want %q", in, got, w)
		}
	}
	if got, want := diagram.CleanInline("a<br>b &amp; <b>c<br>d</b>"), mk.Replace("a b & [b]c[/b] [b]d[/b]"); got != want {
		t.Errorf("CleanInline = %q, want %q", got, want)
	}
}

func TestTrimStatementEnd(t *testing.T) {
	for in, want := range map[string]string{
		"a --> b;":           "a --> b",
		"say #quot;hi#quot;": "say #quot;hi#quot;",
		"x #9829; ":          "x #9829;",
		"fish &amp;":         "fish &amp;",
		"&#38; &#x26;":       "&#38; &#x26;",
		"trailing # ;":       "trailing #",
		";":                  "",
	} {
		if got := diagram.TrimStatementEnd(in); got != want {
			t.Errorf("TrimStatementEnd(%q) = %q, want %q", in, got, want)
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
