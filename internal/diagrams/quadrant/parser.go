package quadrant

import (
	"fmt"
	"regexp"
	"strings"
)

// Point represents a point in the quadrant chart.
type Point struct {
	Name  string
	X     float64
	Y     float64
	Style PointStyle
}

// PointStyle holds styling for a point.
type PointStyle struct {
	Radius      float64
	Color       string
	StrokeColor string
	StrokeWidth string
}

// Document holds parsed quadrant chart.
type Document struct {
	Title     string
	XAxis     AxisDef
	YAxis     AxisDef
	Quadrant1 string
	Quadrant2 string
	Quadrant3 string
	Quadrant4 string
	Points    []Point
	Classes   map[string]ClassDef
}

// AxisDef represents an axis definition.
type AxisDef struct {
	Min   string
	Max   string
	Title string
}

// ClassDef holds style class definitions.
type ClassDef struct {
	Fill        string
	Stroke      string
	Color       string
	StrokeWidth string
	StrokeDash  string
}

// Parser parses quadrant chart syntax.
type Parser struct {
	lines []string
	pos   int
	doc   *Document
}

// Parse parses quadrant chart source.
func Parse(src string) (*Document, error) {
	lines := strings.Split(src, "\n")
	p := &Parser{
		lines: lines,
		doc: &Document{
			Points:  make([]Point, 0),
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

		if strings.HasPrefix(line, "quadrantChart") {
			continue
		}

		if strings.HasPrefix(line, "title") {
			p.doc.Title = p.parseTitle(line)
			continue
		}

		if strings.HasPrefix(line, "x-axis") {
			p.parseXAxis(line)
			continue
		}

		if strings.HasPrefix(line, "y-axis") {
			p.parseYAxis(line)
			continue
		}

		if after, ok := strings.CutPrefix(line, "quadrant-1"); ok {
			p.doc.Quadrant1 = strings.TrimSpace(after)
			continue
		}

		if after, ok := strings.CutPrefix(line, "quadrant-2"); ok {
			p.doc.Quadrant2 = strings.TrimSpace(after)
			continue
		}

		if after, ok := strings.CutPrefix(line, "quadrant-3"); ok {
			p.doc.Quadrant3 = strings.TrimSpace(after)
			continue
		}

		if after, ok := strings.CutPrefix(line, "quadrant-4"); ok {
			p.doc.Quadrant4 = strings.TrimSpace(after)
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

		// Try to parse point
		if p.parsePoint(line) {
			continue
		}

		// Unknown statement, silently ignore
	}

	return p.doc, nil
}

func (p *Parser) parseTitle(line string) string {
	title := strings.TrimSpace(strings.TrimPrefix(line, "title"))
	return strings.Trim(title, "\"'")
}

func (p *Parser) parseXAxis(line string) {
	// x-axis Low --> High
	line = strings.TrimSpace(strings.TrimPrefix(line, "x-axis"))
	parts := strings.Split(line, "-->")
	if len(parts) >= 2 {
		p.doc.XAxis.Min = strings.TrimSpace(parts[0])
		p.doc.XAxis.Max = strings.TrimSpace(parts[1])
	}
}

func (p *Parser) parseYAxis(line string) {
	// y-axis Low --> High
	line = strings.TrimSpace(strings.TrimPrefix(line, "y-axis"))
	parts := strings.Split(line, "-->")
	if len(parts) >= 2 {
		p.doc.YAxis.Min = strings.TrimSpace(parts[0])
		p.doc.YAxis.Max = strings.TrimSpace(parts[1])
	}
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

func (p *Parser) parsePoint(line string) bool {
	// Pattern: Name: [x, y] or Name: [x, y], radius: 10, color: #fff, ...
	// or with class: Name: [x, y]:::className

	// Find colon separator
	before, after, ok := strings.Cut(line, ":")
	if !ok {
		return false
	}

	name := strings.TrimSpace(before)
	rest := strings.TrimSpace(after)

	// Extract class if present
	var className string
	if strings.Contains(rest, ":::") {
		parts := strings.SplitN(rest, ":::", 2)
		rest = strings.TrimSpace(parts[0])
		if len(parts) > 1 {
			className = strings.TrimSpace(parts[1])
		}
	}

	// Parse coordinates
	coordRe := regexp.MustCompile(`^\s*\[\s*([0-9.]+)\s*,\s*([0-9.]+)\s*\]`)
	match := coordRe.FindStringSubmatch(rest)
	if match == nil {
		return false
	}

	var x, y float64
	fmt.Sscanf(match[1], "%f", &x)
	fmt.Sscanf(match[2], "%f", &y)

	pt := Point{
		Name: name,
		X:    x,
		Y:    y,
		Style: PointStyle{
			Radius: 5,
		},
	}

	// Parse optional styling after ]
	afterCoords := rest[len(match[0]):]
	if afterCoords != "" {
		p.parsePointStyle(&pt, afterCoords)
	}

	// Apply class styling if present
	if className != "" {
		if classDef, ok := p.doc.Classes[className]; ok {
			if classDef.Color != "" {
				pt.Style.Color = classDef.Color
			}
			if classDef.Stroke != "" {
				pt.Style.StrokeColor = classDef.Stroke
			}
			if classDef.StrokeWidth != "" {
				pt.Style.StrokeWidth = classDef.StrokeWidth
			}
		}
	}

	p.doc.Points = append(p.doc.Points, pt)
	return true
}

func (p *Parser) parsePointStyle(pt *Point, styling string) {
	// Parse key: value pairs
	// E.g., ", radius: 10, color: #ff3300"
	parts := strings.SplitSeq(styling, ",")
	for part := range parts {
		kv := strings.SplitN(strings.TrimSpace(part), ":", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(kv[0]))
		val := strings.TrimSpace(kv[1])

		switch key {
		case "radius":
			fmt.Sscanf(val, "%f", &pt.Style.Radius)
		case "color":
			pt.Style.Color = val
		case "stroke-color":
			pt.Style.StrokeColor = val
		case "stroke-width":
			pt.Style.StrokeWidth = val
		}
	}
}
