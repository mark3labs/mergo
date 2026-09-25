package sequence

import (
	"fmt"
	"image/color"
	"math"
	"sort"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

// Renderer renders a parsed sequence diagram into a scene.
type Renderer struct {
	d      *Diagram
	sc     *scene.Scene
	th     *theme.Theme
	cfg    *diagram.Config
	layout *Layout
}

// Layout holds computed layout information.
type Layout struct {
	ParticipantX      map[*Participant]float64
	ParticipantWidth  float64
	ParticipantHeight float64
	ActorMargin       float64
	MessageY          float64
	ActivationStack   map[*Participant][]ActivationBar
	MessageNum        float64
	TopMargin         float64
	LeftMargin        float64
}

// ActivationBar tracks active/inactive regions for a participant.
type ActivationBar struct {
	StartY float64
	EndY   float64
	Depth  int
}

func newRenderer(d *Diagram, sc *scene.Scene, th *theme.Theme, cfg *diagram.Config) *Renderer {
	return &Renderer{
		d:   d,
		sc:  sc,
		th:  th,
		cfg: cfg,
		layout: &Layout{
			ParticipantX:      make(map[*Participant]float64),
			ActivationStack:   make(map[*Participant][]ActivationBar),
			TopMargin:         30,
			LeftMargin:        20,
			ActorMargin:       50.0,
			ParticipantWidth:  150.0,
			ParticipantHeight: 65.0,
			MessageY:          100.0,
			MessageNum:        1.0,
		},
	}
}

func (r *Renderer) render() error {
	// Read config
	r.layout.ActorMargin = r.cfg.Float("sequence", "actorMargin", 50)
	r.layout.ParticipantWidth = r.cfg.Float("sequence", "width", 150)
	r.layout.ParticipantHeight = r.cfg.Float("sequence", "height", 65)

	if len(r.d.Participants) == 0 {
		return nil
	}

	mirrorActors := r.cfg.Bool("sequence", "mirrorActors", true)

	// Calculate participant positions
	r.calculateLayout()

	// Draw participant boxes and lifelines
	r.drawParticipants(true)
	r.drawLifelines()

	// Process all statements (messages, notes, blocks)
	r.layout.MessageY = r.layout.TopMargin + r.layout.ParticipantHeight + 40

	if r.d.Autonumber != nil && r.d.Autonumber.Active {
		r.layout.MessageNum = r.d.Autonumber.Start
	}

	r.renderStatements(r.d.Messages, 0, 0)

	// Draw bottom participant boxes if mirrored
	if mirrorActors {
		bottomY := r.layout.MessageY + 40
		for _, p := range r.d.Participants {
			r.drawActorBox(p, r.layout.ParticipantX[p], bottomY)
		}
	}

	return nil
}

func (r *Renderer) calculateLayout() {
	// Measure participant widths
	f := diagram.Font(r.th, 0.9)
	maxWidth := 0.0

	for _, p := range r.d.Participants {
		w, _ := scene.MeasureBlock(p.Alias, f, 0)
		if w > maxWidth {
			maxWidth = w
		}
	}

	// Account for icons and padding
	participantWidth := math.Max(maxWidth+30, r.layout.ParticipantWidth)

	// Calculate spacing accounting for message labels
	baseSpacing := participantWidth + r.layout.ActorMargin

	// Layout horizontally
	x := r.layout.LeftMargin + participantWidth/2
	for _, p := range r.d.Participants {
		r.layout.ParticipantX[p] = x
		x += baseSpacing
	}

	r.sc.Width = x + r.layout.LeftMargin
}

func (r *Renderer) drawParticipants(top bool) {
	y := r.layout.TopMargin
	if !top {
		y = r.layout.MessageY + 40
	}

	for _, p := range r.d.Participants {
		x := r.layout.ParticipantX[p]
		r.drawActorBox(p, x, y)
	}
}

func (r *Renderer) drawActorBox(p *Participant, x, y float64) {
	f := diagram.Font(r.th, 0.9)
	w, h := scene.MeasureBlock(p.Alias, f, 0)
	w += 20
	h += 10

	boxX := x - w/2
	boxY := y - h/2

	// Draw based on participant type
	switch p.Type {
	case TypeActor:
		r.drawStickFigure(x, y)
	case TypeBoundary:
		r.drawBoundarySymbol(x, y, w, h)
	case TypeControl:
		r.drawControlSymbol(x, y, w, h)
	case TypeEntity:
		r.drawEntitySymbol(x, y, w, h)
	case TypeDatabase:
		r.drawDatabaseSymbol(x, y, w, h)
	case TypeQueue:
		r.drawQueueSymbol(x, y, w, h)
	case TypeCollections:
		r.drawCollectionsSymbol(x, y, w, h)
	default:
		// Regular participant: rounded rectangle
		st := scene.Style{
			Fill:   r.th.ActorBkg,
			Stroke: r.th.ActorBorder,
			Shadow: true,
		}
		r.sc.Add(scene.RectPath(boxX, boxY, w, h, 3, st))
	}

	// Draw text
	txt := scene.NewText(x, y, p.Alias, f, r.th.ActorText, scene.AnchorMiddle, scene.VAlignMiddle)
	r.sc.Add(txt)
}

func (r *Renderer) drawStickFigure(x, y float64) {
	headR := 6.0
	st := scene.Style{
		Stroke: r.th.ActorBorder,
		Fill:   r.th.ActorBkg,
	}
	lineSt := scene.Style{
		Stroke:      r.th.ActorBorder,
		StrokeWidth: 1.5,
	}

	// Head
	r.sc.Add(scene.Circle(x, y-12, headR, st))
	// Body
	r.sc.Add(scene.Line(x, y-6, x, y+8, lineSt))
	// Arms
	r.sc.Add(scene.Line(x-8, y, x+8, y, lineSt))
	// Legs
	r.sc.Add(scene.Line(x, y+8, x-6, y+16, lineSt))
	r.sc.Add(scene.Line(x, y+8, x+6, y+16, lineSt))
}

func (r *Renderer) drawBoundarySymbol(x, y, w, h float64) {
	st := scene.Style{
		Fill:   r.th.ActorBkg,
		Stroke: r.th.ActorBorder,
	}
	boxW := w * 0.6
	boxH := h * 0.7

	// Rectangle
	r.sc.Add(scene.RectPath(x-boxW/2+3, y-boxH/2, boxW-3, boxH, 2, st))

	// Semicircle on left
	p := scene.NewPath(st)
	p.Arc(x-boxW/2, y, 4, boxH/2, math.Pi/2, 3*math.Pi/2, false)
	p.Close()
	r.sc.Add(p)
}

func (r *Renderer) drawControlSymbol(x, y, w, h float64) {
	st := scene.Style{
		Fill:   r.th.ActorBkg,
		Stroke: r.th.ActorBorder,
	}
	// Circle
	r.sc.Add(scene.Circle(x, y, math.Min(w, h)/2, st))
	// Cross inside
	lineSt := scene.Style{Stroke: r.th.ActorBorder, StrokeWidth: 1.5}
	r.sc.Add(scene.Line(x-3, y, x+3, y, lineSt))
	r.sc.Add(scene.Line(x, y-3, x, y+3, lineSt))
}

func (r *Renderer) drawEntitySymbol(x, y, w, h float64) {
	st := scene.Style{
		Fill:   r.th.ActorBkg,
		Stroke: r.th.ActorBorder,
	}
	r.sc.Add(scene.RectPath(x-w/2+2, y-h/2, w-4, h, 4, st))
}

func (r *Renderer) drawDatabaseSymbol(x, y, w, h float64) {
	st := scene.Style{
		Fill:   r.th.ActorBkg,
		Stroke: r.th.ActorBorder,
	}
	boxH := h * 0.6
	boxY := y - boxH/2

	// Top ellipse
	p := scene.NewPath(st)
	p.Ellipse(x, boxY, w/2-2, 3)
	r.sc.Add(p)

	// Sides
	r.sc.Add(scene.Line(x-w/2+2, boxY, x-w/2+2, boxY+boxH, scene.Style{Stroke: r.th.ActorBorder}))
	r.sc.Add(scene.Line(x+w/2-2, boxY, x+w/2-2, boxY+boxH, scene.Style{Stroke: r.th.ActorBorder}))

	// Bottom ellipse
	p = scene.NewPath(st)
	p.Ellipse(x, boxY+boxH, w/2-2, 3)
	r.sc.Add(p)
}

func (r *Renderer) drawQueueSymbol(x, y, w, h float64) {
	st := scene.Style{
		Fill:   r.th.ActorBkg,
		Stroke: r.th.ActorBorder,
	}
	boxH := h * 0.7
	boxW := w * 0.7
	step := 4.0

	for i := 2; i >= 0; i-- {
		r.sc.Add(scene.RectPath(
			x-boxW/2+float64(i)*step,
			y-boxH/2+float64(i)*step,
			boxW, boxH, 2, st,
		))
	}
}

func (r *Renderer) drawCollectionsSymbol(x, y, w, h float64) {
	st := scene.Style{
		Fill:   r.th.ActorBkg,
		Stroke: r.th.ActorBorder,
	}
	r.sc.Add(scene.Circle(x-6, y-4, 4, st))
	r.sc.Add(scene.Circle(x+6, y-4, 4, st))
	r.sc.Add(scene.Circle(x, y+6, 4, st))
}

func (r *Renderer) drawLifelines() {
	st := scene.Style{
		Stroke:      r.th.ActorLine,
		StrokeWidth: 1.0,
		Dash:        []float64{4, 4},
	}

	topY := r.layout.TopMargin + r.layout.ParticipantHeight/2 + 20
	bottomY := r.layout.MessageY + 20

	for _, p := range r.d.Participants {
		x := r.layout.ParticipantX[p]
		r.sc.Add(scene.Line(x, topY, x, bottomY, st))
	}
}

func (r *Renderer) renderStatements(stmts []Statement, nestLevel int, baseY float64) {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *Message:
			r.renderMessage(s)

		case *Note:
			r.renderNote(s)

		case *Block:
			r.renderBlock(s, nestLevel)
		}
	}
}

