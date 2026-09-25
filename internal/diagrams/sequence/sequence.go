package sequence

import (
	"encoding/json"
	"fmt"
	"strconv"
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
	if err := renderer.render(); err != nil {
		return nil, err
	}

	return diagram.Finish(sc, th, cfg.Title), nil
}

// ============================================================================
// AST Types
// ============================================================================

// Diagram is the parsed AST for a sequence diagram.
type Diagram struct {
	Title        string
	Participants []*Participant
	Messages     []Statement
	Autonumber   *Autonumber
	Boxes        []*BoxDef
	Config       map[string]any
}

// Participant represents an actor or participant in the sequence.
type Participant struct {
	ID           string          // unique identifier (used in messages)
	Alias        string          // display name (what's shown in the diagram)
	Type         ParticipantType // actor, boundary, control, entity, database, queue, collections, or default
	LineNo       int             // line number for error reporting
	Box          *BoxDef         // if part of a box grouping
	createMsg    *Message        // if created via create statement (node appears at message y)
	destroyMsg   *Message        // if destroyed (lifeline ends with X)
	createLineY  float64         // y position where creation message was
	destroyLineY float64         // y position where destruction happens
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

// Statement is the base interface for all statements in a diagram.
type Statement interface {
	isStatement()
}

// Message is a communication between participants.
type Message struct {
	FromID       string
	ToID         string
	From         *Participant
	To           *Participant
	Label        string
	Arrow        ArrowType
	LineNo       int
	ActivateFrom string // "+" for activate from sender, "" for none
	ActivateTo   string // "+" or "-" for activate/deactivate recipient
	IsSelfLoop   bool
	HasCentral   bool // central lifeline connection via ()
	FromCentral  bool // message originates from central point
	ToCentral    bool // message ends at central point
}

func (m *Message) isStatement() {}

// Note is a note attached to participant(s) or spanning multiple participants.
type Note struct {
	Position NotePosition // left, right, over
	For      []*Participant
	Text     string
	LineNo   int
}

func (n *Note) isStatement() {}

// NotePosition indicates where a note is positioned relative to participants.
type NotePosition int

const (
	NoteRight NotePosition = iota
	NoteLeft
	NoteOver
)

// Block is a control structure (loop, alt, opt, par, critical, break, rect).
type Block struct {
	Kind     BlockKind
	Label    string
	Cond     string // condition text for frames (e.g., "[condition]")
	Children []Statement
	Clauses  []*BlockClause // for alt/par/critical (else/and/option)
	Color    string         // for rect backgrounds (rgb/rgba)
	Nested   bool           // whether this block is nested inside another
	LineNo   int
	Spans    []*Participant // participants touched by contents
}

func (b *Block) isStatement() {}

// BlockClause represents an alternative or parallel branch in a block.
type BlockClause struct {
	Kind     string // "else", "and", "option"
	Cond     string // condition text
	Children []Statement
}

// BlockKind is the type of control block.
type BlockKind int

const (
	BlockLoop BlockKind = iota
	BlockAlt
	BlockOpt
	BlockPar
	BlockCritical
	BlockBreak
	BlockRect
)

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
	Active    bool
}

// BoxDef groups participants visually.
type BoxDef struct {
	Title        string
	Color        string
	Participants []*Participant
	LineNo       int
}

// ============================================================================
// Parser
// ============================================================================

type Parser struct {
	src    string
	lines  []string
	pos    int
	lineNo int // actual line number (for error reporting)
}

func newParser(src string) *Parser {
	lines := strings.Split(src, "\n")
	return &Parser{src: src, lines: lines}
}

func (p *Parser) parse() (*Diagram, error) {
	d := &Diagram{
		Participants: []*Participant{},
		Messages:     []Statement{},
		Config:       map[string]any{},
	}

	for p.pos = 0; p.pos < len(p.lines); p.pos++ {
		p.lineNo = p.pos + 1
		line := strings.TrimSpace(p.lines[p.pos])

		// Skip header
		if strings.HasPrefix(strings.ToLower(line), "sequencediagram") {
			continue
		}

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}

		if err := p.parseLine(line, d); err != nil {
			return nil, fmt.Errorf("line %d: %w", p.lineNo, err)
		}
	}

	// Fill in implicit participants and validate
	d.collectImplicitParticipants()

	return d, nil
}

