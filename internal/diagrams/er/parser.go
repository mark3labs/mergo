package er

import (
	"regexp"
	"strings"
)

// Entity represents an ER entity with attributes.
type Entity struct {
	Name  string
	Label string
	Attrs []Attribute
}

// Attribute represents an entity attribute.
type Attribute struct {
	Type    string
	Name    string
	Keys    string // "PK", "FK", "UK", or combination
	Comment string
}

// Relationship represents a relationship between two entities.
type Relationship struct {
	From        string
	To          string
	Label       string
	FromCard    Cardinality
	ToCard      Cardinality
	Identifying bool // true for -- (solid), false for .. (dashed)
}

// Cardinality is a cardinality marker.
type Cardinality int

const (
	CardUnknown Cardinality = iota
	CardExactlyOne
	CardZeroOrOne
	CardOneOrMore
	CardZeroOrMore
)

// Document holds parsed ER diagram.
type Document struct {
	Title         string
	Direction     string
	Entities      map[string]*Entity
	Relationships []*Relationship
	Classes       map[string]ClassDef
}

// ClassDef holds style class definitions.
type ClassDef struct {
	Fill        string
	Stroke      string
	Color       string
	StrokeWidth string
	StrokeDash  string
}

// Parser parses ER diagram syntax.
type Parser struct {
	lines []string
	pos   int
	doc   *Document
}

// Parse parses ER diagram source.
func Parse(src string) (*Document, error) {
	lines := strings.Split(src, "\n")
	p := &Parser{
		lines: lines,
		doc: &Document{
			Entities:      make(map[string]*Entity),
			Relationships: make([]*Relationship, 0),
			Classes:       make(map[string]ClassDef),
		},
	}
	return p.parse()
}

func (p *Parser) parse() (*Document, error) {
	for p.pos < len(p.lines) {
		line := strings.TrimSpace(p.lines[p.pos])
		p.pos++

		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}

		if strings.HasPrefix(line, "erDiagram") {
			continue
		}

		if strings.HasPrefix(line, "title") {
			p.doc.Title = p.parseTitle(line)
			continue
		}

		if strings.HasPrefix(line, "direction") {
			p.doc.Direction = strings.TrimSpace(strings.TrimPrefix(line, "direction"))
			continue
		}

		if strings.HasPrefix(line, "classDef") {
			p.parseClassDef(line)
			continue
		}

		if strings.HasPrefix(line, "class") {
			// class id className - we ignore for now (styling)
			continue
		}

		if strings.HasPrefix(line, "style") {
			// style id ... - we ignore styling directives
			continue
		}

		// Check for entity or relationship
		if !p.parseEntity(line) && !p.parseRelationship(line) {
			// Unknown statement, silently ignore
			continue
		}
	}
	return p.doc, nil
}

func (p *Parser) parseTitle(line string) string {
	title := strings.TrimSpace(strings.TrimPrefix(line, "title"))
	return strings.Trim(title, "\"'")
}

func (p *Parser) parseClassDef(line string) {
	// classDef className fill:#f9f,stroke:#333,stroke-width:2px
	parts := strings.SplitN(line, " ", 3)
	if len(parts) < 3 {
		return
	}
	className := parts[1]
	styleDef := parts[2]

	cd := ClassDef{}
	for _, style := range strings.Split(styleDef, ",") {
		kv := strings.SplitN(strings.TrimSpace(style), ":", 2)
		if len(kv) != 2 {
			continue
		}
		key, val := strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])
		switch strings.ToLower(key) {
		case "fill":
			cd.Fill = val
		case "stroke":
			cd.Stroke = val
		case "color":
			cd.Color = val
		case "stroke-width":
			cd.StrokeWidth = val
		case "stroke-dasharray":
			cd.StrokeDash = val
		}
	}
	p.doc.Classes[className] = cd
}

func (p *Parser) parseEntity(line string) bool {
	// Entity with attributes: ENTITY_NAME { type name PK, type name FK, ... }
	// or simple entity: ENTITY_NAME or ENTITY_NAME["Label"]
	if !strings.Contains(line, "{") {
		// Simple entity or with label
		match := regexp.MustCompile(`^([A-Z_][A-Z0-9_]*)\s*(?:\["([^"]+)"\])?`).FindStringSubmatch(line)
		if match == nil {
			return false
		}
		name := match[1]
		label := match[2]
		if label == "" {
			label = name
		}
		p.doc.Entities[name] = &Entity{Name: name, Label: label}
		return true
	}

	// Entity with attributes
	start := strings.Index(line, "{")
	if start == -1 {
		return false
	}

	end := strings.Index(line, "}")
	if end == -1 {
		return false
	}

	namepart := strings.TrimSpace(line[:start])
	attrs := strings.TrimSpace(line[start+1 : end])

	// Parse entity name with optional label
	match := regexp.MustCompile(`^([A-Z_][A-Z0-9_]*)\s*(?:\["([^"]+)"\])?`).FindStringSubmatch(namepart)
	if match == nil {
		return false
	}

	name := match[1]
	label := match[2]
	if label == "" {
		label = name
	}

	entity := &Entity{Name: name, Label: label}

	// Parse attributes
	for _, attr := range strings.Split(attrs, ",") {
		attr = strings.TrimSpace(attr)
		if attr == "" {
			continue
		}
		pa := p.parseAttribute(attr)
		if pa != nil {
			entity.Attrs = append(entity.Attrs, *pa)
		}
	}

	p.doc.Entities[name] = entity
	return true
}

