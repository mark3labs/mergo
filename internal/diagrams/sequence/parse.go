package sequence

import (
	"fmt"
	"image/color"
	"regexp"
	"strconv"
	"strings"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/theme"
)

// Kind is a participant kind.
type Kind string

const (
	KindParticipant Kind = "participant"
	KindActor       Kind = "actor"
	KindBoundary    Kind = "boundary"
	KindControl     Kind = "control"
	KindEntity      Kind = "entity"
	KindDatabase    Kind = "database"
	KindCollections Kind = "collections"
	KindQueue       Kind = "queue"
)

// Participant is a lifeline owner.
type Participant struct {
	ID    string
	Label string
	Kind  Kind
	Box   *Box
	index int
	// created by a `create` statement: the box is drawn at the message
	// that creates it.
	created bool
	// destroyed by a `destroy` statement.
	destroyed bool
}

// Box groups participants.
type Box struct {
	Title   string
	Color   color.RGBA
	Members []*Participant
}

// LineStyle of a message.
type LineStyle int

const (
	Solid LineStyle = iota
	Dotted
)

// Head is the arrow head type of a message.
type Head int

const (
	HeadNone Head = iota
	HeadFilled
	HeadCross
	HeadOpen
)

// EventKind enumerates timeline events.
type EventKind int

const (
	EvMessage EventKind = iota
	EvNote
	EvActivate
	EvDeactivate
	EvBlockStart
	EvBlockSep
	EvBlockEnd
	EvRectStart
	EvRectEnd
)

// NotePos is the placement of a note.
type NotePos int

const (
	NoteLeft NotePos = iota
	NoteRight
	NoteOver
)

// Event is one entry of the sequence timeline.
type Event struct {
	Kind EventKind
	Line int

	// messages
	From, To       *Participant
	Text           string
	Style          LineStyle
	Head           Head
	Bidirectional  bool
	ActivateTo     bool
	DeactivateFrom bool
	Number         int  // autonumber (0 = none)
	Creates        bool // message creates To
	Destroys       *Participant

	// notes
	Pos   NotePos
	Parts []*Participant

	// activation
	Target *Participant

	// blocks
	Block string // loop, alt, opt, par, critical, break
	Label string
	Color color.RGBA
}

// Diagram is a parsed sequence diagram.
type Diagram struct {
	Title        string
	Participants []*Participant
	Boxes        []*Box
	Events       []*Event
	byID         map[string]*Participant
}

var (
	// order matters: longest arrows first
	arrowTokens = []string{"<<-->>", "<<->>", "-->>", "->>", "--x", "-x", "--)", "-)", "-->", "->"}
	partTypeRe  = regexp.MustCompile(`^([^@\s]+)\s*@\{(.*?)\}\s*(?:as\s+(.+))?$`)
	kvRe        = regexp.MustCompile(`["']?(\w+)["']?\s*:\s*("[^"]*"|'[^']*'|[^,}]+)`)
)

type parser struct {
	d           *Diagram
	stack       []string // open blocks: loop/alt/.../rect/box
	curBox      *Box
	autonum     bool
	num, step   int
	pendCreate  *Participant
	pendDestroy []*Participant
}

// Parse parses sequence diagram source.
func Parse(src string) (*Diagram, error) {
	d := &Diagram{byID: map[string]*Participant{}}
	p := &parser{d: d, step: 1, num: 1}
	lines := strings.Split(src, "\n")
	header := false
	for i, raw := range lines {
		ln := i + 1
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		// strip trailing ';'
		line = strings.TrimSpace(strings.TrimSuffix(line, ";"))
		if !header {
			if !strings.HasPrefix(line, "sequenceDiagram") {
				return nil, fmt.Errorf("line %d: expected 'sequenceDiagram'", ln)
			}
			header = true
			continue
		}
		if err := p.statement(line, ln); err != nil {
			return nil, err
		}
	}
	if len(p.stack) > 0 {
		return nil, fmt.Errorf("unclosed %q block (missing 'end')", p.stack[len(p.stack)-1])
	}
	return d, nil
}

func (p *parser) participant(id string) *Participant {
	id = strings.TrimSpace(id)
	if pp, ok := p.d.byID[id]; ok {
		return pp
	}
	pp := &Participant{ID: id, Label: id, Kind: KindParticipant, index: len(p.d.Participants)}
	p.d.byID[id] = pp
	p.d.Participants = append(p.d.Participants, pp)
	if p.curBox != nil {
		pp.Box = p.curBox
		p.curBox.Members = append(p.curBox.Members, pp)
	}
	return pp
}

func lower(s string) string { return strings.ToLower(s) }

func cutWord(s string) (string, string) {
	s = strings.TrimSpace(s)
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimSpace(s[i+1:])
}

