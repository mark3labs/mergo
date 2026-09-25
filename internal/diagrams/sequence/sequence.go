package sequence

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/scene"
)

func init() {
	diagram.Register(diagram.Type{
		Name:   "sequence",
		Detect: diagram.Keyword("sequenceDiagram"),
		Render: Render,
	})
}

// Render parses and renders a sequence diagram.
func Render(src string, cfg *diagram.Config) (*scene.Scene, error) {
	th := cfg.Theme
	sc := diagram.NewScene(th)

	parser := newParser(src)
	d, err := parser.parse()
	if err != nil {
		return nil, err
	}

	renderer := newRenderer(d, sc, th, cfg)
	renderer.render()

	return diagram.Finish(sc, th, cfg.Title), nil
}

// Diagram holds the parsed sequence diagram AST.
type Diagram struct {
	participants []*Participant
	messages     []Message
	autonumber   *Autonumber
	title        string
	boxes        []*Box
}

// Participant in a sequence diagram.
type Participant struct {
	ID      string
	Display string // what to show (may include newlines)
	Type    ParticipantType
	Order   int // order of declaration
	Box     *Box
}

// ParticipantType determines the visual symbol.
type ParticipantType int

const (
	TypeParticipant ParticipantType = iota
	TypeActor
	TypeBoundary
	TypeControl
	TypeEntity
	TypeDatabase
	TypeQueue
	TypeCollections
)

// Message is a communication between participants.
type Message struct {
	FromID     string // participant ID
	ToID       string // participant ID
	From       *Participant
	To         *Participant
	Label      string
	Arrow      ArrowType
	LineNo     int
	ActivateOn string // "+" for activate recipient, "-" for deactivate
	SelfLoop   bool
}

// ArrowType determines arrow style and markers.
type ArrowType int

const (
	ArrowSolid ArrowType = iota
	ArrowDotted
	ArrowFilledHead
	ArrowDottedFilledHead
	ArrowBidirectional
	ArrowBidirectionalDotted
	ArrowCross
	ArrowCrossDotted
	ArrowAsync
	ArrowAsyncDotted
)

// Autonumber configures auto-numbering of messages.
type Autonumber struct {
	Start     float64
	Increment float64
}

// Box groups participants.
type Box struct {
	Title        string
	Color        string
	Participants []*Participant
}

// Parser parses sequence diagram source.
type Parser struct {
	src   string
	lines []string
	pos   int
}

func newParser(src string) *Parser {
	var lines []string
	for l := range strings.SplitSeq(src, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	return &Parser{src: src, lines: lines}
}

func (p *Parser) parse() (*Diagram, error) {
	d := &Diagram{
		participants: []*Participant{},
		messages:     []Message{},
	}

	for p.pos = 0; p.pos < len(p.lines); p.pos++ {
		line := p.lines[p.pos]

		// Skip first line (sequenceDiagram header)
		if strings.HasPrefix(strings.ToLower(line), "sequencediagram") {
			continue
		}

		if err := p.parseLine(line, d); err != nil {
			return nil, err
		}
	}

	// Fill in implicit participants and validate
	d.collectImplicitParticipants()
	if err := d.validateAndNormalize(); err != nil {
		return nil, err
	}

	return d, nil
}

func (p *Parser) parseLine(line string, d *Diagram) error {
	switch {
	case strings.HasPrefix(line, "participant "):
		return p.parseParticipant(line, d, false)
	case strings.HasPrefix(line, "actor "):
		return p.parseParticipant(line, d, true)
	case strings.HasPrefix(line, "autonumber"):
		return p.parseAutonumber(line, d)
	case strings.HasPrefix(line, "title "):
		d.title = parseStringArg(line[6:])
		return nil
	case strings.HasPrefix(line, "Note "):
		// Ignore notes for now - would need full parsing
		return nil
	case strings.HasPrefix(line, "loop "), strings.HasPrefix(line, "alt "),
		strings.HasPrefix(line, "opt "), strings.HasPrefix(line, "par "),
		strings.HasPrefix(line, "critical "), strings.HasPrefix(line, "break "),
		strings.HasPrefix(line, "rect "):
		// Ignore control frames for now - would need nesting
		return nil
	case strings.HasPrefix(line, "end"):
		// Ignore frame ending
		return nil
	case strings.HasPrefix(line, "activate "):
		// Ignore for now - handle via arrow syntax
		return nil
	case strings.HasPrefix(line, "deactivate "):
		// Ignore for now - handle via arrow syntax
		return nil
	case strings.HasPrefix(line, "destroy "):
		// Ignore destroy directives for now
		return nil
	case strings.HasPrefix(line, "create "):
		// Ignore create directives for now
		return nil
	case strings.HasPrefix(line, "box "):
		return p.parseBox(line, d)
	case strings.HasPrefix(line, "link "), strings.HasPrefix(line, "links "),
		strings.HasPrefix(line, "properties "):
		// Ignore these
		return nil
	default:
		// Try to parse as message - has arrow
		return p.parseMessage(line, d)
	}
}

func (p *Parser) parseParticipant(line string, d *Diagram, isActor bool) error {
	// Format: participant [ID] [as Display] [@{ "type": "..." }]
	keyword := "participant"
	if isActor {
		keyword = "actor"
	}

	rest := line[len(keyword):]
	rest = strings.TrimSpace(rest)

	// Parse optional JSON config
	var ptype ParticipantType = TypeParticipant
	if isActor {
		ptype = TypeActor
	}
	var inlineAlias string

	if i := strings.Index(rest, "@{"); i >= 0 {
		end := strings.Index(rest[i:], "}")
		if end >= 0 {
			jsonStr := rest[i+2 : i+end]
			var cfg map[string]any
			if err := json.Unmarshal([]byte(jsonStr), &cfg); err == nil {
				if typeStr, ok := cfg["type"].(string); ok {
					ptype = parseParticipantType(typeStr)
				}
				if alias, ok := cfg["alias"].(string); ok {
					inlineAlias = alias
				}
			}
		}
		rest = rest[:i] + rest[i+end+1:]
		rest = strings.TrimSpace(rest)
	}

	// Parse ID and alias
	var id, display string
	if idx := strings.Index(rest, " as "); idx >= 0 {
		id = strings.TrimSpace(rest[:idx])
		display = strings.TrimSpace(rest[idx+4:])
	} else {
		id = rest
		display = inlineAlias
		if display == "" {
			display = id
		}
	}

	// Clean up display name (handle multiline with <br>)
	display = diagram.CleanLabel(display)

	participant := &Participant{
		ID:      id,
		Display: display,
		Type:    ptype,
		Order:   len(d.participants),
	}
	d.participants = append(d.participants, participant)

	return nil
}

func parseParticipantType(t string) ParticipantType {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "actor":
		return TypeActor
	case "boundary":
		return TypeBoundary
	case "control":
		return TypeControl
	case "entity":
		return TypeEntity
	case "database":
		return TypeDatabase
	case "queue":
		return TypeQueue
	case "collections":
		return TypeCollections
	default:
		return TypeParticipant
	}
}

