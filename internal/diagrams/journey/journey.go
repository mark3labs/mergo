// Package journey implements Mermaid user journey diagrams.
package journey

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

func init() {
	diagram.Register(diagram.Type{
		Name:   "journey",
		Detect: diagram.Keyword("journey"),
		Render: Render,
	})
}

// Task is a journey step.
type Task struct {
	Name    string
	Score   int
	Actors  []string
	Section int
}

// Journey is a parsed user journey.
type Journey struct {
	Title    string
	Sections []string
	Tasks    []*Task
	Actors   []string
}

// Parse parses journey source.
func Parse(src string) (*Journey, error) {
	j := &Journey{}
	header := false
	section := -1
	seen := map[string]bool{}
	for i, raw := range strings.Split(src, "\n") {
		ln := i + 1
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		if !header {
			if !strings.HasPrefix(line, "journey") {
				return nil, fmt.Errorf("line %d: expected 'journey'", ln)
			}
			header = true
			continue
		}
		switch {
		case strings.HasPrefix(line, "title "):
			j.Title = diagram.CleanLabel(line[6:])
			continue
		case strings.HasPrefix(line, "section "):
			j.Sections = append(j.Sections, diagram.CleanLabel(line[8:]))
			section = len(j.Sections) - 1
			continue
		case strings.HasPrefix(line, "accTitle") || strings.HasPrefix(line, "accDescr"):
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 2 {
			return nil, fmt.Errorf("line %d: expected 'Task name: score: actors'", ln)
		}
		score, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("line %d: invalid score %q", ln, strings.TrimSpace(parts[1]))
		}
		score = max(1, min(5, score))
		if section < 0 {
			j.Sections = append(j.Sections, "")
			section = 0
		}
		t := &Task{Name: diagram.CleanLabel(parts[0]), Score: score, Section: section}
		if len(parts) == 3 {
			for _, a := range strings.Split(parts[2], ",") {
				if a = strings.TrimSpace(a); a != "" {
					t.Actors = append(t.Actors, a)
					if !seen[a] {
						seen[a] = true
						j.Actors = append(j.Actors, a)
					}
				}
			}
		}
		j.Tasks = append(j.Tasks, t)
	}
	return j, nil
}

const (
	taskW   = 150.0
	taskGap = 16.0
)