func (r *Renderer) renderMessage(msg *Message) {
	if msg.From == nil || msg.To == nil {
		return
	}

	// Handle autonumbering
	msgLabel := msg.Label
	if r.d.Autonumber != nil && r.d.Autonumber.Active {
		if msgLabel != "" {
			msgLabel = fmt.Sprintf("%.0f: %s", r.layout.MessageNum, msgLabel)
		}
		r.layout.MessageNum += r.d.Autonumber.Increment
	}

	x1 := r.layout.ParticipantX[msg.From]
	x2 := r.layout.ParticipantX[msg.To]
	y := r.layout.MessageY

	if msg.IsSelfLoop {
		r.renderSelfMessage(msg, x1, y, msgLabel)
	} else {
		r.renderNormalMessage(msg, x1, y, x2, y, msgLabel)
	}

	// Handle activations
	if msg.ActivateTo == "+" {
		r.pushActivation(msg.To, y)
	} else if msg.ActivateTo == "-" {
		r.popActivation(msg.To, y)
	}

	r.layout.MessageY += 40
}

func (r *Renderer) renderNormalMessage(msg *Message, x1, y1, x2, y2 float64, label string) {
	st := r.getLineStyle(msg.Arrow)

	// Draw line
	r.sc.Add(scene.Line(x1, y1, x2, y2, st))

	// Draw arrow marker
	if msg.Arrow == ArrowFilledHead || msg.Arrow == ArrowDottedFilledHead {
		r.drawFilledArrow(x2, y2)
	} else if msg.Arrow == ArrowAsync || msg.Arrow == ArrowAsyncDotted {
		r.drawOpenArrow(x2, y2, x1, x2)
	} else if msg.Arrow == ArrowCross || msg.Arrow == ArrowCrossDotted {
		r.drawCross(x2, y2)
	} else if msg.Arrow == ArrowBidirectional || msg.Arrow == ArrowBidirectionalDotted {
		r.drawBidirectional(x1, y1, x2, y2)
	}

	// Draw message label
	midX := (x1 + x2) / 2
	r.drawMessageLabel(label, midX, y1-10)
}

