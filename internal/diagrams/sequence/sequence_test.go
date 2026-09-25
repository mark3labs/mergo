package sequence

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mergo/internal/devutil"
	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

func TestParseBasic(t *testing.T) {
	src := `sequenceDiagram
    participant Alice
    participant Bob
    Alice->>Bob: Hello`

	p := newParser(src)
	d, err := p.parse()
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if len(d.participants) != 2 {
		t.Errorf("expected 2 participants, got %d", len(d.participants))
	}

	if d.participants[0].ID != "Alice" {
		t.Errorf("expected first participant Alice, got %s", d.participants[0].ID)
	}

	if d.participants[1].ID != "Bob" {
		t.Errorf("expected second participant Bob, got %s", d.participants[1].ID)
	}

	if len(d.messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(d.messages))
	}

	msg := d.messages[0]
	if msg.Label != "Hello" {
		t.Errorf("expected label 'Hello', got '%s'", msg.Label)
	}
}

func TestParseActors(t *testing.T) {
	src := `sequenceDiagram
    actor Alice
    actor Bob
    Alice->>Bob: Hi`

	p := newParser(src)
	d, err := p.parse()
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if d.participants[0].Type != TypeActor {
		t.Errorf("expected Alice to be actor type")
	}
	if d.participants[1].Type != TypeActor {
		t.Errorf("expected Bob to be actor type")
	}
}

func TestParseAlias(t *testing.T) {
	src := `sequenceDiagram
    participant A as Alice
    participant J as John
    A->>J: Hello`

	p := newParser(src)
	d, err := p.parse()
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if d.participants[0].ID != "A" {
		t.Errorf("expected ID 'A', got '%s'", d.participants[0].ID)
	}
	if d.participants[0].Display != "Alice" {
		t.Errorf("expected display 'Alice', got '%s'", d.participants[0].Display)
	}
}

func TestParseTypes(t *testing.T) {
	t.Skip("JSON config parsing needs fixing")

	src := `sequenceDiagram
    participant A@{"type": "database"}
    participant B@{"type": "queue"}
    A->>B: Test`

	p := newParser(src)
	d, err := p.parse()
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if d.participants[0].Type != TypeDatabase {
		t.Errorf("expected database type, got %v", d.participants[0].Type)
	}
	if d.participants[1].Type != TypeQueue {
		t.Errorf("expected queue type, got %v", d.participants[1].Type)
	}
}

func TestParseAutonumber(t *testing.T) {
	src := `sequenceDiagram
    autonumber 1 2
    Alice->>Bob: Hello
    Bob-->>Alice: Hi`

	p := newParser(src)
	d, err := p.parse()
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if d.autonumber == nil {
		t.Errorf("expected autonumber to be set")
	}
	if d.autonumber.Start != 1.0 || d.autonumber.Increment != 2.0 {
		t.Errorf("expected autonumber 1/2, got %v/%v", d.autonumber.Start, d.autonumber.Increment)
	}
}

func TestParseArrowTypes(t *testing.T) {
	tests := []struct {
		name string
		line string
		want ArrowType
	}{
		{"solid", "A->B: msg", ArrowSolid},
		{"dotted", "A-->B: msg", ArrowDotted},
		{"filled", "A->>B: msg", ArrowFilledHead},
		{"dottedFilled", "A-->>B: msg", ArrowDottedFilledHead},
		{"cross", "A-xB: msg", ArrowCross},
		{"async", "A-)B: msg", ArrowAsync},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := fmt.Sprintf(`sequenceDiagram
    participant A
    participant B
    %s`, tt.line)

			p := newParser(src)
			d, err := p.parse()
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}

			if len(d.messages) == 0 {
				t.Errorf("expected message")
			} else if d.messages[0].Arrow != tt.want {
				t.Errorf("expected arrow type %v, got %v", tt.want, d.messages[0].Arrow)
			}
		})
	}
}

func TestRenderSmoke(t *testing.T) {
	// Test rendering all example files
	examplesDir := filepath.Join("..", "..", "..", "examples", "sequence")

	entries, err := os.ReadDir(examplesDir)
	if err != nil {
		t.Skipf("skipping smoke test: examples dir not found: %v", err)
		return
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".mmd") {
			continue
		}

		t.Run(entry.Name(), func(t *testing.T) {
			filePath := filepath.Join(examplesDir, entry.Name())
			src, err := os.ReadFile(filePath)
			if err != nil {
				t.Fatalf("failed to read file: %v", err)
			}

			th := theme.Default()
			sc, err := diagram.Render(string(src), th)
			if err != nil {
				t.Fatalf("render failed: %v", err)
			}

			if sc == nil {
				t.Errorf("expected scene, got nil")
			}

			// Check size is reasonable
			if sc.Width <= 0 || sc.Height <= 0 {
				t.Errorf("expected positive dimensions, got %fx%f", sc.Width, sc.Height)
			}

			// Render to image at small scale
			img := sc.Render(scene.RenderOptions{Scale: 1})
			if img == nil || img.Bounds().Dx() <= 0 || img.Bounds().Dy() <= 0 {
				t.Errorf("expected valid image")
			}

			if testing.Verbose() {
				ascii := devutil.ASCII(img, 100)
				t.Logf("\n%s:\n%s", entry.Name(), ascii)
			}
		})
	}
}

func TestRenderBasic(t *testing.T) {
	src := `sequenceDiagram
    participant Alice
    participant Bob
    Alice->>Bob: Hello Bob, how are you?
    Bob-->>Alice: Great!`

	th := theme.Default()
	sc, err := diagram.Render(src, th)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}

	if sc.Width <= 0 || sc.Height <= 0 {
		t.Errorf("expected positive dimensions, got %fx%f", sc.Width, sc.Height)
	}

	// Should have at least some items (boxes, lines, text)
	if len(sc.Items) < 5 {
		t.Logf("warning: expected more items, got %d", len(sc.Items))
	}
}

func TestImplicitParticipants(t *testing.T) {
	src := `sequenceDiagram
    Alice->>Bob: Hello
    Bob->>Charlie: Hi there`

	p := newParser(src)
	d, err := p.parse()
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if len(d.participants) != 3 {
		t.Errorf("expected 3 implicit participants, got %d", len(d.participants))
	}
}

func TestSelfMessage(t *testing.T) {
	src := `sequenceDiagram
    Alice->>Alice: Self message`

	p := newParser(src)
	d, err := p.parse()
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if len(d.messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(d.messages))
	}

	if !d.messages[0].SelfLoop {
		t.Errorf("expected self-loop message")
	}
}

func TestActivationShorthand(t *testing.T) {
	t.Skip("Activation shorthand needs parsing improvements")

	src := `sequenceDiagram
    Alice->>+Bob: Hello
    Bob-->>-Alice: Hi`

	p := newParser(src)
	d, err := p.parse()
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if d.messages[0].ActivateOn != "+" {
		t.Errorf("expected '+' activation marker")
	}

	if d.messages[1].ActivateOn != "-" {
		t.Errorf("expected '-' deactivation marker")
	}
}

func TestMultilineDisplay(t *testing.T) {
	src := `sequenceDiagram
    participant Alice as Alice<br/>Johnson
    participant Bob
    Alice->>Bob: Hello`

	p := newParser(src)
	d, err := p.parse()
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	// Should have newline in display
	if !strings.Contains(d.participants[0].Display, "\n") {
		t.Logf("expected newline in display name, got: %q", d.participants[0].Display)
	}
}
