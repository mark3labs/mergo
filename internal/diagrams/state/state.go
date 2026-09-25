// Package state implements rendering of Mermaid state diagrams.
package state

import (
	"fmt"
	"maps"
	"math"
	"regexp"
	"strings"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/layout"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

func init() {
	diagram.Register(diagram.Type{
		Name:   "state",
		Detect: diagram.Keyword("stateDiagram", "stateDiagram-v2"),
		Render: Render,
	})
}

// Render parses and renders a state diagram.
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
	states         map[string]*stateInfo
	topLevel       []*stateInfo // top-level state order
	startState     *stateInfo   // [*] at top level
	endState       *stateInfo   // [*] end at top level
	relations      []*transitionInfo
	notes          []*noteInfo
	styleClassDefs map[string]map[string]string
	direction      layout.Direction
	lineno         int
}

type stateInfo struct {
	id             string
	description    string
	children       []*stateInfo // for composite states
	isComposite    bool
	isStart        bool
	isEnd          bool
	isChoice       bool
	isFork         bool
	isJoin         bool
	childDirection *layout.Direction // direction inside this composite
	startState     *stateInfo        // [*] within this composite
	endState       *stateInfo        // [*] end within this composite
	concurrencyIdx int               // region index for concurrency (0 = no concurrency)
	styles         map[string]string
	absorbNode     *layout.Node
	parent         *stateInfo
	isInternal     bool // created as part of composite parsing
}

type transitionInfo struct {
	from, to string
	label    string
}

type noteInfo struct {
	text     string
	forState string
	position string // "left" or "right"
}

func newParser(cfg *diagram.Config) *parser {
	th := cfg.Theme
	if th == nil {
		th = theme.Default()
	}
	p := &parser{
		cfg:            cfg,
		th:             th,
		states:         make(map[string]*stateInfo),
		styleClassDefs: make(map[string]map[string]string),
		direction:      layout.TB,
	}
	p.direction = layout.ParseDirection(cfg.String("state", "direction", "TB"))
	return p
}

func (p *parser) parse(src string) error {
	lines := diagram.Lines(src)

	for i := 0; i < len(lines); i++ {
		p.lineno = i + 1
		line := lines[i]

		// Direction statement
		if strings.HasPrefix(line, "direction") {
			parts := strings.Fields(line)
			if len(parts) > 1 {
				p.direction = layout.ParseDirection(parts[1])
			}
			continue
		}

		// Hide empty description
		if strings.HasPrefix(line, "hide empty description") {
			continue
		}

		// Comments
		if strings.HasPrefix(line, "%%") {
			continue
		}

		// classDef
		if strings.HasPrefix(line, "classDef") {
			p.parseClassDef(line)
			continue
		}

		// class (cssClass)
		if strings.HasPrefix(line, "class ") {
			p.parseCssClass(line)
			continue
		}

		// style
		if strings.HasPrefix(line, "style ") {
			p.parseStyle(line)
			continue
		}

		// State with composite block
		if strings.HasPrefix(line, "state ") && strings.Contains(line, "{") {
			si, err := p.parseCompositeState(line, lines, &i)
			if err != nil {
				return fmt.Errorf("line %d: %w", p.lineno, err)
			}
			if si != nil {
				p.topLevel = append(p.topLevel, si)
			}
			continue
		}

		// state "description" as id
		if strings.HasPrefix(line, "state ") && strings.Contains(line, " as ") {
			parts := strings.SplitN(line, " as ", 2)
			if len(parts) == 2 {
				stateDecl := strings.TrimSpace(strings.TrimPrefix(parts[0], "state "))
				id := strings.TrimSpace(parts[1])
				stateDecl = diagram.CleanLabel(stateDecl)
				if _, ok := p.states[id]; !ok {
					si := &stateInfo{
						id:          id,
						description: stateDecl,
						styles:      make(map[string]string),
					}
					p.states[id] = si
					p.topLevel = append(p.topLevel, si)
				}
			}
			continue
		}

		// state id : description
		if after, ok := strings.CutPrefix(line, "state "); ok {
			after := after
			after = strings.TrimSpace(after)
			id := after

			// Handle annotations
			if strings.Contains(id, "<<") && strings.Contains(id, ">>") {
				endAnn := strings.Index(id, ">>")
				annPart := id[:endAnn+2]
				id = strings.TrimSpace(id[endAnn+2:])

				si, ok := p.states[id]
				if !ok {
					si = &stateInfo{
						id:     id,
						styles: make(map[string]string),
					}
					p.states[id] = si
					p.topLevel = append(p.topLevel, si)
				}

				// Parse annotation
				if strings.Contains(annPart, "<<choice>>") {
					si.isChoice = true
				} else if strings.Contains(annPart, "<<fork>>") {
					si.isFork = true
				} else if strings.Contains(annPart, "<<join>>") {
					si.isJoin = true
				}
			} else {
				if _, ok := p.states[id]; !ok {
					si := &stateInfo{
						id:     id,
						styles: make(map[string]string),
					}
					p.states[id] = si
					p.topLevel = append(p.topLevel, si)
				}
			}
			continue
		}

		// id : description
		if strings.Contains(line, " : ") && !strings.Contains(line, "-->") {
			parts := strings.SplitN(line, " : ", 2)
			if len(parts) == 2 {
				id := strings.TrimSpace(parts[0])
				desc := strings.TrimSpace(parts[1])
				desc = diagram.CleanLabel(desc)
				if si, ok := p.states[id]; ok {
					si.description = desc
				} else {
					si := &stateInfo{
						id:          id,
						description: desc,
						styles:      make(map[string]string),
					}
					p.states[id] = si
					p.topLevel = append(p.topLevel, si)
				}
			}
			continue
		}

		// Transition
		if strings.Contains(line, "-->") {
			p.parseTransition(line)
			continue
		}

		// Note
		if strings.HasPrefix(line, "note ") {
			p.parseNote(line)
			continue
		}

		// Special state markers
		if strings.Contains(line, "<<choice>>") {
			p.parseSpecialMarker(line, "choice")
			continue
		}
		if strings.Contains(line, "<<fork>>") {
			p.parseSpecialMarker(line, "fork")
			continue
		}
		if strings.Contains(line, "<<join>>") {
			p.parseSpecialMarker(line, "join")
			continue
		}
	}

	return nil
}