func (r *Renderer) renderSelfMessage(msg *Message, x, y float64, label string) {
	offset := 30.0
	if len(r.layout.ActivationStack[msg.From]) > 0 {
		offset += float64(len(r.layout.ActivationStack[msg.From])) * 10
	}

	rightX := x + offset
	bottomY := y + 25

	st := r.getLineStyle(msg.Arrow)

	// Draw self-loop path
	r.sc.Add(scene.Line(x, y, rightX, y, st))
	r.sc.Add(scene.Line(rightX, y, rightX, bottomY, st))
	r.sc.Add(scene.Line(rightX, bottomY, x, bottomY, st))

	// Draw arrow at end
	if msg.Arrow == ArrowFilledHead || msg.Arrow == ArrowDottedFilledHead {
		r.drawFilledArrow(x, bottomY)
	}

	// Draw label
	r.drawMessageLabel(label, (x+rightX)/2, y+15)

	r.layout.MessageY += 25
}

func (r *Renderer) renderNote(note *Note) {
	f := diagram.Font(r.th, 0.85)
	w, h := scene.MeasureBlock(note.Text, f, 0)
	w += 12
	h += 8

	// Determine position
	var x, y float64
	if len(note.For) == 0 {
		return
	}

	switch note.Position {
	case NoteLeft:
		x = r.layout.ParticipantX[note.For[0]] - 60 - w/2
	case NoteRight:
		x = r.layout.ParticipantX[note.For[0]] + 60 + w/2
	case NoteOver:
		if len(note.For) == 1 {
			x = r.layout.ParticipantX[note.For[0]]
		} else {
			// Span over multiple participants
			minX := r.layout.ParticipantX[note.For[0]]
			maxX := r.layout.ParticipantX[note.For[len(note.For)-1]]
			x = (minX + maxX) / 2
		}
	}

	y = r.layout.MessageY

	// Draw note box
	st := scene.Style{
		Fill:   r.th.NoteBkg,
		Stroke: r.th.NoteBorder,
	}
	r.sc.Add(scene.RectPath(x-w/2, y-h/2, w, h, 2, st))

	// Draw text
	txt := scene.NewText(x, y, note.Text, f, r.th.NoteText, scene.AnchorMiddle, scene.VAlignMiddle)
	r.sc.Add(txt)

	r.layout.MessageY += h + 20
}

