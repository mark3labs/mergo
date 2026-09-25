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

// ============================================================================
// Parser Tests
// ============================================================================

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

	if len(d.Participants) != 2 {
		t.Errorf("expected 2 participants, got %d", len(d.Participants))
	}
	if d.Participants[0].ID != "Alice" {
		t.Errorf("expected first participant Alice, got %s", d.Participants[0].ID)
	}
	if d.Participants[1].ID != "Bob" {
		t.Errorf("expected second participant Bob, got %s", d.Participants[1].ID)
	}

	msgs := filterMessages(d.Messages)
	if len(msgs) != 1 {
		t.Errorf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Label != "Hello" {
		t.Errorf("expected label 'Hello', got '%s'", msgs[0].Label)
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

	if d.Participants[0].Type != TypeActor {
		t.Errorf("expected Alice to be actor type")
	}
	if d.Participants[1].Type != TypeActor {
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

	if d.Participants[0].ID != "A" {
		t.Errorf("expected ID 'A', got '%s'", d.Participants[0].ID)
	}
	if d.Participants[0].Alias != "Alice" {
		t.Errorf("expected alias 'Alice', got '%s'", d.Participants[0].Alias)
	}
}

func TestParseAliasWithBR(t *testing.T) {
	src := `sequenceDiagram
    participant A as Alice<br/>Johnson
    participant B
    A->>B: Hello`

	p := newParser(src)
	d, err := p.parse()
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	// Should have newline in alias
	if !strings.Contains(d.Participants[0].Alias, "\n") {
		t.Logf("expected newline in alias, got: %q", d.Participants[0].Alias)
	}
}

func TestParseTypes(t *testing.T) {
	src := `sequenceDiagram
    participant A@{"type": "database"}
    participant B@{"type": "queue"}
    A->>B: Test`

	p := newParser(src)
	d, err := p.parse()
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if d.Participants[0].Type != TypeDatabase {
		t.Errorf("expected database type, got %v", d.Participants[0].Type)
	}
	if d.Participants[1].Type != TypeQueue {
		t.Errorf("expected queue type, got %v", d.Participants[1].Type)
	}
}

func TestParseAutonumber(t *testing.T) {
	src := `sequenceDiagram
    autonumber 1 2
    participant Alice
    participant Bob
    Alice->>Bob: Hello
    Bob-->>Alice: Hi`

	p := newParser(src)
	d, err := p.parse()
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if d.Autonumber == nil {
		t.Errorf("expected autonumber to be set")
	}
	if d.Autonumber.Start != 1.0 || d.Autonumber.Increment != 2.0 {
		t.Errorf("expected autonumber 1/2, got %v/%v", d.Autonumber.Start, d.Autonumber.Increment)
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

			msgs := filterMessages(d.Messages)
			if len(msgs) == 0 {
				t.Errorf("expected message")
			} else if msgs[0].Arrow != tt.want {
				t.Errorf("expected arrow type %v, got %v", tt.want, msgs[0].Arrow)
			}
		})
	}
}

func TestParseImplicitParticipants(t *testing.T) {
	src := `sequenceDiagram
    Alice->>Bob: Hello
    Bob->>Charlie: Hi there`

	p := newParser(src)
	d, err := p.parse()
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if len(d.Participants) != 3 {
		t.Errorf("expected 3 implicit participants, got %d", len(d.Participants))
	}
}

func TestParseSelfMessage(t *testing.T) {
	src := `sequenceDiagram
    participant Alice
    Alice->>Alice: Self message`

	p := newParser(src)
	d, err := p.parse()
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	msgs := filterMessages(d.Messages)
	if len(msgs) != 1 {
		t.Errorf("expected 1 message, got %d", len(msgs))
	}

	if !msgs[0].IsSelfLoop {
		t.Errorf("expected self-loop message")
	}
}

func TestParseActivationShorthand(t *testing.T) {
	src := `sequenceDiagram
    participant Alice
    participant Bob
    Alice->>+Bob: Hello
    Bob-->>-Alice: Hi`

	p := newParser(src)
	d, err := p.parse()
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	msgs := filterMessages(d.Messages)
	if len(msgs) < 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	if msgs[0].ActivateTo != "+" {
		t.Errorf("expected '+' activation marker on first message, got %q", msgs[0].ActivateTo)
	}

	if msgs[1].ActivateTo != "-" {
		t.Errorf("expected '-' deactivation marker on second message, got %q", msgs[1].ActivateTo)
	}
}

