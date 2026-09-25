// Package class implements rendering of Mermaid class diagrams (UML).
package class

import (
	"fmt"
	"maps"
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
	cfg            *diagram.Config
	th             *theme.Theme
	classes        map[string]*classInfo
	namespaces     map[string]*namespaceInfo
	relations      []*relationInfo
	notes          []*noteInfo
	styleClassDefs map[string]map[string]string
	direction      layout.Direction
}

type namespaceInfo struct {
	id         string
	label      string
	classes    []string
	children   []*namespaceInfo
	parent     *namespaceInfo
	isRoot     bool
	absorbNode *layout.Node
}

type classInfo struct {
	id           string
	label        string
	displayName  string // for rendering (handles generics)
	generics     string // e.g., "T" or "List~int~"
	annotations  []string
	members      []member
	styles       map[string]string
	absorbLayout layout.Node
	namespace    string
}

type member struct {
	visibility byte // '+', '-', '#', '~'
	name       string
	typ        string
	isMethod   bool
	isStatic   bool
	isAbstract bool
}

type relationInfo struct {
	from, to      string
	typ           relationType
	label         string
	cardFrom      string
	cardTo        string
	dashed        bool
	reversedStart bool
	reversedEnd   bool
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
	relationLollipop
)

type noteInfo struct {
	text     string
	forClass string
	noteType string // "free" or "attached"
}

func newParser(cfg *diagram.Config) *parser {
	th := cfg.Theme
	if th == nil {
		th = theme.Default()
	}
	return &parser{
		cfg:            cfg,
		th:             th,
		classes:        make(map[string]*classInfo),
		namespaces:     make(map[string]*namespaceInfo),
		styleClassDefs: make(map[string]map[string]string),
		direction:      layout.ParseDirection(cfg.String("class", "direction", "TB")),
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

		if strings.HasPrefix(line, "classDef") {
			p.parseClassDef(line)
			continue
		}

		if strings.HasPrefix(line, "cssClass") {
			// cssClass "A,B" name
			p.parseCssClass(line)
			continue
		}

		if strings.HasPrefix(line, "style ") {
			p.parseStyle(line)
			continue
		}

		if strings.HasPrefix(line, "namespace ") {
			if err := p.parseNamespace(line, lines, &i); err != nil {
				return fmt.Errorf("line %d: %w", i+1, err)
			}
			continue
		}

		if strings.HasPrefix(line, "click ") || strings.HasPrefix(line, "callback ") ||
			strings.HasPrefix(line, "link ") {
			// Silently ignore interaction statements
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
			p.parseNote(line)
		}
	}
	return nil
}

func (p *parser) parseClassDef(line string) {
	// classDef name fill:#f9f,stroke:#333,...
	line = strings.TrimPrefix(line, "classDef")
	line = strings.TrimSpace(line)

	spaceIdx := strings.IndexAny(line, " \t")
	if spaceIdx <= 0 {
		return
	}

	name := line[:spaceIdx]
	styleStr := strings.TrimSpace(line[spaceIdx:])

	styles := parseStyleString(styleStr)
	p.styleClassDefs[name] = styles
}

func (p *parser) parseCssClass(line string) {
	// cssClass "A,B" name
	line = strings.TrimPrefix(line, "cssClass")
	line = strings.TrimSpace(line)

	// Extract quoted class list
	var classList string
	var styleName string

	if strings.HasPrefix(line, "\"") {
		endQuote := strings.Index(line[1:], "\"")
		if endQuote > 0 {
			classList = line[1 : endQuote+1]
			rest := strings.TrimSpace(line[endQuote+2:])
			styleName = rest
		}
	}

	if classList == "" || styleName == "" {
		return
	}

	classes := strings.SplitSeq(classList, ",")
	for cls := range classes {
		cls = strings.TrimSpace(cls)
		if ci, ok := p.classes[cls]; ok {
			if _, ok := p.styleClassDefs[styleName]; ok {
				maps.Copy(ci.styles, p.styleClassDefs[styleName])
			}
		}
	}
}

