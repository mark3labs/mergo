// Package journey implements user journey diagram rendering.
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

// Task represents a task in a section.
type Task struct {
	Name   string
	Score  int // 1-5
	Actors []string
}

// Section represents a section with tasks.
type Section struct {
	Name  string
	Tasks []*Task
}

// Journey holds parsed journey data.
type Journey struct {
	Title     string
	Sections  []*Section
	AllActors []string // deduped and ordered
}

// Render renders a user journey diagram.
func Render(src string, cfg *diagram.Config) (*scene.Scene, error) {
	journey, err := Parse(src, cfg)
	if err != nil {
		return nil, err
	}

	th := cfg.Theme
	sc := diagram.NewScene(th)

	if len(journey.AllActors) == 0 || len(journey.Sections) == 0 {
		return diagram.Finish(sc, th, journey.Title), nil
	}

	// Layout constants
	const (
		padding             = 40.0
		actorBoxWidth       = 100.0
		actorBoxHeight      = 60.0
		dotRadius           = 6.0
		taskBoxHeight       = 50.0
		taskBoxWidth        = 200.0
		sectionHeaderHeight = 40.0
		scoreLineX          = 50.0
		scoreLineY          = 50.0
		scoreLineGap        = 20.0
	)

	// Actor legend on the left
	legendY := padding + 40
	actorY := legendY
	actorColors := make(map[string]color.RGBA)
	for i, actor := range journey.AllActors {
		actorColor := th.PaletteColor(i)
		actorColors[actor] = actorColor

		// Draw actor circle
		circlePath := scene.NewPath(scene.Style{
			Fill:   actorColor,
			Stroke: th.LineColor,
		})
		circlePath.Circle(scoreLineX, actorY, dotRadius)
		sc.Add(circlePath)

		// Draw actor name
		sc.Add(scene.NewText(
			scoreLineX+20, actorY,
			actor,
			diagram.Font(th, 0.9),
			th.TextColor,
			scene.AnchorStart,
			scene.VAlignMiddle,
		))

		actorY += dotRadius*2 + 15
	}

	// Calculate task column positions
	taskX := scoreLineX + 200
	taskGap := taskBoxWidth + 30

	// Draw sections and tasks
	currentY := padding + 40
	for sectionIdx, sec := range journey.Sections {
		// Draw section header
		sectionColor := th.PaletteColor(sectionIdx)
		sectionPath := scene.NewPath(scene.Style{
			Fill:   sectionColor,
			Stroke: th.LineColor,
		})
		sectionPath.Rect(taskX, currentY, taskBoxWidth*float64(len(sec.Tasks)), sectionHeaderHeight, 4)
		sc.Add(sectionPath)

		sc.Add(scene.NewText(
			taskX+taskBoxWidth*float64(len(sec.Tasks))/2, currentY+sectionHeaderHeight/2,
			sec.Name,
			diagram.Font(th, 1),
			th.PaletteTextColor(sectionIdx),
			scene.AnchorMiddle,
			scene.VAlignMiddle,
		))

		currentY += sectionHeaderHeight + 10

		// Draw tasks
		for taskIdx, task := range sec.Tasks {
			colX := taskX + float64(taskIdx)*taskGap

			// Task box colored by section
			taskPath := scene.NewPath(scene.Style{
				Fill:   theme.Lighten(sectionColor, 0.3),
				Stroke: th.LineColor,
			})
			taskPath.Rect(colX, currentY, taskBoxWidth, taskBoxHeight, 4)
			sc.Add(taskPath)

			// Task name
			wrapped := scene.WrapText(task.Name, diagram.Font(th, 0.8), taskBoxWidth-8)
			sc.Add(scene.NewText(
				colX+taskBoxWidth/2, currentY+taskBoxHeight/2,
				wrapped,
				diagram.Font(th, 0.8),
				th.PaletteTextColor(sectionIdx),
				scene.AnchorMiddle,
				scene.VAlignMiddle,
			))

			// Draw actor dots on task
			for _, actor := range task.Actors {
				actorIdx := indexOf(journey.AllActors, actor)
				if actorIdx >= 0 {
					dotY := legendY + float64(actorIdx)*(dotRadius*2+15)
					// Draw line from actor to task
					linePath := scene.NewPath(scene.Style{
						Stroke:      actorColors[actor],
						StrokeWidth: 2,
					})
					linePath.MoveTo(scoreLineX+10, dotY)
					linePath.LineTo(colX-10, currentY+taskBoxHeight/2)
					sc.Add(linePath)

					// Draw dot on task
					dotPath := scene.NewPath(scene.Style{
						Fill:   actorColors[actor],
						Stroke: th.LineColor,
					})
					dotPath.Circle(colX-10, currentY+taskBoxHeight/2, dotRadius)
					sc.Add(dotPath)
				}
			}

			// Draw satisfaction face based on score
			// Position at the right of the task
			faceX := colX + taskBoxWidth + 15
			faceY := currentY + taskBoxHeight/2
			drawFace(sc, faceX, faceY, task.Score, th)
		}

		currentY += taskBoxHeight + 30
	}

	// Draw satisfaction score axis
	scoreAxisX := scoreLineX + 100

	// Draw score line
	scorePath := scene.NewPath(scene.Style{
		Stroke:      th.LineColor,
		StrokeWidth: 1,
		Dash:        []float64{4, 4},
	})
	scorePath.MoveTo(scoreAxisX, legendY)
	scorePath.LineTo(scoreAxisX, currentY)
	sc.Add(scorePath)

	// Draw score labels
	for score := 1; score <= 5; score++ {
		y := legendY + float64(5-score)*scoreLineGap*4
		sc.Add(scene.NewText(
			scoreAxisX-20, y,
			fmt.Sprintf("%d", score),
			diagram.Font(th, 0.75),
			th.TextColor,
			scene.AnchorEnd,
			scene.VAlignMiddle,
		))
	}

	return diagram.Finish(sc, th, journey.Title), nil
}

