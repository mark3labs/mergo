// Package class implements rendering of Mermaid class diagrams (UML).
package class

import (
	"fmt"
	"math"
	"strings"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/layout"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

func init() {
	diagram.Register(diagram.Type{
		Name:   "class",
		Detect: diagram.Keyword("classDiagram", "classDiagram-v2"),
		Render: Render,
	})
}

// Render parses and renders a class diagram.
func Render(src string, cfg *diagram.Config) (*scene.Scene, error) {
	p := newParser(cfg)
	if err := p.parse(src); err != nil {
		return nil, err
	}
	return p.render()
}

type parser struct {
	cfg       *diagram.Config
	th        *theme.Theme
	classes   map[string]*classInfo
	relations []*relationInfo
	notes     []*noteInfo
	direction layout.Direction
}

type classInfo struct {
	id           string
	label        string
	annotations  []string
	members      []member
	styles       map[string]string
	absorbLayout layout.Node
}

type member struct {
	visibility byte
	name       string
	typ        string
	isMethod   bool
	isStatic   bool
	isAbstract bool
}

type relationInfo struct {
	from, to string
	typ      relationType
	label    string
	cardFrom string
	cardTo   string
	dashed   bool
}

type relationType int

const (
	relationInheritance relationType = iota
	relationComposition
	relationAggregation
	relationAssociation
	relationLink
	relationDependency
	relationRealization
)

type noteInfo struct {
	text     string
	forClass string
}

func newParser(cfg *diagram.Config) *parser {
	th := cfg.Theme
	if th == nil {
		th = theme.Default()
	}
	return &parser{
		cfg:       cfg,
		th:        th,
		classes:   make(map[string]*classInfo),
		direction: layout.ParseDirection(cfg.String("class", "direction", "TB")),
	}
}

func (p *parser) parse(src string) error {
	lines := diagram.Lines(src)
	for i := 0; i < len(lines); i++ {
		line := lines[i]

		if strings.HasPrefix(line, "direction") {
			parts := strings.Fields(line)
			if len(parts) > 1 {
				p.direction = layout.ParseDirection(parts[1])
			}
			continue
		}

		if strings.HasPrefix(line, "classDef") || strings.HasPrefix(line, "cssClass") ||
			strings.HasPrefix(line, "style ") || strings.HasPrefix(line, "click ") ||
			strings.HasPrefix(line, "callback ") || strings.HasPrefix(line, "link ") ||
			strings.HasPrefix(line, "namespace ") {
			continue
		}

		if strings.HasPrefix(line, "class ") && strings.Contains(line, "{") {
			if err := p.parseClassWithMembers(line, lines, &i); err != nil {
				return fmt.Errorf("line %d: %w", i+1, err)
			}
			continue
		}

		if strings.HasPrefix(line, "class ") {
			if err := p.parseClassDecl(line); err != nil {
				return fmt.Errorf("line %d: %w", i+1, err)
			}
			continue
		}

		if strings.Contains(line, " : ") && !strings.Contains(line, "--") && !strings.Contains(line, "..") {
			parts := strings.SplitN(line, " : ", 2)
			if len(parts) == 2 {
				className := strings.TrimSpace(parts[0])
				memberStr := strings.TrimSpace(parts[1])
				p.addMember(className, memberStr)
			}
			continue
		}

		if p.tryParseRelation(line) {
			continue
		}

		if strings.HasPrefix(line, "note ") {
			if strings.Contains(line, "for ") {
				parts := strings.SplitN(line, " for ", 2)
				if len(parts) == 2 {
					rest := strings.TrimSpace(parts[1])
					spaceIdx := strings.IndexAny(rest, " \t")
					if spaceIdx > 0 {
						className := rest[:spaceIdx]
						noteText := rest[spaceIdx:]
						noteText = diagram.CleanLabel(noteText)
						p.notes = append(p.notes, &noteInfo{text: noteText, forClass: className})
					}
				}
			}
		}
	}
	return nil
}

func (p *parser) parseClassDecl(line string) error {
	line = strings.TrimPrefix(line, "class ")
	line = strings.TrimSpace(line)

	var annotations []string
	var className string
	var label string

	// Check for annotation (classname << annotation >>)
	if annIdx := strings.Index(line, "<<"); annIdx > 0 {
		className = strings.TrimSpace(line[:annIdx])
		rest := line[annIdx:]
		if endIdx := strings.Index(rest, ">>"); endIdx > 0 {
			ann := rest[2:endIdx]
			annotations = append(annotations, ann)
		}
	} else {
		className = line
	}

	// Handle label ["..."]
	if strings.Contains(className, "[\"") {
		idx := strings.Index(className, "[\"")
		label = className[idx+2:]
		className = className[:idx]
		if endIdx := strings.Index(label, "\"]"); endIdx > 0 {
			label = label[:endIdx]
		}
	} else if strings.HasPrefix(className, "`") && strings.Contains(className, "`") {
		endIdx := strings.LastIndex(className, "`")
		if endIdx > 0 {
			className = className[1:endIdx]
		}
	}

	className = strings.TrimSpace(className)
	if className == "" {
		return fmt.Errorf("empty class name")
	}

	// Remove generic suffix ~T~
	if idx := strings.Index(className, "~"); idx > 0 {
		className = className[:idx]
	}

	ci := &classInfo{
		id:          className,
		label:       label,
		annotations: annotations,
		styles:      make(map[string]string),
	}
	p.classes[className] = ci
	return nil
}

func (p *parser) parseClassWithMembers(line string, lines []string, idx *int) error {
	before, _, ok := strings.Cut(line, "{")
	if !ok {
		return fmt.Errorf("missing {")
	}

	classDecl := before
	classDecl = strings.TrimPrefix(classDecl, "class ")
	classDecl = strings.TrimSpace(classDecl)

	var annotations []string
	var className string
	var label string

	// Check for annotation
	if annIdx := strings.Index(classDecl, "<<"); annIdx > 0 {
		className = strings.TrimSpace(classDecl[:annIdx])
		rest := classDecl[annIdx:]
		if endIdx := strings.Index(rest, ">>"); endIdx > 0 {
			ann := rest[2:endIdx]
			annotations = append(annotations, ann)
		}
	} else {
		className = classDecl
	}

	// Handle label
	if strings.Contains(className, "[\"") {
		idx := strings.Index(className, "[\"")
		label = className[idx+2:]
		className = className[:idx]
		if endIdx := strings.Index(label, "\"]"); endIdx > 0 {
			label = label[:endIdx]
		}
	}

	className = strings.TrimSpace(className)
	if idx := strings.Index(className, "~"); idx > 0 {
		className = className[:idx]
	}

	if className == "" {
		return fmt.Errorf("empty class name")
	}

	ci := &classInfo{
		id:          className,
		label:       label,
		annotations: annotations,
		styles:      make(map[string]string),
	}

	for *idx++; *idx < len(lines); *idx++ {
		memberLine := strings.TrimSpace(lines[*idx])
		if memberLine == "" {
			continue
		}
		if memberLine == "}" {
			break
		}

		if strings.HasPrefix(memberLine, "<<") && strings.Contains(memberLine, ">>") {
			endIdx := strings.Index(memberLine, ">>")
			ann := memberLine[2:endIdx]
			ci.annotations = append(ci.annotations, ann)
			continue
		}

		mem := p.parseMember(memberLine)
		if mem != nil {
			ci.members = append(ci.members, *mem)
		}
	}

	p.classes[className] = ci
	return nil
}

func (p *parser) parseMember(line string) *member {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}

	m := &member{}

	if len(line) > 0 {
		switch line[0] {
		case '+', '-', '#', '~':
			m.visibility = line[0]
			line = strings.TrimSpace(line[1:])
		}
	}

	if strings.Contains(line, "(") {
		m.isMethod = true
		parenIdx := strings.Index(line, "(")
		m.name = strings.TrimSpace(line[:parenIdx])

		closeIdx := strings.LastIndex(line, ")")
		if closeIdx > parenIdx {
			afterParen := strings.TrimSpace(line[closeIdx+1:])
			if strings.HasSuffix(afterParen, "$") {
				m.isStatic = true
				afterParen = strings.TrimSuffix(afterParen, "$")
			}
			if strings.HasSuffix(afterParen, "*") {
				m.isAbstract = true
				afterParen = strings.TrimSuffix(afterParen, "*")
			}
			if afterParen != "" {
				m.typ = afterParen
			}
		}
	} else {
		m.isMethod = false
		if strings.HasSuffix(line, "$") {
			m.isStatic = true
			line = strings.TrimSuffix(line, "$")
		}
		if colonIdx := strings.LastIndex(line, ":"); colonIdx > 0 {
			m.name = strings.TrimSpace(line[:colonIdx])
			m.typ = strings.TrimSpace(line[colonIdx+1:])
		} else {
			m.name = line
		}
	}

	return m
}

