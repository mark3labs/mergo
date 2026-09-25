package class

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

func parse(t *testing.T, src string) *Diagram {
	t.Helper()
	d, err := Parse("classDiagram\n" + src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return d
}

func TestClassesAndMembers(t *testing.T) {
	d := parse(t, `class Animal {
        <<abstract>>
        +int age
        +String gender
        +isMammal()* bool
        +mate()$
        -List~String~ tags
    }
    class Box~T~
    class Shape["Geometric shape"]
    class Duck:::highlight
    Animal : +String name
    Animal : +swim() void
    <<interface>> Flyer
    class Map~K,List~V~~`)
	a := d.Classes["Animal"]
	if len(a.Attributes) != 4 || len(a.Methods) != 3 {
		t.Fatalf("members: %+v", a)
	}
	if a.Annotations[0] != "abstract" {
		t.Errorf("annotations %v", a.Annotations)
	}
	if !a.Methods[0].Abstract || a.Methods[0].Text != "+isMammal() bool" {
		t.Errorf("abstract method: %+v", a.Methods[0])
	}
	if !a.Methods[1].Static || a.Methods[1].Text != "+mate()" {
		t.Errorf("static method: %+v", a.Methods[1])
	}
	if a.Attributes[2].Text != "-List<String> tags" {
		t.Errorf("generic attr: %q", a.Attributes[2].Text)
	}
	if d.Classes["Box"].Label != "Box<T>" {
		t.Errorf("generic class label %q", d.Classes["Box"].Label)
	}
	if d.Classes["Map"].Label != "Map<K,List<V>>" {
		t.Errorf("nested generic %q", d.Classes["Map"].Label)
	}
	if d.Classes["Shape"].Label != "Geometric shape" {
		t.Errorf("label %q", d.Classes["Shape"].Label)
	}
	if d.Classes["Duck"].Classes[0] != "highlight" {
		t.Error(":::class")
	}
	if d.Classes["Flyer"].Annotations[0] != "interface" {
		t.Error("annotation statement")
	}
}

func TestRelations(t *testing.T) {
	cases := map[string]Relation{
		`A <|-- B`:                  {From: "A", To: "B", FromEnd: EndInheritance},
		`A *-- B`:                   {From: "A", To: "B", FromEnd: EndComposition},
		`A o-- B`:                   {From: "A", To: "B", FromEnd: EndAggregation},
		`A --> B`:                   {From: "A", To: "B", ToEnd: EndArrow},
		`A -- B`:                    {From: "A", To: "B"},
		`A ..> B`:                   {From: "A", To: "B", ToEnd: EndArrow, Dashed: true},
		`A ..|> B`:                  {From: "A", To: "B", ToEnd: EndInheritance, Dashed: true},
		`A .. B`:                    {From: "A", To: "B", Dashed: true},
		`A <|--|> B`:                {From: "A", To: "B", FromEnd: EndInheritance, ToEnd: EndInheritance},
		`A --o B`:                   {From: "A", To: "B", ToEnd: EndAggregation},
		`A ()-- B`:                  {From: "A", To: "B", FromEnd: EndLollipop},
		`Too--Bar`:                  {From: "Too", To: "Bar"},
		`A "1" --> "*" B : has`:     {From: "A", To: "B", ToEnd: EndArrow, FromCard: "1", ToCard: "*", Label: "has"},
		`Customer "1" --o "0..n" O`: {From: "Customer", To: "O", ToEnd: EndAggregation, FromCard: "1", ToCard: "0..n"},
		`A<|--B`:                    {From: "A", To: "B", FromEnd: EndInheritance},
		`List~int~ <|-- Impl`:       {From: "List", To: "Impl", FromEnd: EndInheritance},
	}
	for src, w := range cases {
		d := parse(t, src)
		if len(d.Relations) != 1 {
			t.Errorf("%s: %d relations", src, len(d.Relations))
			continue
		}
		if got := *d.Relations[0]; got != w {
			t.Errorf("%s:\n got %+v\nwant %+v", src, got, w)
		}
	}
}

func TestNamespacesNotesStyles(t *testing.T) {
	d := parse(t, `namespace Shapes {
        class Triangle
        class Square {
            +int side
        }
    }
    note "free note"
    note for Triangle "three\nsides"
    classDef hot fill:#f00,color:#fff
    cssClass "Triangle,Square" hot
    style Square stroke:#00f,stroke-width:3px
    direction LR`)
	if d.Dir != "LR" {
		t.Error("direction")
	}
	ns := d.Namespaces["Shapes"]
	if ns == nil || len(ns.Classes) != 2 || d.Classes["Square"].Namespace != "Shapes" {
		t.Fatalf("namespace %+v", ns)
	}
	if len(d.Notes) != 2 || d.Notes[1].For != "Triangle" || d.Notes[1].Text != "three\nsides" {
		t.Errorf("notes %+v %+v", d.Notes[0], d.Notes[1])
	}
	if d.Classes["Square"].Classes[0] != "hot" || d.Classes["Square"].Style["stroke-width"] != "3px" {
		t.Error("styles")
	}
}

func TestErrors(t *testing.T) {
	for _, src := range []string{"class A {\n+x", "namespace X {\nclass A", "}", "A <|-- ", "what is this"} {
		if _, err := Parse("classDiagram\n" + src); err == nil {
			t.Errorf("%q: expected error", src)
		}
	}
}

func files(t *testing.T) []string {
	fs, _ := filepath.Glob("../../../examples/class/*.mmd")
	if len(fs) == 0 {
		t.Fatal("no examples")
	}
	return fs
}

func TestNeverPanics(t *testing.T) {
	for _, f := range files(t) {
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
	}
}

func TestExamples(t *testing.T) {
	for _, f := range files(t) {
		b, _ := os.ReadFile(f)
		doc, err := diagram.Preprocess(string(b))
		if err != nil {
			t.Fatal(err)
		}
		d, err := Parse(doc.Source)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		r := &renderer{d: d, th: theme.Default(), cfg: &diagram.Config{Theme: theme.Default()}}
		r.render()
		var rects []scene.Rect
		for _, id := range d.Order {
			rects = append(rects, r.nodes[id].Rect())
		}
		for i := range rects {
			for j := i + 1; j < len(rects); j++ {
				a, c := rects[i], rects[j]
				if a.X < c.X+c.W-1 && c.X < a.X+a.W-1 && a.Y < c.Y+c.H-1 && c.Y < a.Y+a.H-1 {
					t.Errorf("%s: classes %s and %s overlap", f, d.Order[i], d.Order[j])
				}
			}
		}
		sc, err := diagram.Render(string(b), theme.DarkTheme())
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if testing.Verbose() {
			t.Logf("%s\n%s", f, devutil.ASCII(sc.Render(scene.RenderOptions{Scale: 1}), 120))
		}
		if !strings.Contains(f, "x") && sc.Width < 50 {
			t.Errorf("%s: too small", f)
		}
	}
}
