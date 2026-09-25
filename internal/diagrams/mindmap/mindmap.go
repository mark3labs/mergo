package mindmap

import (
	"fmt"
	"image/color"
	"math"
	"regexp"
	"strings"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

func init() {
	diagram.Register(diagram.Type{
		Name:   "mindmap",
		Detect: diagram.Keyword("mindmap"),
		Render: Render,
	})
}

// Render renders a mindmap diagram.
func Render(src string, cfg *diagram.Config) (*scene.Scene, error) {
	th := cfg.Theme
	sc := diagram.NewScene(th)

	// Don't use diagram.Lines since it trims all whitespace (we need indentation)
	lines := strings.Split(src, "\n")
	var trimmedLines []string
	for _, line := range lines {
		line = strings.TrimRight(line, "\r") // trim line endings but keep indentation
		if strings.TrimSpace(line) != "" {
			trimmedLines = append(trimmedLines, line)
		}
	}

	if len(trimmedLines) == 0 {
		return diagram.Finish(sc, th, cfg.Title), nil
	}

	root, err := parseNodes(trimmedLines)
	if err != nil {
		return nil, err
	}

	if root == nil {
		return diagram.Finish(sc, th, cfg.Title), nil
	}

	// Layout the mindmap in a radial/balanced tree
	layoutRadial(root, 0, 0)

	// Render the tree
	renderNode(sc, root, th, 0)

	return diagram.Finish(sc, th, cfg.Title), nil
}

type Node struct {
	Text     string
	Shape    ShapeType
	Children []*Node
	Depth    int
	X, Y     float64 // Layout position
	Width    float64
	Height   float64
}

type ShapeType int

const (
	ShapeDefault ShapeType = iota
	ShapeSquare
	ShapeRounded
	ShapeCircle
	ShapeBang
	ShapeCloud
	ShapeHexagon
)

// parseNodes parses the mindmap structure from lines of text.
// Returns the root node or nil if no root found.
func parseNodes(lines []string) (*Node, error) {
	var root *Node
	var nodeStack []*Node // Stack to track parent nodes by depth

	for lineIdx, line := range lines {
		if line == "" {
			continue
		}

		// Count indentation
		indent := 0
		for _, c := range line {
			if c == ' ' || c == '\t' {
				indent++
			} else {
				break
			}
		}
		depth := indent / 2 // Assume 2 spaces per level

		// Trim the line
		trimmed := strings.TrimSpace(line)

		// Skip directives and special lines
		if strings.HasPrefix(trimmed, "%%") || trimmed == "" {
			continue
		}

		// Skip icon lines (::icon(...))
		if strings.HasPrefix(trimmed, "::icon") {
			continue
		}

		// Parse class line (:::class)
		if strings.HasPrefix(trimmed, ":::") {
			continue
		}

		// Parse node text and shape
		text, shape := parseNodeShape(trimmed)
		if text == "" {
			continue
		}

		text = diagram.CleanLabel(text)

		node := &Node{
			Text:  text,
			Shape: shape,
			Depth: depth,
		}

		if root == nil {
			root = node
			nodeStack = []*Node{root}
		} else {
			// Determine parent based on indentation
			// If depth is <= current stack depth, pop until we find the right parent
			for len(nodeStack) > depth {
				nodeStack = nodeStack[:len(nodeStack)-1]
			}

			if len(nodeStack) == 0 {
				return nil, fmt.Errorf("line %d: invalid indentation", lineIdx+1)
			}

			parent := nodeStack[len(nodeStack)-1]
			parent.Children = append(parent.Children, node)
			nodeStack = append(nodeStack, node)
		}
	}

	return root, nil
}

// parseNodeShape extracts the node text and shape type from a line.
// Handles: id[text] (square), id(text) (rounded), id((text)) (circle),
// id))text(( (bang), id)text( (cloud), id{{text}} (hexagon), text (default)
func parseNodeShape(s string) (text string, shape ShapeType) {
	s = strings.TrimSpace(s)

	// Try to match id with shape: id[...], id(...), id((..)), etc.
	// Pattern: optional id followed by shape brackets

	// Check for hexagon: {{...}}
	if strings.Contains(s, "{{") && strings.Contains(s, "}}") {
		// Extract text between {{ and }}
		start := strings.Index(s, "{{")
		end := strings.LastIndex(s, "}}")
		if start < end {
			return strings.TrimSpace(s[start+2 : end]), ShapeHexagon
		}
	}

	// Check for bang: ))text((
	if strings.Contains(s, "))") && strings.Contains(s, "((") {
		start := strings.Index(s, "))")
		end := strings.LastIndex(s, "((")
		if start < end {
			return strings.TrimSpace(s[start+2 : end]), ShapeBang
		}
	}

	// Check for cloud: )text(
	if strings.Contains(s, ")") && strings.Contains(s, "(") {
		// Need to be careful: )text( is cloud, but ))text(( is bang and (text) is rounded
		if !strings.Contains(s, "((") && !strings.Contains(s, "))") {
			re := regexp.MustCompile(`[^()\[\]]*\)([^)]+)\(`)
			match := re.FindStringSubmatch(s)
			if match != nil {
				return strings.TrimSpace(match[1]), ShapeCloud
			}
		}
	}

	// Check for circle: ((text))
	if strings.Contains(s, "((") && strings.Contains(s, "))") {
		start := strings.Index(s, "((")
		end := strings.LastIndex(s, "))")
		if start < end {
			return strings.TrimSpace(s[start+2 : end]), ShapeCircle
		}
	}

	// Check for rounded: (text)
	if strings.Contains(s, "(") && strings.Contains(s, ")") {
		re := regexp.MustCompile(`[^[\]]*\(([^)]+)\)`)
		match := re.FindStringSubmatch(s)
		if match != nil {
			return strings.TrimSpace(match[1]), ShapeRounded
		}
	}

	// Check for square: [text]
	if strings.Contains(s, "[") && strings.Contains(s, "]") {
		start := strings.Index(s, "[")
		end := strings.LastIndex(s, "]")
		if start < end {
			return strings.TrimSpace(s[start+1 : end]), ShapeSquare
		}
	}

	// Default: plain text
	return s, ShapeDefault
}

// layoutRadial arranges nodes in a radial/balanced tree layout
// The root is at (x, y), branches extend outward with alternating left/right
func layoutRadial(root *Node, x, y float64) {
	if root == nil {
		return
	}

	root.X = x
	root.Y = y

	if len(root.Children) == 0 {
		root.Width = 60
		root.Height = 40
		return
	}

	// Calculate dimensions for children
	numChildren := len(root.Children)
	branchRadius := 120 + float64(root.Depth)*30

	// Distribute children around the root
	for i, child := range root.Children {
		// Determine left/right side: even indices go right, odd go left
		side := 1.0
		if i%2 == 1 {
			side = -1.0
		}

		// Angle within the hemisphere
		angleCount := (numChildren + 1) / 2
		angle := float64(i/2) * math.Pi / float64(angleCount) * side

		// Position child
		cx := x + branchRadius*math.Cos(angle)
		cy := y + branchRadius*math.Sin(angle)

		layoutRadial(child, cx, cy)
	}

	root.Width = 70
	root.Height = 50
}

// renderNode draws a node and its children
func renderNode(sc *scene.Scene, node *Node, th *theme.Theme, branchIndex int) {
	if node == nil {
		return
	}

	// Determine node color based on depth
	var fillColor, strokeColor, textColor color.RGBA
	if node.Depth == 0 {
		// Root node
		fillColor = th.PrimaryColor
		textColor = th.PrimaryTextColor
		strokeColor = th.PrimaryBorderColor
	} else {
		// Branch nodes use palette colors
		fillColor = th.PaletteColor(branchIndex)
		textColor = th.PaletteTextColor(branchIndex)
		strokeColor = th.PrimaryBorderColor
	}

	// Draw node shape
	drawNodeShape(sc, node, fillColor, strokeColor, textColor, th)

	// Draw connecting lines to children with decreasing thickness
	for childIdx, child := range node.Children {
		// Line thickness decreases with depth
		thickness := math.Max(1, 4-float64(child.Depth)*0.5)

		// Draw curved connector
		connector := scene.NewPath(scene.Style{
			Stroke:      strokeColor,
			StrokeWidth: thickness,
			RoundCaps:   true,
		})

		// Cubic bezier from node to child
		dx := child.X - node.X
		dy := child.Y - node.Y
		midX := node.X + dx*0.5
		midY := node.Y + dy*0.5

		connector.MoveTo(node.X, node.Y)
		connector.CubicTo(midX-dy*0.2, midY+dx*0.2, midX+dy*0.2, midY-dx*0.2, child.X, child.Y)
		sc.Add(connector)

		// Recursively render children with adjusted palette index
		renderNode(sc, child, th, branchIndex+childIdx)
	}

	// Draw node text
	f := scene.Font{Size: th.FontSize * 0.9}
	text := scene.NewText(node.X, node.Y, node.Text, f, textColor, scene.AnchorMiddle, scene.VAlignMiddle)
	sc.Add(text)
}

// drawNodeShape draws the node with its specific shape
func drawNodeShape(sc *scene.Scene, node *Node, fill, stroke, textColor color.RGBA, th *theme.Theme) {
	var path *scene.Path

	sw := 2.0
	if node.Depth == 0 {
		sw = 3.0
	}

	st := scene.Style{
		Fill:        fill,
		Stroke:      stroke,
		StrokeWidth: sw,
		Shadow:      node.Depth == 0,
		RoundCaps:   true,
	}

	x, y := node.X, node.Y
	w, h := node.Width, node.Height
	hw, hh := w/2, h/2

	switch node.Shape {
	case ShapeDefault:
		// Rounded rectangle (default)
		path = scene.NewPath(st)
		path.Rect(x-hw, y-hh, w, h, 5)

	case ShapeSquare:
		// Sharp rectangle
		path = scene.NewPath(st)
		path.Rect(x-hw, y-hh, w, h, 0)

	case ShapeRounded:
		// Rounded rectangle
		path = scene.NewPath(st)
		path.Rect(x-hw, y-hh, w, h, 8)

	case ShapeCircle:
		// Circle
		path = scene.NewPath(st)
		path.Circle(x, y, math.Max(hw, hh))

	case ShapeBang:
		// Bang shape (asymmetric)
		path = scene.NewPath(st)
		path.Polygon(
			scene.Pt(x-hw, y),
			scene.Pt(x, y-hh),
			scene.Pt(x+hw, y),
			scene.Pt(x+hw/2, y+hh),
		)

	case ShapeCloud:
		// Cloud shape (rounded bumps)
		path = scene.NewPath(st)
		r := hw / 2
		path.MoveTo(x-hw/2, y+hh)
		path.Arc(x-hw, y, r, r, 0, math.Pi, false)
		path.Arc(x, y, r, r, 0, math.Pi, true)
		path.Arc(x+hw, y, r, r, 0, math.Pi, true)
		path.Arc(x+hw/2, y, r, r, 0, math.Pi, true)
		path.Close()

	case ShapeHexagon:
		// Hexagon
		path = scene.NewPath(st)
		angles := []float64{0, math.Pi / 3, 2 * math.Pi / 3, math.Pi, 4 * math.Pi / 3, 5 * math.Pi / 3}
		var pts []scene.Point
		for _, a := range angles {
			pts = append(pts, scene.Pt(x+hw*math.Cos(a), y+hh*math.Sin(a)))
		}
		path.Polygon(pts...)
	}

	if path != nil {
		sc.Add(path)
	}
}