// Render renders a user journey.
func Render(src string, cfg *diagram.Config) (*scene.Scene, error) {
	j, err := Parse(src)
	if err != nil {
		return nil, err
	}
	th := cfg.Theme
	sc := diagram.NewScene(th)
	title := j.Title
	if title == "" {
		title = cfg.Title
	}
	if len(j.Tasks) == 0 {
		sc.Add(scene.NewText(0, 0, "(empty journey)", diagram.Font(th, 1), th.TextColor, scene.AnchorMiddle, scene.VAlignMiddle))
		return diagram.Finish(sc, th, title), nil
	}
	secFont := scene.Font{Size: th.FontSize * 0.95, Bold: true}
	taskFont := scene.Font{Size: th.FontSize * 0.88}
	legendFont := scene.Font{Size: th.FontSize * 0.9}
	actorColor := func(a string) color.RGBA {
		for i, x := range j.Actors {
			if x == a {
				return th.ChartColor(i)
			}
		}
		return th.LineColor
	}

	// legend
	legendW := 0.0
	for _, a := range j.Actors {
		legendW = math.Max(legendW, scene.MeasureText(a, legendFont))
	}
	legendW += 40
	for i, a := range j.Actors {
		y := float64(i)*26 + 12
		sc.Add(scene.Circle(10, y, 8, scene.Style{Fill: actorColor(a), Stroke: th.Background, StrokeWidth: 1.5}))
		sc.Add(scene.NewText(26, y, a, legendFont, th.TextColor, scene.AnchorStart, scene.VAlignMiddle))
	}
	x0 := legendW + 20

	// measure heights
	secH := 0.0
	for _, s := range j.Sections {
		_, h := scene.MeasureBlock(scene.WrapText(s, secFont, taskW*2), secFont, 0)
		secH = math.Max(secH, h+18)
	}
	taskH := 0.0
	for _, t := range j.Tasks {
		_, h := scene.MeasureBlock(scene.WrapText(t.Name, taskFont, taskW-20), taskFont, 0)
		taskH = math.Max(taskH, h+22)
	}
	yTask := secH + 10
	faceTop := yTask + taskH + 34
	faceStep := 34.0
	faceBottom := faceTop + 4*faceStep
	axisY := faceBottom + 36

	xs := make([]float64, len(j.Tasks))
	for i := range j.Tasks {
		xs[i] = x0 + float64(i)*(taskW+taskGap)
	}
	// sections
	for si, s := range j.Sections {
		first, last := -1, -1
		for i, t := range j.Tasks {
			if t.Section == si {
				if first < 0 {
					first = i
				}
				last = i
			}
		}
		if first < 0 || s == "" {
			continue
		}
		c := th.PaletteColor(si)
		sx := xs[first]
		sw := xs[last] + taskW - sx
		sc.Add(scene.RectPath(sx, 0, sw, secH, 8, scene.Style{Fill: theme.Darken(c, 0.06), Shadow: true}))
		sc.Add(scene.NewText(sx+sw/2, secH/2, scene.WrapText(s, secFont, sw-16), secFont, th.PaletteTextColor(si), scene.AnchorMiddle, scene.VAlignMiddle))
	}
	// guide lines for scores
	for s := 1; s <= 5; s++ {
		y := faceTop + float64(5-s)*faceStep
		sc.Add(scene.Line(x0-6, y, xs[len(xs)-1]+taskW+6, y, scene.Style{Stroke: theme.WithAlpha(th.GridColor, 110), StrokeWidth: 1}))
	}
	// tasks
	var prev scene.Point
	for i, t := range j.Tasks {
		c := th.PaletteColor(t.Section)
		fill := theme.Mix(c, th.Background, 0.25)
		x := xs[i]
		cx := x + taskW/2
		faceY := faceTop + float64(5-t.Score)*faceStep
		sc.Add(scene.Line(cx, yTask+taskH, cx, axisY, scene.Style{Stroke: theme.WithAlpha(th.LineColor, 120), StrokeWidth: 1.2, Dash: []float64{4, 4}}))
		sc.Add(scene.RectPath(x, yTask, taskW, taskH, 8, scene.Style{Fill: fill, Stroke: c, StrokeWidth: 1.3, Shadow: true}))
		sc.Add(scene.NewText(cx, yTask+taskH/2+3, scene.WrapText(t.Name, taskFont, taskW-20), taskFont, theme.ContrastText(fill), scene.AnchorMiddle, scene.VAlignMiddle))
		for ai, a := range t.Actors {
			sc.Add(scene.Circle(x+12+float64(ai)*14, yTask, 6, scene.Style{Fill: actorColor(a), Stroke: th.Background, StrokeWidth: 1.5}))
		}
		cur := scene.Pt(cx, faceY)
		if i > 0 {
			sc.Add(scene.Line(prev.X, prev.Y, cur.X, cur.Y, scene.Style{Stroke: theme.WithAlpha(th.LineColor, 90), StrokeWidth: 2}))
		}
		prev = cur
	}
	for i, t := range j.Tasks {
		face(sc, xs[i]+taskW/2, faceTop+float64(5-t.Score)*faceStep, t.Score, th)
	}
	// axis
	sc.Add(scene.Edge([]scene.Point{{X: x0 - 10, Y: axisY}, {X: xs[len(xs)-1] + taskW + 20, Y: axisY}}, scene.EdgeOpts{
		Style: scene.Style{Stroke: th.LineColor, StrokeWidth: 2.5}, End: scene.MarkerArrow, Marker: scene.MarkerOpts{Size: 12},
	})...)
	return diagram.Finish(sc, th, title), nil
}

// face draws a smiley whose expression depends on the score (1..5).
func face(sc *scene.Scene, cx, cy float64, score int, th *theme.Theme) {
	var fill color.RGBA
	switch {
	case score >= 4:
		fill = theme.Hex("#8bd17c")
	case score == 3:
		fill = theme.Hex("#ffd966")
	default:
		fill = theme.Hex("#f4a07a")
	}
	ink := theme.Hex("#2b2b2b")
	r := 15.0
	sc.Add(scene.Circle(cx, cy, r, scene.Style{Fill: fill, Stroke: theme.Darken(fill, 0.35), StrokeWidth: 1.5, Shadow: true}))
	sc.Add(scene.Circle(cx-5, cy-4, 1.9, scene.Style{Fill: ink}), scene.Circle(cx+5, cy-4, 1.9, scene.Style{Fill: ink}))
	mouth := scene.NewPath(scene.Style{Stroke: ink, StrokeWidth: 1.8, RoundCaps: true})
	switch {
	case score >= 4:
		mouth.MoveTo(cx-6, cy+3).QuadTo(cx, cy+10, cx+6, cy+3)
	case score == 3:
		mouth.MoveTo(cx-6, cy+6).LineTo(cx+6, cy+6)
	default:
		mouth.MoveTo(cx-6, cy+9).QuadTo(cx, cy+2, cx+6, cy+9)
	}
	sc.Add(mouth)
	_ = th
}
