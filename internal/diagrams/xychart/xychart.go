package xychart

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
		Name:   "xychart",
		Detect: func(h string) bool { return strings.HasPrefix(h, "xychart") },
		Render: Render,
	})
	diagram.Register(diagram.Type{
		Name:   "xychart-beta",
		Detect: func(h string) bool { return strings.HasPrefix(h, "xychart-beta") },
		Render: Render,
	})
}

// Render renders an XY chart diagram.
func Render(src string, cfg *diagram.Config) (*scene.Scene, error) {
	th := cfg.Theme
	sc := diagram.NewScene(th)

	lines := diagram.Lines(src)
	if len(lines) == 0 {
		return diagram.Finish(sc, th, cfg.Title), nil
	}

	// Parse orientation from first line
	horizontal := false
	if len(lines) > 0 && strings.Contains(lines[0], "horizontal") {
		horizontal = true
		lines = lines[1:]
	}

	// Parse chart data
	chart := &XYChart{
		DataSeries: []*Series{},
		Horizontal: horizontal,
	}

	// Parse commands
	for lineIdx, line := range lines {
		if err := parseChartLine(chart, line, lineIdx+1); err != nil {
			return nil, err
		}
	}

	// Read config
	chartCfg := cfg.Section("xychart")
	width := cfg.Float("xychart", "width", 700)
	height := cfg.Float("xychart", "height", 500)
	if chartCfg != nil {
		if w, ok := chartCfg["width"].(float64); ok {
			width = w
		}
		if h, ok := chartCfg["height"].(float64); ok {
			height = h
		}
	}

	chart.Width = width
	chart.Height = height

	// Layout and render
	renderXYChart(sc, chart, th)

	return diagram.Finish(sc, th, cfg.Title), nil
}

type XYChart struct {
	Title      string
	XAxis      *Axis
	YAxis      *Axis
	DataSeries []*Series
	Width      float64
	Height     float64
	Horizontal bool
}

type Axis struct {
	Title      string
	Categories []string
	Min        float64
	Max        float64
	HasRange   bool
	IsNumeric  bool
}

type Series struct {
	Name   string
	Type   string // "line" or "bar"
	Values []float64
	Color  color.RGBA
}

func parseChartLine(chart *XYChart, line string, lineNum int) error {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}

	parts := strings.Fields(line)
	if len(parts) == 0 {
		return nil
	}

	cmd := parts[0]

	switch cmd {
	case "title":
		// title "text" or title text
		chart.Title = diagram.CleanLabel(strings.Join(parts[1:], " "))

	case "x-axis":
		return parseAxis(chart, line, true)

	case "y-axis":
		return parseAxis(chart, line, false)

	case "line":
		return parseSeries(chart, line, "line")

	case "bar":
		return parseSeries(chart, line, "bar")

	default:
		// Ignore unknown commands
		return nil
	}

	return nil
}

func parseAxis(chart *XYChart, line string, isX bool) error {
	// x-axis "title" [cat1, cat2] or x-axis title min --> max
	// y-axis "title" min --> max
	line = strings.TrimSpace(line)

	// Remove "x-axis" or "y-axis"
	if isX {
		line = strings.TrimPrefix(line, "x-axis")
	} else {
		line = strings.TrimPrefix(line, "y-axis")
	}
	line = strings.TrimSpace(line)

	axis := &Axis{}

	// Check for categories [cat1, cat2, ...]
	if strings.Contains(line, "[") && strings.Contains(line, "]") {
		// Extract title (if before bracket)
		bracketIdx := strings.Index(line, "[")
		if bracketIdx > 0 {
			axis.Title = diagram.CleanLabel(line[:bracketIdx])
		}

		// Extract categories
		endBracket := strings.LastIndex(line, "]")
		catStr := line[bracketIdx+1 : endBracket]

		// Parse comma-separated list
		for cat := range strings.SplitSeq(catStr, ",") {
			cat = strings.TrimSpace(cat)
			cat = diagram.CleanLabel(cat)
			if cat != "" {
				axis.Categories = append(axis.Categories, cat)
			}
		}
		axis.IsNumeric = false
	} else if strings.Contains(line, "-->") {
		// Numeric range: title min --> max
		parts := strings.Split(line, "-->")
		if len(parts) == 2 {
			rightPart := strings.TrimSpace(parts[1])
			maxVal, err := strconv.ParseFloat(rightPart, 64)
			if err != nil {
				return fmt.Errorf("invalid max value: %v", err)
			}
			axis.Max = maxVal

			leftPart := strings.TrimSpace(parts[0])
			parts2 := strings.Fields(leftPart)
			axis.Title = diagram.CleanLabel(strings.Join(parts2[:len(parts2)-1], " "))

			minVal, err := strconv.ParseFloat(parts2[len(parts2)-1], 64)
			if err != nil {
				return fmt.Errorf("invalid min value: %v", err)
			}
			axis.Min = minVal
			axis.HasRange = true
			axis.IsNumeric = true
		}
	} else {
		// Just title
		axis.Title = diagram.CleanLabel(line)
		axis.IsNumeric = true
	}

	if isX {
		chart.XAxis = axis
	} else {
		chart.YAxis = axis
	}

	return nil
}

