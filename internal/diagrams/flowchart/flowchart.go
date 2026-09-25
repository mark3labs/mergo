// Package flowchart implements rendering of Mermaid flowchart diagrams.
package flowchart

import (
	"image/color"
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
		Name:   "flowchart",
		Detect: diagram.Keyword("flowchart", "graph", "flowchart-elk"),
		Render: Render,
	})
}

// Render parses and renders a flowchart diagram.
func Render(src string, cfg *diagram.Config) (*scene.Scene, error) {
	doc, err := parseFlowchart(src)
	if err != nil {
		return nil, err
	}

	th := cfg.Theme
	if th == nil {
		th = theme.Default()
	}

	sc := diagram.NewScene(th)

	if len(doc.nodes) == 0 {
		return diagram.Finish(sc, th, cfg.Title), nil
	}

	// Extract config
	nodeSpacing := cfg.Float("flowchart", "nodeSpacing", 50)
	rankSpacing := cfg.Float("flowchart", "rankSpacing", 50)
	padding := cfg.Float("flowchart", "padding", 15)
	curveType := cfg.String("flowchart", "curve", "basis")

	direction := layout.ParseDirection(doc.direction)

	// Build layout graph
	g := &layout.Graph{
		Dir:     direction,
		NodeSep: nodeSpacing,
		RankSep: rankSpacing,
		EdgeSep: 20,
		Nodes:   make([]*layout.Node, 0),
		Edges:   make([]*layout.Edge, 0),
	}

	// Create layout nodes
	layoutNodes := make(map[string]*layout.Node)
	for nodeID, nodeData := range doc.nodes {
		font := scene.Font{Size: th.FontSize}
		w, h := scene.MeasureBlock(nodeData.label, font, 1.25)
		w += padding * 2
		h += padding * 2

		lnode := &layout.Node{
			ID:   nodeID,
			W:    w,
			H:    h,
			Data: nodeData,
		}
		layoutNodes[nodeID] = lnode
		g.Nodes = append(g.Nodes, lnode)
	}

	// Create layout edges
	for _, edge := range doc.edges {
		fromNode := layoutNodes[edge.from]
		toNode := layoutNodes[edge.to]
		if fromNode == nil || toNode == nil {
			continue
		}

		ledge := &layout.Edge{
			From:   fromNode,
			To:     toNode,
			MinLen: edge.minLen,
			Weight: 1,
			Data:   edge,
		}

		if edge.label != "" {
			font := scene.Font{Size: th.FontSize}
			w, h := scene.MeasureBlock(edge.label, font, 1.25)
			ledge.LabelW = w + 4
			ledge.LabelH = h + 2
		}

		g.Edges = append(g.Edges, ledge)
	}

	// Perform layout
	layout.Layout(g)

	// Draw edges
	for _, ledge := range g.Edges {
		edge := ledge.Data.(*edgeInfo)
		drawEdge(sc, ledge, edge, th, curveType)
	}

	// Draw nodes
	for _, lnode := range layoutNodes {
		nodeData := lnode.Data.(*nodeInfo)
		drawNode(sc, lnode, nodeData, th)
	}

	// Draw edge labels
	for _, ledge := range g.Edges {
		edge := ledge.Data.(*edgeInfo)
		if edge.label != "" && (ledge.Label.X != 0 || ledge.Label.Y != 0) {
			drawEdgeLabel(sc, edge, ledge, th)
		}
	}

	return diagram.Finish(sc, th, cfg.Title), nil
}

type nodeInfo struct {
	label string
	shape int
	style map[string]string
}

type edgeInfo struct {
	from   string
	to     string
	label  string
	style  int
	minLen int
}

type flowchartDoc struct {
	direction string
	nodes     map[string]*nodeInfo
	edges     []*edgeInfo
}