func (p *Parser) parseLine(line string, d *Diagram) error {
	switch {
	case strings.HasPrefix(line, "title "):
		d.Title = diagram.CleanLabel(line[6:])
		return nil

	case strings.HasPrefix(line, "participant "):
		return p.parseParticipant(line[12:], d, false)

	case strings.HasPrefix(line, "actor "):
		return p.parseParticipant(line[6:], d, true)

	case strings.HasPrefix(line, "autonumber"):
		return p.parseAutonumber(line, d)

	case strings.HasPrefix(line, "create "):
		return p.parseCreate(line[7:], d)

	case strings.HasPrefix(line, "destroy "):
		return p.parseDestroy(line[8:], d)

	case strings.HasPrefix(line, "activate "):
		return p.parseActivate(line[9:], d)

	case strings.HasPrefix(line, "deactivate "):
		return p.parseDeactivate(line[11:], d)

	case strings.HasPrefix(line, "Note "):
		return p.parseNote(line, d)

	case strings.HasPrefix(line, "loop "):
		return p.parseBlock(line, d, BlockLoop)

	case strings.HasPrefix(line, "alt "):
		return p.parseBlock(line, d, BlockAlt)

	case strings.HasPrefix(line, "opt "):
		return p.parseBlock(line, d, BlockOpt)

	case strings.HasPrefix(line, "par "):
		return p.parseBlock(line, d, BlockPar)

	case strings.HasPrefix(line, "critical "):
		return p.parseBlock(line, d, BlockCritical)

	case strings.HasPrefix(line, "break "):
		return p.parseBlock(line, d, BlockBreak)

	case strings.HasPrefix(line, "rect "):
		return p.parseRect(line, d)

	case strings.HasPrefix(line, "box "):
		return p.parseBox(line, d)

	case strings.HasPrefix(line, "else"):
		// Ignore else outside of blocks
		return nil

	case strings.HasPrefix(line, "and"):
		// Ignore and outside of blocks
		return nil

	case strings.HasPrefix(line, "option"):
		// Ignore option outside of blocks
		return nil

	case strings.HasPrefix(line, "end"):
		// Ignore standalone end
		return nil

	case strings.HasPrefix(line, "link "), strings.HasPrefix(line, "links "),
		strings.HasPrefix(line, "properties "):
		// Ignore these non-critical features
		return nil

	default:
		// Try to parse as message
		if msg, err := p.parseMessage(line); err == nil && msg != nil {
			d.Messages = append(d.Messages, msg)
		}
		return nil
	}
}

// extractJSONField extracts a field value from a JSON-like string with unquoted keys.
// e.g., "type: database" or "type: \"database\""
func extractJSONField(jsonStr, fieldName string) string {
	// Look for fieldName:
	pattern := fieldName + ":"
	_, after, ok := strings.Cut(jsonStr, pattern)
	if !ok {
		return ""
	}

	rest := strings.TrimSpace(after)

	// Extract value until comma, brace, or end
	var value string
	var inQuote bool
	for i, ch := range rest {
		if ch == '"' {
			inQuote = !inQuote
			if inQuote {
				continue
			}
		}
		if !inQuote && (ch == ',' || ch == '}') {
			value = rest[:i]
			break
		}
		if i == len(rest)-1 {
			value = rest
		}
	}

	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"`)
	value = strings.TrimSpace(value)
	return value
}

