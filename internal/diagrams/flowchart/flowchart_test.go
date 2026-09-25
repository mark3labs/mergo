package flowchart

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mergo/internal/devutil"
	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

func mustParse(t *testing.T, src string) *Graph {
	t.Helper()
	g, err := Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v\nsource:\n%s", err, src)
	}
	return g
}

func TestHeaderAndDirection(t *testing.T) {
	for src, want := range map[string]string{
		"graph":            "TB",
		"graph TD":         "TB",
		"flowchart LR":     "LR",
		"flowchart RL\nA":  "RL",
		"graph BT;A-->B":   "BT",
		"flowchart-elk TB": "TB",
	} {
		g := mustParse(t, src)
		if g.Dir != want {
			t.Errorf("%q: dir %s, want %s", src, g.Dir, want)
		}
	}
	if _, err := Parse("flowchart XY"); err == nil {
		t.Error("expected error for bad direction")
	}
}

func TestShapes(t *testing.T) {
	cases := map[string]Shape{
		"A[t]":                                ShapeRect,
		"A(t)":                                ShapeRounded,
		"A([t])":                              ShapeStadium,
		"A[[t]]":                              ShapeSubroutine,
		"A[(t)]":                              ShapeCylinder,
		"A((t))":                              ShapeCircle,
		"A(((t)))":                            ShapeDoubleCircle,
		"A>t]":                                ShapeOdd,
		"A{t}":                                ShapeDiamond,
		"A{{t}}":                              ShapeHexagon,
		"A[/t/]":                              ShapeLeanRight,
		"A[\\t\\]":                            ShapeLeanLeft,
		"A[/t\\]":                             ShapeTrapezoid,
		"A[\\t/]":                             ShapeTrapezoidAlt,
		`A@{ shape: cyl, label: "DB" }`:       ShapeCylinder,
		`A@{ shape: doc }`:                    "doc",
		`A@{ shape: notch-rect, label: "x" }`: "notch-rect",
		`A@{ shape: nonsense }`:               ShapeRect,
	}
	for src, want := range cases {
		g := mustParse(t, "flowchart TB\n"+src)
		n := g.Nodes["A"]
		if n == nil {
			t.Errorf("%s: node A missing", src)
			continue
		}
		if n.Shape != want {
			t.Errorf("%s: shape %s, want %s", src, n.Shape, want)
		}
		if !strings.Contains(src, "@{") && n.Label != "t" {
			t.Errorf("%s: label %q", src, n.Label)
		}
	}
	g := mustParse(t, "flowchart TB\n"+`A@{ shape: cyl, label: "My DB" }`)
	if g.Nodes["A"].Label != "My DB" {
		t.Errorf("label from @{}: %q", g.Nodes["A"].Label)
	}
}

func TestQuotedAndSpecialLabels(t *testing.T) {
	g := mustParse(t, `flowchart LR
    A["text (with) [brackets]"] --> B["say #quot;hi#quot; #9829;"]
    C["line1<br/>line2"]
    D["`+"`**bold** text`"+`"]
    E("a (nested) paren")`)
	if got := g.Nodes["A"].Label; got != "text (with) [brackets]" {
		t.Errorf("A label %q", got)
	}
	if got := g.Nodes["B"].Label; got != `say "hi" ♥` {
		t.Errorf("B label %q", got)
	}
	if got := g.Nodes["C"].Label; got != "line1\nline2" {
		t.Errorf("C label %q", got)
	}
	if got := g.Nodes["D"].Label; got != "\ue000bold\ue001 text" {
		t.Errorf("D label %q", got)
	}
	if got := g.Nodes["E"].Label; got != "a (nested) paren" {
		t.Errorf("E label %q", got)
	}
}