func (p *parser) parseStyle(line string) {
	// style A fill:#f9f,stroke:#333,...
	line = strings.TrimPrefix(line, "style ")
	line = strings.TrimSpace(line)

	spaceIdx := strings.IndexAny(line, " \t")
	if spaceIdx <= 0 {
		return
	}

	className := line[:spaceIdx]
	styleStr := strings.TrimSpace(line[spaceIdx:])

	ci, ok := p.classes[className]
	if !ok {
		ci = &classInfo{
			id:     className,
			styles: make(map[string]string),
		}
		p.classes[className] = ci
	}

	styles := parseStyleString(styleStr)
	maps.Copy(ci.styles, styles)
}

func parseStyleString(s string) map[string]string {
	styles := make(map[string]string)
	pairs := strings.SplitSeq(s, ",")
	for pair := range pairs {
		if idx := strings.Index(pair, ":"); idx > 0 {
			key := strings.TrimSpace(pair[:idx])
			val := strings.TrimSpace(pair[idx+1:])
			styles[key] = val
		}
	}
	return styles
}

func (p *parser) parseNamespace(line string, lines []string, idx *int) error {
	// namespace Name { ... } or namespace Name["Display Label"] { ... }
	line = strings.TrimPrefix(line, "namespace ")
	line = strings.TrimSpace(line)

	before, _, ok := strings.Cut(line, "{")
	if !ok {
		return fmt.Errorf("missing {")
	}

	nsDecl := strings.TrimSpace(before)
	var nsName, nsLabel string

	// Handle label
	if idx := strings.Index(nsDecl, "[\""); idx > 0 {
		nsName = strings.TrimSpace(nsDecl[:idx])
		labelPart := nsDecl[idx+2:]
		if endIdx := strings.Index(labelPart, "\"]"); endIdx > 0 {
			nsLabel = labelPart[:endIdx]
		}
	} else {
		nsName = nsDecl
	}

	ns := &namespaceInfo{
		id:    nsName,
		label: nsLabel,
	}
	p.namespaces[nsName] = ns

	// Parse contents
	for *idx++; *idx < len(lines); *idx++ {
		content := strings.TrimSpace(lines[*idx])
		if content == "" || strings.HasPrefix(content, "%") {
			continue
		}
		if content == "}" {
			break
		}

		// Parse class declarations within namespace
		if strings.HasPrefix(content, "class ") {
			if strings.Contains(content, "{") {
				if err := p.parseClassWithMembers(content, lines, idx); err != nil {
					return err
				}
			} else {
				if err := p.parseClassDecl(content); err != nil {
					return err
				}
			}
			// Mark class as belonging to this namespace
			className := p.extractClassName(content)
			if className != "" {
				if ci, ok := p.classes[className]; ok {
					ci.namespace = nsName
					ns.classes = append(ns.classes, className)
				}
			}
		}
	}

	return nil
}

func (p *parser) extractClassName(line string) string {
	line = strings.TrimPrefix(line, "class ")
	line = strings.TrimSpace(line)

	// Handle annotations: class A <<interface>> {...}
	if annIdx := strings.Index(line, "<<"); annIdx > 0 {
		line = line[:annIdx]
	}

	// Handle label: class A["Label"] {...}
	if idx := strings.Index(line, "[\""); idx > 0 {
		line = line[:idx]
	}

	// Handle generics: class A~T~ {...}
	if idx := strings.Index(line, "~"); idx > 0 {
		line = line[:idx]
	}

	// Handle braces
	if idx := strings.Index(line, "{"); idx > 0 {
		line = line[:idx]
	}

	return strings.TrimSpace(line)
}