func (p *Parser) parseParticipant(rest string, d *Diagram, isActor bool) error {
	ptype := TypeParticipant
	if isActor {
		ptype = TypeActor
	}

	var inlineAlias string

	// Check for JSON config like: ID@{"type": "database", "alias": "..."}
	// Also handle unquoted keys: ID@{type: database}
	if idx := strings.Index(rest, "@{"); idx >= 0 {
		endIdx := strings.Index(rest[idx:], "}")
		if endIdx >= 0 {
			jsonStr := rest[idx+2 : idx+endIdx]
			var cfg map[string]any
			// Add braces to make it valid JSON
			fullJSON := "{" + jsonStr + "}"
			if err := json.Unmarshal([]byte(fullJSON), &cfg); err == nil {
				if typeStr, ok := cfg["type"].(string); ok {
					ptype = parseParticipantType(typeStr)
				}
				if alias, ok := cfg["alias"].(string); ok {
					inlineAlias = diagram.CleanLabel(alias)
				}
			} else {
				// Try to parse manually for unquoted keys/values
				if typeMatch := extractJSONField(jsonStr, "type"); typeMatch != "" {
					ptype = parseParticipantType(typeMatch)
				}
				if aliasMatch := extractJSONField(jsonStr, "alias"); aliasMatch != "" {
					inlineAlias = diagram.CleanLabel(aliasMatch)
				}
			}
		}
		rest = rest[:idx] + rest[idx+endIdx+1:]
		rest = strings.TrimSpace(rest)
	}

	// Parse ID and alias: "ID" or "ID as Display"
	var id, alias string
	if idx := strings.Index(rest, " as "); idx >= 0 {
		id = strings.TrimSpace(rest[:idx])
		alias = diagram.CleanLabel(strings.TrimSpace(rest[idx+4:]))
	} else {
		id = strings.TrimSpace(rest)
		if inlineAlias != "" {
			alias = inlineAlias
		} else {
			alias = id
		}
	}

	if id == "" {
		return fmt.Errorf("participant must have an ID")
	}

	// Check for duplicates
	for _, p := range d.Participants {
		if p.ID == id {
			return fmt.Errorf("participant %q already defined", id)
		}
	}

	participant := &Participant{
		ID:     id,
		Alias:  alias,
		Type:   ptype,
		LineNo: p.lineNo,
	}
	d.Participants = append(d.Participants, participant)

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
	rest := strings.TrimSpace(line[10:])

	if strings.ToLower(rest) == "off" {
		if d.Autonumber != nil {
			d.Autonumber.Active = false
		}
		return nil
	}

	parts := strings.Fields(rest)
	start, incr := 1.0, 1.0

	if len(parts) >= 1 && parts[0] != "" {
		if f, err := strconv.ParseFloat(parts[0], 64); err == nil {
			start = f
		}
	}
	if len(parts) >= 2 {
		if f, err := strconv.ParseFloat(parts[1], 64); err == nil {
			incr = f
		}
	}

	d.Autonumber = &Autonumber{Start: start, Increment: incr, Active: true}
	return nil
}

func (p *Parser) parseCreate(rest string, d *Diagram) error {
	// create [participant|actor] ID [as Alias]
	isActor := false
	if strings.HasPrefix(rest, "participant ") {
		rest = rest[12:]
	} else if strings.HasPrefix(rest, "actor ") {
		rest = rest[6:]
		isActor = true
	}

	if err := p.parseParticipant(rest, d, isActor); err != nil {
		return err
	}

	// The next statement should be a message to/from this participant
	// which will be marked as its creation point
	return nil
}

func (p *Parser) parseDestroy(rest string, d *Diagram) error {
	participantID := strings.TrimSpace(rest)
	if participantID == "" {
		return fmt.Errorf("destroy requires a participant ID")
	}
	// Mark this participant for destruction on the next message
	// This is handled during rendering
	return nil
}

func (p *Parser) parseActivate(rest string, d *Diagram) error {
	participantID := strings.TrimSpace(rest)
	if participantID == "" {
		return fmt.Errorf("activate requires a participant ID")
	}
	// Handled via arrow suffixes, but this is also valid syntax
	return nil
}

func (p *Parser) parseDeactivate(rest string, d *Diagram) error {
	participantID := strings.TrimSpace(rest)
	if participantID == "" {
		return fmt.Errorf("deactivate requires a participant ID")
	}
	// Handled via arrow suffixes, but this is also valid syntax
	return nil
}