func (p *parser) statement(line string, ln int) error {
	kw, rest := cutWord(line)
	lkw := lower(kw)
	switch {
	case lkw == "participant" || lkw == "actor":
		return p.declare(lkw, rest, ln, false)
	case lkw == "create":
		k, r := cutWord(rest)
		if lower(k) != "participant" && lower(k) != "actor" {
			return fmt.Errorf("line %d: expected 'create participant' or 'create actor'", ln)
		}
		return p.declare(lower(k), r, ln, true)
	case lkw == "destroy":
		if rest == "" {
			return fmt.Errorf("line %d: destroy needs a participant", ln)
		}
		pp := p.participant(rest)
		pp.destroyed = true
		p.pendDestroy = append(p.pendDestroy, pp)
		return nil
	case lkw == "title" || strings.HasPrefix(line, "title:"):
		t := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "title"), ":"))
		p.d.Title = diagram.CleanLabel(t)
		return nil
	case lkw == "autonumber":
		f := strings.Fields(rest)
		if len(f) > 0 && lower(f[0]) == "off" {
			p.autonum = false
			return nil
		}
		p.autonum = true
		if len(f) > 0 {
			if v, err := strconv.Atoi(f[0]); err == nil {
				p.num = v
			}
		}
		if len(f) > 1 {
			if v, err := strconv.Atoi(f[1]); err == nil {
				p.step = v
			}
		}
		return nil
	case lkw == "activate" || lkw == "deactivate":
		if rest == "" {
			return fmt.Errorf("line %d: %s needs a participant", ln, lkw)
		}
		k := EvActivate
		if lkw == "deactivate" {
			k = EvDeactivate
		}
		p.d.Events = append(p.d.Events, &Event{Kind: k, Line: ln, Target: p.participant(rest)})
		return nil
	case lkw == "note":
		return p.note(rest, ln)
	case lkw == "loop" || lkw == "alt" || lkw == "opt" || lkw == "par" || lkw == "critical" || lkw == "break" || lkw == "par_over":
		b := lkw
		if b == "par_over" {
			b = "par"
		}
		p.stack = append(p.stack, b)
		p.d.Events = append(p.d.Events, &Event{Kind: EvBlockStart, Line: ln, Block: b, Label: diagram.CleanLabel(rest)})
		return nil
	case lkw == "else" || lkw == "and" || lkw == "option":
		want := map[string]string{"else": "alt", "and": "par", "option": "critical"}[lkw]
		if len(p.stack) == 0 || p.stack[len(p.stack)-1] != want {
			return fmt.Errorf("line %d: '%s' outside of a '%s' block", ln, lkw, want)
		}
		p.d.Events = append(p.d.Events, &Event{Kind: EvBlockSep, Line: ln, Block: lkw, Label: diagram.CleanLabel(rest)})
		return nil
	case lkw == "rect":
		c, err := theme.ParseColor(rest)
		if err != nil {
			c = theme.RGBA(200, 200, 200, 80)
		}
		p.stack = append(p.stack, "rect")
		p.d.Events = append(p.d.Events, &Event{Kind: EvRectStart, Line: ln, Color: c})
		return nil
	case lkw == "box":
		if p.curBox != nil {
			return fmt.Errorf("line %d: boxes cannot be nested", ln)
		}
		b := parseBox(rest)
		p.d.Boxes = append(p.d.Boxes, b)
		p.curBox = b
		p.stack = append(p.stack, "box")
		return nil
	case lkw == "end":
		if len(p.stack) == 0 {
			return fmt.Errorf("line %d: 'end' without an open block", ln)
		}
		top := p.stack[len(p.stack)-1]
		p.stack = p.stack[:len(p.stack)-1]
		switch top {
		case "box":
			p.curBox = nil
		case "rect":
			p.d.Events = append(p.d.Events, &Event{Kind: EvRectEnd, Line: ln})
		default:
			p.d.Events = append(p.d.Events, &Event{Kind: EvBlockEnd, Line: ln, Block: top})
		}
		return nil
	case lkw == "links" || lkw == "link" || lkw == "properties" || lkw == "details" || lkw == "accTitle" || lkw == "accDescr":
		return nil
	}
	return p.message(line, ln)
}

func parseBox(rest string) *Box {
	b := &Box{}
	rest = strings.TrimSpace(rest)
	lr := lower(rest)
	for _, fn := range []string{"rgb(", "rgba(", "hsl(", "hsla("} {
		if strings.HasPrefix(lr, fn) {
			if i := strings.IndexByte(rest, ')'); i > 0 {
				if c, err := theme.ParseColor(rest[:i+1]); err == nil {
					b.Color = c
				}
				b.Title = diagram.CleanLabel(rest[i+1:])
				return b
			}
		}
	}
	w, r := cutWord(rest)
	if lower(w) == "transparent" {
		b.Title = diagram.CleanLabel(r)
		return b
	}
	if c, err := theme.ParseColor(w); err == nil && w != "" && !strings.HasPrefix(w, "\"") {
		b.Color = c
		b.Title = diagram.CleanLabel(r)
		return b
	}
	b.Title = diagram.CleanLabel(rest)
	return b
}

