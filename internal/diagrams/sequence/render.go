package sequence

import (
	"fmt"
	"math"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

// Renderer draws a sequence diagram into a scene.
type Renderer struct {
	d           *Diagram
	sc          *scene.Scene
	th          *theme.Theme
	cfg         *diagram.Config
	positions   map[*Participant]float64
	width       float64
	height      float64
	margin      float64
	boxHeight   float64
	messageY    float64
	lifelineX   map[*Participant]float64
	activations map[*Participant]int // depth of activations
}

func newRenderer(d *Diagram, sc *scene.Scene, th *theme.Theme, cfg *diagram.Config) *Renderer {
	return &Renderer{
		d:           d,
		sc:          sc,
		th:          th,
		cfg:         cfg,
		positions:   make(map[*Participant]float64),
		lifelineX:   make(map[*Participant]float64),
		activations: make(map[*Participant]int),
		margin:      30,
		boxHeight:   50,
		messageY:    60,
	}
}

func (r *Renderer) render() {
	// Get configuration
	mirrorActors := r.cfg.Bool("sequence", "mirrorActors", true)
	actorMargin := r.cfg.Float("sequence", "actorMargin", 20)

	if len(r.d.participants) == 0 {
		return
	}

	// Calculate positions
	r.calculatePositions(actorMargin)

	// Draw participant boxes (top)
	r.drawParticipantBoxes(true)

	// Draw messages
	messageNum := 1.0
	if r.d.autonumber != nil {
		messageNum = r.d.autonumber.Start
	}

	for _, msg := range r.d.messages {
		if r.d.autonumber != nil {
			r.drawMessage(msg, messageNum)
			messageNum += r.d.autonumber.Increment
		} else {
			r.drawMessage(msg, 0)
		}
		r.messageY += 60 // Spacing between messages
	}

	// Draw lifelines
	r.drawLifelines()

	// Draw participant boxes (bottom) if mirror enabled
	if mirrorActors {
		bottomY := r.messageY + 20
		for _, p := range r.d.participants {
			x := r.lifelineX[p]
			r.drawActorBox(p, x, bottomY)
		}
	}
}

func (r *Renderer) calculatePositions(actorMargin float64) {
	// Calculate required width for participants
	f := diagram.Font(r.th, 0.9)
	maxWidth := 0.0

	for _, p := range r.d.participants {
		w, _ := scene.MeasureBlock(p.Display, f, 0)
		if w > maxWidth {
			maxWidth = w
		}
	}

	// Add padding and icon space
	participantWidth := maxWidth + 30

	// Also consider message label widths between adjacent participants
	for i := 0; i < len(r.d.participants)-1; i++ {
		minSpacing := participantWidth + 40 // minimum between neighbors
		maxMsgWidth := 0.0

		// Look at messages between these two participants
		for _, msg := range r.d.messages {
			if (msg.From == r.d.participants[i] && msg.To == r.d.participants[i+1]) ||
				(msg.From == r.d.participants[i+1] && msg.To == r.d.participants[i]) {
				mw, _ := scene.MeasureBlock(msg.Label, f, 0)
				if mw > maxMsgWidth {
					maxMsgWidth = mw
				}
			}
		}

		if maxMsgWidth+20 > minSpacing {
			minSpacing = maxMsgWidth + 40
		}
	}

	// Position participants horizontally
	x := r.margin + participantWidth/2
	for _, p := range r.d.participants {
		r.lifelineX[p] = x
		x += participantWidth + 30
	}

	r.width = x + r.margin
}

func (r *Renderer) drawParticipantBoxes(top bool) {
	y := r.margin
	if !top {
		y = r.messageY + 40
	}

	for _, p := range r.d.participants {
		x := r.lifelineX[p]
		r.drawActorBox(p, x, y)
	}
}

func (r *Renderer) drawActorBox(p *Participant, x, y float64) {
	f := diagram.Font(r.th, 0.9)
	w, h := scene.MeasureBlock(p.Display, f, 0)
	w += 20
	h += 10

	boxX := x - w/2
	boxY := y - h/2

	// Draw appropriate shape based on participant type
	switch p.Type {
	case TypeActor:
		r.drawActorStickFigure(x, y, w, h)
	case TypeDatabase:
		r.drawDatabaseSymbol(x, y, w, h)
	case TypeQueue:
		r.drawQueueSymbol(x, y, w, h)
	case TypeCollections:
		r.drawCollectionsSymbol(x, y, w, h)
	case TypeBoundary:
		r.drawBoundarySymbol(x, y, w, h)
	case TypeControl:
		r.drawControlSymbol(x, y, w, h)
	case TypeEntity:
		r.drawEntitySymbol(x, y, w, h)
	default:
		// Regular participant: rounded rectangle
		st := scene.Style{
			Fill:   r.th.ActorBkg,
			Stroke: r.th.ActorBorder,
			Shadow: true,
		}
		r.sc.Add(scene.RectPath(boxX, boxY, w, h, 8, st))
	}

	// Draw text
	txt := scene.NewText(x, y, p.Display, f, r.th.ActorText, scene.AnchorMiddle, scene.VAlignMiddle)
	r.sc.Add(txt)
}

func (r *Renderer) drawActorStickFigure(x, y float64, w, h float64) {
	// Draw a stick figure
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

func (r *Renderer) drawDatabaseSymbol(x, y float64, w, h float64) {
	// Draw a database cylinder
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

func (r *Renderer) drawQueueSymbol(x, y float64, w, h float64) {
	// Draw a queue (like stacked rectangles)
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

func (r *Renderer) drawCollectionsSymbol(x, y float64, w, h float64) {
	// Draw a collections symbol (like a bag with multiple items)
	st := scene.Style{
		Fill:   r.th.ActorBkg,
		Stroke: r.th.ActorBorder,
	}

	// Draw multiple circles
	r.sc.Add(scene.Circle(x-6, y-4, 4, st))
	r.sc.Add(scene.Circle(x+6, y-4, 4, st))
	r.sc.Add(scene.Circle(x, y+6, 4, st))
}

func (r *Renderer) drawBoundarySymbol(x, y float64, w, h float64) {
	// Draw a boundary symbol (like a semicircle on left side of rectangle)
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

func (r *Renderer) drawControlSymbol(x, y float64, w, h float64) {
	// Draw a control symbol (circle with something inside)
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

func (r *Renderer) drawEntitySymbol(x, y float64, w, h float64) {
	// Draw an entity symbol (rectangle)
	st := scene.Style{
		Fill:   r.th.ActorBkg,
		Stroke: r.th.ActorBorder,
	}
	r.sc.Add(scene.RectPath(x-w/2+2, y-h/2, w-4, h, 4, st))
}

func (r *Renderer) drawLifelines() {
	st := scene.Style{
		Stroke:      r.th.ActorLine,
		StrokeWidth: 1.0,
		Dash:        []float64{4, 4},
	}

	topY := r.margin + r.boxHeight/2
	bottomY := r.messageY + 20

	for _, p := range r.d.participants {
		x := r.lifelineX[p]
		r.sc.Add(scene.Line(x, topY, x, bottomY, st))
	}
}

func (r *Renderer) drawMessage(msg Message, seqNum float64) {
	if msg.From == nil || msg.To == nil {
		return
	}

	x1 := r.lifelineX[msg.From]
	x2 := r.lifelineX[msg.To]

	y := r.messageY

	// Draw self-loop if applicable
	if msg.SelfLoop {
		r.drawSelfMessage(msg, x1, y, seqNum)
		return
	}

	// Draw line and arrow
	r.drawMessageLine(msg, x1, y, x2, y, seqNum)

	// Draw activation boxes if needed
	if msg.ActivateOn == "+" {
		r.drawActivationBox(msg.To, y+15)
		r.activations[msg.To]++
	} else if msg.ActivateOn == "-" {
		if r.activations[msg.To] > 0 {
			r.activations[msg.To]--
		}
	}
}

func (r *Renderer) drawSelfMessage(msg Message, x, y float64, seqNum float64) {
	// Draw a loop back to the same participant
	offset := 30.0
	if r.activations[msg.From] > 0 {
		offset += float64(r.activations[msg.From]) * 10
	}

	rightX := x + offset
	centerY := y + 15

	// Draw right vertical line
	st := r.getLineStyle(msg.Arrow)
	r.sc.Add(scene.Line(x, y, rightX, y, st))
	r.sc.Add(scene.Line(rightX, y, rightX, centerY+15, st))
	r.sc.Add(scene.Line(rightX, centerY+15, x, centerY+15, st))

	// Draw arrow marker
	r.drawArrowHead(msg.Arrow, x, centerY+15)

	// Draw label
	r.drawMessageLabel(msg, x+(rightX-x)/2, centerY, seqNum)
}

func (r *Renderer) drawMessageLine(msg Message, x1, y1, x2, y2 float64, seqNum float64) {
	st := r.getLineStyle(msg.Arrow)

	// Shorten line for arrow markers
	dx := x2 - x1
	dy := y2 - y1
	dist := math.Hypot(dx, dy)
	if dist > 0 {
		factor := 1.0 - 5/dist // Leave room for arrow
		if factor > 0.3 {
			x2End := x1 + dx*factor
			y2End := y1 + dy*factor
			r.sc.Add(scene.Line(x1, y1, x2End, y2End, st))
		}
	}

	// Draw arrow head/marker
	if msg.Arrow == ArrowFilledHead || msg.Arrow == ArrowDottedFilledHead ||
		msg.Arrow == ArrowAsync || msg.Arrow == ArrowAsyncDotted {
		r.drawArrowHead(msg.Arrow, x2, y2)
	} else if msg.Arrow == ArrowCross || msg.Arrow == ArrowCrossDotted {
		r.drawCrossMarker(x2, y2)
	} else if msg.Arrow == ArrowBidirectional || msg.Arrow == ArrowBidirectionalDotted {
		r.drawBidirectionalArrows(x1, y1, x2, y2, st)
	}

	// Draw label centered above the line
	midX := (x1 + x2) / 2
	midY := (y1+y2)/2 - 8
	r.drawMessageLabel(msg, midX, midY, seqNum)
}

func (r *Renderer) drawArrowHead(atype ArrowType, x, y float64) {
	// Draw based on arrow type
	st := scene.Style{
		Fill:   r.th.SignalColor,
		Stroke: r.th.SignalColor,
	}

	switch atype {
	case ArrowFilledHead, ArrowDottedFilledHead:
		// Filled triangle
		sz := 6.0
		r.sc.Add(scene.NewPath(st).Polygon(
			scene.Pt(x, y),
			scene.Pt(x-sz, y-sz),
			scene.Pt(x-sz*0.5, y),
			scene.Pt(x-sz, y+sz),
		))
	case ArrowAsync, ArrowAsyncDotted:
		// Half arrow
		sz := 6.0
		lineSt := scene.Style{Stroke: r.th.SignalColor, StrokeWidth: 1.5}
		r.sc.Add(scene.Line(x-sz, y-sz, x, y, lineSt))
		r.sc.Add(scene.Line(x-sz, y+sz, x, y, lineSt))
	}
}

func (r *Renderer) drawCrossMarker(x, y float64) {
	st := scene.Style{
		Stroke:      r.th.SignalColor,
		StrokeWidth: 1.5,
	}
	sz := 5.0
	r.sc.Add(scene.Line(x-sz, y-sz, x+sz, y+sz, st))
	r.sc.Add(scene.Line(x+sz, y-sz, x-sz, y+sz, st))
}

func (r *Renderer) drawBidirectionalArrows(x1, y1, x2, y2 float64, st scene.Style) {
	// Draw arrows on both ends
	sz := 6.0

	// Arrow at end
	dx := x2 - x1
	dy := y2 - y1
	dist := math.Hypot(dx, dy)
	if dist > 0 {
		ux := dx / dist
		uy := dy / dist
		r.sc.Add(scene.NewPath(scene.Style{Fill: r.th.SignalColor, Stroke: r.th.SignalColor}).Polygon(
			scene.Pt(x2, y2),
			scene.Pt(x2-ux*sz-uy*sz, y2-uy*sz+ux*sz),
			scene.Pt(x2-ux*sz*0.5, y2-uy*sz*0.5),
			scene.Pt(x2-ux*sz+uy*sz, y2-uy*sz-ux*sz),
		))
	}

	// Arrow at start
	if dist > 0 {
		ux := -dx / dist
		uy := -dy / dist
		r.sc.Add(scene.NewPath(scene.Style{Fill: r.th.SignalColor, Stroke: r.th.SignalColor}).Polygon(
			scene.Pt(x1, y1),
			scene.Pt(x1-ux*sz-uy*sz, y1-uy*sz+ux*sz),
			scene.Pt(x1-ux*sz*0.5, y1-uy*sz*0.5),
			scene.Pt(x1-ux*sz+uy*sz, y1-uy*sz-ux*sz),
		))
	}
}

func (r *Renderer) drawMessageLabel(msg Message, x, y float64, seqNum float64) {
	f := diagram.Font(r.th, 0.85)

	label := msg.Label
	if seqNum > 0 {
		label = fmt.Sprintf("%.0f: %s", seqNum, msg.Label)
	}

	if label != "" {
		// Draw label with background
		w, h := scene.MeasureBlock(label, f, 0)
		w += 6
		h += 4

		// Background rect
		bgSt := scene.Style{
			Fill:   r.th.Background,
			Stroke: r.th.SignalColor,
		}
		r.sc.Add(scene.RectPath(x-w/2, y-h/2, w, h, 2, bgSt))

		// Text
		txt := scene.NewText(x, y, label, f, r.th.SignalText, scene.AnchorMiddle, scene.VAlignMiddle)
		r.sc.Add(txt)
	}
}

func (r *Renderer) drawActivationBox(p *Participant, y float64) {
	x := r.lifelineX[p]
	w := 10.0 + float64(r.activations[p])*2
	h := 40.0

	st := scene.Style{
		Fill:   r.th.ActivationBkg,
		Stroke: r.th.ActivationBorder,
	}
	r.sc.Add(scene.RectPath(x-w/2, y, w, h, 2, st))
}

func (r *Renderer) getLineStyle(atype ArrowType) scene.Style {
	st := scene.Style{
		Stroke:      r.th.SignalColor,
		StrokeWidth: 1.5,
	}

	switch atype {
	case ArrowDotted, ArrowDottedFilledHead, ArrowCrossDotted, ArrowAsyncDotted, ArrowBidirectionalDotted:
		st.Dash = []float64{4, 4}
	}

	return st
}