func (p *Parser) parseNote(line string, d *Diagram) error {
	// Note [left of|right of|over] [Participant][,Participant]: Text
	rest := strings.TrimSpace(line[4:])

	var position NotePosition
	var participantPart string

	if strings.HasPrefix(rest, "left of ") {
		position = NoteLeft
		rest = rest[8:]
	} else if strings.HasPrefix(rest, "right of ") {
		position = NoteRight
		rest = rest[9:]
	} else if strings.HasPrefix(rest, "over ") {
		position = NoteOver
		rest = rest[5:]
	}

	// Split on colon to separate participants from text
	colonIdx := strings.Index(rest, ":")
	if colonIdx < 0 {
		return nil // Skip malformed notes
	}

	participantPart = strings.TrimSpace(rest[:colonIdx])
	noteText := diagram.CleanLabel(rest[colonIdx+1:])

	// Parse participant list
	var participants []*Participant
	for pidStr := range strings.SplitSeq(participantPart, ",") {
		pidStr = strings.TrimSpace(pidStr)
		if pidStr == "" {
			continue
		}
		pid := findParticipant(d, pidStr)
		if pid != nil {
			participants = append(participants, pid)
		} else {
			// Implicit participant
			pid = &Participant{ID: pidStr, Alias: pidStr, Type: TypeParticipant, LineNo: p.lineNo}
			d.Participants = append(d.Participants, pid)
			participants = append(participants, pid)
		}
	}

	note := &Note{
		Position: position,
		For:      participants,
		Text:     noteText,
		LineNo:   p.lineNo,
	}
	d.Messages = append(d.Messages, note)
	return nil
}

func (p *Parser) parseBlock(line string, d *Diagram, kind BlockKind) error {
	var blockKeyword string
	var cond string

	switch kind {
	case BlockLoop:
		blockKeyword = "loop"
		cond = strings.TrimSpace(line[5:])
	case BlockAlt:
		blockKeyword = "alt"
		cond = strings.TrimSpace(line[4:])
	case BlockOpt:
		blockKeyword = "opt"
		cond = strings.TrimSpace(line[4:])
	case BlockPar:
		blockKeyword = "par"
		cond = strings.TrimSpace(line[4:])
	case BlockCritical:
		blockKeyword = "critical"
		cond = strings.TrimSpace(line[9:])
	case BlockBreak:
		blockKeyword = "break"
		cond = strings.TrimSpace(line[6:])
	}

	p.pos++
	block := &Block{
		Kind:   kind,
		Label:  cond,
		Cond:   cond,
		LineNo: p.lineNo,
	}

	var currentClause *BlockClause
	if kind == BlockAlt || kind == BlockPar || kind == BlockCritical {
		currentClause = &BlockClause{Kind: blockKeyword, Cond: cond, Children: []Statement{}}
	}

	// Parse block contents
	for p.pos < len(p.lines) {
		p.lineNo = p.pos + 1
		line := strings.TrimSpace(p.lines[p.pos])

		if line == "" || strings.HasPrefix(line, "%%") {
			p.pos++
			continue
		}

		if line == "end" {
			if currentClause != nil {
				block.Clauses = append(block.Clauses, currentClause)
			}
			d.Messages = append(d.Messages, block)
			return nil
		}

		if kind == BlockAlt && strings.HasPrefix(line, "else") {
			if currentClause != nil {
				block.Clauses = append(block.Clauses, currentClause)
			}
			cond := strings.TrimSpace(line[4:])
			currentClause = &BlockClause{Kind: "else", Cond: cond, Children: []Statement{}}
			p.pos++
			continue
		}

		if kind == BlockPar && strings.HasPrefix(line, "and ") {
			if currentClause != nil {
				block.Clauses = append(block.Clauses, currentClause)
			}
			cond := strings.TrimSpace(line[4:])
			currentClause = &BlockClause{Kind: "and", Cond: cond, Children: []Statement{}}
			p.pos++
			continue
		}

		if kind == BlockCritical && strings.HasPrefix(line, "option ") {
			if currentClause != nil {
				block.Clauses = append(block.Clauses, currentClause)
			}
			cond := strings.TrimSpace(line[7:])
			currentClause = &BlockClause{Kind: "option", Cond: cond, Children: []Statement{}}
			p.pos++
			continue
		}

		// Parse statement inside block
		stmt, err := p.parseBlockStatement(line)
		if err == nil && stmt != nil {
			if currentClause != nil {
				currentClause.Children = append(currentClause.Children, stmt)
			} else {
				block.Children = append(block.Children, stmt)
			}
		}
		p.pos++
	}

	return fmt.Errorf("block not closed with 'end'")
}

