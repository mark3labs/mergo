// Package timeline implements timeline diagram rendering.
package timeline

import (
	"image/color"
	"math"
	"slices"
	"strings"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

func init() {
	diagram.Register(diagram.Type{
		Name:   "timeline",
		Detect: diagram.Keyword("timeline"),
		Render: Render,
	})
}

// Period represents a time period on the timeline.
type Period struct {
	Name   string
	Events []string
}

// Section represents a grouping of periods.
type Section struct {
	Name    string
	Periods []*Period
}

// Timeline holds parsed timeline data.
type Timeline struct {
	Title             string
	Sections          []*Section
	AllPeriods        []*Period // in order
	DisableMulticolor bool
}

// Render renders a timeline diagram.
func Render(src string, cfg *diagram.Config) (*scene.Scene, error) {
	tl, err := Parse(src, cfg)
	if err != nil {
		return nil, err
	}

	th := cfg.Theme
	sc := diagram.NewScene(th)

	if len(tl.AllPeriods) == 0 {
		return diagram.Finish(sc, th, tl.Title), nil
	}

	// Layout constants
	const (
		padding         = 40.0
		periodBoxHeight = 50.0
		periodBoxWidth  = 120.0
		periodGap       = 20.0
		eventBoxHeight  = 40.0
		eventBoxWidth   = 150.0
		lineHeight      = 30.0
		sectionHeight   = 60.0
		dotRadius       = 4.0
	)

	timelineY := padding + 40
	baselineY := timelineY + 20

	// Draw main timeline line
	timelineWidth := float64(len(tl.AllPeriods)) * (periodBoxWidth + periodGap)
	linePath := scene.NewPath(scene.Style{
		Stroke:      th.LineColor,
		StrokeWidth: 2,
	})
	linePath.MoveTo(padding, baselineY)
	linePath.LineTo(padding+timelineWidth, baselineY)
	sc.Add(linePath)

	// Draw period boxes above the line
	periodX := padding
	colorIndex := 0
	for i, period := range tl.AllPeriods {
		sectionIdx := 0
		for j, sec := range tl.Sections {
			if slices.Contains(sec.Periods, period) {
				sectionIdx = j
			}
		}

		if !tl.DisableMulticolor {
			colorIndex = i
		} else if len(tl.Sections) > 0 {
			colorIndex = sectionIdx
		}

		bgColor := th.PaletteColor(colorIndex)
		textColor := th.PaletteTextColor(colorIndex)

		// Period box
		boxPath := scene.NewPath(scene.Style{
			Fill:   bgColor,
			Stroke: th.LineColor,
		})
		boxPath.Rect(periodX, timelineY-periodBoxHeight/2, periodBoxWidth, periodBoxHeight, 4)
		sc.Add(boxPath)

		// Period text
		wrapped := scene.WrapText(period.Name, diagram.Font(th, 0.875), periodBoxWidth-8)
		sc.Add(scene.NewText(
			periodX+periodBoxWidth/2, timelineY,
			wrapped,
			diagram.Font(th, 0.875),
			textColor,
			scene.AnchorMiddle,
			scene.VAlignMiddle,
		))

		// Dot on the line
		dotPath := scene.NewPath(scene.Style{
			Fill:   th.LineColor,
			Stroke: color.RGBA{A: 0},
		})
		dotPath.Circle(periodX+periodBoxWidth/2, baselineY, dotRadius)
		sc.Add(dotPath)

		// Draw events below
		eventY := baselineY + 40
		for _, event := range period.Events {
			wrapped := scene.WrapText(event, diagram.Font(th, 0.8), eventBoxWidth-8)
			eventPath := scene.NewPath(scene.Style{
				Fill:   theme.Lighten(bgColor, 0.3),
				Stroke: th.LineColor,
			})
			eventPath.Rect(periodX+periodBoxWidth/2-eventBoxWidth/2, eventY, eventBoxWidth, eventBoxHeight, 3)
			sc.Add(eventPath)

			sc.Add(scene.NewText(
				periodX+periodBoxWidth/2, eventY+eventBoxHeight/2,
				wrapped,
				diagram.Font(th, 0.8),
				textColor,
				scene.AnchorMiddle,
				scene.VAlignMiddle,
			))

			dashedPath := scene.NewPath(scene.Style{
				Stroke:      th.LineColor,
				StrokeWidth: 1,
				Dash:        []float64{2, 2},
			})
			dashedPath.MoveTo(periodX+periodBoxWidth/2, baselineY+dotRadius)
			dashedPath.LineTo(periodX+periodBoxWidth/2, eventY)
			sc.Add(dashedPath)

			eventY += eventBoxHeight + 10
		}

		periodX += periodBoxWidth + periodGap
	}

	// Draw section headers if sections exist
	if len(tl.Sections) > 0 {
		sectionY := timelineY - periodBoxHeight/2 - sectionHeight - 10
		for i, sec := range tl.Sections {
			// Find span of this section
			minX := padding + timelineWidth
			maxX := padding

			for _, p := range sec.Periods {
				for j, period := range tl.AllPeriods {
					if period == p {
						x := padding + float64(j)*(periodBoxWidth+periodGap)
						minX = math.Min(minX, x)
						maxX = math.Max(maxX, x+periodBoxWidth)
					}
				}
			}

			if minX < maxX {
				sectionColor := th.PaletteColor(i)
				sectionPath := scene.NewPath(scene.Style{
					Fill:   sectionColor,
					Stroke: th.LineColor,
				})
				sectionPath.Rect(minX, sectionY, maxX-minX, sectionHeight, 4)
				sc.Add(sectionPath)

				wrapped := scene.WrapText(sec.Name, diagram.Font(th, 1), maxX-minX-10)
				sc.Add(scene.NewText(
					(minX+maxX)/2, sectionY+sectionHeight/2,
					wrapped,
					diagram.Font(th, 1),
					th.PaletteTextColor(i),
					scene.AnchorMiddle,
					scene.VAlignMiddle,
				))
			}
		}
	}

	return diagram.Finish(sc, th, tl.Title), nil
}

// Parse parses timeline source code.
func Parse(src string, cfg *diagram.Config) (*Timeline, error) {
	tl := &Timeline{
		AllPeriods: make([]*Period, 0),
		Sections:   make([]*Section, 0),
	}

	if cfg.Raw != nil {
		sec := cfg.Section("timeline")
		if sec != nil {
			if disableMulti, ok := sec["disableMulticolor"].(bool); ok {
				tl.DisableMulticolor = disableMulti
			}
		}
	}

	var currentSection *Section
	lines := diagram.Lines(src)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "title ") {
			tl.Title = diagram.CleanLabel(trimmed[6:])
		} else if strings.HasPrefix(trimmed, "section ") {
			sectionName := diagram.CleanLabel(trimmed[8:])
			currentSection = &Section{
				Name:    sectionName,
				Periods: make([]*Period, 0),
			}
			tl.Sections = append(tl.Sections, currentSection)
		} else {
			// Period and events line
			// Format: "Period : Event1 : Event2" or
			//         "       : Event" (continuation)
			colonParts := strings.Split(trimmed, ":")

			if len(colonParts) == 1 || (len(colonParts) > 1 && strings.TrimSpace(colonParts[0]) == "") {
				// Continuation line - add to last period
				if len(tl.AllPeriods) > 0 {
					lastPeriod := tl.AllPeriods[len(tl.AllPeriods)-1]
					for i := 1; i < len(colonParts); i++ {
						event := diagram.CleanLabel(colonParts[i])
						if event != "" {
							lastPeriod.Events = append(lastPeriod.Events, event)
						}
					}
				}
			} else {
				// New period
				periodName := diagram.CleanLabel(colonParts[0])
				period := &Period{
					Name:   periodName,
					Events: make([]string, 0),
				}

				// Add events
				for i := 1; i < len(colonParts); i++ {
					event := diagram.CleanLabel(colonParts[i])
					if event != "" {
						period.Events = append(period.Events, event)
					}
				}

				tl.AllPeriods = append(tl.AllPeriods, period)

				if currentSection != nil {
					currentSection.Periods = append(currentSection.Periods, period)
				}
			}
		}
	}

	return tl, nil
}
