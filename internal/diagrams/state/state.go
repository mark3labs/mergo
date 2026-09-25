// Package state implements rendering of Mermaid state diagrams.
package state

import (
	"fmt"
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
	cfg       *diagram.Config
	th        *theme.Theme
	states    map[string]*stateInfo
	start     string
	end       string
	relations []*transitionInfo
	notes     []*noteInfo
	direction layout.Direction
}

type stateInfo struct {
	id                string
	description       string
	children          []*stateInfo // for composite states
	isComposite       bool
	isStart           bool
	isEnd             bool
	isChoice          bool
	isFork            bool
	isJoin            bool
	childDirection    *layout.Direction
	startState        *stateInfo // [*] within this composite
	endState          *stateInfo // [*] end marker
	concurrentRegions []int      // indices of regions separated by --
	styles            map[string]string
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
		cfg:       cfg,
		th:        th,
		states:    make(map[string]*stateInfo),
		direction: layout.TB,
	}
	p.direction = layout.ParseDirection(cfg.String("state", "direction", "TB"))
	return p
}

func (p *parser) parse(src string) error {
	lines := diagram.Lines(src)

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Direction statement
		if strings.HasPrefix(line, "direction") {
			parts := strings.Fields(line)
			if len(parts) > 1 {
				p.direction = layout.ParseDirection(parts[1])
			}
			continue
		}

		// Skip styling and config directives
		if strings.HasPrefix(line, "classDef") || strings.HasPrefix(line, "class ") ||
			strings.HasPrefix(line, "style ") || strings.HasPrefix(line, "hide ") {
			continue
		}

		// State declaration with description
		if strings.HasPrefix(line, "state ") && strings.Contains(line, " as ") {
			parts := strings.SplitN(line, " as ", 2)
			if len(parts) == 2 {
				stateDecl := strings.TrimSpace(strings.TrimPrefix(parts[0], "state "))
				id := strings.TrimSpace(parts[1])
				stateDecl = diagram.CleanLabel(stateDecl)
				if _, ok := p.states[id]; !ok {
					p.states[id] = &stateInfo{
						id:          id,
						description: stateDecl,
						styles:      make(map[string]string),
					}
				}
			}
			continue
		}

		// State with block (composite state)
		if strings.HasPrefix(line, "state ") && strings.Contains(line, "{") {
			if err := p.parseCompositeState(line, lines, &i); err != nil {
				return fmt.Errorf("line %d: %w", i+1, err)
			}
			continue
		}

		// Simple state declaration
		if after, ok := strings.CutPrefix(line, "state "); ok {
			id := strings.TrimSpace(after)
			id = diagram.CleanLabel(id)
			if _, ok := p.states[id]; !ok {
				p.states[id] = &stateInfo{
					id:     id,
					styles: make(map[string]string),
				}
			}
			continue
		}

		// State description (id : description)
		if strings.Contains(line, " : ") && !strings.Contains(line, "--") {
			parts := strings.SplitN(line, " : ", 2)
			if len(parts) == 2 {
				id := strings.TrimSpace(parts[0])
				desc := strings.TrimSpace(parts[1])
				desc = diagram.CleanLabel(desc)
				if _, ok := p.states[id]; !ok {
					p.states[id] = &stateInfo{
						id:          id,
						description: desc,
						styles:      make(map[string]string),
					}
				} else {
					p.states[id].description = desc
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

		// Special markers: <<choice>>, <<fork>>, <<join>>
		if strings.Contains(line, "<<choice>>") {
			id := regexp.MustCompile(`(\w+)\s+<<choice>>`).FindStringSubmatch(line)
			if len(id) > 1 {
				if _, ok := p.states[id[1]]; !ok {
					p.states[id[1]] = &stateInfo{
						id:       id[1],
						isChoice: true,
						styles:   make(map[string]string),
					}
				} else {
					p.states[id[1]].isChoice = true
				}
			}
			continue
		}
		if strings.Contains(line, "<<fork>>") {
			id := regexp.MustCompile(`(\w+)\s+<<fork>>`).FindStringSubmatch(line)
			if len(id) > 1 {
				if _, ok := p.states[id[1]]; !ok {
					p.states[id[1]] = &stateInfo{
						id:     id[1],
						isFork: true,
						styles: make(map[string]string),
					}
				} else {
					p.states[id[1]].isFork = true
				}
			}
			continue
		}
		if strings.Contains(line, "<<join>>") {
			id := regexp.MustCompile(`(\w+)\s+<<join>>`).FindStringSubmatch(line)
			if len(id) > 1 {
				if _, ok := p.states[id[1]]; !ok {
					p.states[id[1]] = &stateInfo{
						id:     id[1],
						isJoin: true,
						styles: make(map[string]string),
					}
				} else {
					p.states[id[1]].isJoin = true
				}
			}
			continue
		}
	}

	return nil
}

func (p *parser) parseCompositeState(line string, lines []string, idx *int) error {
	// state Name { ... }
	before, _, ok := strings.Cut(line, "{")
	if !ok {
		return fmt.Errorf("missing {")
	}

	stateDecl := strings.TrimPrefix(before, "state ")
	stateDecl = strings.TrimSpace(stateDecl)
	id := stateDecl
	desc := ""

	// Handle "name" as "id"
	if strings.Contains(stateDecl, " as ") {
		parts := strings.SplitN(stateDecl, " as ", 2)
		desc = diagram.CleanLabel(strings.TrimSpace(parts[0]))
		id = strings.TrimSpace(parts[1])
	} else if strings.HasPrefix(stateDecl, `"`) && strings.Contains(stateDecl, `"`) {
		endQuote := strings.LastIndex(stateDecl, `"`)
		desc = stateDecl[1:endQuote]
		rest := strings.TrimSpace(stateDecl[endQuote+1:])
		if strings.HasPrefix(rest, "as ") {
			id = strings.TrimSpace(rest[3:])
		} else {
			id = desc
		}
		desc = diagram.CleanLabel(desc)
	}

	si := &stateInfo{
		id:          id,
		description: desc,
		isComposite: true,
		styles:      make(map[string]string),
	}

	// Parse contents
	for *idx++; *idx < len(lines); *idx++ {
		memberLine := strings.TrimSpace(lines[*idx])
		if memberLine == "" {
			continue
		}
		if memberLine == "}" {
			break
		}

		// Nested state or transition within composite
		if strings.HasPrefix(memberLine, "state ") || strings.Contains(memberLine, "-->") ||
			strings.Contains(memberLine, " : ") {
			// Would need recursive parsing here; for now, simplified
		}

		// Concurrency separator
		if memberLine == "--" {
			si.concurrentRegions = append(si.concurrentRegions, len(si.children))
			continue
		}
	}

	p.states[id] = si
	return nil
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
	// note for State : text

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

func (p *parser) render() (*scene.Scene, error) {
	sc := diagram.NewScene(p.th)

	// Build layout graph
	g := &layout.Graph{
		Dir:     p.direction,
		NodeSep: p.cfg.Float("state", "nodeSpacing", 50),
		RankSep: p.cfg.Float("state", "rankSpacing", 50),
	}

	// Create nodes for each state
	nodeMap := make(map[string]*layout.Node)
	for id, si := range p.states {
		w, h := p.measureState(si)
		n := &layout.Node{
			ID:   id,
			W:    w,
			H:    h,
			Data: si,
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

	// Run layout
	layout.Layout(g)

	// Draw states
	for id, si := range p.states {
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
		p.drawTransition(sc, rel, e)
	}

	return diagram.Finish(sc, p.th, ""), nil
}

func (p *parser) measureState(si *stateInfo) (float64, float64) {
	fs := p.th.FontSize
	lineH := fs * 1.25
	minW := 80.0

	// Measure text
	if si.isStart || si.isEnd {
		// Small circles
		return 20, 20
	}

	if si.isChoice {
		// Diamond
		return 40, 40
	}

	if si.isFork || si.isJoin {
		// Bar
		return 60, 12
	}

	// Regular state
	h := lineH + 8 // padding
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
		// Bullseye circle (filled circle with unfilled ring)
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

	sc.Add(scene.NewPath(scene.Style{
		Fill:   p.th.PrimaryColor,
		Stroke: p.th.PrimaryBorderColor,
		Shadow: true,
	}).Rect(x, y, w, h, 8))

	curY := y + 6

	// State name
	nameFont := diagram.Font(p.th, 1)
	sc.Add(scene.NewText(cx, curY, si.id, nameFont, p.th.PrimaryTextColor,
		scene.AnchorMiddle, scene.VAlignTop))
	curY += lineH

	// Description (if present)
	if si.description != "" {
		// Separator line
		sep := scene.NewPath(scene.Style{Stroke: p.th.PrimaryBorderColor, StrokeWidth: 1})
		sep.MoveTo(x+4, curY).LineTo(x+w-4, curY)
		sc.Add(sep)
		curY += 4

		descFont := diagram.Font(p.th, 0.9)
		sc.Add(scene.NewText(cx, curY, si.description, descFont, p.th.PrimaryTextColor,
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
