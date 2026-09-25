// Package gantt implements Gantt chart diagram rendering.
package gantt

import (
	"fmt"
	"image/color"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/scene"
)

func init() {
	diagram.Register(diagram.Type{
		Name:   "gantt",
		Detect: diagram.Keyword("gantt"),
		Render: Render,
	})
}

// Task represents a single task/milestone in the Gantt chart.
type Task struct {
	Name        string
	ID          string
	StartDate   time.Time
	EndDate     time.Time
	IsMilestone bool
	IsVert      bool
	IsDone      bool
	IsActive    bool
	IsCrit      bool
	DependsOn   []string // task IDs to wait for
	UntilID     string
	Section     string
}

// Section represents a section in the Gantt chart.
type Section struct {
	Name  string
	Tasks []*Task
}

// Gantt holds parsed Gantt chart data.
type Gantt struct {
	Title          string
	DateFormat     string
	AxisFormat     string
	TickInterval   string
	Excludes       []string // days/dates to exclude
	WeekendStart   string   // "friday" or "saturday" (default)
	Section        []Section
	AllTasks       map[string]*Task
	Now            time.Time
	TodayMarker    string // "off" or style
	CompactMode    bool
	InclusiveDates bool
	TopAxis        bool
}

// Render renders a Gantt diagram.
func Render(src string, cfg *diagram.Config) (*scene.Scene, error) {
	gantt, err := Parse(src, cfg)
	if err != nil {
		return nil, err
	}

	th := cfg.Theme
	sc := diagram.NewScene(th)

	// Measure text to calculate layout sizes
	taskFont := diagram.Font(th, 1)
	axisFont := diagram.Font(th, 0.875)

	// Calculate overall date bounds
	minDate := gantt.Now
	maxDate := gantt.Now
	hasDate := false
	for _, t := range gantt.AllTasks {
		if !hasDate {
			minDate = t.StartDate
			maxDate = t.EndDate
			hasDate = true
		} else {
			if t.StartDate.Before(minDate) {
				minDate = t.StartDate
			}
			if t.EndDate.After(maxDate) {
				maxDate = t.EndDate
			}
		}
	}

	// If we have no tasks, use now as bounds
	if !hasDate {
		minDate = gantt.Now
		maxDate = gantt.Now.AddDate(0, 0, 30)
	}

	// Add margins to date range
	dateDiff := maxDate.Sub(minDate).Hours() / 24
	if dateDiff < 1 {
		dateDiff = 30
	}
	minDate = minDate.AddDate(0, 0, -1)
	maxDate = maxDate.AddDate(0, 0, 1)
	dateDiff = maxDate.Sub(minDate).Hours() / 24

	// Calculate pixel width: aim for ~1200px width
	targetWidth := 1200.0
	pixelsPerDay := targetWidth / dateDiff

	// Adjust pixel scale to be reasonable (5-30 px/day)
	if pixelsPerDay < 5 {
		pixelsPerDay = 5
	} else if pixelsPerDay > 30 {
		pixelsPerDay = 30
	}

	chartWidth := dateDiff * pixelsPerDay

	// Layout
	const (
		leftMargin   = 200.0
		topMargin    = 80.0
		taskHeight   = 24.0
		taskGap      = 4.0
		sectionGap   = 30.0
		axisHeight   = 40.0
		bottomMargin = 40.0
	)

	y := topMargin
	if gantt.TopAxis {
		y = topMargin + axisHeight
	}

	maxSectionNameWidth := 0.0
	for _, sec := range gantt.Section {
		w, _ := scene.MeasureBlock(sec.Name, diagram.Font(th, 1), 0)
		if w > maxSectionNameWidth {
			maxSectionNameWidth = w
		}
	}
	for _, t := range gantt.AllTasks {
		if t.Section == "" {
			w, _ := scene.MeasureBlock(t.Name, taskFont, 0)
			if w > maxSectionNameWidth {
				maxSectionNameWidth = w
			}
		}
	}

	actualLeftMargin := math.Min(maxSectionNameWidth+40, 250)

	// Draw background sections
	sectionY := y
	for i, sec := range gantt.Section {
		sectionHeight := float64(len(sec.Tasks))*(taskHeight+taskGap) + sectionGap
		bgColor := th.SectionBkg[i%len(th.SectionBkg)]
		path := scene.NewPath(scene.Style{
			Fill:   bgColor,
			Stroke: color.RGBA{A: 0},
		})
		path.Rect(actualLeftMargin, sectionY, chartWidth, sectionHeight, 0)
		sc.Add(path)

		// Draw section title on the left
		if sec.Name != "" {
			sc.Add(scene.NewText(
				actualLeftMargin-20, sectionY+sectionHeight/2,
				sec.Name,
				diagram.Font(th, 1),
				th.TextColor,
				scene.AnchorEnd,
				scene.VAlignMiddle,
			))
		}

		sectionY += sectionHeight
	}

	// Draw tasks
	sectionY = y
	for _, sec := range gantt.Section {
		taskY := sectionY
		for _, task := range sec.Tasks {
			if task.IsVert {
				// Draw vertical line
				xPos := actualLeftMargin + task.StartDate.Sub(minDate).Hours()/24*pixelsPerDay
				path := scene.NewPath(scene.Style{
					Stroke:      th.GridColor,
					StrokeWidth: 2,
				})
				path.MoveTo(xPos, topMargin)
				path.LineTo(xPos, sectionY+float64(len(sec.Tasks))*(taskHeight+taskGap)+sectionGap)
				sc.Add(path)
			} else if task.IsMilestone {
				// Draw diamond
				x := actualLeftMargin + task.StartDate.Sub(minDate).Hours()/24*pixelsPerDay
				diamondSize := 8.0
				path := scene.NewPath(scene.Style{
					Fill:   th.MilestoneFill,
					Stroke: th.TaskBorder,
				})
				path.MoveTo(x, taskY+taskHeight/2-diamondSize)
				path.LineTo(x+diamondSize, taskY+taskHeight/2)
				path.LineTo(x, taskY+taskHeight/2+diamondSize)
				path.LineTo(x-diamondSize, taskY+taskHeight/2)
				path.Close()
				sc.Add(path)
			} else {
				// Draw task bar
				startX := actualLeftMargin + task.StartDate.Sub(minDate).Hours()/24*pixelsPerDay
				endX := actualLeftMargin + task.EndDate.Sub(minDate).Hours()/24*pixelsPerDay
				barWidth := math.Max(endX-startX, 4)

				fillColor := th.TaskBkg
				borderColor := th.TaskBorder
				if task.IsDone {
					fillColor = th.DoneTask
					borderColor = th.DoneBorder
				} else if task.IsActive {
					fillColor = th.ActiveTask
					borderColor = th.ActiveBorder
				} else if task.IsCrit {
					fillColor = th.CritTask
					borderColor = th.CritBorder
				}

				path := scene.NewPath(scene.Style{
					Fill:   fillColor,
					Stroke: borderColor,
				})
				path.Rect(startX, taskY, barWidth, taskHeight, 3)
				sc.Add(path)

				// Draw task label
				labelX := startX + barWidth/2
				labelW, _ := scene.MeasureBlock(task.Name, taskFont, 0)
				if labelW+4 <= barWidth {
					// Label fits inside bar
					sc.Add(scene.NewText(
						labelX, taskY+taskHeight/2,
						task.Name,
						taskFont,
						th.TaskText,
						scene.AnchorMiddle,
						scene.VAlignMiddle,
					))
				} else {
					// Label to the right
					sc.Add(scene.NewText(
						endX+4, taskY+taskHeight/2,
						task.Name,
						taskFont,
						th.TextColor,
						scene.AnchorStart,
						scene.VAlignMiddle,
					))
				}
			}

			taskY += taskHeight + taskGap
		}
		sectionY += float64(len(sec.Tasks))*(taskHeight+taskGap) + sectionGap
	}

	// Draw time axis at bottom
	axisY := sectionY + 10
	path := scene.NewPath(scene.Style{
		Stroke:      th.GridColor,
		StrokeWidth: 1,
	})
	path.MoveTo(actualLeftMargin, axisY)
	path.LineTo(actualLeftMargin+chartWidth, axisY)
	sc.Add(path)

	// Draw tick marks
	tickInterval := calculateTickInterval(dateDiff)
	currentDate := minDate
	for currentDate.Before(maxDate) || currentDate.Equal(maxDate) {
		xPos := actualLeftMargin + currentDate.Sub(minDate).Hours()/24*pixelsPerDay
		tickPath := scene.NewPath(scene.Style{
			Stroke:      th.GridColor,
			StrokeWidth: 1,
		})
		tickPath.MoveTo(xPos, axisY)
		tickPath.LineTo(xPos, axisY+4)
		sc.Add(tickPath)

		// Draw label
		label := formatDate(currentDate, gantt.AxisFormat)
		sc.Add(scene.NewText(
			xPos, axisY+12,
			label,
			axisFont,
			th.TextColor,
			scene.AnchorMiddle,
			scene.VAlignTop,
		))

		currentDate = addDateInterval(currentDate, tickInterval)
	}

	// Draw today marker if enabled and within range
	if gantt.TodayMarker != "off" && (gantt.Now.Equal(minDate) || gantt.Now.After(minDate)) && gantt.Now.Before(maxDate) {
		todayX := actualLeftMargin + gantt.Now.Sub(minDate).Hours()/24*pixelsPerDay
		todayPath := scene.NewPath(scene.Style{
			Stroke:      th.TodayLine,
			StrokeWidth: 2,
			Dash:        []float64{4, 4},
		})
		todayPath.MoveTo(todayX, topMargin)
		todayPath.LineTo(todayX, sectionY)
		sc.Add(todayPath)
	}

	// Draw excluded days shading
	for d := minDate; d.Before(maxDate); d = d.AddDate(0, 0, 1) {
		if isExcludedDay(d, gantt.Excludes, gantt.WeekendStart) {
			xPos := actualLeftMargin + d.Sub(minDate).Hours()/24*pixelsPerDay
			excludePath := scene.NewPath(scene.Style{
				Fill:   th.ExcludeBkg,
				Stroke: color.RGBA{A: 0},
			})
			excludePath.Rect(xPos, topMargin, pixelsPerDay, sectionY-topMargin, 0)
			sc.Add(excludePath)
		}
	}

	return diagram.Finish(sc, th, gantt.Title), nil
}