func (r *Renderer) renderBlock(block *Block, nestLevel int) {
	// Determine which participants are involved
	block.Spans = r.getBlockParticipants(block)
	if len(block.Spans) == 0 {
		return
	}

	// Get bounds
	minX := r.layout.ParticipantX[block.Spans[0]]
	maxX := r.layout.ParticipantX[block.Spans[len(block.Spans)-1]]
	minX -= 20
	maxX += 20

	startY := r.layout.MessageY
	prevMessageY := r.layout.MessageY

	// Render contents
	r.renderStatements(block.Children, nestLevel+1, startY)

	// Render clauses
	for _, clause := range block.Clauses {
		r.renderStatements(clause.Children, nestLevel+1, prevMessageY)
	}

	endY := r.layout.MessageY

	// Draw block frame
	r.drawBlockFrame(block, minX, maxX, startY, endY, nestLevel)

	r.layout.MessageY = endY + 20
}

func (r *Renderer) drawBlockFrame(block *Block, minX, maxX, startY, endY float64, nestLevel int) {
	insetX := float64(nestLevel) * 10
	insetY := float64(nestLevel) * 5

	// Draw background for rect blocks
	if block.Kind == BlockRect {
		st := scene.Style{
			Fill:   parseRGBA(block.Color),
			Stroke: color.RGBA{0, 0, 0, 0},
		}
		r.sc.Add(scene.RectPath(minX+insetX, startY+insetY, maxX-minX-2*insetX, endY-startY-insetY, 0, st))
		return
	}

	// Draw border
	borderSt := scene.Style{
		Stroke:      r.th.LabelBoxBorder,
		StrokeWidth: 1.5,
		Dash:        []float64{3, 3},
		Fill:        color.RGBA{0, 0, 0, 0},
	}
	r.sc.Add(scene.RectPath(minX+insetX, startY+insetY, maxX-minX-2*insetX, endY-startY-insetY, 0, borderSt))

	// Draw label box
	f := diagram.Font(r.th, 0.85)
	labelW, labelH := scene.MeasureBlock(block.Label, f, 0)
	labelW += 8
	labelH += 4

	labelSt := scene.Style{
		Fill:   r.th.LabelBoxBkg,
		Stroke: r.th.LabelBoxBorder,
	}
	labelX := minX + insetX + 5
	labelY := startY + insetY

	// Pentagon-like label box
	r.sc.Add(scene.RectPath(labelX, labelY, labelW, labelH, 2, labelSt))

	txt := scene.NewText(labelX+labelW/2, labelY+labelH/2, block.Label, f, r.th.LabelText, scene.AnchorMiddle, scene.VAlignMiddle)
	r.sc.Add(txt)

	// Draw condition for alt/par/critical clauses
	if len(block.Clauses) > 0 {
		clauseY := startY + insetY + 30
		for _, clause := range block.Clauses {
			if clause.Cond != "" {
				dashSt := scene.Style{
					Stroke: r.th.LabelBoxBorder,
					Dash:   []float64{3, 3},
				}
				r.sc.Add(scene.Line(minX+insetX, clauseY, maxX-insetX, clauseY, dashSt))

				condW, condH := scene.MeasureBlock(clause.Cond, f, 0)
				condW += 4
				r.sc.Add(scene.RectPath(labelX, clauseY-condH/2, condW, condH, 1, labelSt))
				txt := scene.NewText(labelX+condW/2, clauseY, clause.Cond, f, r.th.LabelText, scene.AnchorMiddle, scene.VAlignMiddle)
				r.sc.Add(txt)
			}
			clauseY += 40
		}
	}
}