func (p *Parser) parseBlockStatement(line string) (Statement, error) {
	// Parse a single statement inside a block
	if msg, err := p.parseMessage(line); err == nil {
		return msg, nil
	}
	if strings.HasPrefix(line, "Note ") {
		if err := p.parseNote(line, &Diagram{Participants: []*Participant{}}); err == nil {
			// Return a dummy, we'll re-parse properly
			return nil, nil
		}
	}
	return nil, fmt.Errorf("unknown statement in block")
}

func (p *Parser) parseRect(line string, d *Diagram) error {
	// rect [rgb|rgba|hsl|hsla](...)
	//   ... content ...
	// end

	rest := strings.TrimSpace(line[4:])
	color := rest

	p.pos++
	block := &Block{
		Kind:   BlockRect,
		Label:  "",
		Color:  color,
		LineNo: p.lineNo,
	}

	// Parse block contents
	for p.pos < len(p.lines) {
		p.lineNo = p.pos + 1
		line := strings.TrimSpace(p.lines[p.pos])

		if line == "" || strings.HasPrefix(line, "%%") {
			p.pos++
			continue
		}

		if line == "end" {
			d.Messages = append(d.Messages, block)
			return nil
		}

		if msg, err := p.parseMessage(line); err == nil && msg != nil {
			block.Children = append(block.Children, msg)
		}
		p.pos++
	}

	return fmt.Errorf("rect block not closed with 'end'")
}

func (p *Parser) parseBox(line string, d *Diagram) error {
	// box [color] [title]
	//   participant ...
	//   participant ...
	// end

	rest := strings.TrimSpace(line[3:])

	// Try to parse color and title
	var color, title string
	if rest != "" {
		if isColorValue(rest) {
			spaceIdx := strings.Index(rest, " ")
			if spaceIdx < 0 {
				color = rest
			} else {
				color = rest[:spaceIdx]
				title = strings.TrimSpace(rest[spaceIdx:])
			}
		} else {
			title = rest
		}
	}

	p.pos++
	box := &BoxDef{
		Title:        title,
		Color:        color,
		Participants: []*Participant{},
		LineNo:       p.lineNo,
	}

	// Parse participants in box
	for p.pos < len(p.lines) {
		p.lineNo = p.pos + 1
		line := strings.TrimSpace(p.lines[p.pos])

		if line == "" || strings.HasPrefix(line, "%%") {
			p.pos++
			continue
		}

		if line == "end" {
			d.Boxes = append(d.Boxes, box)
			return nil
		}

		if strings.HasPrefix(line, "participant ") {
			isActor := false
			if err := p.parseParticipant(line[12:], d, isActor); err != nil {
				return err
			}
			if len(d.Participants) > 0 {
				lastP := d.Participants[len(d.Participants)-1]
				lastP.Box = box
				box.Participants = append(box.Participants, lastP)
			}
		} else if strings.HasPrefix(line, "actor ") {
			if err := p.parseParticipant(line[6:], d, true); err != nil {
				return err
			}
			if len(d.Participants) > 0 {
				lastP := d.Participants[len(d.Participants)-1]
				lastP.Box = box
				box.Participants = append(box.Participants, lastP)
			}
		}

		p.pos++
	}

	return fmt.Errorf("box block not closed with 'end'")
}

func (p *Parser) parseMessage(line string) (*Message, error) {
	// Participant [->(arrow)][-/+] Participant[+/-]: message text

	// Find arrow
	arrowMatch := findArrow(line)
	if arrowMatch.start < 0 {
		return nil, nil
	}

	fromStr := strings.TrimSpace(line[:arrowMatch.start])
	arrowStr := line[arrowMatch.start:arrowMatch.end]
	afterArrow := strings.TrimSpace(line[arrowMatch.end:])

	// Extract to and message text
	var toStr, msgStr string
	if before, after, ok := strings.Cut(afterArrow, ":"); ok {
		toStr = strings.TrimSpace(before)
		msgStr = diagram.CleanLabel(after)
	} else {
		toStr = afterArrow
	}

	// Check for activation markers
	var activateFrom, activateTo string
	fromStr, activateFrom = extractActivation(fromStr)
	toStr, activateTo = extractActivation(toStr)

	fromID := strings.TrimSpace(fromStr)
	toID := strings.TrimSpace(toStr)
	arrowType := parseArrowType(arrowStr)

	msg := &Message{
		FromID:       fromID,
		ToID:         toID,
		Label:        msgStr,
		Arrow:        arrowType,
		LineNo:       p.lineNo,
		ActivateFrom: activateFrom,
		ActivateTo:   activateTo,
		IsSelfLoop:   fromID == toID,
	}

	return msg, nil
}