func (p *parser) addMember(className, memberStr string) {
	ci, ok := p.classes[className]
	if !ok {
		ci = &classInfo{
			id:     className,
			styles: make(map[string]string),
		}
		p.classes[className] = ci
	}
	mem := p.parseMember(memberStr)
	if mem != nil {
		ci.members = append(ci.members, *mem)
	}
}

func (p *parser) tryParseRelation(line string) bool {
	relations := []struct {
		pattern string
		typ     relationType
		dashed  bool
	}{
		{"<|--", relationInheritance, false},
		{"--|>", relationInheritance, false},
		{"*--", relationComposition, false},
		{"--*", relationComposition, false},
		{"o--", relationAggregation, false},
		{"--o", relationAggregation, false},
		{"-->", relationAssociation, false},
		{"<--", relationAssociation, false},
		{"..|>", relationRealization, true},
		{"<|..", relationRealization, true},
		{"..>", relationDependency, true},
		{"<..", relationDependency, true},
		{"--", relationLink, false},
		{"..", relationLink, true},
	}

	for _, rel := range relations {
		idx := strings.Index(line, rel.pattern)
		if idx <= 0 {
			continue
		}

		from := strings.TrimSpace(line[:idx])
		afterRel := line[idx+len(rel.pattern):]

		var to, label string
		if colonIdx := strings.Index(afterRel, ":"); colonIdx > 0 {
			to = strings.TrimSpace(afterRel[:colonIdx])
			label = strings.TrimSpace(afterRel[colonIdx+1:])
		} else {
			to = strings.TrimSpace(afterRel)
		}

		from = strings.TrimSpace(from)
		to = strings.TrimSpace(to)

		if _, ok := p.classes[from]; !ok {
			p.classes[from] = &classInfo{id: from, styles: make(map[string]string)}
		}
		if _, ok := p.classes[to]; !ok {
			p.classes[to] = &classInfo{id: to, styles: make(map[string]string)}
		}

		label = diagram.CleanLabel(label)

		ri := &relationInfo{
			from:   from,
			to:     to,
			typ:    rel.typ,
			label:  label,
			dashed: rel.dashed,
		}
		p.relations = append(p.relations, ri)
		return true
	}

	return false
}

