package quadrant

import "testing"

func TestParse(t *testing.T) {
	c, err := Parse(`quadrantChart
    title Reach
    x-axis Low Reach --> High Reach
    y-axis Low Engagement
    quadrant-1 Expand
    quadrant-3 Re-evaluate
    Campaign A: [0.3, 0.6]
    Campaign B:::hot: [0.45, 0.23] radius: 12, color: #ff3300
    classDef hot color: #00ff00, radius: 8, stroke-width: 2px`)
	if err != nil {
		t.Fatal(err)
	}
	if c.XLow != "Low Reach" || c.XHigh != "High Reach" || c.YLow != "Low Engagement" || c.YHigh != "" {
		t.Errorf("axes %+v", c)
	}
	if c.Quadrants[0] != "Expand" || c.Quadrants[2] != "Re-evaluate" {
		t.Errorf("quadrants %v", c.Quadrants)
	}
	b := c.Points[1]
	if b.Label != "Campaign B" || b.Class != "hot" || b.Radius != 12 || b.Color.R != 0xff || b.StrokeWidth != 2 {
		t.Errorf("point %+v", b)
	}
	if _, err := Parse("quadrantChart\nX: [1.5, 0.2]"); err == nil {
		t.Error("expected range error")
	}
}