func parseFlowchart(src string) (*flowchartDoc, error) {
	doc := &flowchartDoc{
		direction: "TB",
		nodes:     make(map[string]*nodeInfo),
		edges:     make([]*edgeInfo, 0),
	}

	lines := strings.Split(src, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}

		// Parse header
		if i == 0 && (strings.HasPrefix(line, "graph ") || strings.HasPrefix(line, "flowchart ")) {
			parts := strings.Fields(line)
			if len(parts) > 1 {
				switch parts[1] {
				case "TB", "TD", "BT", "LR", "RL":
					doc.direction = parts[1]
					if doc.direction == "TD" {
						doc.direction = "TB"
					}
				}
			}
			continue
		}

		// Skip unsupported statements
		if strings.HasPrefix(line, "classDef") || strings.HasPrefix(line, "class ") ||
			strings.HasPrefix(line, "style ") || strings.HasPrefix(line, "linkStyle") ||
			strings.HasPrefix(line, "click") || strings.HasPrefix(line, "callback") ||
			strings.HasPrefix(line, "direction") || strings.HasPrefix(line, "subgraph") ||
			strings.HasPrefix(line, "end") || strings.HasPrefix(line, "accTitle") ||
			strings.HasPrefix(line, "accDescr") {
			continue
		}

		// Try to parse as statement (node or edge)
		parseStatement(doc, line)
	}

	return doc, nil
}

func parseStatement(doc *flowchartDoc, line string) {
	// Split on edges and parse
	edges := findEdges(line)
	if len(edges) > 0 {
		// Parse as edge chain
		parseEdgeChain(doc, line)
	} else {
		// Parse as node definition
		parseNodeDef(doc, line)
	}
}

func findEdges(line string) []string {
	edgeOps := []string{"~~~", "<-->", "o--o", "x--x", "--o", "--x", "-->", "---", "-.-", "-..->", "===>", "===", "==>"}
	var found []string
	for _, op := range edgeOps {
		if strings.Contains(line, op) {
			found = append(found, op)
		}
	}
	return found
}

func parseEdgeChain(doc *flowchartDoc, line string) {
	// Simple edge parsing: extract A --> B, handle labels
	// A --> B or A -->|label| B or A -- label --> B

	// Find operator
	var op string
	opIdx := -1
	for _, o := range []string{"~~~", "<-->", "o--o", "x--x", "-..->", "-.->", "-.-", "-->", "---", "===>", "==>", "===", "--o", "--x"} {
		if idx := strings.Index(line, o); idx >= 0 {
			if opIdx < 0 || idx < opIdx {
				op = o
				opIdx = idx
			}
		}
	}

	if opIdx < 0 {
		return
	}

	// Extract from and to, and label
	from := strings.TrimSpace(line[:opIdx])
	rest := line[opIdx+len(op):]

	// Handle label between pipes or brackets
	var label string
	if strings.HasPrefix(rest, "|") {
		endIdx := strings.Index(rest[1:], "|")
		if endIdx > 0 {
			label = strings.TrimSpace(rest[1 : endIdx+1])
			rest = rest[endIdx+2:]
		}
	} else if strings.HasPrefix(rest, "[") {
		endIdx := strings.Index(rest[1:], "]")
		if endIdx > 0 {
			label = strings.TrimSpace(rest[1 : endIdx+1])
			rest = rest[endIdx+2:]
		}
	}

	to := strings.TrimSpace(rest)
	if to == "" {
		return
	}

	// Extract node IDs
	from = extractNodeID(from)
	to = extractNodeID(to)

	if from != "" && to != "" {
		// Ensure nodes exist
		if _, ok := doc.nodes[from]; !ok {
			doc.nodes[from] = &nodeInfo{label: from, shape: 0, style: make(map[string]string)}
		}
		if _, ok := doc.nodes[to]; !ok {
			doc.nodes[to] = &nodeInfo{label: to, shape: 0, style: make(map[string]string)}
		}

		doc.edges = append(doc.edges, &edgeInfo{
			from:   from,
			to:     to,
			label:  label,
			style:  edgeStyleForOp(op),
			minLen: minLenForOp(op),
		})
	}
}

func extractNodeID(s string) string {
	s = strings.TrimSpace(s)
	// Extract just the ID part (before [ or other delimiters)
	for i, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return s[:i]
		}
	}
	return s
}

