package layout

import (
	"testing"
)

func overlaps(a, b *Node) bool {
	ra, rb := a.Rect(), b.Rect()
	return ra.X < rb.X+rb.W && rb.X < ra.X+ra.W && ra.Y < rb.Y+rb.H && rb.Y < ra.Y+ra.H
}

func TestLayeredBasics(t *testing.T) {
	mk := func(id string) *Node { return &Node{ID: id, W: 80, H: 40} }
	a, b, c, d := mk("a"), mk("b"), mk("c"), mk("d")
	g := &Graph{Dir: TB, Nodes: []*Node{a, b, c, d}, Edges: []*Edge{
		{From: a, To: b}, {From: a, To: c}, {From: b, To: d}, {From: c, To: d, LabelW: 50, LabelH: 20}, {From: d, To: a},
	}}
	Layout(g)
	if !(a.Y < b.Y && b.Y < d.Y && a.Y < c.Y) {
		t.Errorf("ranks wrong: a=%v b=%v c=%v d=%v", a.Y, b.Y, c.Y, d.Y)
	}
	nodes := []*Node{a, b, c, d}
	for i := range nodes {
		for j := i + 1; j < len(nodes); j++ {
			if overlaps(nodes[i], nodes[j]) {
				t.Errorf("%s overlaps %s", nodes[i].ID, nodes[j].ID)
			}
		}
	}
	for _, e := range g.Edges {
		if len(e.Points) < 2 {
			t.Errorf("edge %s->%s has no route", e.From.ID, e.To.ID)
		}
	}
	// the labeled edge has its label between c and d
	lab := g.Edges[3].Label
	if !(lab.Y > c.Y && lab.Y < d.Y) {
		t.Errorf("label at %v not between %v and %v", lab, c.Y, d.Y)
	}
	if g.Width <= 0 || g.Height <= 0 {
		t.Error("graph size")
	}
}

func TestDirectionsAndClusters(t *testing.T) {
	for _, dir := range []Direction{TB, BT, LR, RL} {
		x := &Node{ID: "x", W: 60, H: 30}
		y := &Node{ID: "y", W: 60, H: 30}
		z := &Node{ID: "z", W: 60, H: 30}
		cl := &Node{ID: "cl", Children: []*Node{y, z}, Pad: 10, PadTop: 20}
		g := &Graph{Dir: dir, Nodes: []*Node{x, cl}, Edges: []*Edge{{From: x, To: y}, {From: y, To: z}, {From: x, To: x}}}
		Layout(g)
		switch dir {
		case TB:
			if !(x.Y < y.Y) {
				t.Errorf("TB: x should be above y")
			}
		case BT:
			if !(x.Y > y.Y) {
				t.Errorf("BT: x should be below y")
			}
		case LR:
			if !(x.X < y.X) {
				t.Errorf("LR: x should be left of y")
			}
		case RL:
			if !(x.X > y.X) {
				t.Errorf("RL: x should be right of y")
			}
		}
		cr := cl.Rect()
		for _, n := range cl.Children {
			r := n.Rect()
			if r.X < cr.X || r.Y < cr.Y || r.X+r.W > cr.X+cr.W || r.Y+r.H > cr.Y+cr.H {
				t.Errorf("%v: child %s outside cluster", dir, n.ID)
			}
		}
		if overlaps(x, cl) {
			t.Errorf("%v: node overlaps cluster", dir)
		}
		if len(g.Edges[2].Points) < 3 {
			t.Errorf("%v: self loop not routed", dir)
		}
	}
}

func TestIsotonic(t *testing.T) {
	got := isotonic([]float64{3, 1, 2, 5}, []float64{1, 1, 1, 1})
	want := []float64{2, 2, 2, 5}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("isotonic = %v, want %v", got, want)
		}
	}
}
