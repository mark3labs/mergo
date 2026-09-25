package sequence

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
	d, err := Parse("sequenceDiagram\n" + src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return d
}

func TestParticipantsAndAliases(t *testing.T) {
	d := parse(t, `participant Client as Client App
    actor U as User<br/>Person
    participant DB@{ "type": "database", "alias": "Main DB" }
    participant Q@{ 'type': 'queue' }
    Client->>Server: hi
    Server-->>Client: yo
    U->>DB: q`)
	if len(d.Participants) != 5 {
		t.Fatalf("participants = %d: %+v", len(d.Participants), d.Participants)
	}
	want := []struct {
		id, label string
		kind      Kind
	}{
		{"Client", "Client App", KindParticipant},
		{"U", "User\nPerson", KindActor},
		{"DB", "Main DB", KindDatabase},
		{"Q", "Q", KindQueue},
		{"Server", "Server", KindParticipant},
	}
	for i, w := range want {
		p := d.Participants[i]
		if p.ID != w.id || p.Label != w.label || p.Kind != w.kind {
			t.Errorf("participant %d = %+v, want %+v", i, p, w)
		}
	}
}

func TestArrows(t *testing.T) {
	cases := map[string]struct {
		style LineStyle
		head  Head
		bidi  bool
	}{
		"A->B: x":     {Solid, HeadNone, false},
		"A-->B: x":    {Dotted, HeadNone, false},
		"A->>B: x":    {Solid, HeadFilled, false},
		"A-->>B: x":   {Dotted, HeadFilled, false},
		"A-xB: x":     {Solid, HeadCross, false},
		"A--xB: x":    {Dotted, HeadCross, false},
		"A-)B: x":     {Solid, HeadOpen, false},
		"A--)B: x":    {Dotted, HeadOpen, false},
		"A<<->>B: x":  {Solid, HeadFilled, true},
		"A<<-->>B: x": {Dotted, HeadFilled, true},
	}
	for src, w := range cases {
		d := parse(t, src)
		ev := d.Events[0]
		if ev.Style != w.style || ev.Head != w.head || ev.Bidirectional != w.bidi || ev.From.ID != "A" || ev.To.ID != "B" || ev.Text != "x" {
			t.Errorf("%s: got %+v", src, ev)
		}
	}
	d := parse(t, "Web Server->>+DB Node: query\nDB Node-->>-Web Server: rows")
	if d.Events[0].From.ID != "Web Server" || d.Events[0].To.ID != "DB Node" || !d.Events[0].ActivateTo {
		t.Errorf("spaces/activation: %+v", d.Events[0])
	}
	if !d.Events[1].DeactivateFrom {
		t.Error("deactivate shorthand")
	}
}

