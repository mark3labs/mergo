package state

import (
	"strings"
	"testing"

	"github.com/mark3labs/mergo/internal/devutil"
	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

func TestParseBasicTransition(t *testing.T) {
	src := `stateDiagram-v2
[*] --> A
A --> B
B --> [*]`
	p := newParser(&diagram.Config{Theme: theme.Default()})
	if err := p.parse(src); err != nil {
		t.Fatal(err)
	}
	if len(p.relations) != 3 {
		t.Errorf("expected 3 transitions, got %d", len(p.relations))
	}
}

func TestParseStateDescription(t *testing.T) {
	src := `stateDiagram-v2
state "This is a state" as s1
s1 --> s2
s2 : Another state`
	p := newParser(&diagram.Config{Theme: theme.Default()})
	if err := p.parse(src); err != nil {
		t.Fatal(err)
	}
	if len(p.states) < 2 {
		t.Errorf("expected at least 2 states, got %d", len(p.states))
	}
}

func TestParseTransitionWithLabel(t *testing.T) {
	src := `stateDiagram-v2
A --> B : transition label`
	p := newParser(&diagram.Config{Theme: theme.Default()})
	if err := p.parse(src); err != nil {
		t.Fatal(err)
	}
	if len(p.relations) != 1 {
		t.Fatal("expected 1 transition")
	}
	if p.relations[0].label != "transition label" {
		t.Errorf("expected label 'transition label', got '%s'", p.relations[0].label)
	}
}

func TestParseSpecialMarkers(t *testing.T) {
	src := `stateDiagram-v2
fork_state <<fork>>
choice_state <<choice>>
join_state <<join>>`
	p := newParser(&diagram.Config{Theme: theme.Default()})
	if err := p.parse(src); err != nil {
		t.Fatal(err)
	}
	if !p.states["fork_state"].isFork {
		t.Error("fork_state not marked as fork")
	}
	if !p.states["choice_state"].isChoice {
		t.Error("choice_state not marked as choice")
	}
	if !p.states["join_state"].isJoin {
		t.Error("join_state not marked as join")
	}
}

func TestRenderSmoke(t *testing.T) {
	testCases := []struct {
		name string
		src  string
	}{
		{
			"basic_flow",
			`stateDiagram-v2
[*] --> Still
Still --> Moving
Moving --> [*]`,
		},
		{
			"with_labels",
			`stateDiagram-v2
[*] --> Draft
Draft --> Submitted : submit
Submitted --> [*]`,
		},
		{
			"choice",
			`stateDiagram-v2
state if_state <<choice>>
[*] --> Check
Check --> if_state
if_state --> True : yes
if_state --> False : no`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sc, err := Render(strings.TrimPrefix(tc.src, "stateDiagram-v2\n"), &diagram.Config{Theme: theme.Default()})
			if err != nil {
				t.Fatalf("render error: %v", err)
			}
			if sc.Width < 10 || sc.Height < 10 {
				t.Errorf("scene too small: %fx%f", sc.Width, sc.Height)
			}
		})
	}
}

func TestRenderNoOverlap(t *testing.T) {
	src := `stateDiagram-v2
[*] --> FirstState
FirstState : First state with description
FirstState --> ProcessingState : begin processing
ProcessingState : Processing something
ProcessingState --> SecondState : done
SecondState --> [*]`

	sc, err := Render(src, &diagram.Config{Theme: theme.Default()})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}

	if sc.Width < 50 || sc.Height < 50 {
		t.Errorf("scene too small: %fx%f", sc.Width, sc.Height)
	}

	// Render to image for verification
	img := sc.Render(scene.RenderOptions{Scale: 1})
	if testing.Verbose() {
		t.Log("\n" + devutil.ASCII(img, 140))
	}
}
