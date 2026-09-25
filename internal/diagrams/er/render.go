package er

import (
	"fmt"
	"image/color"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/layout"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

func init() {
	diagram.Register(diagram.Type{
		Name:   "er",
		Detect: diagram.Keyword("erDiagram"),
		Render: Render,
	})
}

// Render renders an ER diagram.
func Render(src string, cfg *diagram.Config) (*scene.Scene, error) {
	doc, err := Parse(src)
	if err != nil {
		return nil, err
	}

	th := cfg.Theme
	sc := diagram.NewScene(th)

	if len(doc.Entities) == 0 {
		return sc, nil
	}

	// Create layout graph
	g := &layout.Graph{
		Dir:     layout.ParseDirection(doc.Direction),
		NodeSep: 100,
		RankSep: 100,
	}

	// Create layout nodes for entities
	nodeMap := make(map[string]*layout.Node)
	for _, ent := range doc.Entities {
		// Measure entity box
		w, h := measureEntity(ent, th)
		n := &layout.Node{
			ID:   ent.Name,
			W:    w,
			H:    h,
			Data: ent,
		}
		nodeMap[ent.Name] = n
		g.Nodes = append(g.Nodes, n)
	}

	// Create layout edges
	for _, rel := range doc.Relationships {
		from, ok := nodeMap[rel.From]
		if !ok {
			continue
		}
		to, ok := nodeMap[rel.To]
		if !ok {
			continue
		}
		e := &layout.Edge{
			From:   from,
			To:     to,
			Data:   rel,
			MinLen: 1,
		}
		g.Edges = append(g.Edges, e)
	}

	// Layout
	layout.Layout(g)

	// Draw entities
	for _, n := range g.Nodes {
		ent := n.Data.(*Entity)
		drawEntity(sc, ent, n.X, n.Y, th)
	}

	// Draw relationships
	for _, e := range g.Edges {
		rel := e.Data.(*Relationship)
		drawRelationship(sc, rel, e.Points, th)
		if rel.Label != "" {
			label := diagram.CleanLabel(rel.Label)
			drawEdgeLabel(sc, label, e.Label, th)
		}
	}

	return diagram.Finish(sc, th, doc.Title), nil
}

func measureEntity(ent *Entity, th *theme.Theme) (float64, float64) {
	// Header: entity name in bold
	headerFont := diagram.BoldFont(th, 1)
	headerW, headerH := scene.MeasureBlock(ent.Label, headerFont, 0)

	// Attributes
	attrFont := diagram.Font(th, 1)
	attrW := headerW
	totalH := headerH + 8 // padding around header

	for _, attr := range ent.Attrs {
		// Format: type | name | keys | comment
		line := fmt.Sprintf("%-8s %-12s %-8s %s", attr.Type, attr.Name, attr.Keys, attr.Comment)
		w, h := scene.MeasureBlock(line, attrFont, 0)
		if w > attrW {
			attrW = w
		}
		totalH += h + 4
	}

	if attrW < headerW+16 {
		attrW = headerW + 16
	}
	if totalH < headerH+40 {
		totalH = headerH + 40
	}

	return attrW + 16, totalH + 16
}

func drawEntity(sc *scene.Scene, ent *Entity, cx, cy float64, th *theme.Theme) {
	w, h := measureEntity(ent, th)
	x, y := cx-w/2, cy-h/2

	// Border
	st := scene.Style{
		Fill:        th.PrimaryColor,
		Stroke:      th.PrimaryBorderColor,
		StrokeWidth: 1.5,
	}
	sc.Add(scene.RectPath(x, y, w, h, 2, st))

	// Header with entity name
	headerFont := diagram.BoldFont(th, 1)
	_, headerH := scene.MeasureBlock(ent.Label, headerFont, 0)

	// Header background
	headerBg := scene.Style{
		Fill:   th.PrimaryBorderColor,
		Stroke: color.RGBA{0, 0, 0, 0},
	}
	sc.Add(scene.RectPath(x, y, w, headerH+8, 0, headerBg))

	// Header text
	sc.Add(scene.NewText(cx, y+4+headerH/2, ent.Label, headerFont, th.PrimaryTextColor, scene.AnchorMiddle, scene.VAlignMiddle))

	// Draw divider line
	st.Stroke = th.PrimaryBorderColor
	st.Fill = color.RGBA{0, 0, 0, 0}
	sc.Add(scene.Line(x, y+headerH+8, x+w, y+headerH+8, st))

	// Draw attributes
	attrFont := diagram.Font(th, 0.9)
	attrY := y + headerH + 12
	alternateRow := false

	for _, attr := range ent.Attrs {
		line := fmt.Sprintf("%s %s", attr.Type, attr.Name)
		if attr.Keys != "" {
			line += fmt.Sprintf(" %s", attr.Keys)
		}
		if attr.Comment != "" {
			line += fmt.Sprintf(" %s", attr.Comment)
		}

		_, attrH := scene.MeasureBlock(line, attrFont, 0)

		// Alternating row background
		if alternateRow {
			altBg := scene.Style{
				Fill:   theme.Mix(th.PrimaryColor, th.Background, 0.3),
				Stroke: color.RGBA{0, 0, 0, 0},
			}
			sc.Add(scene.RectPath(x, attrY-2, w, attrH+4, 0, altBg))
		}

		// Attribute text
		sc.Add(scene.NewText(x+8, attrY+attrH/2, line, attrFont, th.PrimaryTextColor, scene.AnchorStart, scene.VAlignMiddle))

		attrY += attrH + 4
		alternateRow = !alternateRow
	}
}

func drawRelationship(sc *scene.Scene, rel *Relationship, points []scene.Point, th *theme.Theme) {
	if len(points) < 2 {
		return
	}

	// Determine markers
	startMarker := cardinalityToMarker(rel.FromCard)
	endMarker := cardinalityToMarker(rel.ToCard)

	// Line style
	dash := []float64{}
	if !rel.Identifying {
		dash = []float64{4, 4}
	}

	st := scene.Style{
		Stroke:      th.LineColor,
		StrokeWidth: 1.5,
		Dash:        dash,
	}

	opts := scene.EdgeOpts{
		Style:  st,
		Curve:  scene.CurveLinear,
		Start:  startMarker,
		End:    endMarker,
		Marker: scene.MarkerOpts{Stroke: th.LineColor, Hollow: th.Background},
	}

	sc.Add(scene.Edge(points, opts)...)
}

func drawEdgeLabel(sc *scene.Scene, label string, pos scene.Point, th *theme.Theme) {
	if label == "" {
		return
	}

	font := diagram.Font(th, 0.9)
	w, h := scene.MeasureBlock(label, font, 0)

	// Label background
	bgSt := scene.Style{
		Fill:   th.EdgeLabelBg,
		Stroke: color.RGBA{0, 0, 0, 0},
	}
	sc.Add(scene.RectPath(pos.X-w/2-4, pos.Y-h/2-2, w+8, h+4, 2, bgSt))

	// Label text
	sc.Add(scene.NewText(pos.X, pos.Y, label, font, th.TextColor, scene.AnchorMiddle, scene.VAlignMiddle))
}

func cardinalityToMarker(card Cardinality) scene.MarkerKind {
	switch card {
	case CardExactlyOne:
		return scene.MarkerERExactlyOne
	case CardZeroOrOne:
		return scene.MarkerERZeroOrOne
	case CardOneOrMore:
		return scene.MarkerEROneOrMore
	case CardZeroOrMore:
		return scene.MarkerERZeroOrMore
	default:
		return scene.MarkerNone
	}
}