func (p *parser) declare(kind, rest string, ln int, create bool) error {
	if rest == "" {
		return fmt.Errorf("line %d: %s needs a name", ln, kind)
	}
	k := KindParticipant
	if kind == "actor" {
		k = KindActor
	}
	id, label := rest, ""
	var props map[string]string
	if m := partTypeRe.FindStringSubmatch(rest); m != nil {
		id = m[1]
		label = m[3]
		props = map[string]string{}
		for _, kv := range kvRe.FindAllStringSubmatch(m[2], -1) {
			v := strings.TrimSpace(kv[2])
			v = strings.Trim(v, `"'`)
			props[lower(kv[1])] = v
		}
	} else if before, after, ok := strings.Cut(rest, " as "); ok {
		id = strings.TrimSpace(before)
		label = after
	}
	pp := p.participant(id)
	pp.Kind = k
	if props != nil {
		switch Kind(lower(props["type"])) {
		case KindBoundary, KindControl, KindEntity, KindDatabase, KindCollections, KindQueue, KindActor, KindParticipant:
			pp.Kind = Kind(lower(props["type"]))
		}
		if a := props["alias"]; a != "" {
			label = a
		}
	}
	if label != "" {
		pp.Label = diagram.CleanLabel(label)
	} else {
		pp.Label = diagram.CleanLabel(id)
	}
	if create {
		pp.created = true
		p.pendCreate = pp
	}
	return nil
}

func (p *parser) note(rest string, ln int) error {
	lr := lower(rest)
	var pos NotePos
	switch {
	case strings.HasPrefix(lr, "left of "):
		pos, rest = NoteLeft, rest[8:]
	case strings.HasPrefix(lr, "right of "):
		pos, rest = NoteRight, rest[9:]
	case strings.HasPrefix(lr, "over "):
		pos, rest = NoteOver, rest[5:]
	default:
		return fmt.Errorf("line %d: note must be 'left of', 'right of' or 'over'", ln)
	}
	who, text, ok := strings.Cut(rest, ":")
	if !ok {
		return fmt.Errorf("line %d: note needs ': text'", ln)
	}
	var parts []*Participant
	for id := range strings.SplitSeq(who, ",") {
		id = strings.TrimSpace(id)
		if id != "" {
			parts = append(parts, p.participant(id))
		}
	}
	if len(parts) == 0 {
		return fmt.Errorf("line %d: note needs a participant", ln)
	}
	p.d.Events = append(p.d.Events, &Event{Kind: EvNote, Line: ln, Pos: pos, Parts: parts, Text: diagram.CleanLabel(text)})
	return nil
}

func (p *parser) message(line string, ln int) error {
	// find the first arrow token
	best, bestTok := -1, ""
	for _, tok := range arrowTokens {
		if i := strings.Index(line, tok); i > 0 && (best < 0 || i < best || (i == best && len(tok) > len(bestTok))) {
			best, bestTok = i, tok
		}
	}
	if best < 0 {
		return fmt.Errorf("line %d: unrecognized statement %q", ln, truncate(line, 40))
	}
	from := strings.TrimSpace(line[:best])
	rest := line[best+len(bestTok):]
	ev := &Event{Kind: EvMessage, Line: ln}
	rest = strings.TrimLeft(rest, " \t")
	if strings.HasPrefix(rest, "+") {
		ev.ActivateTo = true
		rest = rest[1:]
	} else if strings.HasPrefix(rest, "-") {
		ev.DeactivateFrom = true
		rest = rest[1:]
	}
	to, text, _ := strings.Cut(rest, ":")
	to = strings.TrimSpace(to)
	if from == "" || to == "" {
		return fmt.Errorf("line %d: message needs a sender and a receiver", ln)
	}
	switch bestTok {
	case "->":
		ev.Style, ev.Head = Solid, HeadNone
	case "-->":
		ev.Style, ev.Head = Dotted, HeadNone
	case "->>":
		ev.Style, ev.Head = Solid, HeadFilled
	case "-->>":
		ev.Style, ev.Head = Dotted, HeadFilled
	case "-x":
		ev.Style, ev.Head = Solid, HeadCross
	case "--x":
		ev.Style, ev.Head = Dotted, HeadCross
	case "-)":
		ev.Style, ev.Head = Solid, HeadOpen
	case "--)":
		ev.Style, ev.Head = Dotted, HeadOpen
	case "<<->>":
		ev.Style, ev.Head, ev.Bidirectional = Solid, HeadFilled, true
	case "<<-->>":
		ev.Style, ev.Head, ev.Bidirectional = Dotted, HeadFilled, true
	}
	ev.From = p.participant(from)
	ev.To = p.participant(to)
	ev.Text = diagram.CleanLabel(text)
	if p.autonum {
		ev.Number = p.num
		p.num += p.step
	}
	if p.pendCreate != nil && (p.pendCreate == ev.To || p.pendCreate == ev.From) {
		ev.Creates = p.pendCreate == ev.To
		p.pendCreate = nil
	}
	for i, d := range p.pendDestroy {
		if d == ev.From || d == ev.To {
			ev.Destroys = d
			p.pendDestroy = append(p.pendDestroy[:i], p.pendDestroy[i+1:]...)
			break
		}
	}
	p.d.Events = append(p.d.Events, ev)
	return nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