func (p *Parser) parseAutonumber(line string, d *Diagram) error {
	rest := line[len("autonumber"):]
	rest = strings.TrimSpace(rest)

	parts := strings.Fields(rest)
	start, incr := 1.0, 1.0

	if len(parts) >= 1 {
		if f, err := parseFloat(parts[0]); err == nil {
			start = f
		}
	}
	if len(parts) >= 2 {
		if f, err := parseFloat(parts[1]); err == nil {
			incr = f
		}
	}

	d.autonumber = &Autonumber{Start: start, Increment: incr}
	return nil
}

func (p *Parser) parseBox(line string, d *Diagram) error {
	// box [color] [title]
	// ... participants ...
	// end
	rest := line[3:]
	rest = strings.TrimSpace(rest)

	var color, title string
	// Simple parsing: first token might be a color, rest is title
	parts := strings.SplitN(rest, " ", 2)
	if len(parts) == 2 && isColorName(parts[0]) {
		color = parts[0]
		title = parts[1]
	} else if len(parts) > 0 && parts[0] != "" {
		title = rest
	}

	box := &Box{
		Title:        title,
		Color:        color,
		Participants: []*Participant{},
	}

	// Collect participants until 'end'
	for p.pos++; p.pos < len(p.lines); p.pos++ {
		l := p.lines[p.pos]
		if strings.TrimSpace(l) == "end" {
			break
		}
		// Check if this line declares a participant within the box
		if strings.HasPrefix(l, "participant ") || strings.HasPrefix(l, "actor ") {
			// Parse and add to box
			tempD := &Diagram{}
			if err := p.parseLine(l, tempD); err != nil {
				return err
			}
			if len(tempD.participants) > 0 {
				participant := tempD.participants[0]
				participant.Box = box
				box.Participants = append(box.Participants, participant)
			}
		}
	}

	d.boxes = append(d.boxes, box)
	return nil
}