func parseSeries(chart *XYChart, line string, seriesType string) error {
	// line "name" [1, 2, 3] or line [1, 2, 3]
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, seriesType)
	line = strings.TrimSpace(line)

	series := &Series{
		Type: seriesType,
	}

	// Check for quoted name
	if strings.HasPrefix(line, "\"") {
		endQuote := strings.Index(line[1:], "\"")
		if endQuote >= 0 {
			series.Name = line[1 : endQuote+1]
			line = strings.TrimSpace(line[endQuote+2:])
		}
	}

	// Parse values [1, 2, 3]
	if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
		valStr := line[1 : len(line)-1]
		for val := range strings.SplitSeq(valStr, ",") {
			val = strings.TrimSpace(val)
			if v, err := strconv.ParseFloat(val, 64); err == nil {
				series.Values = append(series.Values, v)
			}
		}
	}

	// Assign color from palette
	_ = len(chart.DataSeries) // colorIdx

	chart.DataSeries = append(chart.DataSeries, series)
	return nil
}

func renderXYChart(sc *scene.Scene, chart *XYChart, th *theme.Theme) {
	// Chart dimensions
	pad := 40.0
	titleH := 30.0
	axisLabelH := 25.0
	axisLabelW := 40.0

	plotX := pad + axisLabelW
	plotY := pad + titleH
	plotW := chart.Width - plotX - pad
	plotH := chart.Height - plotY - axisLabelH - pad

	// Draw title
	if chart.Title != "" {
		titleF := scene.Font{Size: th.FontSize * 1.2, Bold: true}
		titleText := scene.NewText(pad+plotW/2, pad+titleH/2, chart.Title, titleF, th.TitleColor, scene.AnchorMiddle, scene.VAlignMiddle)
		sc.Add(titleText)
	}

	// Determine data ranges
	minY, maxY := findYRange(chart)
	if chart.YAxis != nil && chart.YAxis.HasRange {
		minY = chart.YAxis.Min
		maxY = chart.YAxis.Max
	}

	// Add some padding to y range
	if maxY == minY {
		maxY = minY + 1
	}
	yRange := maxY - minY
	minY -= yRange * 0.05
	maxY += yRange * 0.05

	numXPoints := getMaxDataPoints(chart)
	if chart.XAxis != nil && len(chart.XAxis.Categories) > 0 {
		numXPoints = len(chart.XAxis.Categories)
	}

	// Draw axes
	// X axis line
	xAxisPath := scene.NewPath(scene.Style{
		Stroke:      th.LineColor,
		StrokeWidth: 2,
	})
	xAxisPath.MoveTo(plotX, plotY+plotH)
	xAxisPath.LineTo(plotX+plotW, plotY+plotH)
	sc.Add(xAxisPath)

	// Y axis line
	yAxisPath := scene.NewPath(scene.Style{
		Stroke:      th.LineColor,
		StrokeWidth: 2,
	})
	yAxisPath.MoveTo(plotX, plotY)
	yAxisPath.LineTo(plotX, plotY+plotH)
	sc.Add(yAxisPath)

	// Draw axis labels and ticks
	xStep := plotW / float64(numXPoints-1)
	if numXPoints <= 1 {
		xStep = plotW
	}

	labelF := scene.Font{Size: th.FontSize * 0.85}

	// X-axis labels
	for i := 0; i < numXPoints; i++ {
		x := plotX + float64(i)*xStep
		// Tick
		tick := scene.NewPath(scene.Style{
			Stroke:      th.LineColor,
			StrokeWidth: 1,
		})
		tick.MoveTo(x, plotY+plotH)
		tick.LineTo(x, plotY+plotH+5)
		sc.Add(tick)

		// Label
		var label string
		if chart.XAxis != nil && len(chart.XAxis.Categories) > i {
			label = chart.XAxis.Categories[i]
		} else {
			label = fmt.Sprintf("%d", i)
		}
		labelText := scene.NewText(x, plotY+plotH+15, label, labelF, th.TextColor, scene.AnchorMiddle, scene.VAlignTop)
		sc.Add(labelText)
	}

	// Y-axis labels and gridlines
	numYTicks := 5
	yStep := plotH / float64(numYTicks)
	for i := 0; i <= numYTicks; i++ {
		y := plotY + plotH - float64(i)*yStep
		v := minY + float64(i)*(maxY-minY)/float64(numYTicks)

		// Tick
		tick := scene.NewPath(scene.Style{
			Stroke:      th.LineColor,
			StrokeWidth: 1,
		})
		tick.MoveTo(plotX-5, y)
		tick.LineTo(plotX, y)
		sc.Add(tick)

		// Gridline
		gridline := scene.NewPath(scene.Style{
			Stroke:      color.RGBA{200, 200, 200, 50},
			StrokeWidth: 1,
			Dash:        []float64{3, 2},
		})
		gridline.MoveTo(plotX, y)
		gridline.LineTo(plotX+plotW, y)
		sc.Add(gridline)

		// Label
		label := fmt.Sprintf("%.1f", v)
		labelText := scene.NewText(plotX-10, y, label, labelF, th.TextColor, scene.AnchorEnd, scene.VAlignMiddle)
		sc.Add(labelText)
	}

	// Draw axis titles
	if chart.XAxis != nil && chart.XAxis.Title != "" {
		titleF := scene.Font{Size: th.FontSize * 0.95, Bold: true}
		xTitle := scene.NewText(plotX+plotW/2, chart.Height-10, chart.XAxis.Title, titleF, th.TextColor, scene.AnchorMiddle, scene.VAlignBottom)
		sc.Add(xTitle)
	}

	if chart.YAxis != nil && chart.YAxis.Title != "" {
		titleF := scene.Font{Size: th.FontSize * 0.95, Bold: true}
		yTitle := scene.NewText(15, plotY+plotH/2, chart.YAxis.Title, titleF, th.TextColor, scene.AnchorMiddle, scene.VAlignMiddle)
		yTitle.Rotate = -90
		sc.Add(yTitle)
	}

	// Draw data series
	for seriesIdx, series := range chart.DataSeries {
		c := th.PaletteColor(seriesIdx)
		textC := th.PaletteTextColor(seriesIdx)

		switch series.Type {
		case "bar":
			drawBarSeries(sc, series, chart, plotX, plotY, plotW, plotH, minY, maxY, c, textC, seriesIdx, len(chart.DataSeries))
		case "line":
			drawLineSeries(sc, series, chart, plotX, plotY, plotW, plotH, minY, maxY, c, textC)
		}
	}
}