func parseNodeDef(doc *flowchartDoc, line string) {
	// A[text], A(text), A{text}, etc.
	// Extract ID and label
	if line == "" {
		return
	}

	// Find node ID (sequence of alphanumeric)
	idEnd := 0
	for idEnd < len(line) && ((line[idEnd] >= 'a' && line[idEnd] <= 'z') ||
		(line[idEnd] >= 'A' && line[idEnd] <= 'Z') ||
		(line[idEnd] >= '0' && line[idEnd] <= '9') ||
		line[idEnd] == '_') {
		idEnd++
	}

	if idEnd == 0 {
		return
	}

	nodeID := line[:idEnd]
	rest := line[idEnd:]

	// Parse shape and label
	var label string
	shape := 0

	if rest != "" {
		// Extract shape and label: [label], (label), {label}, etc.
		switch rest[0] {
		case '[':
			// Rectangle or variant
			if strings.HasPrefix(rest, "[[") {
				// Subroutine
				endIdx := strings.Index(rest, "]]")
				if endIdx > 0 {
					label = rest[2:endIdx]
					shape = 2
				}
			} else if strings.HasPrefix(rest, "[/") {
				// Parallelogram
				endIdx := strings.Index(rest, "/]")
				if endIdx > 0 {
					label = rest[2:endIdx]
					shape = 3
				}
			} else {
				// Regular rectangle
				endIdx := strings.Index(rest[1:], "]")
				if endIdx > 0 {
					label = rest[1 : endIdx+1]
					label = unquoteLabel(label)
					shape = 0
				}
			}
		case '(':
			// Rounded or stadium
			if strings.HasPrefix(rest, "((") {
				// Circle
				endIdx := strings.Index(rest, "))")
				if endIdx > 0 {
					label = rest[2:endIdx]
					shape = 4
				}
			} else if strings.HasPrefix(rest, "([") {
				// Stadium
				endIdx := strings.Index(rest, "])")
				if endIdx > 0 {
					label = rest[2:endIdx]
					shape = 1
				}
			} else {
				// Rounded
				endIdx := strings.Index(rest[1:], ")")
				if endIdx > 0 {
					label = rest[1 : endIdx+1]
					shape = 1
				}
			}
		case '{':
			// Diamond
			if strings.HasPrefix(rest, "{{") {
				// Hexagon
				endIdx := strings.Index(rest, "}}")
				if endIdx > 0 {
					label = rest[2:endIdx]
					shape = 5
				}
			} else {
				// Diamond
				endIdx := strings.Index(rest[1:], "}")
				if endIdx > 0 {
					label = rest[1 : endIdx+1]
					shape = 5
				}
			}
		case '>':
			// Asymmetric
			endIdx := strings.Index(rest, "]")
			if endIdx > 0 {
				label = rest[1:endIdx]
				shape = 6
			}
		}
	}

	if label == "" {
		label = nodeID
	}

	doc.nodes[nodeID] = &nodeInfo{
		label: label,
		shape: shape,
		style: make(map[string]string),
	}
}

func unquoteLabel(s string) string {
	if strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"") {
		s = s[1 : len(s)-1]
	}
	// Handle entities
	s = strings.ReplaceAll(s, "#quot;", "\"")
	s = strings.ReplaceAll(s, "<br>", "\n")
	// Strip markdown
	s = regexp.MustCompile(`\*{1,2}([^*]+)\*{1,2}`).ReplaceAllString(s, "$1")
	return strings.TrimSpace(s)
}

func edgeStyleForOp(op string) int {
	switch op {
	case "-->":
		return 1 // arrow
	case "---":
		return 0 // open
	case "-.-", "-.->":
		return 2 // dotted
	case "===", "===>":
		return 3 // thick
	case "~~~":
		return 4 // invisible
	case "--o":
		return 5 // circle
	case "--x":
		return 6 // cross
	case "<-->":
		return 7 // bidirect
	}
	return 1
}

func minLenForOp(op string) int {
	// Count extra dashes
	if strings.HasPrefix(op, "-") {
		return (len(op) - 3) / 2
	}
	return 1
}