func (p *Parser) parseMessage(line string, d *Diagram) error {
	// Format: From [Arrow] To: Message
	// or Self message: From [Arrow] From: Message

	// Find the arrow
	arrowMatch := findArrow(line)
	if arrowMatch.start < 0 {
		return nil // Skip if no arrow found
	}

	fromStr := strings.TrimSpace(line[:arrowMatch.start])
	afterArrow := strings.TrimSpace(line[arrowMatch.end:])

	// Check for message text (after colon)
	var toStr, msgStr string
	if before, after, ok := strings.Cut(afterArrow, ":"); ok {
		toStr = strings.TrimSpace(before)
		msgStr = strings.TrimSpace(after)
	} else {
		toStr = afterArrow
	}

	// Check for activate/deactivate markers on from side
	activate := ""
	if strings.HasSuffix(fromStr, "+") && len(fromStr) > 1 && fromStr[len(fromStr)-2] != '-' && fromStr[len(fromStr)-2] != '>' {
		fromStr = strings.TrimSpace(fromStr[:len(fromStr)-1])
		activate = "+"
	}

	// Check for activate/deactivate markers on to side
	if strings.HasSuffix(toStr, "+") && len(toStr) > 1 && toStr[len(toStr)-2] != '-' && toStr[len(toStr)-2] != '>' {
		toStr = strings.TrimSpace(toStr[:len(toStr)-1])
		activate = "+"
	} else if strings.HasSuffix(toStr, "-") && len(toStr) > 1 && toStr[len(toStr)-2] != '-' && toStr[len(toStr)-2] != 'x' {
		toStr = strings.TrimSpace(toStr[:len(toStr)-1])
		activate = "-"
	}

	fromID := fromStr
	toID := toStr
	arrowType := parseArrowType(line[arrowMatch.start:arrowMatch.end])

	msg := Message{
		FromID:     fromID,
		ToID:       toID,
		From:       findParticipant(d, fromID),
		To:         findParticipant(d, toID),
		Label:      diagram.CleanLabel(msgStr),
		Arrow:      arrowType,
		LineNo:     p.pos,
		ActivateOn: activate,
		SelfLoop:   fromID == toID,
	}

	// Store message even if participants not yet found (for implicit collection)
	d.messages = append(d.messages, msg)

	return nil
}

type arrowMatch struct {
	start, end int
	arrow      ArrowType
}

func findArrow(line string) arrowMatch {
	// Check for arrow types in order of specificity (longest first)
	arrows := []struct {
		pattern string
		atype   ArrowType
	}{
		{"<<-->>", ArrowBidirectionalDotted},
		{"<<->>", ArrowBidirectional},
		{"-->>", ArrowDottedFilledHead},
		{"->>", ArrowFilledHead},
		{"--x", ArrowCrossDotted},
		{"-x", ArrowCross},
		{"--)", ArrowAsyncDotted},
		{"-)", ArrowAsync},
		{"-->", ArrowDotted},
		{"->", ArrowSolid},
	}

	for _, a := range arrows {
		if idx := strings.Index(line, a.pattern); idx >= 0 {
			return arrowMatch{idx, idx + len(a.pattern), a.atype}
		}
	}

	return arrowMatch{-1, -1, ArrowSolid}
}

func parseArrowType(arrowStr string) ArrowType {
	switch arrowStr {
	case "->":
		return ArrowSolid
	case "-->":
		return ArrowDotted
	case "->>":
		return ArrowFilledHead
	case "-->>":
		return ArrowDottedFilledHead
	case "<<->>":
		return ArrowBidirectional
	case "<<-->>":
		return ArrowBidirectionalDotted
	case "-x":
		return ArrowCross
	case "--x":
		return ArrowCrossDotted
	case "-)":
		return ArrowAsync
	case "--)":
		return ArrowAsyncDotted
	default:
		return ArrowSolid
	}
}

func findParticipant(d *Diagram, id string) *Participant {
	id = strings.TrimSpace(id)
	for _, p := range d.participants {
		if p.ID == id {
			return p
		}
	}
	return nil
}

func (d *Diagram) collectImplicitParticipants() {
	// Extract participant IDs mentioned in messages
	seenIDs := make(map[string]bool)
	var implicitIDs []string

	for _, p := range d.participants {
		seenIDs[p.ID] = true
	}

	// Collect all message participants that haven't been explicitly declared
	for _, msg := range d.messages {
		if msg.FromID != "" && !seenIDs[msg.FromID] {
			seenIDs[msg.FromID] = true
			implicitIDs = append(implicitIDs, msg.FromID)
		}
		if msg.ToID != "" && !seenIDs[msg.ToID] {
			seenIDs[msg.ToID] = true
			implicitIDs = append(implicitIDs, msg.ToID)
		}
	}

	// Create implicit participants
	order := len(d.participants)
	for _, id := range implicitIDs {
		p := &Participant{
			ID:      id,
			Display: id,
			Type:    TypeParticipant,
			Order:   order,
		}
		d.participants = append(d.participants, p)
		order++
	}

	// Re-link all messages to actual participant pointers
	for i := range d.messages {
		msg := &d.messages[i]
		msg.From = findParticipant(d, msg.FromID)
		msg.To = findParticipant(d, msg.ToID)
	}
}

func (d *Diagram) validateAndNormalize() error {
	// Ensure all messages reference valid participants
	for i, msg := range d.messages {
		if msg.From == nil || msg.To == nil {
			return fmt.Errorf("message %d has missing participant", i)
		}
	}
	return nil
}

// Helper functions

func parseStringArg(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

func parseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}

func isColorName(s string) bool {
	colorNames := map[string]bool{
		"red": true, "blue": true, "green": true, "yellow": true,
		"purple": true, "orange": true, "pink": true, "brown": true,
		"gray": true, "black": true, "white": true, "aqua": true,
		"navy": true, "teal": true, "olive": true, "lime": true,
		"maroon": true, "silver": true, "fuchsia": true, "transparent": true,
	}
	s = strings.ToLower(strings.TrimSpace(s))
	return colorNames[s] || strings.HasPrefix(s, "rgb") || strings.HasPrefix(s, "#")
}