func TestEdges(t *testing.T) {
	type want struct {
		kind       LinkKind
		start, end Marker
		length     int
		label      string
	}
	cases := map[string]want{
		"A-->B":             {LinkNormal, 0, MarkerArrow, 1, ""},
		"A --- B":           {LinkNormal, 0, 0, 1, ""},
		"A---->B":           {LinkNormal, 0, MarkerArrow, 3, ""},
		"A-.->B":            {LinkDotted, 0, MarkerArrow, 1, ""},
		"A-.-B":             {LinkDotted, 0, 0, 1, ""},
		"A-...->B":          {LinkDotted, 0, MarkerArrow, 3, ""},
		"A==>B":             {LinkThick, 0, MarkerArrow, 1, ""},
		"A===B":             {LinkThick, 0, 0, 1, ""},
		"A --o B":           {LinkNormal, 0, MarkerCircle, 1, ""},
		"A --x B":           {LinkNormal, 0, MarkerCross, 1, ""},
		"A <--> B":          {LinkNormal, MarkerArrow, MarkerArrow, 1, ""},
		"A o--o B":          {LinkNormal, MarkerCircle, MarkerCircle, 1, ""},
		"A x--x B":          {LinkNormal, MarkerCross, MarkerCross, 1, ""},
		"A ~~~ B":           {LinkInvisible, 0, 0, 1, ""},
		"A -- hello --> B":  {LinkNormal, 0, MarkerArrow, 1, "hello"},
		"A -- hello ---> B": {LinkNormal, 0, MarkerArrow, 2, "hello"},
		"A -- open --- B":   {LinkNormal, 0, 0, 1, "open"},
		"A -->|hi there| B": {LinkNormal, 0, MarkerArrow, 1, "hi there"},
		"A -. dots .-> B":   {LinkDotted, 0, MarkerArrow, 1, "dots"},
		"A == big ==> B":    {LinkThick, 0, MarkerArrow, 1, "big"},
		"A e1@--> B":        {LinkNormal, 0, MarkerArrow, 1, ""},
		"a-b --> c-d":       {LinkNormal, 0, MarkerArrow, 1, ""},
	}
	for src, w := range cases {
		g := mustParse(t, "flowchart LR\n"+src)
		if len(g.Edges) != 1 {
			t.Errorf("%s: %d edges", src, len(g.Edges))
			continue
		}
		e := g.Edges[0]
		if e.Kind != w.kind || e.Start != w.start || e.End != w.end || e.Length != w.length || e.Label != w.label {
			t.Errorf("%s: got kind=%d start=%q end=%q len=%d label=%q; want %+v", src, e.Kind, e.Start, e.End, e.Length, e.Label, w)
		}
	}
	g := mustParse(t, "flowchart LR\na-b --> c-d")
	if g.Edges[0].From != "a-b" || g.Edges[0].To != "c-d" {
		t.Errorf("dashed ids: %s -> %s", g.Edges[0].From, g.Edges[0].To)
	}
	g = mustParse(t, "flowchart LR\nA e1@--> B")
	if g.Edges[0].ID != "e1" {
		t.Errorf("edge id %q", g.Edges[0].ID)
	}
}

func TestChainsAndAmpersand(t *testing.T) {
	g := mustParse(t, "flowchart TD\nA --> B --> C\nD & E --> F & G")
	if len(g.Edges) != 6 {
		t.Fatalf("edges = %d, want 6", len(g.Edges))
	}
	got := []string{}
	for _, e := range g.Edges {
		got = append(got, e.From+e.To)
	}
	want := "AB BC DF DG EF EG"
	if strings.Join(got, " ") != want {
		t.Errorf("edges %v, want %s", got, want)
	}
	for i, e := range g.Edges {
		if e.Index != i {
			t.Errorf("edge %d index %d", i, e.Index)
		}
	}
	g = mustParse(t, "graph TD; A-->B; A-->C; B-->D; C-->D")
	if len(g.Edges) != 4 || len(g.Nodes) != 4 {
		t.Errorf("semicolon statements: %d edges %d nodes", len(g.Edges), len(g.Nodes))
	}
}

