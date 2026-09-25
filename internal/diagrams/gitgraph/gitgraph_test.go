package gitgraph

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
	g, err := Parse(`gitGraph TB:
    commit id: "a" tag: "v1"
    commit type: HIGHLIGHT
    branch dev order: 2
    commit id: "d1"
    checkout main
    commit id: "m2" type: REVERSE
    merge dev tag: "v2"
    switch dev
    cherry-pick id: "m2"`, "main", 0)
	if err != nil {
		t.Fatal(err)
	}
	if g.Dir != "TB" || len(g.Commits) != 6 || len(g.Branches) != 2 {
		t.Fatalf("dir=%s commits=%d branches=%d", g.Dir, len(g.Commits), len(g.Branches))
	}
	if g.Commits[0].Tags[0] != "v1" || g.Commits[1].Type != Highlight || g.Commits[3].Type != Reverse {
		t.Error("attributes")
	}
	m := g.Commits[4]
	if m.Type != Merge || len(m.Parents) != 2 || m.Parents[1] != "d1" || m.Tags[0] != "v2" {
		t.Errorf("merge %+v", m)
	}
	cp := g.Commits[5]
	if cp.Type != CherryPick || cp.Branch != "dev" || cp.Parents[1] != "m2" {
		t.Errorf("cherry-pick %+v", cp)
	}
	if g.byName["dev"].Order != 2 {
		t.Error("order")
	}
	// generated ids are deterministic
	g2, _ := Parse("gitGraph\ncommit\ncommit", "main", 0)
	g3, _ := Parse("gitGraph\ncommit\ncommit", "main", 0)
	if g2.Commits[1].ID != g3.Commits[1].ID {
		t.Error("ids not deterministic")
	}
	for _, bad := range []string{"checkout nope", "merge main", "branch main", "cherry-pick id: \"x\"", "frobnicate"} {
		if _, err := Parse("gitGraph\ncommit\n"+bad, "main", 0); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}

func TestExamples(t *testing.T) {
	files, _ := filepath.Glob("../../../examples/gitgraph/*.mmd")
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
		sc, err := diagram.Render(s, theme.DarkTheme())
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if testing.Verbose() {
			t.Logf("%s\n%s", f, devutil.ASCII(sc.Render(scene.RenderOptions{Scale: 1}), 120))
		}
	}
}