func (r *Renderer) getBlockParticipants(block *Block) []*Participant {
	seen := make(map[*Participant]bool)
	var result []*Participant

	var collect func([]Statement)
	collect = func(stmts []Statement) {
		for _, stmt := range stmts {
			switch s := stmt.(type) {
			case *Message:
				if s.From != nil && !seen[s.From] {
					seen[s.From] = true
					result = append(result, s.From)
				}
				if s.To != nil && !seen[s.To] {
					seen[s.To] = true
					result = append(result, s.To)
				}
			case *Block:
				collect(s.Children)
				for _, clause := range s.Clauses {
					collect(clause.Children)
				}
			case *Note:
				for _, p := range s.For {
					if !seen[p] {
						seen[p] = true
						result = append(result, p)
					}
				}
			}
		}
	}

	collect(block.Children)
	for _, clause := range block.Clauses {
		collect(clause.Children)
	}

	// Sort by original order
	sort.Slice(result, func(i, j int) bool {
		var idx1, idx2 int
		for k, p := range r.d.Participants {
			if p == result[i] {
				idx1 = k
			}
			if p == result[j] {
				idx2 = k
			}
		}
		return idx1 < idx2
	})

	return result
}

func (r *Renderer) pushActivation(p *Participant, y float64) {
	bar := ActivationBar{StartY: y, Depth: len(r.layout.ActivationStack[p])}
	r.layout.ActivationStack[p] = append(r.layout.ActivationStack[p], bar)
}

func (r *Renderer) popActivation(p *Participant, y float64) {
	if len(r.layout.ActivationStack[p]) > 0 {
		bar := &r.layout.ActivationStack[p][len(r.layout.ActivationStack[p])-1]
		bar.EndY = y
		r.layout.ActivationStack[p] = r.layout.ActivationStack[p][:len(r.layout.ActivationStack[p])-1]
		r.drawActivationBar(p, bar)
	}
}

func (r *Renderer) drawActivationBar(p *Participant, bar *ActivationBar) {
	x := r.layout.ParticipantX[p]
	w := 10.0 + float64(bar.Depth)*3
	h := bar.EndY - bar.StartY

	st := scene.Style{
		Fill:   r.th.ActivationBkg,
		Stroke: r.th.ActivationBorder,
	}
	r.sc.Add(scene.RectPath(x-w/2, bar.StartY, w, h, 2, st))
}