func calculateTickInterval(dayCount float64) string {
	if dayCount <= 7 {
		return "1day"
	} else if dayCount <= 30 {
		return "1day"
	} else if dayCount <= 90 {
		return "1week"
	} else if dayCount <= 365 {
		return "2week"
	}
	return "1month"
}

func addDateInterval(d time.Time, interval string) time.Time {
	switch interval {
	case "1day":
		return d.AddDate(0, 0, 1)
	case "1week":
		return d.AddDate(0, 0, 7)
	case "2week":
		return d.AddDate(0, 0, 14)
	case "1month":
		return d.AddDate(0, 1, 0)
	default:
		return d.AddDate(0, 0, 1)
	}
}

func formatDate(t time.Time, format string) string {
	// Convert d3 strftime to Go time format
	if format == "" {
		format = "%Y-%m-%d"
	}
	// Simple replacements
	result := format
	result = strings.ReplaceAll(result, "%Y", t.Format("2006"))
	result = strings.ReplaceAll(result, "%y", t.Format("06"))
	result = strings.ReplaceAll(result, "%m", t.Format("01"))
	result = strings.ReplaceAll(result, "%d", t.Format("02"))
	result = strings.ReplaceAll(result, "%b", t.Format("Jan"))
	result = strings.ReplaceAll(result, "%B", t.Format("January"))
	result = strings.ReplaceAll(result, "%a", t.Format("Mon"))
	result = strings.ReplaceAll(result, "%A", t.Format("Monday"))
	result = strings.ReplaceAll(result, "%H", t.Format("15"))
	result = strings.ReplaceAll(result, "%M", t.Format("04"))
	result = strings.ReplaceAll(result, "%S", t.Format("05"))
	result = strings.ReplaceAll(result, "%e", fmt.Sprintf("%2d", t.Day()))
	return result
}