func (p *parser) render() (*scene.Scene, error) {
	sc := diagram.NewScene(p.th)

	g := &layout.Graph{
		Dir:     p.direction,
		NodeSep: p.cfg.Float("class", "nodeSpacing", 50),
		RankSep: p.cfg.Float("class", "rankSpacing", 50),
	}

	nodeMap := make(map[string]*layout.Node)
	for id, ci := range p.classes {
		w, h := p.measureClass(ci)
		ci.absorbLayout = layout.Node{
			ID:   id,
			W:    w,
			H:    h,
			Data: ci,
		}
		ci.absorbLayout.Clip = layout.ClipRect
		nodeMap[id] = &ci.absorbLayout
		g.Nodes = append(g.Nodes, &ci.absorbLayout)
	}

	for _, rel := range p.relations {
		from := nodeMap[rel.from]
		to := nodeMap[rel.to]
		if from == nil || to == nil {
			continue
		}
		e := &layout.Edge{From: from, To: to}
		if rel.label != "" {
			labelW, labelH := scene.MeasureBlock(rel.label, diagram.Font(p.th, 1), 1.25)
			e.LabelW = labelW
			e.LabelH = labelH
		}
		g.Edges = append(g.Edges, e)
	}

	layout.Layout(g)

	for id, ci := range p.classes {
		n := nodeMap[id]
		p.drawClass(sc, ci, n.X, n.Y)
	}

	for i, rel := range p.relations {
		if i >= len(g.Edges) {
			continue
		}
		e := g.Edges[i]
		if e.From == nil || e.To == nil {
			continue
		}
		p.drawRelation(sc, rel, e)
	}

	return diagram.Finish(sc, p.th, ""), nil
}