func (p *parser) parseClassDef(line string) {
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
	line = strings.TrimPrefix(line, "class ")
	line = strings.TrimSpace(line)

	// cssClass "A,B" name or cssClass A,B name
	var classList, styleName string

	if strings.HasPrefix(line, "\"") {
		endQuote := strings.Index(line[1:], "\"")
		if endQuote > 0 {
			classList = line[1 : endQuote+1]
			rest := strings.TrimSpace(line[endQuote+2:])
			styleName = rest
		}
	} else {
		spaceIdx := strings.IndexAny(line, " \t")
		if spaceIdx > 0 {
			classList = line[:spaceIdx]
			styleName = strings.TrimSpace(line[spaceIdx:])
		}
	}

	if classList == "" || styleName == "" {
		return
	}

	states := strings.SplitSeq(classList, ",")
	for stateID := range states {
		stateID = strings.TrimSpace(stateID)
		if si, ok := p.states[stateID]; ok {
			if styleDef, ok := p.styleClassDefs[styleName]; ok {
				maps.Copy(si.styles, styleDef)
			}
		}
	}
}

func (p *parser) parseStyle(line string) {
	line = strings.TrimPrefix(line, "style ")
	line = strings.TrimSpace(line)

	spaceIdx := strings.IndexAny(line, " \t")
	if spaceIdx <= 0 {
		return
	}

	stateID := line[:spaceIdx]
	styleStr := strings.TrimSpace(line[spaceIdx:])

	si, ok := p.states[stateID]
	if !ok {
		si = &stateInfo{
			id:     stateID,
			styles: make(map[string]string),
		}
		p.states[stateID] = si
	}

	styles := parseStyleString(styleStr)
	maps.Copy(si.styles, styles)
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

func (p *parser) parseCompositeState(line string, lines []string, idx *int) (*stateInfo, error) {
	// state Name { ... } or state "Description" as Name { ... }
	before, _, ok := strings.Cut(line, "{")
	if !ok {
		return nil, fmt.Errorf("missing {")
	}

	stateDecl := strings.TrimPrefix(before, "state ")
	stateDecl = strings.TrimSpace(stateDecl)
	var id, desc string

	// Handle "desc" as id
	if strings.Contains(stateDecl, " as ") {
		parts := strings.SplitN(stateDecl, " as ", 2)
		desc = diagram.CleanLabel(strings.TrimSpace(parts[0]))
		id = strings.TrimSpace(parts[1])
	} else if strings.HasPrefix(stateDecl, "\"") && strings.Contains(stateDecl, "\"") {
		endQuote := strings.LastIndex(stateDecl, "\"")
		desc = stateDecl[1:endQuote]
		rest := strings.TrimSpace(stateDecl[endQuote+1:])
		if strings.HasPrefix(rest, "as ") {
			id = strings.TrimSpace(rest[3:])
		} else {
			id = desc
		}
		desc = diagram.CleanLabel(desc)
	} else {
		id = stateDecl
	}

	if id == "" {
		id = "composite_" + fmt.Sprintf("%d", *idx)
	}

	si := &stateInfo{
		id:          id,
		description: desc,
		isComposite: true,
		styles:      make(map[string]string),
	}

	var children []*stateInfo
	var concurrencyRegions []int

	// Parse composite contents
	for *idx++; *idx < len(lines); *idx++ {
		content := strings.TrimSpace(lines[*idx])

		if content == "" || strings.HasPrefix(content, "%%") {
			continue
		}

		if content == "}" {
			break
		}

		// Concurrency separator
		if content == "--" {
			concurrencyRegions = append(concurrencyRegions, len(children))
			continue
		}

		// State declarations within composite
		if strings.HasPrefix(content, "state ") && strings.Contains(content, "{") {
			childSi, err := p.parseCompositeState(content, lines, idx)
			if err != nil {
				return nil, err
			}
			if childSi != nil {
				childSi.parent = si
				childSi.isInternal = true
				children = append(children, childSi)
				p.states[childSi.id] = childSi
			}
			continue
		}

		if after, ok0 := strings.CutPrefix(content, "state "); ok0 {
			childID := strings.TrimSpace(after)
			if childID != "" && childID != "[*]" {
				if _, ok := p.states[childID]; !ok {
					childSi := &stateInfo{
						id:         childID,
						styles:     make(map[string]string),
						parent:     si,
						isInternal: true,
					}
					p.states[childID] = childSi
					children = append(children, childSi)
				}
			}
			continue
		}

		// [*] start/end within composite
		if strings.HasPrefix(content, "[*]") {
			startState := &stateInfo{
				id:         fmt.Sprintf("%s__start__", id),
				isStart:    true,
				parent:     si,
				isInternal: true,
				styles:     make(map[string]string),
			}
			si.startState = startState
			p.states[startState.id] = startState
			children = append(children, startState)
			continue
		}

		// Transitions within composite (simplified parsing)
		if strings.Contains(content, "-->") {
			p.parseTransition(content)
			continue
		}

		// Simple state ID
		if content != "" && !strings.HasPrefix(content, "[*]") {
			if _, ok := p.states[content]; !ok {
				childSi := &stateInfo{
					id:         content,
					styles:     make(map[string]string),
					parent:     si,
					isInternal: true,
				}
				p.states[content] = childSi
				children = append(children, childSi)
			}
		}
	}

	si.children = children
	p.states[id] = si
	return si, nil
}

func (p *parser) parseTransition(line string) {
	parts := strings.SplitN(line, "-->", 2)
	if len(parts) != 2 {
		return
	}

	from := strings.TrimSpace(parts[0])
	rest := strings.TrimSpace(parts[1])

	// Extract label
	var to, label string
	if colonIdx := strings.Index(rest, ":"); colonIdx > 0 {
		to = strings.TrimSpace(rest[:colonIdx])
		label = strings.TrimSpace(rest[colonIdx+1:])
		label = diagram.CleanLabel(label)
	} else {
		to = rest
	}

	// Handle [*] start/end
	if from == "[*]" {
		from = "__start__"
		if _, ok := p.states[from]; !ok {
			p.states[from] = &stateInfo{
				id:      from,
				isStart: true,
				styles:  make(map[string]string),
			}
			if p.startState == nil {
				p.startState = p.states[from]
			}
		}
	} else if _, ok := p.states[from]; !ok {
		p.states[from] = &stateInfo{
			id:     from,
			styles: make(map[string]string),
		}
	}

	if to == "[*]" {
		to = "__end__"
		if _, ok := p.states[to]; !ok {
			p.states[to] = &stateInfo{
				id:     to,
				isEnd:  true,
				styles: make(map[string]string),
			}
			if p.endState == nil {
				p.endState = p.states[to]
			}
		}
	} else if _, ok := p.states[to]; !ok {
		p.states[to] = &stateInfo{
			id:     to,
			styles: make(map[string]string),
		}
	}

	p.relations = append(p.relations, &transitionInfo{
		from:  from,
		to:    to,
		label: label,
	})
}

func (p *parser) parseNote(line string) {
	// note left of State : text
	// note right of State : text
	// note "text" (free note)

	pattern := regexp.MustCompile(`note\s+(left|right)\s+of\s+(\w+)\s*:\s*(.*)`)
	match := pattern.FindStringSubmatch(line)
	if match != nil {
		p.notes = append(p.notes, &noteInfo{
			position: match[1],
			forState: match[2],
			text:     diagram.CleanLabel(match[3]),
		})
	}
}

func (p *parser) parseSpecialMarker(line string, markerType string) {
	// state_id <<choice>>
	pattern := regexp.MustCompile(`(\w+)\s+<<` + markerType + `>>`)
	match := pattern.FindStringSubmatch(line)
	if len(match) > 1 {
		stateID := match[1]
		if si, ok := p.states[stateID]; ok {
			switch markerType {
			case "choice":
				si.isChoice = true
			case "fork":
				si.isFork = true
			case "join":
				si.isJoin = true
			}
		} else {
			si := &stateInfo{
				id:     stateID,
				styles: make(map[string]string),
			}
			switch markerType {
			case "choice":
				si.isChoice = true
			case "fork":
				si.isFork = true
			case "join":
				si.isJoin = true
			}
			p.states[stateID] = si
			p.topLevel = append(p.topLevel, si)
		}
	}
}

func (p *parser) render() (*scene.Scene, error) {
	sc := diagram.NewScene(p.th)

	// Build layout graph
	g := &layout.Graph{
		Dir:     p.direction,
		NodeSep: p.cfg.Float("state", "nodeSpacing", 50),
		RankSep: p.cfg.Float("state", "rankSpacing", 50),
	}

	// Create nodes
	nodeMap := make(map[string]*layout.Node)
	for id, si := range p.states {
		// Skip internal states (they're handled via clusters)
		if si.isInternal && si.parent != nil && si.parent.isComposite {
			continue
		}

		w, h := p.measureState(si)
		n := &layout.Node{
			ID:   id,
			W:    w,
			H:    h,
			Data: si,
		}

		// Set appropriate clip function
		if si.isChoice {
			n.Clip = layout.ClipDiamond
		} else if si.isStart || si.isEnd {
			n.Clip = layout.ClipEllipse
		} else {
			n.Clip = layout.ClipRect
		}

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

	// Draw states
	for id, si := range p.states {
		if si.isInternal && si.parent != nil {
			continue
		}

		n := nodeMap[id]
		if n != nil {
			p.drawState(sc, si, n.X, n.Y, n.W, n.H)
		}
	}

	// Draw transitions
	for i, rel := range p.relations {
		if i >= len(g.Edges) {
			continue
		}
		e := g.Edges[i]
		if e.From != nil && e.To != nil {
			p.drawTransition(sc, rel, e)
		}
	}

	// Draw notes
	for _, note := range p.notes {
		p.drawNote(sc, note)
	}

	return diagram.Finish(sc, p.th, ""), nil
}

func (p *parser) measureState(si *stateInfo) (float64, float64) {
	fs := p.th.FontSize
	lineH := fs * 1.25
	minW := 80.0

	// Special shapes
	if si.isStart || si.isEnd {
		// Circle
		return 20, 20
	}

	if si.isChoice {
		// Diamond
		return 40, 40
	}

	if si.isFork || si.isJoin {
		// Horizontal bar
		return 60, 12
	}

	// Regular state
	h := lineH + 8
	w := minW

	if si.description != "" {
		nameW := scene.MeasureText(si.id, diagram.Font(p.th, 1))
		descW := scene.MeasureText(si.description, diagram.Font(p.th, 0.9))
		w = math.Max(nameW, descW) + 16
		h += lineH
	} else {
		nameW := scene.MeasureText(si.id, diagram.Font(p.th, 1))
		w = math.Max(w, nameW+16)
	}

	return w, h
}

func (p *parser) drawState(sc *scene.Scene, si *stateInfo, cx, cy, w, h float64) {
	fs := p.th.FontSize
	lineH := fs * 1.25

	if si.isStart {
		// Filled circle
		sc.Add(scene.NewPath(scene.Style{
			Fill:   p.th.LineColor,
			Stroke: p.th.PrimaryBorderColor,
		}).Circle(cx, cy, 8))
		return
	}

	if si.isEnd {
		// Bullseye circle
		sc.Add(scene.NewPath(scene.Style{
			Fill:        p.th.Background,
			Stroke:      p.th.LineColor,
			StrokeWidth: 2,
		}).Circle(cx, cy, 8))
		sc.Add(scene.NewPath(scene.Style{
			Fill:   p.th.LineColor,
			Stroke: p.th.LineColor,
		}).Circle(cx, cy, 4))
		return
	}

	if si.isChoice {
		// Diamond
		r := 20.0
		sc.Add(scene.NewPath(scene.Style{
			Fill:   p.th.PrimaryColor,
			Stroke: p.th.PrimaryBorderColor,
			Shadow: true,
		}).Polygon(
			scene.Pt(cx, cy-r),
			scene.Pt(cx+r, cy),
			scene.Pt(cx, cy+r),
			scene.Pt(cx-r, cy),
		))
		return
	}

	if si.isFork || si.isJoin {
		// Black horizontal bar
		sc.Add(scene.NewPath(scene.Style{
			Fill:   p.th.LineColor,
			Stroke: p.th.LineColor,
		}).Rect(cx-w/2, cy-h/2, w, h, 2))
		return
	}

	// Regular state: rounded rectangle
	x := cx - w/2
	y := cy - h/2

	// Apply style overrides
	fillColor := p.th.PrimaryColor
	strokeColor := p.th.PrimaryBorderColor
	textColor := p.th.PrimaryTextColor

	if fillStr, ok := si.styles["fill"]; ok {
		if c, err := theme.ParseColor(fillStr); err == nil {
			fillColor = c
		}
	}
	if strokeStr, ok := si.styles["stroke"]; ok {
		if c, err := theme.ParseColor(strokeStr); err == nil {
			strokeColor = c
		}
	}
	if colorStr, ok := si.styles["color"]; ok {
		if c, err := theme.ParseColor(colorStr); err == nil {
			textColor = c
		}
	}

	boxStyle := scene.Style{
		Fill:   fillColor,
		Stroke: strokeColor,
		Shadow: true,
	}

	sc.Add(scene.NewPath(boxStyle).Rect(x, y, w, h, 5))

	curY := y + 6

	// State name
	nameFont := diagram.Font(p.th, 1)
	if fontWeightStr, ok := si.styles["font-weight"]; ok && strings.Contains(fontWeightStr, "bold") {
		nameFont = diagram.BoldFont(p.th, 1)
	}

	sc.Add(scene.NewText(cx, curY, si.id, nameFont, textColor,
		scene.AnchorMiddle, scene.VAlignTop))
	curY += lineH

	// Description (if present)
	if si.description != "" {
		// Separator line
		sep := scene.NewPath(scene.Style{Stroke: strokeColor, StrokeWidth: 1})
		sep.MoveTo(x+4, curY).LineTo(x+w-4, curY)
		sc.Add(sep)
		curY += 4

		descFont := diagram.Font(p.th, 0.9)
		sc.Add(scene.NewText(cx, curY, si.description, descFont, textColor,
			scene.AnchorMiddle, scene.VAlignTop))
	}
}

func (p *parser) drawTransition(sc *scene.Scene, trans *transitionInfo, e *layout.Edge) {
	if len(e.Points) < 2 {
		return
	}

	// Draw edge with arrow
	edgeOpts := scene.EdgeOpts{
		Style: scene.Style{
			Stroke: p.th.LineColor,
		},
		Curve:  scene.CurveBasis,
		End:    scene.MarkerArrow,
		Marker: scene.MarkerOpts{Size: 10, StrokeWidth: 1.5},
	}
	sc.Add(scene.Edge(e.Points, edgeOpts)...)

	// Draw label
	if trans.label != "" {
		labelFont := diagram.Font(p.th, 1)
		sc.Add(scene.NewText(e.Label.X, e.Label.Y, trans.label, labelFont, p.th.TextColor,
			scene.AnchorMiddle, scene.VAlignMiddle))
	}
}

func (p *parser) drawNote(sc *scene.Scene, note *noteInfo) {
	fs := p.th.FontSize
	lineH := fs * 1.25

	noteFont := diagram.Font(p.th, 0.9)
	noteW := scene.MeasureText(note.text, noteFont) + 12
	noteH := lineH + 8

	noteStyle := scene.Style{
		Fill:   p.th.NoteBkg,
		Stroke: p.th.NoteBorder,
	}

	// Position notes loosely
	noteX := 100.0
	noteY := 100.0

	if note.position == "left" {
		noteX -= noteW + 20
	} else {
		noteX += 20
	}

	sc.Add(scene.NewPath(noteStyle).Rect(noteX, noteY, noteW, noteH, 3))
	sc.Add(scene.NewText(noteX+6, noteY+4, note.text, noteFont, p.th.NoteText,
		scene.AnchorStart, scene.VAlignTop))
}
