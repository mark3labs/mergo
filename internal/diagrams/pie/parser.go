package pie

import (
	"fmt"
	"strings"

	"github.com/mark3labs/mergo/internal/diagram"
)

// Slice represents a pie slice.
type Slice struct {
	Label string
	Value float64
}

// Document holds parsed pie chart.
type Document struct {
	Title    string
	ShowData bool
	Slices   []Slice
	Classes  map[string]ClassDef
}

// ClassDef holds style class definitions.
type ClassDef struct {
	Fill        string
	Stroke      string
	Color       string
	StrokeWidth string
	StrokeDash  string
}

// Parser parses pie chart syntax.
type Parser struct {
	lines []string
	pos   int
	doc   *Document
}

// Parse parses pie chart source.
func Parse(src string) (*Document, error) {
	lines := strings.Split(src, "\n")
	p := &Parser{
		lines: lines,
		doc: &Document{
			Slices:  make([]Slice, 0),
			Classes: make(map[string]ClassDef),
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

		// Parse header
		if strings.HasPrefix(line, "pie") {
			p.parseHeader(line)
			continue
		}

		if strings.HasPrefix(line, "title") {
			p.doc.Title = p.parseTitle(line)
			continue
		}

		if strings.HasPrefix(line, "classDef") {
			p.parseClassDef(line)
			continue
		}

		if strings.HasPrefix(line, "class") {
			// class id className - we ignore styling
			continue
		}

		if strings.HasPrefix(line, "style") {
			// style directives - ignore
			continue
		}

		// Parse slice
		if p.parseSlice(line) {
			continue
		}

		// Unknown statement, silently ignore
	}

	return p.doc, nil
}

func (p *Parser) parseHeader(line string) {
	// pie title "My Title" showData or pie showData
	line = strings.TrimPrefix(line, "pie")
	line = strings.TrimSpace(line)

	// Check for showData
	if strings.Contains(strings.ToLower(line), "showdata") {
		p.doc.ShowData = true
		line = strings.ReplaceAll(strings.ToLower(line), "showdata", "")
		line = strings.TrimSpace(line)
	}

	// Extract title if present
	if after, ok := strings.CutPrefix(line, "title"); ok {
		line = after
		line = strings.TrimSpace(line)
		p.doc.Title = strings.Trim(line, "\"'")
	}
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
	for style := range strings.SplitSeq(styleDef, ",") {
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

func (p *Parser) parseSlice(line string) bool {
	// Pattern: "label" : value or label : value
	// Find colon separator
	colonIdx := strings.LastIndex(line, ":")
	if colonIdx < 0 {
		return false
	}

	labelPart := strings.TrimSpace(line[:colonIdx])
	valuePart := strings.TrimSpace(line[colonIdx+1:])

	// Extract label (remove quotes if present)
	label := diagram.CleanInline(strings.Trim(labelPart, "\"'"))
	if label == "" {
		return false
	}

	// Parse value
	var value float64
	_, err := fmt.Sscanf(valuePart, "%f", &value)
	if err != nil {
		// Try to parse as integer
		_, err = fmt.Sscanf(valuePart, "%d", &value)
		if err != nil {
			return false
		}
	}

	if value < 0 {
		return false
	}

	p.doc.Slices = append(p.doc.Slices, Slice{Label: label, Value: value})
	return true
}