func (p *parser) measureClass(ci *classInfo) (float64, float64) {
	fs := p.th.FontSize
	maxW := 0.0
	lineH := fs * 1.25

	nameStr := ci.id
	if ci.label != "" {
		nameStr = ci.label
	}
	w := scene.MeasureText(nameStr, diagram.BoldFont(p.th, 1))
	maxW = math.Max(maxW, w)

	for _, ann := range ci.annotations {
		w := scene.MeasureText("«"+ann+"»", diagram.Font(p.th, 0.85))
		maxW = math.Max(maxW, w)
	}

	for _, mem := range ci.members {
		vis := string(mem.visibility)
		name := mem.name
		if mem.isStatic {
			name = name + "$"
		}
		if mem.isAbstract {
			name = name + "*"
		}
		memStr := vis + name
		if mem.typ != "" {
			memStr += " : " + mem.typ
		}
		w := scene.MeasureText(memStr, diagram.Font(p.th, 1))
		maxW = math.Max(maxW, w)
	}

	boxW := maxW + 16
	h := lineH * 1.5
	if len(ci.annotations) > 0 {
		h += lineH * float64(len(ci.annotations))
	}
	if len(ci.members) > 0 {
		h += lineH * 1.5
		h += lineH * float64(len(ci.members))
	}

	return boxW, h
}

func (p *parser) drawClass(sc *scene.Scene, ci *classInfo, cx, cy float64) {
	fs := p.th.FontSize
	lineH := fs * 1.25
	w := ci.absorbLayout.W
	h := ci.absorbLayout.H
	x := cx - w/2
	y := cy - h/2

	boxStyle := scene.Style{
		Fill:   p.th.PrimaryColor,
		Stroke: p.th.PrimaryBorderColor,
		Shadow: true,
	}
	sc.Add(scene.NewPath(boxStyle).Rect(x, y, w, h, 2))

	curY := y + 8

	for _, ann := range ci.annotations {
		annStr := "«" + strings.ToLower(ann) + "»"
		annFont := diagram.Font(p.th, 0.85)
		sc.Add(scene.NewText(cx, curY, annStr, annFont, p.th.PrimaryTextColor,
			scene.AnchorMiddle, scene.VAlignTop))
		curY += lineH
	}

	nameStr := ci.id
	if ci.label != "" {
		nameStr = ci.label
	}
	nameFont := diagram.BoldFont(p.th, 1)
	sc.Add(scene.NewText(cx, curY, nameStr, nameFont, p.th.PrimaryTextColor,
		scene.AnchorMiddle, scene.VAlignTop))
	curY += lineH + 2

	separatorY := curY
	if len(ci.members) > 0 || len(ci.annotations) > 0 {
		sep := scene.NewPath(scene.Style{Stroke: p.th.PrimaryBorderColor, StrokeWidth: 1})
		sep.MoveTo(x+4, separatorY).LineTo(x+w-4, separatorY)
		sc.Add(sep)
		curY += 6
	}

	if len(ci.members) > 0 {
		memberFont := diagram.Font(p.th, 1)
		for _, mem := range ci.members {
			vis := string(mem.visibility)
			name := mem.name
			suffix := ""
			if mem.isStatic {
				suffix += "$"
			}
			if mem.isAbstract {
				suffix += "*"
			}
			memStr := vis + name + suffix
			if mem.typ != "" {
				memStr += " : " + mem.typ
			}
			sc.Add(scene.NewText(x+8, curY, memStr, memberFont, p.th.PrimaryTextColor,
				scene.AnchorStart, scene.VAlignTop))
			curY += lineH
		}
	}
}

func (p *parser) drawRelation(sc *scene.Scene, rel *relationInfo, e *layout.Edge) {
	if len(e.Points) < 2 {
		return
	}

	endMarker := scene.MarkerNone

	switch rel.typ {
	case relationInheritance:
		endMarker = scene.MarkerTriangleOpen
	case relationComposition:
		endMarker = scene.MarkerDiamondFilled
	case relationAggregation:
		endMarker = scene.MarkerDiamondOpen
	case relationAssociation, relationDependency:
		endMarker = scene.MarkerOpenArrow
	}

	lineStyle := scene.Style{
		Stroke: p.th.LineColor,
		Dash:   nil,
	}
	if rel.dashed {
		lineStyle.Dash = []float64{5, 5}
	}

	edgeOpts := scene.EdgeOpts{
		Style:  lineStyle,
		Curve:  scene.CurveBasis,
		End:    endMarker,
		Marker: scene.MarkerOpts{Size: 10, StrokeWidth: 1.5},
	}
	sc.Add(scene.Edge(e.Points, edgeOpts)...)

	if rel.label != "" {
		labelFont := diagram.Font(p.th, 1)
		sc.Add(scene.NewText(e.Label.X, e.Label.Y, rel.label, labelFont, p.th.TextColor,
			scene.AnchorMiddle, scene.VAlignMiddle))
	}
}