func extractActivation(s string) (string, string) {
	s = strings.TrimSpace(s)
	// Check for prefix activation marker (e.g., "+Bob" or "-Bob")
	if len(s) > 0 && (s[0] == '+' || s[0] == '-') {
		return strings.TrimSpace(s[1:]), string(s[0])
	}
	// Check for suffix activation marker (e.g., "Bob+" or "Bob-")
	if strings.HasSuffix(s, "+") && len(s) > 1 {
		return strings.TrimSpace(s[:len(s)-1]), "+"
	}
	if strings.HasSuffix(s, "-") && len(s) > 1 {
		return strings.TrimSpace(s[:len(s)-1]), "-"
	}
	return s, ""
}

type arrowMatch struct {
	start int
	end   int
}

var arrowPatterns = []struct {
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

func findArrow(line string) arrowMatch {
	for _, ap := range arrowPatterns {
		if idx := strings.Index(line, ap.pattern); idx >= 0 {
			return arrowMatch{idx, idx + len(ap.pattern)}
		}
	}
	return arrowMatch{-1, -1}
}

func parseArrowType(arrowStr string) ArrowType {
	for _, ap := range arrowPatterns {
		if ap.pattern == arrowStr {
			return ap.atype
		}
	}
	return ArrowSolid
}

func findParticipant(d *Diagram, id string) *Participant {
	id = strings.TrimSpace(id)
	for _, p := range d.Participants {
		if p.ID == id {
			return p
		}
	}
	return nil
}

func (d *Diagram) collectImplicitParticipants() {
	seenIDs := make(map[string]bool)
	var implicitIDs []string

	for _, p := range d.Participants {
		seenIDs[p.ID] = true
	}

	// Collect all participant IDs mentioned in messages
	var collectIDs func([]Statement)
	collectIDs = func(stmts []Statement) {
		for _, stmt := range stmts {
			switch s := stmt.(type) {
			case *Message:
				if s.FromID != "" && !seenIDs[s.FromID] {
					seenIDs[s.FromID] = true
					implicitIDs = append(implicitIDs, s.FromID)
				}
				if s.ToID != "" && !seenIDs[s.ToID] {
					seenIDs[s.ToID] = true
					implicitIDs = append(implicitIDs, s.ToID)
				}
			case *Block:
				collectIDs(s.Children)
				for _, clause := range s.Clauses {
					collectIDs(clause.Children)
				}
			case *Note:
				for _, p := range s.For {
					seenIDs[p.ID] = true
				}
			}
		}
	}

	collectIDs(d.Messages)

	// Create implicit participants
	order := len(d.Participants)
	for _, id := range implicitIDs {
		p := &Participant{
			ID:     id,
			Alias:  id,
			Type:   TypeParticipant,
			LineNo: 0,
		}
		d.Participants = append(d.Participants, p)
		order++
	}

	// Link messages to actual participant pointers
	var linkParticipants func([]Statement)
	linkParticipants = func(stmts []Statement) {
		for i, stmt := range stmts {
			switch s := stmt.(type) {
			case *Message:
				stmts[i].(*Message).From = findParticipant(d, s.FromID)
				stmts[i].(*Message).To = findParticipant(d, s.ToID)
			case *Block:
				linkParticipants(s.Children)
				for _, clause := range s.Clauses {
					linkParticipants(clause.Children)
				}
			case *Note:
				// Already linked during parsing
			}
		}
	}

	linkParticipants(d.Messages)
}

// Helper to check if a string looks like a color value
func isColorValue(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	if strings.HasPrefix(s, "rgb") || strings.HasPrefix(s, "#") {
		return true
	}
	colorNames := map[string]bool{
		"transparent": true, "aqua": true, "red": true, "blue": true,
		"green": true, "yellow": true, "purple": true, "orange": true,
	}
	firstWord := strings.Fields(s)[0]
	return colorNames[firstWord]
}