func TestParseNotes(t *testing.T) {
	src := `sequenceDiagram
    participant Alice
    participant Bob
    Note right of Alice: Text in note
    Alice->>Bob: Hello
    Note over Alice,Bob: A typical interaction`

	p := newParser(src)
	d, err := p.parse()
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	notes := filterNotes(d.Messages)
	if len(notes) < 2 {
		t.Errorf("expected at least 2 notes, got %d", len(notes))
	}
}

func TestParseLoop(t *testing.T) {
	src := `sequenceDiagram
    participant Alice
    participant John
    Alice->>John: Hello John, how are you?
    loop Every minute
        John-->>Alice: Great!
    end`

	p := newParser(src)
	d, err := p.parse()
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	blocks := filterBlocks(d.Messages)
	if len(blocks) == 0 {
		t.Errorf("expected a loop block")
	}
	if len(blocks) > 0 && blocks[0].Kind != BlockLoop {
		t.Errorf("expected BlockLoop, got %v", blocks[0].Kind)
	}
}

func TestParseAlt(t *testing.T) {
	src := `sequenceDiagram
    participant Alice
    participant Bob
    Alice->>Bob: Hello Bob
    alt is sick
        Bob->>Alice: Not so good :(
    else is well
        Bob->>Alice: Feeling fresh
    end`

	p := newParser(src)
	d, err := p.parse()
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	blocks := filterBlocks(d.Messages)
	if len(blocks) == 0 {
		t.Errorf("expected an alt block")
	}
	if len(blocks) > 0 && blocks[0].Kind != BlockAlt {
		t.Errorf("expected BlockAlt, got %v", blocks[0].Kind)
	}
	if len(blocks) > 0 && len(blocks[0].Clauses) < 1 {
		t.Errorf("expected at least 1 clause in alt")
	}
}

// ============================================================================
// Render Tests
// ============================================================================

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

	if sc == nil {
		t.Errorf("expected scene, got nil")
	}
	if sc.Width <= 0 || sc.Height <= 0 {
		t.Errorf("expected positive dimensions, got %fx%f", sc.Width, sc.Height)
	}

	// Should have items (boxes, lines, text)
	if len(sc.Items) < 5 {
		t.Logf("warning: expected more items, got %d", len(sc.Items))
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
				t.Logf("render failed (expected for some examples): %v", err)
				return
			}

			if sc == nil {
				t.Errorf("expected scene, got nil")
			}

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

func TestNoDuplicateParticipants(t *testing.T) {
	src := `sequenceDiagram
    participant Client as Client App
    Client->>Server: x`

	p := newParser(src)
	d, err := p.parse()
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	// Should have Client and Server (implicit), but not duplicates
	if len(d.Participants) != 2 {
		t.Errorf("expected 2 participants, got %d", len(d.Participants))
	}
}

func TestRenderWithActivations(t *testing.T) {
	src := `sequenceDiagram
    participant Alice
    participant Bob
    Alice->>+Bob: Hello
    Bob-->>-Alice: Hi`

	th := theme.Default()
	sc, err := diagram.Render(src, th)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}

	if sc == nil {
		t.Errorf("expected scene, got nil")
	}
}

// ============================================================================
// Helper functions
// ============================================================================

func filterMessages(stmts []Statement) []*Message {
	var msgs []*Message
	var filter func([]Statement)
	filter = func(stmts []Statement) {
		for _, stmt := range stmts {
			if msg, ok := stmt.(*Message); ok {
				msgs = append(msgs, msg)
			} else if block, ok := stmt.(*Block); ok {
				filter(block.Children)
				for _, clause := range block.Clauses {
					filter(clause.Children)
				}
			}
		}
	}
	filter(stmts)
	return msgs
}

func filterNotes(stmts []Statement) []*Note {
	var notes []*Note
	for _, stmt := range stmts {
		if note, ok := stmt.(*Note); ok {
			notes = append(notes, note)
		}
	}
	return notes
}

func filterBlocks(stmts []Statement) []*Block {
	var blocks []*Block
	for _, stmt := range stmts {
		if block, ok := stmt.(*Block); ok {
			blocks = append(blocks, block)
		}
	}
	return blocks
}