func TestBlocksNotesAndNesting(t *testing.T) {
	d := parse(t, `loop Every minute
        A->>B: ping
        alt ok
            B-->>A: pong
            Note right of B: fine
        else failed
            rect rgb(200, 150, 255)
                B-->>A: error
            end
        end
    end
    par One
        A->>B: x
    and Two
        A->>C: y
    end
    critical Connect
        A->>DB: c
    option Timeout
        A->>A: retry
    end
    break stop
        A->>B: bye
    end
    opt maybe
        Note over A,B: shared
    end`)
	var kinds []string
	for _, e := range d.Events {
		switch e.Kind {
		case EvBlockStart:
			kinds = append(kinds, e.Block)
		case EvBlockSep:
			kinds = append(kinds, "|"+e.Block)
		case EvBlockEnd:
			kinds = append(kinds, "end")
		case EvRectStart:
			kinds = append(kinds, "rect")
		case EvRectEnd:
			kinds = append(kinds, "/rect")
		case EvNote:
			kinds = append(kinds, "note")
		}
	}
	got := strings.Join(kinds, " ")
	want := "loop alt note |else rect /rect end end par |and end critical |option end break end opt note end"
	if got != want {
		t.Errorf("events:\n got %s\nwant %s", got, want)
	}
	for _, bad := range []string{"end", "loop x\nA->>B: y", "else x", "alt a\nand b\nend"} {
		if _, err := Parse("sequenceDiagram\n" + bad); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}

func TestAutonumberCreateDestroyBox(t *testing.T) {
	d := parse(t, `autonumber 10 5
    box Aqua Front
        participant A
        participant B
    end
    box rgb(33,66,99) Back end
        participant C
    end
    A->>B: one
    create participant D
    B->>D: two
    destroy D
    D-->>B: three
    autonumber off
    A->>B: four`)
	var nums []int
	for _, e := range d.Events {
		if e.Kind == EvMessage {
			nums = append(nums, e.Number)
		}
	}
	if len(nums) != 4 || nums[0] != 10 || nums[1] != 15 || nums[2] != 20 || nums[3] != 0 {
		t.Errorf("numbers %v", nums)
	}
	if len(d.Boxes) != 2 || d.Boxes[0].Title != "Front" || len(d.Boxes[0].Members) != 2 || d.Boxes[1].Title != "Back end" {
		t.Errorf("boxes %+v", d.Boxes)
	}
	if d.Boxes[0].Color.A == 0 || d.Boxes[1].Color.B != 99 {
		t.Errorf("box colors %+v %+v", d.Boxes[0].Color, d.Boxes[1].Color)
	}
	if !d.Events[1].Creates {
		t.Error("create not attached to message")
	}
	if d.Events[2].Destroys == nil || d.Events[2].Destroys.ID != "D" {
		t.Error("destroy not attached to message")
	}
}

func TestErrorsHaveLines(t *testing.T) {
	_, err := Parse("sequenceDiagram\nA->>B: ok\nthis is nonsense")
	if err == nil || !strings.Contains(err.Error(), "line 3") {
		t.Errorf("err = %v", err)
	}
}

func exampleFiles(t *testing.T) []string {
	files, _ := filepath.Glob("../../../examples/sequence/*.mmd")
	if len(files) == 0 {
		t.Fatal("no examples")
	}
	return files
}

func TestNeverPanics(t *testing.T) {
	for _, f := range exampleFiles(t) {
		b, _ := os.ReadFile(f)
		s := string(b)
		for i := 0; i <= len(s); i += 2 {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("%s prefix %d: panic %v", f, i, r)
					}
				}()
				doc, err := diagram.Preprocess(s[:i])
				if err == nil && doc.Type == "sequence" {
					_, _ = Render(doc.Source, &diagram.Config{Theme: theme.Default()})
				}
			}()
		}
	}
}

func overlap(a, b scene.Rect) bool {
	return a.X < b.X+b.W-0.5 && b.X < a.X+a.W-0.5 && a.Y < b.Y+b.H-0.5 && b.Y < a.Y+a.H-0.5
}

func TestExamplesGeometry(t *testing.T) {
	for _, f := range exampleFiles(t) {
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
		r.layoutX()
		r.layoutY()
		// header boxes don't overlap
		for i, p := range d.Participants {
			for _, q := range d.Participants[i+1:] {
				a, c := r.ps[p], r.ps[q]
				if p.created || q.created {
					continue
				}
				if a.x+a.w/2 > c.x-c.w/2+0.5 {
					t.Errorf("%s: participants %s and %s overlap", f, p.ID, q.ID)
				}
			}
		}
		// message labels don't overlap each other
		var labels []scene.Rect
		for _, m := range r.msgs {
			if m.ev.Text == "" {
				continue
			}
			w, h := scene.MeasureBlock(m.ev.Text, r.font, 0)
			x := (m.x1+m.x2)/2 - w/2
			if m.self {
				x = m.x1 + 8
			}
			labels = append(labels, scene.Rect{X: x, Y: m.y - 5 - h, W: w, H: h})
		}
		for i := range labels {
			for j := i + 1; j < len(labels); j++ {
				if overlap(labels[i], labels[j]) {
					t.Errorf("%s: message labels %d and %d overlap", f, i, j)
				}
			}
		}
		// frames contain their messages vertically
		for _, fr := range r.frames {
			if fr.bottom <= fr.top {
				t.Errorf("%s: bad frame %+v", f, fr)
			}
		}
		sc, err := diagram.Render(string(b), theme.Default())
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if testing.Verbose() {
			t.Logf("%s\n%s", f, devutil.ASCII(sc.Render(scene.RenderOptions{Scale: 1}), 130))
		}
	}
}

func TestEntityAtLineEnd(t *testing.T) {
	// the ';' closing an entity is not a statement terminator
	d := parse(t, "A->>B: #quot;ok#quot;\nA->>B: fish &amp; chips;\nA->>B: plain;")
	for i, want := range []string{`"ok"`, "fish & chips", "plain"} {
		if got := d.Events[i].Text; got != want {
			t.Errorf("event %d text %q, want %q", i, got, want)
		}
	}
}