func drawNode(sc *scene.Scene, lnode *layout.Node, nodeData *nodeInfo, th *theme.Theme) {
	x, y, w, h := lnode.X, lnode.Y, lnode.W, lnode.H

	style := scene.Style{
		Fill:        th.PrimaryColor,
		Stroke:      th.PrimaryBorderColor,
		StrokeWidth: 1.5,
		Shadow:      true,
	}

	p := scene.NewPath(style)

	// Draw shape based on type
	switch nodeData.shape {
	case 1: // Rounded/Stadium
		r := h / 2
		p.MoveTo(x-w/2+r, y-h/2).
			LineTo(x+w/2-r, y-h/2).
			Arc(x+w/2-r, y, r, r, -math.Pi/2, math.Pi/2, true).
			LineTo(x-w/2+r, y+h/2).
			Arc(x-w/2+r, y, r, r, math.Pi/2, 3*math.Pi/2, true).
			Close()

	case 2: // Subroutine
		p.MoveTo(x-w/2, y-h/2).
			LineTo(x+w/2, y-h/2).
			LineTo(x+w/2, y+h/2).
			LineTo(x-w/2, y+h/2).
			Close()

	case 3: // Parallelogram
		offset := w * 0.2
		p.MoveTo(x-w/2, y-h/2).
			LineTo(x+w/2, y-h/2).
			LineTo(x+w/2-offset, y+h/2).
			LineTo(x-w/2-offset, y+h/2).
			Close()

	case 4: // Circle
		r := w / 2
		p.MoveTo(x-r, y).
			Arc(x, y, r, r, math.Pi, 0, true).
			Arc(x, y, r, r, 0, math.Pi, true).
			Close()

	case 5: // Diamond/Hexagon
		p.MoveTo(x, y-h/2).
			LineTo(x+w/2, y).
			LineTo(x, y+h/2).
			LineTo(x-w/2, y).
			Close()

	case 6: // Asymmetric
		p.MoveTo(x-w/2, y-h/2).
			LineTo(x+w/2-h*0.3, y-h/2).
			LineTo(x+w/2, y).
			LineTo(x+w/2-h*0.3, y+h/2).
			LineTo(x-w/2, y+h/2).
			Close()

	default: // Rectangle
		p.MoveTo(x-w/2, y-h/2).
			LineTo(x+w/2, y-h/2).
			LineTo(x+w/2, y+h/2).
			LineTo(x-w/2, y+h/2).
			Close()
	}

	sc.Add(p)

	// Draw label
	labelFont := scene.Font{Size: th.FontSize}
	sc.Add(scene.NewText(x, y, nodeData.label, labelFont, th.PrimaryTextColor, scene.AnchorMiddle, scene.VAlignMiddle))
}

func drawEdge(sc *scene.Scene, ledge *layout.Edge, edge *edgeInfo, th *theme.Theme, curveType string) {
	if len(ledge.Points) == 0 {
		return
	}

	strokeWidth := 1.5
	var dash []float64
	var endMarker scene.MarkerKind

	switch edge.style {
	case 2: // dotted
		dash = []float64{3, 3}
	case 3: // thick
		strokeWidth = 3.5
	case 4: // invisible
		return
	case 5: // circle
		endMarker = scene.MarkerCircle
	case 6: // cross
		endMarker = scene.MarkerCross
	case 7: // bidirect
		endMarker = scene.MarkerArrow
	default:
		endMarker = scene.MarkerArrow
	}

	// Map curve type
	curveKind := scene.CurveBasis
	switch curveType {
	case "linear":
		curveKind = scene.CurveLinear
	case "cardinal", "catmullRom", "monotoneX", "natural":
		curveKind = scene.CurveCatmull
	}

	edgeOpts := scene.EdgeOpts{
		Style: scene.Style{
			Stroke:      th.LineColor,
			StrokeWidth: strokeWidth,
			Dash:        dash,
		},
		Curve:  curveKind,
		End:    endMarker,
		Marker: scene.MarkerOpts{Size: 8, StrokeWidth: 1.5},
	}

	sc.Add(scene.Edge(ledge.Points, edgeOpts)...)
}

func drawEdgeLabel(sc *scene.Scene, edge *edgeInfo, ledge *layout.Edge, th *theme.Theme) {
	font := scene.Font{Size: th.FontSize}
	w, h := scene.MeasureBlock(edge.label, font, 1.25)

	// Background
	style := scene.Style{
		Fill:   th.EdgeLabelBg,
		Stroke: color.RGBA{A: 0},
	}

	p := scene.NewPath(style)
	p.MoveTo(ledge.Label.X-w/2-2, ledge.Label.Y-h/2-1).
		LineTo(ledge.Label.X+w/2+2, ledge.Label.Y-h/2-1).
		LineTo(ledge.Label.X+w/2+2, ledge.Label.Y+h/2+1).
		LineTo(ledge.Label.X-w/2-2, ledge.Label.Y+h/2+1).
		Close()
	sc.Add(p)

	// Text
	sc.Add(scene.NewText(ledge.Label.X, ledge.Label.Y, edge.label, font, th.TextColor, scene.AnchorMiddle, scene.VAlignMiddle))
}