func isExcludedDay(d time.Time, excludes []string, weekendStart string) bool {
	dateStr := d.Format("2006-01-02")
	for _, ex := range excludes {
		if ex == dateStr {
			return true
		}
		dayName := strings.ToLower(d.Weekday().String())
		if ex == dayName {
			return true
		}
		if ex == "weekends" {
			if weekendStart == "friday" {
				if d.Weekday() == time.Friday || d.Weekday() == time.Saturday {
					return true
				}
			} else {
				if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
					return true
				}
			}
		}
	}
	return false
}

// Parse parses Gantt chart source code.
func Parse(src string, cfg *diagram.Config) (*Gantt, error) {
	gantt := &Gantt{
		DateFormat:   "YYYY-MM-DD",
		AxisFormat:   "%Y-%m-%d",
		TickInterval: "",
		WeekendStart: "saturday",
		AllTasks:     make(map[string]*Task),
		Now:          time.Now(),
		TodayMarker:  "",
	}

	if cfg.Raw != nil {
		// Check config
		if s, ok := cfg.Raw["displayMode"].(string); ok && s == "compact" {
			gantt.CompactMode = true
		}
		if s, ok := cfg.Raw["inclusiveEndDates"].(bool); ok {
			gantt.InclusiveDates = s
		}
		if s, ok := cfg.Raw["topAxis"].(bool); ok {
			gantt.TopAxis = s
		}

		// Check gantt subsection
		sec := cfg.Section("gantt")
		if sec != nil {
			if s, ok := sec["dateFormat"].(string); ok {
				gantt.DateFormat = s
			}
			if s, ok := sec["axisFormat"].(string); ok {
				gantt.AxisFormat = s
			}
			if s, ok := sec["tickInterval"].(string); ok {
				gantt.TickInterval = s
			}
			if s, ok := sec["todayMarker"].(string); ok {
				gantt.TodayMarker = s
			}
		}
	}

	var currentSection string
	lines := diagram.Lines(src)

	for lineNum, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Commands
		if strings.HasPrefix(trimmed, "title ") {
			gantt.Title = diagram.CleanLabel(trimmed[6:])
		} else if strings.HasPrefix(trimmed, "dateFormat ") {
			gantt.DateFormat = strings.TrimSpace(trimmed[11:])
		} else if strings.HasPrefix(trimmed, "axisFormat ") {
			gantt.AxisFormat = strings.TrimSpace(trimmed[11:])
		} else if strings.HasPrefix(trimmed, "tickInterval ") {
			gantt.TickInterval = strings.TrimSpace(trimmed[13:])
		} else if strings.HasPrefix(trimmed, "excludes ") {
			ex := strings.TrimSpace(trimmed[9:])
			gantt.Excludes = append(gantt.Excludes, strings.Fields(ex)...)
		} else if strings.HasPrefix(trimmed, "weekend ") {
			ws := strings.TrimSpace(trimmed[8:])
			if ws == "friday" || ws == "saturday" {
				gantt.WeekendStart = ws
			}
		} else if strings.HasPrefix(trimmed, "section ") {
			currentSection = diagram.CleanLabel(trimmed[8:])
			gantt.Section = append(gantt.Section, Section{Name: currentSection})
		} else if strings.HasPrefix(trimmed, "todayMarker ") {
			gantt.TodayMarker = strings.TrimSpace(trimmed[12:])
		} else if strings.HasPrefix(trimmed, "click ") {
			// Ignore click lines
		} else if strings.HasPrefix(trimmed, "vert ") || strings.HasPrefix(trimmed, "includes ") {
			// Ignore vert and includes for now
		} else {
			// Task line
			task, err := parseTask(trimmed, gantt.DateFormat, lineNum+1)
			if err != nil {
				// Silently ignore unparseable task lines
				continue
			}
			task.Section = currentSection
			gantt.AllTasks[task.ID] = task

			if len(gantt.Section) == 0 {
				gantt.Section = append(gantt.Section, Section{Name: ""})
			}
			gantt.Section[len(gantt.Section)-1].Tasks = append(gantt.Section[len(gantt.Section)-1].Tasks, task)
		}
	}

	// Resolve task dependencies
	for _, task := range gantt.AllTasks {
		for _, depID := range task.DependsOn {
			if depTask, ok := gantt.AllTasks[depID]; ok {
				task.StartDate = depTask.EndDate
			}
		}

		// Handle until
		if task.UntilID != "" {
			if untilTask, ok := gantt.AllTasks[task.UntilID]; ok {
				task.EndDate = untilTask.StartDate
			}
		}
	}

	return gantt, nil
}