func (p *Parser) parseAttribute(attr string) *Attribute {
	// type name PK, FK, UK "comment"
	// or just: type name
	attr = strings.TrimSpace(attr)

	// Extract comment if present
	comment := ""
	if strings.Contains(attr, "\"") {
		parts := strings.SplitN(attr, "\"", 2)
		attr = strings.TrimSpace(parts[0])
		if len(parts) > 1 {
			comment = strings.TrimSpace(parts[1])
			comment = strings.TrimSuffix(comment, "\"")
		}
	}

	// Split remaining into type, name, and keys
	words := strings.Fields(attr)
	if len(words) < 2 {
		return nil
	}

	aType := words[0]
	aName := words[1]
	aKeys := strings.Join(words[2:], " ")

	return &Attribute{
		Type:    aType,
		Name:    aName,
		Keys:    aKeys,
		Comment: comment,
	}
}

func (p *Parser) parseRelationship(line string) bool {
	// Pattern: ENTITY1 card1--linecard2 ENTITY2 : "label"
	// Where card1, linecard2 are like: ||--o{  or ||..o{ etc.
	// The pattern is: cards<2> [--or..] cards<2>

	// Extract label if present
	label := ""
	if idx := strings.Index(line, ":"); idx >= 0 {
		label = strings.TrimSpace(line[idx+1:])
		label = strings.Trim(label, "\"'")
		line = strings.TrimSpace(line[:idx])
	}

	// Find the line style (-- or ..)
	lineStyleIdx := strings.Index(line, "--")
	if lineStyleIdx < 0 {
		lineStyleIdx = strings.Index(line, "..")
	}
	if lineStyleIdx < 0 {
		return p.parseRelationshipWords(line)
	}

	lineStyle := "--"
	if lineStyleIdx > 0 && line[lineStyleIdx:lineStyleIdx+2] == ".." {
		lineStyle = ".."
	}

	// Split into before and after the line style
	before := strings.TrimSpace(line[:lineStyleIdx])
	after := strings.TrimSpace(line[lineStyleIdx+2:])

	// Extract from entity and from cardinality
	beforeWords := strings.Fields(before)
	if len(beforeWords) < 2 {
		return false
	}
	from := beforeWords[len(beforeWords)-2]     // entity name
	fromCard := beforeWords[len(beforeWords)-1] // cardinality

	// Extract to cardinality and to entity
	afterWords := strings.Fields(after)
	if len(afterWords) < 2 {
		return false
	}
	toCard := afterWords[0] // cardinality
	to := afterWords[1]     // entity name

	// Verify entities exist
	if _, ok := p.doc.Entities[from]; !ok {
		return false
	}
	if _, ok := p.doc.Entities[to]; !ok {
		return false
	}

	rel := &Relationship{
		From:        from,
		To:          to,
		Label:       label,
		FromCard:    parseCardinality(fromCard),
		ToCard:      parseCardinality(toCard),
		Identifying: lineStyle == "--",
	}

	p.doc.Relationships = append(p.doc.Relationships, rel)
	return true
}

func (p *Parser) parseRelationshipWords(line string) bool {
	// Support word aliases like "one or zero", "zero or more", etc.
	// Example: CUSTOMER one or zero -- ORDER : "has"
	words := strings.Fields(line)
	if len(words) < 3 {
		return false
	}

	from := words[0]
	if _, ok := p.doc.Entities[from]; !ok {
		return false
	}

	// Find the line style indicator (-- or ..)
	lineStyleIdx := -1
	var lineStyle string
	for i, w := range words {
		if w == "--" || w == ".." {
			lineStyleIdx = i
			lineStyle = w
			break
		}
	}
	if lineStyleIdx < 1 {
		return false
	}

	to := words[lineStyleIdx+1]
	if _, ok := p.doc.Entities[to]; !ok {
		return false
	}

	// Extract cardinalities
	fromCardStr := strings.Join(words[1:lineStyleIdx], " ")
	toCardStr := strings.Join(words[lineStyleIdx+2:], " ")

	// Extract label if present
	labelStr := ""
	if idx := strings.Index(toCardStr, ":"); idx >= 0 {
		labelStr = strings.TrimSpace(toCardStr[idx+1:])
		labelStr = strings.Trim(labelStr, "\"'")
		toCardStr = strings.TrimSpace(toCardStr[:idx])
	}

	rel := &Relationship{
		From:        from,
		To:          to,
		Label:       labelStr,
		FromCard:    parseCardinalityWord(fromCardStr),
		ToCard:      parseCardinalityWord(toCardStr),
		Identifying: lineStyle == "--",
	}

	p.doc.Relationships = append(p.doc.Relationships, rel)
	return true
}

func parseCardinality(s string) Cardinality {
	s = strings.TrimSpace(s)
	switch s {
	case "||":
		return CardExactlyOne
	case "|o", "o|":
		return CardZeroOrOne
	case "}|", "|{":
		return CardOneOrMore
	case "}o", "o{":
		return CardZeroOrMore
	default:
		return CardUnknown
	}
}

func parseCardinalityWord(s string) Cardinality {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "only one", "one", "exactly one", "1":
		return CardExactlyOne
	case "one or zero", "zero or one", "0..1":
		return CardZeroOrOne
	case "one or more", "1+":
		return CardOneOrMore
	case "zero or more", "many", "*", "0..*":
		return CardZeroOrMore
	default:
		return CardUnknown
	}
}