func (p *parser) parseClassDecl(line string) error {
	line = strings.TrimPrefix(line, "class ")
	line = strings.TrimSpace(line)

	var annotations []string
	var className string
	var label string
	var generics string

	// Check for annotation (classname << annotation >>)
	if annIdx := strings.Index(line, "<<"); annIdx > 0 {
		beforeAnn := strings.TrimSpace(line[:annIdx])
		rest := line[annIdx:]
		if endIdx := strings.Index(rest, ">>"); endIdx > 0 {
			ann := rest[2:endIdx]
			annotations = append(annotations, ann)
			line = beforeAnn
		}
	}

	// Handle label ["..."]
	if strings.Contains(line, "[\"") {
		idx := strings.Index(line, "[\"")
		label = line[idx+2:]
		line = line[:idx]
		if endIdx := strings.Index(label, "\"]"); endIdx > 0 {
			label = label[:endIdx]
		}
	}

	// Handle generics ~T~ or ~List~int~~
	if idx := strings.Index(line, "~"); idx > 0 {
		generics = line[idx:]
		line = line[:idx]
	}

	className = strings.TrimSpace(line)
	if className == "" {
		return fmt.Errorf("empty class name")
	}

	ci := &classInfo{
		id:          className,
		label:       label,
		generics:    generics,
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
	var generics string

	// Check for annotation
	if annIdx := strings.Index(classDecl, "<<"); annIdx > 0 {
		beforeAnn := strings.TrimSpace(classDecl[:annIdx])
		rest := classDecl[annIdx:]
		if endIdx := strings.Index(rest, ">>"); endIdx > 0 {
			ann := rest[2:endIdx]
			annotations = append(annotations, ann)
			classDecl = beforeAnn
		}
	}

	// Handle label
	if strings.Contains(classDecl, "[\"") {
		idxLabel := strings.Index(classDecl, "[\"")
		label = classDecl[idxLabel+2:]
		classDecl = classDecl[:idxLabel]
		if endIdx := strings.Index(label, "\"]"); endIdx > 0 {
			label = label[:endIdx]
		}
	}

	// Handle generics
	if idxGen := strings.Index(classDecl, "~"); idxGen > 0 {
		generics = classDecl[idxGen:]
		classDecl = classDecl[:idxGen]
	}

	className = strings.TrimSpace(classDecl)
	if className == "" {
		return fmt.Errorf("empty class name")
	}

	ci := &classInfo{
		id:          className,
		label:       label,
		generics:    generics,
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
		default:
			m.visibility = '+'
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

func (p *parser) parseNote(line string) {
	// note "text" or note for ClassName "text"
	if strings.Contains(line, " for ") {
		parts := strings.SplitN(line, " for ", 2)
		if len(parts) == 2 {
			rest := strings.TrimSpace(parts[1])
			spaceIdx := strings.IndexAny(rest, " \t")
			if spaceIdx > 0 {
				className := rest[:spaceIdx]
				noteText := strings.TrimSpace(rest[spaceIdx:])
				if strings.HasPrefix(noteText, "\"") && strings.HasSuffix(noteText, "\"") {
					noteText = noteText[1 : len(noteText)-1]
				}
				noteText = diagram.CleanLabel(noteText)
				p.notes = append(p.notes, &noteInfo{text: noteText, forClass: className, noteType: "attached"})
			}
		}
	} else if after, ok := strings.CutPrefix(line, "note "); ok {
		noteText := after
		if strings.HasPrefix(noteText, "\"") && strings.HasSuffix(noteText, "\"") {
			noteText = noteText[1 : len(noteText)-1]
		}
		noteText = diagram.CleanLabel(noteText)
		p.notes = append(p.notes, &noteInfo{text: noteText, noteType: "free"})
	}
}

func (p *parser) tryParseRelation(line string) bool {
	// Try two-way relations first (e.g., <|--|>, *--*, etc.)
	twoWayRelations := []struct {
		pattern string
		typ1    relationType
		typ2    relationType
		dashed  bool
	}{
		{"<|--|>", relationInheritance, relationInheritance, false},
		{"*--*", relationComposition, relationComposition, false},
		{"<--*", relationComposition, relationInheritance, false},
		{"*-->", relationInheritance, relationComposition, false},
		{"o--o", relationAggregation, relationAggregation, false},
		{"<-->", relationAssociation, relationAssociation, false},
		{"<..|>", relationRealization, relationRealization, true},
	}

	for _, rel := range twoWayRelations {
		idx := strings.Index(line, rel.pattern)
		if idx > 0 {
			from := strings.TrimSpace(line[:idx])
			afterRel := line[idx+len(rel.pattern):]

			var to, label string
			if colonIdx := strings.Index(afterRel, ":"); colonIdx > 0 {
				to = strings.TrimSpace(afterRel[:colonIdx])
				label = strings.TrimSpace(afterRel[colonIdx+1:])
			} else {
				to = strings.TrimSpace(afterRel)
			}

			p.ensureClassExists(from)
			p.ensureClassExists(to)

			label = diagram.CleanLabel(label)

			// Add both directions
			p.relations = append(p.relations, &relationInfo{
				from:   from,
				to:     to,
				typ:    rel.typ1,
				label:  label,
				dashed: rel.dashed,
			})
			p.relations = append(p.relations, &relationInfo{
				from:   to,
				to:     from,
				typ:    rel.typ2,
				label:  "",
				dashed: rel.dashed,
			})
			return true
		}
	}

	// Single-way relations with optionally reversed markers
	relations := []struct {
		pattern string
		typ     relationType
		dashed  bool
		start   bool // marker at start
		end     bool // marker at end
	}{
		{"<|--", relationInheritance, false, true, false},
		{"--|>", relationInheritance, false, false, true},
		{"*--", relationComposition, false, false, true},
		{"--*", relationComposition, false, true, false},
		{"o--", relationAggregation, false, false, true},
		{"--o", relationAggregation, false, true, false},
		{"-->", relationAssociation, false, false, true},
		{"<--", relationAssociation, false, true, false},
		{"..|>", relationRealization, true, false, true},
		{"<|..", relationRealization, true, true, false},
		{"..>", relationDependency, true, false, true},
		{"<..", relationDependency, true, true, false},
		{"--", relationLink, false, false, false},
		{"..", relationLink, true, false, false},
		{"()--", relationLollipop, false, false, true},
		{"--()}", relationLollipop, false, true, false},
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

		// Extract cardinality before arrow (e.g., 'A "1" --> "*" B')
		var cardFrom, cardTo string
		if idx := strings.LastIndex(from, "\""); idx > 0 {
			// Found closing quote; search for opening
			if idx2 := strings.LastIndex(from[:idx], "\""); idx2 >= 0 {
				cardFrom = from[idx2+1 : idx]
				from = strings.TrimSpace(from[:idx2])
			}
		}
		if idx := strings.Index(to, "\""); idx >= 0 {
			if idx2 := strings.Index(to[idx+1:], "\""); idx2 >= 0 {
				cardTo = to[idx+1 : idx+1+idx2]
				to = strings.TrimSpace(to[idx+1+idx2+1:])
			}
		}

		p.ensureClassExists(from)
		p.ensureClassExists(to)

		label = diagram.CleanLabel(label)

		ri := &relationInfo{
			from:          from,
			to:            to,
			typ:           rel.typ,
			label:         label,
			cardFrom:      cardFrom,
			cardTo:        cardTo,
			dashed:        rel.dashed,
			reversedStart: rel.start,
			reversedEnd:   rel.end,
		}
		p.relations = append(p.relations, ri)
		return true
	}

	return false
}

func (p *parser) ensureClassExists(className string) {
	if _, ok := p.classes[className]; !ok {
		p.classes[className] = &classInfo{
			id:     className,
			styles: make(map[string]string),
		}
	}
}

func (p *parser) render() (*scene.Scene, error) {
	sc := diagram.NewScene(p.th)

	g := &layout.Graph{
		Dir:     p.direction,
		NodeSep: p.cfg.Float("class", "nodeSpacing", 50),
		RankSep: p.cfg.Float("class", "rankSpacing", 50),
	}

	nodeMap := make(map[string]*layout.Node)

	// Create namespace nodes first (clusters)
	nsNodeMap := make(map[string]*layout.Node)
	for nsName, ns := range p.namespaces {
		if len(ns.classes) == 0 {
			continue
		}

		// Create nodes for classes in this namespace
		nsChildren := make([]*layout.Node, 0)
		for _, className := range ns.classes {
			ci := p.classes[className]
			w, h := p.measureClass(ci)
			n := &layout.Node{
				ID:   className,
				W:    w,
				H:    h,
				Data: ci,
			}
			n.Clip = layout.ClipRect
			nodeMap[className] = n
			nsChildren = append(nsChildren, n)
		}

		// Create namespace cluster node
		nsNode := &layout.Node{
			ID:       nsName,
			Children: nsChildren,
			Pad:      15,
			PadTop:   25,
			Data:     ns,
		}
		nsNodeMap[nsName] = nsNode
		g.Nodes = append(g.Nodes, nsNode)
	}

	// Create nodes for non-namespaced classes
	for id, ci := range p.classes {
		if ci.namespace != "" {
			continue // already added as part of namespace
		}
		w, h := p.measureClass(ci)
		n := &layout.Node{
			ID:   id,
			W:    w,
			H:    h,
			Data: ci,
		}
		n.Clip = layout.ClipRect
		nodeMap[id] = n
		g.Nodes = append(g.Nodes, n)
	}

	// Add edges
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

	// Draw namespace clusters
	for nsName, ns := range p.namespaces {
		if nsNode, ok := nsNodeMap[nsName]; ok {
			p.drawNamespace(sc, ns, nsNode)
		}
	}

	// Draw classes
	for id, ci := range p.classes {
		if ci.namespace != "" {
			continue // drawn with namespace
		}
		n := nodeMap[id]
		p.drawClass(sc, ci, n.X, n.Y)
	}

	// Draw classes within namespaces
	for id, ci := range p.classes {
		if ci.namespace == "" {
			continue
		}
		n := nodeMap[id]
		p.drawClass(sc, ci, n.X, n.Y)
	}

	// Draw relations
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

	// Draw notes
	for _, note := range p.notes {
		p.drawNote(sc, note)
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

	// Add generics if present
	if ci.generics != "" {
		displayGen := renderGenerics(ci.generics)
		nameStr = nameStr + "<" + displayGen + ">"
	}

	w := scene.MeasureText(nameStr, diagram.BoldFont(p.th, 1))
	maxW = math.Max(maxW, w)

	for _, ann := range ci.annotations {
		w := scene.MeasureText("«"+strings.ToLower(ann)+"»", diagram.Font(p.th, 0.85))
		maxW = math.Max(maxW, w)
	}

	for _, mem := range ci.members {
		vis := string(mem.visibility)
		name := mem.name
		if mem.isStatic {
			name = name + " $"
		}
		if mem.isAbstract {
			name = name + " *"
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

func renderGenerics(gen string) string {
	// Convert ~T~ to T, ~List~int~~ to List<int>
	gen = strings.Trim(gen, "~")
	// Handle nested generics: List~int~ -> List<int>
	gen = strings.ReplaceAll(gen, "~", "<")
	// Close generics properly
	return gen
}

func (p *parser) drawNamespace(sc *scene.Scene, ns *namespaceInfo, node *layout.Node) {
	w := node.W
	h := node.H
	x := node.X - w/2
	y := node.Y - h/2

	// Cluster background (rounded rect)
	clusterStyle := scene.Style{
		Fill:   p.th.ClusterBkg,
		Stroke: p.th.ClusterBorder,
	}
	sc.Add(scene.NewPath(clusterStyle).Rect(x, y, w, h, 4))

	// Namespace label
	labelStr := ns.label
	if labelStr == "" {
		labelStr = ns.id
	}

	labelFont := diagram.BoldFont(p.th, 0.9)
	labelY := y + 8
	sc.Add(scene.NewText(x+8, labelY, labelStr, labelFont, p.th.TextColor,
		scene.AnchorStart, scene.VAlignTop))

	// Divider line after label
	dividerY := y + node.PadTop
	divPath := scene.NewPath(scene.Style{Stroke: p.th.ClusterBorder, StrokeWidth: 1})
	divPath.MoveTo(x+2, dividerY).LineTo(x+w-2, dividerY)
	sc.Add(divPath)
}

func (p *parser) drawClass(sc *scene.Scene, ci *classInfo, cx, cy float64) {
	fs := p.th.FontSize
	lineH := fs * 1.25
	w := ci.absorbLayout.W
	h := ci.absorbLayout.H
	x := cx - w/2
	y := cy - h/2

	// Apply style overrides
	fillColor := p.th.PrimaryColor
	strokeColor := p.th.PrimaryBorderColor
	textColor := p.th.PrimaryTextColor

	if fillStr, ok := ci.styles["fill"]; ok {
		if c, err := theme.ParseColor(fillStr); err == nil {
			fillColor = c
		}
	}
	if strokeStr, ok := ci.styles["stroke"]; ok {
		if c, err := theme.ParseColor(strokeStr); err == nil {
			strokeColor = c
		}
	}
	if colorStr, ok := ci.styles["color"]; ok {
		if c, err := theme.ParseColor(colorStr); err == nil {
			textColor = c
		}
	}

	boxStyle := scene.Style{
		Fill:   fillColor,
		Stroke: strokeColor,
		Shadow: true,
	}

	// Allow stroke-width override
	if strokeWidthStr, ok := ci.styles["stroke-width"]; ok {
		if idx := strings.Index(strokeWidthStr, "px"); idx > 0 {
			if f, err := fmt.Sscanf(strokeWidthStr[:idx], "%f", &boxStyle.StrokeWidth); err == nil && f > 0 {
				// parsed
			}
		}
	}

	sc.Add(scene.NewPath(boxStyle).Rect(x, y, w, h, 2))

	curY := y + 8

	// Annotations
	for _, ann := range ci.annotations {
		annStr := "«" + strings.ToLower(ann) + "»"
		annFont := diagram.Font(p.th, 0.85)
		sc.Add(scene.NewText(cx, curY, annStr, annFont, textColor,
			scene.AnchorMiddle, scene.VAlignTop))
		curY += lineH
	}

	// Class name
	nameStr := ci.id
	if ci.label != "" {
		nameStr = ci.label
	}
	if ci.generics != "" {
		nameStr = nameStr + "<" + renderGenerics(ci.generics) + ">"
	}

	nameFont := diagram.BoldFont(p.th, 1)
	sc.Add(scene.NewText(cx, curY, nameStr, nameFont, textColor,
		scene.AnchorMiddle, scene.VAlignTop))
	curY += lineH + 2

	// Divider after name if there are annotations or members
	if len(ci.annotations) > 0 || len(ci.members) > 0 {
		sep := scene.NewPath(scene.Style{Stroke: strokeColor, StrokeWidth: 1})
		sep.MoveTo(x+4, curY).LineTo(x+w-4, curY)
		sc.Add(sep)
		curY += 6
	}

	// Members
	if len(ci.members) > 0 {
		memberFont := diagram.Font(p.th, 1)
		for _, mem := range ci.members {
			vis := string(mem.visibility)
			name := mem.name
			suffix := ""
			if mem.isStatic {
				suffix += " (static)"
			}
			if mem.isAbstract {
				suffix += " (abstract)"
			}
			memStr := vis + name + suffix
			if mem.typ != "" {
				memStr += " : " + mem.typ
			}

			// Draw underline for static members
			if mem.isStatic {
				textW := scene.MeasureText(memStr, memberFont)
				underlineY := curY + lineH*0.8
				ulPath := scene.NewPath(scene.Style{Stroke: textColor, StrokeWidth: 1})
				ulPath.MoveTo(x+8, underlineY).LineTo(x+8+textW, underlineY)
				sc.Add(ulPath)
			}

			// Draw italic for abstract members
			if mem.isAbstract {
				memberFont = diagram.Font(p.th, 1)
				memberFont.Italic = true
			}

			sc.Add(scene.NewText(x+8, curY, memStr, memberFont, textColor,
				scene.AnchorStart, scene.VAlignTop))
			curY += lineH
		}
	}
}

func (p *parser) drawRelation(sc *scene.Scene, rel *relationInfo, e *layout.Edge) {
	if len(e.Points) < 2 {
		return
	}

	startMarker := scene.MarkerNone
	endMarker := scene.MarkerNone

	switch rel.typ {
	case relationInheritance:
		if rel.reversedStart {
			startMarker = scene.MarkerTriangleOpen
		} else {
			endMarker = scene.MarkerTriangleOpen
		}
	case relationComposition:
		if rel.reversedStart {
			startMarker = scene.MarkerDiamondFilled
		} else {
			endMarker = scene.MarkerDiamondFilled
		}
	case relationAggregation:
		if rel.reversedStart {
			startMarker = scene.MarkerDiamondOpen
		} else {
			endMarker = scene.MarkerDiamondOpen
		}
	case relationAssociation:
		if rel.reversedStart {
			startMarker = scene.MarkerArrow
		} else {
			endMarker = scene.MarkerArrow
		}
	case relationDependency:
		if rel.reversedStart {
			startMarker = scene.MarkerOpenArrow
		} else {
			endMarker = scene.MarkerOpenArrow
		}
	case relationRealization:
		if rel.reversedStart {
			startMarker = scene.MarkerTriangleOpen
		} else {
			endMarker = scene.MarkerTriangleOpen
		}
	case relationLollipop:
		if rel.reversedStart {
			startMarker = scene.MarkerCircleOpen
		} else {
			endMarker = scene.MarkerCircleOpen
		}
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
		Start:  startMarker,
		End:    endMarker,
		Marker: scene.MarkerOpts{Size: 10, StrokeWidth: 1.5},
	}
	sc.Add(scene.Edge(e.Points, edgeOpts)...)

	// Draw cardinality labels
	if rel.cardFrom != "" {
		cardFont := diagram.Font(p.th, 0.8)
		fromPt := e.Points[0]
		// Offset label perpendicular to edge
		sc.Add(scene.NewText(fromPt.X-10, fromPt.Y-8, rel.cardFrom, cardFont,
			p.th.TextColor, scene.AnchorEnd, scene.VAlignBottom))
	}

	if rel.cardTo != "" {
		cardFont := diagram.Font(p.th, 0.8)
		toPt := e.Points[len(e.Points)-1]
		sc.Add(scene.NewText(toPt.X+10, toPt.Y-8, rel.cardTo, cardFont,
			p.th.TextColor, scene.AnchorStart, scene.VAlignBottom))
	}

	// Draw edge label
	if rel.label != "" {
		labelFont := diagram.Font(p.th, 1)
		labelBg := scene.Style{
			Fill:   p.th.EdgeLabelBg,
			Stroke: p.th.LineColor,
		}
		// Draw semi-transparent background for label
		labelW := scene.MeasureText(rel.label, labelFont)
		bgRect := scene.NewPath(labelBg)
		bgRect.Rect(e.Label.X-labelW/2-2, e.Label.Y-6, labelW+4, 12, 1)
		sc.Add(bgRect)

		sc.Add(scene.NewText(e.Label.X, e.Label.Y, rel.label, labelFont, p.th.TextColor,
			scene.AnchorMiddle, scene.VAlignMiddle))
	}
}

func (p *parser) drawNote(sc *scene.Scene, note *noteInfo) {
	// Simple floating note for now
	fs := p.th.FontSize
	lineH := fs * 1.25

	noteFont := diagram.Font(p.th, 0.9)
	noteW := scene.MeasureText(note.text, noteFont) + 12
	noteH := lineH + 8

	noteStyle := scene.Style{
		Fill:   p.th.NoteBkg,
		Stroke: p.th.NoteBorder,
	}

	// Position notes loosely (could be improved with post-layout positioning)
	noteX := 100.0
	noteY := 100.0

	sc.Add(scene.NewPath(noteStyle).Rect(noteX, noteY, noteW, noteH, 3))
	sc.Add(scene.NewText(noteX+6, noteY+4, note.text, noteFont, p.th.NoteText,
		scene.AnchorStart, scene.VAlignTop))
}