func drawFace(sc *scene.Scene, x, y float64, score int, th *theme.Theme) {
	radius := 8.0
	faceColor := th.PaletteColor(score - 1) // Use score to select face color

	// Draw face circle
	facePath := scene.NewPath(scene.Style{
		Fill:   faceColor,
		Stroke: th.LineColor,
	})
	facePath.Circle(x, y, radius)
	sc.Add(facePath)

	// Draw expression based on score
	// Score 1: sad, 2-3: neutral, 4-5: happy
	eyeY := y - radius/2
	mouthY := y + radius/2

	// Eyes
	eyePath := scene.NewPath(scene.Style{
		Fill:   theme.ContrastText(faceColor),
		Stroke: color.RGBA{A: 0},
	})
	eyePath.Circle(x-radius/3, eyeY, 1.5)
	eyePath.Circle(x+radius/3, eyeY, 1.5)
	sc.Add(eyePath)

	// Mouth
	mouthPath := scene.NewPath(scene.Style{
		Stroke:      theme.ContrastText(faceColor),
		StrokeWidth: 1,
	})

	switch {
	case score <= 2:
		// Sad mouth (downward arc)
		mouthPath.Arc(x, mouthY, radius/2, radius/3, math.Pi, 2*math.Pi, false)
	case score <= 3:
		// Neutral mouth (horizontal line)
		mouthPath.MoveTo(x-radius/3, mouthY)
		mouthPath.LineTo(x+radius/3, mouthY)
	default:
		// Happy mouth (upward arc)
		mouthPath.Arc(x, mouthY+radius/4, radius/2, radius/3, 0, math.Pi, false)
	}
	sc.Add(mouthPath)
}

func indexOf(slice []string, s string) int {
	for i, v := range slice {
		if v == s {
			return i
		}
	}
	return -1
}

// Parse parses user journey source code.
func Parse(src string, cfg *diagram.Config) (*Journey, error) {
	journey := &Journey{
		Sections:  make([]*Section, 0),
		AllActors: make([]string, 0),
	}

	lines := diagram.Lines(src)
	var currentSection *Section
	actorSet := make(map[string]bool)

	for lineNum, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "title ") {
			journey.Title = diagram.CleanLabel(trimmed[6:])
		} else if strings.HasPrefix(trimmed, "section ") {
			sectionName := diagram.CleanLabel(trimmed[8:])
			currentSection = &Section{
				Name:  sectionName,
				Tasks: make([]*Task, 0),
			}
			journey.Sections = append(journey.Sections, currentSection)
		} else {
			// Task line: "Task name: score: actor1, actor2"
			colonParts := strings.Split(trimmed, ":")
			if len(colonParts) < 3 {
				// Invalid task line, skip
				continue
			}

			taskName := diagram.CleanLabel(colonParts[0])
			scoreStr := strings.TrimSpace(colonParts[1])
			actorStr := strings.TrimSpace(strings.Join(colonParts[2:], ":"))

			score, err := strconv.Atoi(scoreStr)
			if err != nil || score < 1 || score > 5 {
				return nil, fmt.Errorf("line %d: invalid score %q, must be 1-5", lineNum+1, scoreStr)
			}

			// Parse actors
			var actors []string
			if actorStr != "" {
				for actor := range strings.SplitSeq(actorStr, ",") {
					actor = diagram.CleanLabel(actor)
					if actor != "" {
						actors = append(actors, actor)
						if !actorSet[actor] {
							actorSet[actor] = true
							journey.AllActors = append(journey.AllActors, actor)
						}
					}
				}
			}

			task := &Task{
				Name:   taskName,
				Score:  score,
				Actors: actors,
			}

			if currentSection != nil {
				currentSection.Tasks = append(currentSection.Tasks, task)
			}
		}
	}

	return journey, nil
}