func parseTask(line string, dateFormat string, lineNum int) (*Task, error) {
	// Task line format: "Name : [tags,] [id,] startDate, endDate|duration"
	before, after, ok := strings.Cut(line, ":")
	if !ok {
		return nil, fmt.Errorf("line %d: no colon in task", lineNum)
	}

	name := diagram.CleanLabel(before)
	metaStr := strings.TrimSpace(after)

	parts := splitByComma(metaStr)
	if len(parts) == 0 {
		return nil, fmt.Errorf("line %d: no metadata", lineNum)
	}

	task := &Task{
		Name:      name,
		ID:        fmt.Sprintf("task_%d", lineNum),
		StartDate: time.Now(),
		EndDate:   time.Now().AddDate(0, 0, 1),
	}

	// Parse tags and metadata
	idx := 0
	for idx < len(parts) {
		part := strings.TrimSpace(parts[idx])

		// Check for tags
		if part == "done" {
			task.IsDone = true
			idx++
		} else if part == "active" {
			task.IsActive = true
			idx++
		} else if part == "crit" {
			task.IsCrit = true
			idx++
		} else if part == "milestone" {
			task.IsMilestone = true
			idx++
		} else if part == "vert" {
			task.IsVert = true
			idx++
		} else {
			break
		}
	}

	// Parse ID (optional)
	if idx < len(parts) && !isDateOrDuration(strings.TrimSpace(parts[idx])) && !strings.HasPrefix(strings.TrimSpace(parts[idx]), "after") && !strings.HasPrefix(strings.TrimSpace(parts[idx]), "until") {
		task.ID = strings.TrimSpace(parts[idx])
		idx++
	}

	// Now parse timing info
	if idx >= len(parts) {
		return nil, fmt.Errorf("line %d: no timing info", lineNum)
	}

	// Try to parse remaining parts as timing
	for i := idx; i < len(parts); i++ {
		part := strings.TrimSpace(parts[i])

		if strings.HasPrefix(part, "after ") {
			deps := strings.TrimSpace(part[6:])
			task.DependsOn = strings.Fields(deps)
			// Start date will be set from dependencies
		} else if strings.HasPrefix(part, "until ") {
			task.UntilID = strings.TrimSpace(part[6:])
		} else if isDateOrDuration(part) {
			// This is a date or duration
			if i == idx {
				// First timing part - could be start date or just duration
				if t, err := parseDate(part, dateFormat); err == nil {
					task.StartDate = t
				} else if d, err := parseDuration(part); err == nil {
					task.EndDate = task.StartDate.Add(d)
				}
			} else if i == idx+1 && len(task.DependsOn) == 0 {
				// Second timing part - could be end date or duration
				if t, err := parseDate(part, dateFormat); err == nil {
					task.EndDate = t
				} else if d, err := parseDuration(part); err == nil {
					task.EndDate = task.StartDate.Add(d)
				}
			}
		}
	}

	return task, nil
}

