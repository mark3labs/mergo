package er

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
	d, err := Parse(`erDiagram
    direction LR
    CUSTOMER ||--o{ ORDER : places
    ORDER ||--|{ LINE-ITEM : contains
    A |o..o| B : "maybe"
    C }|--|| D : x
    E one or more optionally to zero or more F : words
    "Quoted Name" ||--|| G : q
    P["Person record"] {
        string name PK "the name"
        int age
        uuid org_id PK, FK
        list~string~ tags
    }
    classDef hot fill:#f00
    class P hot`)
	if err != nil {
		t.Fatal(err)
	}
	if d.Dir != "LR" || len(d.Rels) != 6 {
		t.Fatalf("dir %s rels %d", d.Dir, len(d.Rels))
	}
	r := d.Rels[0]
	if r.FromCard != ExactlyOne || r.ToCard != ZeroOrMore || !r.Identifying || r.Label != "places" {
		t.Errorf("rel0 %+v", r)
	}
	if r := d.Rels[2]; r.FromCard != ZeroOrOne || r.ToCard != ZeroOrOne || r.Identifying || r.Label != "maybe" {
		t.Errorf("rel2 %+v", r)
	}
	if r := d.Rels[3]; r.FromCard != OneOrMore || r.ToCard != ExactlyOne {
		t.Errorf("rel3 %+v", r)
	}
	if r := d.Rels[4]; r.FromCard != OneOrMore || r.ToCard != ZeroOrMore || r.Identifying {
		t.Errorf("rel4 %+v", r)
	}
	if _, ok := d.Entities["Quoted Name"]; !ok {
		t.Error("quoted entity")
	}
	p := d.Entities["P"]
	if p.Label != "Person record" || len(p.Attrs) != 4 || p.Classes[0] != "hot" {
		t.Fatalf("P = %+v", p)
	}
	if a := p.Attrs[0]; a.Name != "name" || a.Keys[0] != "PK" || a.Comment != "the name" {
		t.Errorf("attr0 %+v", a)
	}
	if a := p.Attrs[2]; len(a.Keys) != 2 || a.Keys[1] != "FK" {
		t.Errorf("attr2 %+v", a)
	}
	if p.Attrs[3].Type != "list<string>" {
		t.Errorf("generic type %q", p.Attrs[3].Type)
	}
	for _, bad := range []string{"A {\nint", "A ||--o{", "A {\nthis is not ok at all x\n}"} {
		if _, err := Parse("erDiagram\n" + bad); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}

func TestExamples(t *testing.T) {
	files, _ := filepath.Glob("../../../examples/er/*.mmd")
	if len(files) == 0 {
		t.Fatal("no examples")
	}
	for _, f := range files {
		b, _ := os.ReadFile(f)
		s := string(b)
		for i := 0; i <= len(s); i += 4 {
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
		if sc.Width < 100 || sc.Height < 100 {
			t.Errorf("%s: too small %vx%v", f, sc.Width, sc.Height)
		}
		if testing.Verbose() {
			t.Logf("%s\n%s", f, devutil.ASCII(sc.Render(scene.RenderOptions{Scale: 1}), 120))
		}
	}
}