func TestInlineNodeDefinitionsInEdges(t *testing.T) {
	g := mustParse(t, "flowchart TD\nA[Start] --> B{Ok?} -->|yes| C([Done])")
	if g.Nodes["A"].Label != "Start" || g.Nodes["B"].Shape != ShapeDiamond || g.Nodes["C"].Shape != ShapeStadium {
		t.Errorf("inline definitions not parsed: %+v %+v %+v", g.Nodes["A"], g.Nodes["B"], g.Nodes["C"])
	}
	if g.Edges[1].Label != "yes" {
		t.Errorf("label %q", g.Edges[1].Label)
	}
}

func TestSubgraphs(t *testing.T) {
	g := mustParse(t, `flowchart TB
    c1-->a2
    subgraph one
        a1-->a2
    end
    subgraph two [Second Group]
        direction LR
        b1-->b2
        subgraph inner["Inner title"]
            x
        end
    end
    subgraph "Quoted title"
        q
    end
    one --> two`)
	if len(g.Subgraphs) != 4 {
		t.Fatalf("subgraphs = %d", len(g.Subgraphs))
	}
	if g.Subgraphs["two"].Title != "Second Group" || g.Subgraphs["two"].Dir != "LR" {
		t.Errorf("two: %+v", g.Subgraphs["two"])
	}
	if g.Subgraphs["inner"].Parent != "two" || g.Subgraphs["inner"].Title != "Inner title" {
		t.Errorf("inner: %+v", g.Subgraphs["inner"])
	}
	if g.Nodes["a2"].Subgraph != "one" {
		t.Errorf("a2 should be in one (first mentioned outside, then inside), got %q", g.Nodes["a2"].Subgraph)
	}
	if g.Nodes["c1"].Subgraph != "" {
		t.Errorf("c1 subgraph %q", g.Nodes["c1"].Subgraph)
	}
	if g.Nodes["x"].Subgraph != "inner" {
		t.Errorf("x subgraph %q", g.Nodes["x"].Subgraph)
	}
	if _, ok := g.Nodes["one"]; ok {
		t.Error("subgraph id should not be a node")
	}
	if _, err := Parse("flowchart TB\nsubgraph a\nx"); err == nil {
		t.Error("expected error for unclosed subgraph")
	}
	if _, err := Parse("flowchart TB\nend"); err == nil {
		t.Error("expected error for stray end")
	}
}

func TestStyles(t *testing.T) {
	g := mustParse(t, `flowchart LR
    A:::hot --> B
    classDef hot fill:#f00,stroke:#333,stroke-width:4px
    classDef a,b color:#fff
    class B a
    style A fill:rgb(1, 2, 3),stroke-dasharray: 5 5
    linkStyle 0 stroke:#ff3,stroke-width:4px
    linkStyle default color:red
    click A callback "tooltip"`)
	if g.ClassDefs["hot"]["fill"] != "#f00" || g.ClassDefs["hot"]["stroke-width"] != "4px" {
		t.Errorf("classDef: %v", g.ClassDefs["hot"])
	}
	if g.ClassDefs["b"]["color"] != "#fff" {
		t.Errorf("multi classDef: %v", g.ClassDefs)
	}
	if strings.Join(g.Nodes["A"].Classes, ",") != "hot" || strings.Join(g.Nodes["B"].Classes, ",") != "a" {
		t.Errorf("classes: %v %v", g.Nodes["A"].Classes, g.Nodes["B"].Classes)
	}
	if g.Nodes["A"].Style["fill"] != "rgb(1, 2, 3)" || g.Nodes["A"].Style["stroke-dasharray"] != "5 5" {
		t.Errorf("style: %v", g.Nodes["A"].Style)
	}
	if g.LinkStyles[0]["stroke"] != "#ff3" || g.LinkStyles[-1]["color"] != "red" {
		t.Errorf("linkStyles: %v", g.LinkStyles)
	}
}

func TestErrorsHaveLineNumbers(t *testing.T) {
	for _, src := range []string{
		"flowchart TD\nA --> B\nA -->",
		"flowchart TD\nA[unclosed",
		"flowchart TD\nA --> B\n --> C",
	} {
		_, err := Parse(src)
		if err == nil || !strings.Contains(err.Error(), "line ") {
			t.Errorf("%q: expected line-numbered error, got %v", src, err)
		}
	}
}