func splitByComma(s string) []string {
	var parts []string
	var current strings.Builder
	for _, r := range s {
		if r == ',' {
			parts = append(parts, current.String())
			current.Reset()
		} else {
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

func isDateOrDuration(s string) bool {
	// Check if it looks like a date or duration
	if _, err := parseDate(s, "YYYY-MM-DD"); err == nil {
		return true
	}
	if _, err := parseDuration(s); err == nil {
		return true
	}
	return false
}

func parseDate(s string, format string) (time.Time, error) {
	s = strings.TrimSpace(s)
	// Convert format string from Mermaid dayjs tokens to Go tokens
	goFormat := convertDateFormat(format)
	return time.Parse(goFormat, s)
}

func convertDateFormat(dayjsFormat string) string {
	// Mermaid dayjs tokens to Go time format
	result := dayjsFormat
	result = strings.ReplaceAll(result, "YYYY", "2006")
	result = strings.ReplaceAll(result, "YY", "06")
	result = strings.ReplaceAll(result, "MMMM", "January")
	result = strings.ReplaceAll(result, "MMM", "Jan")
	result = strings.ReplaceAll(result, "MM", "01")
	result = strings.ReplaceAll(result, "M", "1") // Must come after MM
	result = strings.ReplaceAll(result, "DDDD", "Monday")
	result = strings.ReplaceAll(result, "DDD", "Mon")
	result = strings.ReplaceAll(result, "DD", "02")
	result = strings.ReplaceAll(result, "D", "2") // Must come after DD
	result = strings.ReplaceAll(result, "HH", "15")
	result = strings.ReplaceAll(result, "H", "15") // Must come after HH
	result = strings.ReplaceAll(result, "mm", "04")
	result = strings.ReplaceAll(result, "m", "4") // Must come after mm
	result = strings.ReplaceAll(result, "ss", "05")
	result = strings.ReplaceAll(result, "s", "5") // Must come after ss
	result = strings.ReplaceAll(result, "SSS", "000")
	result = strings.ReplaceAll(result, "SS", "00")
	result = strings.ReplaceAll(result, "S", "0")             // Must come after SS
	result = strings.ReplaceAll(result, "X", "1136214245")    // Unix timestamp seconds
	result = strings.ReplaceAll(result, "x", "1136214245000") // Unix timestamp millis
	return result
}

func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	re := regexp.MustCompile(`^([\d.]+)(ms|s|m|h|d|w|M|y)$`)
	matches := re.FindStringSubmatch(s)
	if matches == nil {
		return 0, fmt.Errorf("invalid duration: %s", s)
	}

	num, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, err
	}

	unit := matches[2]
	switch unit {
	case "ms":
		return time.Duration(num*1e6) * time.Nanosecond, nil
	case "s":
		return time.Duration(num) * time.Second, nil
	case "m":
		return time.Duration(num) * time.Minute, nil
	case "h":
		return time.Duration(num) * time.Hour, nil
	case "d":
		return time.Duration(num*24) * time.Hour, nil
	case "w":
		return time.Duration(num*7*24) * time.Hour, nil
	case "M":
		// Approximate month as 30 days
		return time.Duration(num*30*24) * time.Hour, nil
	case "y":
		// Approximate year as 365 days
		return time.Duration(num*365*24) * time.Hour, nil
	default:
		return 0, fmt.Errorf("unknown duration unit: %s", unit)
	}
}
