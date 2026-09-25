package state

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mergo/internal/devutil"
	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

func parse(t *testing.T, src string) *Diagram {
	t.Helper()
	d, err := Parse("stateDiagram-v2\n" + src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return d
}

func TestStatesAndTransitions(t *testing.T) {
	d := parse(t, `[*] --> Still
    Still --> [*]
    Still --> Moving : push
    Moving --> Crash
    state "Long description" as LD
    Moving : is moving
    Moving : fast
    Crash:::bad
    state c <<choice>>
    state f <<fork>>
    state j <<join>>`)
	if len(d.Transitions) != 4 {
		t.Fatalf("transitions = %d", len(d.Transitions))
	}
	start, end := d.States[d.Transitions[0].From], d.States[d.Transitions[1].To]
	if start.Kind != KindStart || end.Kind != KindEnd || start.ID == end.ID {
		t.Errorf("start/end: %+v %+v", start, end)
	}
	if d.Transitions[2].Label != "push" {
		t.Error("label")
	}
	if d.States["LD"].Label != "Long description" {
		t.Error("state as")
	}
	if len(d.States["Moving"].Descs) != 2 {
		t.Error("descriptions")
	}
	if d.States["Crash"].Classes[0] != "bad" {
		t.Error(":::")
	}
	if d.States["c"].Kind != KindChoice || d.States["f"].Kind != KindFork || d.States["j"].Kind != KindJoin {
		t.Error("pseudo states")
	}
}

func TestCompositeScopes(t *testing.T) {
	d := parse(t, `[*] --> A
    state A {
        direction LR
        [*] --> a1
        a1 --> [*]
        state Inner {
            [*] --> i1
        }
        --
        [*] --> b1
    }
    A --> [*]`)
	a := d.States["A"]
	if !a.IsComposite() || a.Dir != "LR" || len(a.Regions) != 2 {
		t.Fatalf("composite %+v", a)
	}
	if d.States["a1"].Parent != "A" || d.States["i1"].Parent != "Inner" || d.States["Inner"].Parent != "A" {
		t.Error("parents")
	}
	// [*] in different scopes are different nodes
	starts := map[string]bool{}
	for _, tr := range d.Transitions {
		if d.States[tr.From].Kind == KindStart {
			starts[tr.From] = true
		}
	}
	if len(starts) != 4 {
		t.Errorf("start pseudo states = %d, want 4 (root, A region 0, Inner, A region 1)", len(starts))
	}
}

func TestNotesAndStyles(t *testing.T) {
	d := parse(t, `A --> B
    note right of A : short
    note left of B
        multi
        line
    end note
    classDef hot fill:#f00
    class A, B hot
    style B stroke:#00f`)
	if len(d.Notes) != 2 || d.Notes[0].Text != "short" || !d.Notes[1].Left || d.Notes[1].Text != "multi\nline" {
		t.Errorf("notes %+v %+v", d.Notes[0], d.Notes[1])
	}
	if d.States["A"].Classes[0] != "hot" || d.States["B"].Style["stroke"] != "#00f" {
		t.Error("styles")
	}
}

func TestErrors(t *testing.T) {
	for _, src := range []string{"state A {\nB", "}", "note left of A\ntext", "A -> B -> C what"} {
		if _, err := Parse("stateDiagram-v2\n" + src); err == nil {
			t.Errorf("%q: expected error", src)
		}
	}
}

func files(t *testing.T) []string {
	fs, _ := filepath.Glob("../../../examples/state/*.mmd")
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

func TestExamplesGeometry(t *testing.T) {
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
		r := newRenderer(d, &diagram.Config{Theme: theme.Default()})
		r.render()
		var ids []string
		for _, id := range d.Order {
			if _, ok := r.nodes[id]; ok {
				ids = append(ids, id)
			}
		}
		for i := range ids {
			for j := i + 1; j < len(ids); j++ {
				a, c := r.nodes[ids[i]].Rect(), r.nodes[ids[j]].Rect()
				if a.X < c.X+c.W-1 && c.X < a.X+a.W-1 && a.Y < c.Y+c.H-1 && c.Y < a.Y+a.H-1 {
					t.Errorf("%s: %s and %s overlap", f, ids[i], ids[j])
				}
			}
		}
		// children inside their composite
		for id, s := range d.States {
			if s.Parent == "" {
				continue
			}
			var cr scene.Rect
			if n, ok := r.nodes[id]; ok {
				cr = n.Rect()
			} else {
				cr = r.clusters[id].Rect()
			}
			pr := r.clusters[s.Parent].Rect()
			if cr.X < pr.X-0.5 || cr.Y < pr.Y-0.5 || cr.X+cr.W > pr.X+pr.W+0.5 || cr.Y+cr.H > pr.Y+pr.H+0.5 {
				t.Errorf("%s: %s escapes composite %s", f, id, s.Parent)
			}
		}
		sc, err := diagram.Render(string(b), theme.MustGet("forest"))
		if err != nil {
			t.Fatal(err)
		}
		if testing.Verbose() {
			t.Logf("%s\n%s", f, devutil.ASCII(sc.Render(scene.RenderOptions{Scale: 1}), 120))
		}
	}
}