func TestNeverPanics(t *testing.T) {
	files, _ := filepath.Glob("../../../examples/flowchart/*.mmd")
	for _, f := range files {
		b, _ := os.ReadFile(f)
		s := string(b)
		for i := 0; i <= len(s); i += 3 {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("%s prefix %d panicked: %v", f, i, r)
					}
				}()
				_, _ = diagram.Render(s[:i], theme.Default())
			}()
		}
	}
}

// TestExamplesRender renders all examples and checks geometric sanity.
func TestExamplesRender(t *testing.T) {
	files, _ := filepath.Glob("../../../examples/flowchart/*.mmd")
	if len(files) == 0 {
		t.Fatal("no examples found")
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, th := range []*theme.Theme{theme.Default(), theme.DarkTheme(), theme.MustGet(theme.DefaultName, true)} {
			sc, err := diagram.Render(string(b), th)
			if err != nil {
				t.Errorf("%s: %v", f, err)
				continue
			}
			if sc.Width < 50 || sc.Height < 30 || sc.Width > 8000 || sc.Height > 8000 {
				t.Errorf("%s: suspicious size %.0fx%.0f", f, sc.Width, sc.Height)
			}
			if testing.Verbose() && th.Name == "default" {
				img := sc.Render(scene.RenderOptions{Scale: 1})
				t.Logf("%s\n%s", f, devutil.ASCII(img, 120))
			}
		}
	}
}

func TestLayoutNoOverlap(t *testing.T) {
	files, _ := filepath.Glob("../../../examples/flowchart/*.mmd")
	for _, f := range files {
		b, _ := os.ReadFile(f)
		doc, err := diagram.Preprocess(string(b))
		if err != nil {
			t.Fatal(err)
		}
		g, err := Parse(doc.Source)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		r := &renderer{g: g, th: theme.Default(), cfg: &diagram.Config{Theme: theme.Default()}}
		r.render()
		var rects []scene.Rect
		var ids []string
		for _, id := range g.NodeOrder {
			rects = append(rects, r.lnodes[id].Rect())
			ids = append(ids, id)
		}
		for i := range rects {
			for j := i + 1; j < len(rects); j++ {
				a, c := rects[i], rects[j]
				if a.X < c.X+c.W-1 && c.X < a.X+a.W-1 && a.Y < c.Y+c.H-1 && c.Y < a.Y+a.H-1 {
					t.Errorf("%s: nodes %s and %s overlap: %+v %+v", f, ids[i], ids[j], a, c)
				}
			}
		}
		// clusters contain their nodes
		for id, sg := range g.Subgraphs {
			cr := r.clusters[id].Rect()
			for _, nid := range sg.Children {
				nr := r.lnodes[nid].Rect()
				if nr.X < cr.X-0.5 || nr.Y < cr.Y-0.5 || nr.X+nr.W > cr.X+cr.W+0.5 || nr.Y+nr.H > cr.Y+cr.H+0.5 {
					t.Errorf("%s: node %s outside cluster %s", f, nid, id)
				}
			}
		}
	}
}

func TestSemicolonEntities(t *testing.T) {
	g := mustParse(t, `flowchart LR
    A -->|#quot;x#quot;| B; B -- #9829; --> C
    classDef k fill:#333; class C k
    style B fill:#abc,stroke:#333;B-->D`)
	if len(g.Edges) != 3 {
		t.Fatalf("edges %d, want 3", len(g.Edges))
	}
	if got := g.Edges[0].Label; got != `"x"` {
		t.Errorf("edge 0 label %q", got)
	}
	if got := g.Edges[1].Label; got != "♥" {
		t.Errorf("edge 1 label %q", got)
	}
	if c := g.Nodes["C"].Classes; len(c) != 1 || c[0] != "k" {
		t.Errorf("C classes %v", c)
	}
	if g.ClassDefs["k"]["fill"] != "#333" || g.Nodes["B"].Style["stroke"] != "#333" {
		t.Errorf("styles %v %v", g.ClassDefs, g.Nodes["B"].Style)
	}
}