func drawBarSeries(sc *scene.Scene, series *Series, chart *XYChart, plotX, plotY, plotW, plotH, minY, maxY float64, c, textC color.RGBA, seriesIdx, numSeries int) {
	numXPoints := len(series.Values)
	if chart.XAxis != nil && len(chart.XAxis.Categories) > 0 {
		numXPoints = len(chart.XAxis.Categories)
	}

	xStep := plotW / float64(numXPoints)
	if numXPoints <= 1 {
		xStep = plotW
	}

	barWidth := xStep * 0.7 / float64(numSeries)
	barOffset := (xStep - barWidth*float64(numSeries)) / 2

	yRange := maxY - minY
	if yRange == 0 {
		yRange = 1
	}

	for i, val := range series.Values {
		if i >= numXPoints {
			break
		}

		x := plotX + float64(i+1)*xStep - xStep/2 + barOffset + float64(seriesIdx)*barWidth
		barH := (val - minY) / yRange * plotH

		y := plotY + plotH - barH

		// Draw bar
		barPath := scene.NewPath(scene.Style{
			Fill:        c,
			Stroke:      c,
			StrokeWidth: 1,
			Shadow:      true,
		})
		barPath.Rect(x, y, barWidth, barH, 2)
		sc.Add(barPath)
	}
}

func drawLineSeries(sc *scene.Scene, series *Series, chart *XYChart, plotX, plotY, plotW, plotH, minY, maxY float64, c, textC color.RGBA) {
	numXPoints := len(series.Values)
	if chart.XAxis != nil && len(chart.XAxis.Categories) > 0 {
		numXPoints = len(chart.XAxis.Categories)
	}

	xStep := plotW / float64(numXPoints-1)
	if numXPoints <= 1 {
		xStep = plotW
	}

	yRange := maxY - minY
	if yRange == 0 {
		yRange = 1
	}

	// Draw line
	linePath := scene.NewPath(scene.Style{
		Stroke:      c,
		StrokeWidth: 2,
		RoundCaps:   true,
	})

	for i, val := range series.Values {
		if i >= numXPoints {
			break
		}

		x := plotX + float64(i)*xStep
		barH := (val - minY) / yRange * plotH
		y := plotY + plotH - barH

		if i == 0 {
			linePath.MoveTo(x, y)
		} else {
			linePath.LineTo(x, y)
		}

		// Draw point marker
		marker := scene.NewPath(scene.Style{
			Fill:        c,
			Stroke:      c,
			StrokeWidth: 1,
		})
		marker.Circle(x, y, 3)
		sc.Add(marker)
	}

	sc.Add(linePath)
}

func findYRange(chart *XYChart) (float64, float64) {
	minY, maxY := math.Inf(1), math.Inf(-1)

	for _, series := range chart.DataSeries {
		for _, val := range series.Values {
			if val < minY {
				minY = val
			}
			if val > maxY {
				maxY = val
			}
		}
	}

	if math.IsInf(minY, 1) {
		minY = 0
	}
	if math.IsInf(maxY, -1) {
		maxY = 10
	}

	return minY, maxY
}

func getMaxDataPoints(chart *XYChart) int {
	max := 1
	for _, series := range chart.DataSeries {
		if len(series.Values) > max {
			max = len(series.Values)
		}
	}
	return max
}