func (r *Renderer) getLineStyle(atype ArrowType) scene.Style {
	st := scene.Style{
		Stroke:      r.th.SignalColor,
		StrokeWidth: 1.5,
	}

	switch atype {
	case ArrowDotted, ArrowDottedFilledHead, ArrowCrossDotted, ArrowAsyncDotted, ArrowBidirectionalDotted:
		st.Dash = []float64{3, 3}
	}

	return st
}

func (r *Renderer) drawFilledArrow(x, y float64) {
	sz := 6.0
	st := scene.Style{
		Fill:   r.th.SignalColor,
		Stroke: r.th.SignalColor,
	}
	r.sc.Add(scene.NewPath(st).Polygon(
		scene.Pt(x, y),
		scene.Pt(x-sz, y-sz),
		scene.Pt(x-sz*0.5, y),
		scene.Pt(x-sz, y+sz),
	))
}

func (r *Renderer) drawOpenArrow(x, y, fromX, toX float64) {
	sz := 6.0
	st := scene.Style{Stroke: r.th.SignalColor, StrokeWidth: 1.5}
	r.sc.Add(scene.Line(x-sz-sz, y-sz, x, y, st))
	r.sc.Add(scene.Line(x-sz-sz, y+sz, x, y, st))
}

func (r *Renderer) drawCross(x, y float64) {
	st := scene.Style{
		Stroke:      r.th.SignalColor,
		StrokeWidth: 1.5,
	}
	sz := 5.0
	r.sc.Add(scene.Line(x-sz, y-sz, x+sz, y+sz, st))
	r.sc.Add(scene.Line(x+sz, y-sz, x-sz, y+sz, st))
}

func (r *Renderer) drawBidirectional(x1, y1, x2, y2 float64) {
	sz := 6.0
	st := scene.Style{
		Fill:   r.th.SignalColor,
		Stroke: r.th.SignalColor,
	}

	dx := x2 - x1
	dy := y2 - y1
	dist := math.Hypot(dx, dy)
	if dist == 0 {
		return
	}

	ux := dx / dist
	uy := dy / dist

	// Arrow at end
	r.sc.Add(scene.NewPath(st).Polygon(
		scene.Pt(x2, y2),
		scene.Pt(x2-ux*sz-uy*sz, y2-uy*sz+ux*sz),
		scene.Pt(x2-ux*sz*0.5, y2-uy*sz*0.5),
		scene.Pt(x2-ux*sz+uy*sz, y2-uy*sz-ux*sz),
	))

	// Arrow at start
	ux = -ux
	uy = -uy
	r.sc.Add(scene.NewPath(st).Polygon(
		scene.Pt(x1, y1),
		scene.Pt(x1-ux*sz-uy*sz, y1-uy*sz+ux*sz),
		scene.Pt(x1-ux*sz*0.5, y1-uy*sz*0.5),
		scene.Pt(x1-ux*sz+uy*sz, y1-uy*sz-ux*sz),
	))
}

func (r *Renderer) drawMessageLabel(text string, x, y float64) {
	if text == "" {
		return
	}

	f := diagram.Font(r.th, 0.85)
	w, h := scene.MeasureBlock(text, f, 0)
	w += 6
	h += 4

	bgSt := scene.Style{
		Fill:   r.th.Background,
		Stroke: r.th.SignalColor,
	}
	r.sc.Add(scene.RectPath(x-w/2, y-h/2, w, h, 2, bgSt))

	txt := scene.NewText(x, y, text, f, r.th.SignalText, scene.AnchorMiddle, scene.VAlignMiddle)
	r.sc.Add(txt)
}

// Helper to parse color from string (rgb/rgba format)
func parseRGBA(s string) color.RGBA {
	// Simple parsing for rgb(r,g,b) and rgba(r,g,b,a)
	// For now, return a semi-transparent light color
	return color.RGBA{200, 200, 255, 50}
}
