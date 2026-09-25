package gitgraph

import (
	"testing"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

func TestGitGraphBasic(t *testing.T) {
	src := `commit id: "A"
  commit id: "B"
  commit id: "C"`

	th := theme.Default()
	_, err := Render(src, &diagram.Config{Theme: th})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
}

func TestGitGraphBranching(t *testing.T) {
	src := `commit id: "1"
  commit id: "2"
  branch develop
  checkout develop
  commit id: "3"
  commit id: "4"
  checkout main
  commit id: "5"
  merge develop id: "6"`

	th := theme.Default()
	_, err := Render(src, &diagram.Config{Theme: th})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
}

func TestGitGraphCommitTypes(t *testing.T) {
	src := `commit id: "A" type: NORMAL
  commit id: "B" type: REVERSE
  commit id: "C" type: HIGHLIGHT`

	th := theme.Default()
	_, err := Render(src, &diagram.Config{Theme: th})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
}

func TestGitGraphWithTags(t *testing.T) {
	src := `commit id: "A" tag: "v1.0.0"
  commit id: "B" tag: "v1.1.0"
  branch feature
  commit id: "C"
  checkout main
  merge feature id: "D" tag: "v2.0.0"`

	th := theme.Default()
	_, err := Render(src, &diagram.Config{Theme: th})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
}

func TestParseCommit(t *testing.T) {
	g := &GitGraph{
		Commits:       make(map[string]*Commit),
		Branches:      make(map[string]*Branch),
		CommitOrder:   []*Commit{},
		MainBranch:    "main",
		CurrentBranch: "main",
	}
	g.Branches[g.MainBranch] = &Branch{Name: g.MainBranch, Order: 0}

	err := parseCommit(g, `commit id: "test" type: HIGHLIGHT tag: "v1"`)
	if err != nil {
		t.Fatalf("parseCommit failed: %v", err)
	}

	if len(g.CommitOrder) != 1 {
		t.Errorf("expected 1 commit, got %d", len(g.CommitOrder))
	}

	commit := g.CommitOrder[0]
	if commit.ID != "test" {
		t.Errorf("commit.ID = %q, want %q", commit.ID, "test")
	}
	if commit.Type != "HIGHLIGHT" {
		t.Errorf("commit.Type = %q, want %q", commit.Type, "HIGHLIGHT")
	}
	if commit.Tag != "v1" {
		t.Errorf("commit.Tag = %q, want %q", commit.Tag, "v1")
	}
}

func TestGenerateCommitID(t *testing.T) {
	g := &GitGraph{
		CommitOrder: []*Commit{},
	}

	id1 := generateCommitID(g)
	if id1 == "" {
		t.Fatal("generated ID is empty")
	}

	g.CommitOrder = append(g.CommitOrder, &Commit{})
	id2 := generateCommitID(g)

	if id1 == id2 {
		t.Errorf("IDs should be different: %q vs %q", id1, id2)
	}
}

func TestGitGraphRender(t *testing.T) {
	src := `commit id: "A"
  branch develop
  commit id: "B"
  checkout main
  commit id: "C"
  merge develop`

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

	// Render to image
	img := sc.Render(scene.RenderOptions{Scale: 1})
	if img == nil {
		t.Fatal("rendered image is nil")
	}
}

func TestGitGraphEmptySource(t *testing.T) {
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

func TestBranchOperations(t *testing.T) {
	g := &GitGraph{
		Commits:       make(map[string]*Commit),
		Branches:      make(map[string]*Branch),
		CommitOrder:   []*Commit{},
		MainBranch:    "main",
		CurrentBranch: "main",
	}
	g.Branches[g.MainBranch] = &Branch{Name: g.MainBranch, Order: 0}

	// Branch doesn't exist yet
	err := parseCommand(g, "checkout develop", 1)
	if err == nil {
		t.Fatal("expected error for non-existent branch checkout")
	}

	// Create branch
	err = parseCommand(g, "branch develop", 1)
	if err != nil {
		t.Fatalf("branch creation failed: %v", err)
	}

	if _, ok := g.Branches["develop"]; !ok {
		t.Fatal("develop branch not created")
	}

	// Checkout branch
	err = parseCommand(g, "checkout develop", 2)
	if err != nil {
		t.Fatalf("checkout failed: %v", err)
	}

	if g.CurrentBranch != "develop" {
		t.Errorf("CurrentBranch = %q, want %q", g.CurrentBranch, "develop")
	}
}
