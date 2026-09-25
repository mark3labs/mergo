// Package gitgraph implements Mermaid git graphs.
package gitgraph

import (
	"fmt"
	"hash/fnv"
	"image/color"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

func init() {
	diagram.Register(diagram.Type{
		Name:   "gitGraph",
		Detect: diagram.Keyword("gitGraph"),
		Render: Render,
	})
}

// CommitType is NORMAL, REVERSE or HIGHLIGHT.
type CommitType int

const (
	Normal CommitType = iota
	Reverse
	Highlight
	Merge
	CherryPick
)

// Commit is a commit.
type Commit struct {
	ID      string
	Seq     int
	Branch  string
	Parents []string
	Tags    []string
	Type    CommitType
	Msg     string
	custom  bool
}

// Branch is a branch.
type Branch struct {
	Name  string
	Order float64
	Head  string
	index int
}

// Graph is a parsed git graph.
type Graph struct {
	Dir      string // LR, TB, BT
	Commits  []*Commit
	byID     map[string]*Commit
	Branches []*Branch
	byName   map[string]*Branch
}

var attrRe = regexp.MustCompile(`(\w+)\s*:\s*("(?:[^"\\]|\\.)*"|\S+)`)

func attrs(s string) map[string][]string {
	out := map[string][]string{}
	for _, m := range attrRe.FindAllStringSubmatch(s, -1) {
		v := m[2]
		if len(v) >= 2 && v[0] == '"' {
			v = strings.ReplaceAll(v[1:len(v)-1], `\"`, `"`)
		}
		out[m[1]] = append(out[m[1]], v)
	}
	return out
}

// Parse parses a git graph. mainName is the name of the main branch.
func Parse(src, mainName string, mainOrder float64) (*Graph, error) {
	g := &Graph{Dir: "LR", byID: map[string]*Commit{}, byName: map[string]*Branch{}}
	main := &Branch{Name: mainName, Order: mainOrder}
	g.Branches = append(g.Branches, main)
	g.byName[mainName] = main
	cur := main
	header := false
	for i, raw := range strings.Split(src, "\n") {
		ln := i + 1
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		if !header {
			if !strings.HasPrefix(line, "gitGraph") {
				return nil, fmt.Errorf("line %d: expected 'gitGraph'", ln)
			}
			rest := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "gitGraph"), ":"))
			switch strings.ToUpper(rest) {
			case "TB", "TD":
				g.Dir = "TB"
			case "BT":
				g.Dir = "BT"
			}
			header = true
			continue
		}
		kw, rest := cutWord(line)
		a := attrs(rest)
		switch kw {
		case "commit":
			c := g.newCommit(cur, a, Normal)
			if t, ok := a["type"]; ok {
				switch strings.ToUpper(t[0]) {
				case "REVERSE":
					c.Type = Reverse
				case "HIGHLIGHT":
					c.Type = Highlight
				}
			}
		case "branch":
			name := strings.Fields(rest)
			if len(name) == 0 {
				return nil, fmt.Errorf("line %d: branch needs a name", ln)
			}
			bn := strings.Trim(name[0], `"`)
			if _, dup := g.byName[bn]; dup {
				return nil, fmt.Errorf("line %d: branch %q already exists", ln, bn)
			}
			b := &Branch{Name: bn, Order: float64(len(g.Branches)), Head: cur.Head, index: len(g.Branches)}
			if o, ok := a["order"]; ok {
				if v, err := strconv.ParseFloat(o[0], 64); err == nil {
					b.Order = v
				}
			}
			g.Branches = append(g.Branches, b)
			g.byName[bn] = b
			cur = b
		case "checkout", "switch":
			bn := strings.Trim(strings.TrimSpace(rest), `"`)
			b, ok := g.byName[bn]
			if !ok {
				return nil, fmt.Errorf("line %d: unknown branch %q", ln, bn)
			}
			cur = b
		case "merge":
			f := strings.Fields(rest)
			if len(f) == 0 {
				return nil, fmt.Errorf("line %d: merge needs a branch", ln)
			}
			bn := strings.Trim(f[0], `"`)
			other, ok := g.byName[bn]
			if !ok {
				return nil, fmt.Errorf("line %d: cannot merge unknown branch %q", ln, bn)
			}
			if other == cur {
				return nil, fmt.Errorf("line %d: cannot merge branch %q into itself", ln, bn)
			}
			if other.Head == "" {
				return nil, fmt.Errorf("line %d: branch %q has no commits to merge", ln, bn)
			}
			c := g.newCommit(cur, a, Merge)
			c.Parents = append(c.Parents, other.Head)
			if !c.custom {
				c.Msg = "merge " + bn
			}
			if t, ok := a["type"]; ok {
				switch strings.ToUpper(t[0]) {
				case "REVERSE":
					c.Type = Reverse
				case "HIGHLIGHT":
					c.Type = Highlight
				}
			}
		case "cherry-pick":
			ids := a["id"]
			if len(ids) == 0 {
				return nil, fmt.Errorf("line %d: cherry-pick needs id: \"...\"", ln)
			}
			src, ok := g.byID[ids[0]]
			if !ok {
				return nil, fmt.Errorf("line %d: unknown commit %q", ln, ids[0])
			}
			if src.Branch == cur.Name {
				return nil, fmt.Errorf("line %d: cannot cherry-pick a commit from the same branch", ln)
			}
			delete(a, "id")
			c := g.newCommit(cur, a, CherryPick)
			c.Parents = append(c.Parents, src.ID)
			c.Msg = "cherry-pick: " + src.ID
			if len(a["tag"]) == 0 {
				c.Tags = []string{"cherry-pick: " + src.ID}
			}
		default:
			return nil, fmt.Errorf("line %d: unknown command %q", ln, kw)
		}
	}
	return g, nil
}

func (g *Graph) newCommit(b *Branch, a map[string][]string, t CommitType) *Commit {
	seq := len(g.Commits)
	c := &Commit{Seq: seq, Branch: b.Name, Type: t}
	if id, ok := a["id"]; ok {
		c.ID = id[0]
		c.custom = true
	} else {
		h := fnv.New32a()
		_, _ = fmt.Fprintf(h, "%d-%s", seq, b.Name)
		c.ID = fmt.Sprintf("%d-%07x", seq, h.Sum32()&0xfffffff)
	}
	for _, t := range a["tag"] {
		c.Tags = append(c.Tags, diagram.CleanInline(t))
	}
	if m, ok := a["msg"]; ok {
		c.Msg = m[0]
	}
	if b.Head != "" {
		c.Parents = []string{b.Head}
	}
	b.Head = c.ID
	g.Commits = append(g.Commits, c)
	g.byID[c.ID] = c
	return c
}

func cutWord(s string) (string, string) {
	s = strings.TrimSpace(s)
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimSpace(s[i+1:])
}

// ---------------------------------------------------------------------------

// Render renders a git graph.
func Render(src string, cfg *diagram.Config) (*scene.Scene, error) {
	mainName := cfg.String("gitGraph", "mainBranchName", "main")
	g, err := Parse(src, mainName, cfg.Float("gitGraph", "mainBranchOrder", 0))
	if err != nil {
		return nil, err
	}
	th := cfg.Theme
	sc := diagram.NewScene(th)
	r := &renderer{
		g: g, th: th,
		showBranches: cfg.Bool("gitGraph", "showBranches", true),
		showLabels:   cfg.Bool("gitGraph", "showCommitLabel", true),
		rotate:       cfg.Bool("gitGraph", "rotateCommitLabel", true),
		parallel:     cfg.Bool("gitGraph", "parallelCommits", false),
	}
	r.draw(sc)
	return diagram.Finish(sc, th, cfg.Title), nil
}

type renderer struct {
	g                                          *Graph
	th                                         *theme.Theme
	showBranches, showLabels, rotate, parallel bool

	lane    map[string]int     // branch -> lane index
	pos     map[string]float64 // commit -> position along the flow
	laneGap float64
}

func (r *renderer) color(branch string) color.RGBA {
	return r.th.ChartColor(r.g.byName[branch].index)
}

// point returns the canvas position of a commit.
func (r *renderer) point(c *Commit) scene.Point {
	along := r.pos[c.ID]
	across := float64(r.lane[c.Branch]) * r.laneGap
	switch r.g.Dir {
	case "TB":
		return scene.Pt(across, along)
	case "BT":
		return scene.Pt(across, -along)
	}
	return scene.Pt(along, across)
}

func (r *renderer) draw(sc *scene.Scene) {
	g, th := r.g, r.th
	// lanes sorted by order
	brs := append([]*Branch(nil), g.Branches...)
	sort.SliceStable(brs, func(i, j int) bool { return brs[i].Order < brs[j].Order })
	r.lane = map[string]int{}
	for i, b := range brs {
		r.lane[b.Name] = i
	}
	horiz := g.Dir == "LR"
	labelFont := scene.Font{Size: th.FontSize * 0.75}
	branchFont := scene.Font{Size: th.FontSize * 0.85, Bold: true}
	r.laneGap = 64
	if !horiz {
		maxBW := 0.0
		for _, b := range brs {
			maxBW = math.Max(maxBW, scene.MeasureText(b.Name, branchFont)+24)
		}
		r.laneGap = math.Max(90, maxBW+14)
	}

	// positions along the flow
	r.pos = map[string]float64{}
	step := 56.0
	if horiz && r.showLabels && !r.rotate {
		// horizontal labels need room
		for _, c := range g.Commits {
			step = math.Max(step, scene.MeasureText(r.label(c), labelFont)+16)
		}
	}
	start := 0.0
	if r.showBranches && horiz {
		for _, b := range brs {
			start = math.Max(start, scene.MeasureText(b.Name, branchFont)+44)
		}
	} else if r.showBranches {
		start = 50
	}
	for _, c := range g.Commits {
		if r.parallel {
			// place a commit right after its parents
			p := start
			for _, pid := range c.Parents {
				p = math.Max(p, r.pos[pid]+step)
			}
			if len(c.Parents) == 0 {
				p = start + step/2
			}
			r.pos[c.ID] = p
		} else {
			r.pos[c.ID] = start + step/2 + float64(c.Seq)*step
		}
	}
	end := start
	for _, v := range r.pos {
		end = math.Max(end, v)
	}
	end += step / 2

	// branch lines
	for _, b := range brs {
		col := r.color(b.Name)
		first, last := end, 0.0
		any := false
		for _, c := range g.Commits {
			if c.Branch == b.Name {
				first = math.Min(first, r.pos[c.ID])
				last = math.Max(last, r.pos[c.ID])
				any = true
			}
		}
		if !any {
			continue
		}
		lineStart := start
		if b.index > 0 {
			lineStart = first
		}
		lineEnd := end
		a := r.point(&Commit{Branch: b.Name})
		p1, p2 := a, a
		switch g.Dir {
		case "TB":
			p1.Y, p2.Y = lineStart, lineEnd
		case "BT":
			p1.Y, p2.Y = -lineStart, -lineEnd
		default:
			p1.X, p2.X = lineStart, lineEnd
		}
		sc.Add(scene.Line(p1.X, p1.Y, p2.X, p2.Y, scene.Style{Stroke: theme.WithAlpha(col, 150), StrokeWidth: 2, Dash: []float64{6, 5}}))
		if r.showBranches {
			r.drawBranchLabel(sc, b, branchFont)
		}
	}

	// connections
	for _, c := range g.Commits {
		for pi, pid := range c.Parents {
			p := g.byID[pid]
			if p == nil {
				continue
			}
			col := r.color(c.Branch)
			if pi > 0 {
				col = r.color(p.Branch)
			}
			if c.Type == CherryPick && pi > 0 {
				col = theme.WithAlpha(col, 170)
			}
			r.connect(sc, p, c, col, c.Type == CherryPick && pi > 0)
		}
	}

	// commits and labels
	for _, c := range g.Commits {
		r.drawCommit(sc, c, labelFont)
	}
}

func (r *renderer) label(c *Commit) string { return c.ID }

// connect draws a curved connection from parent to child.
func (r *renderer) connect(sc *scene.Scene, p, c *Commit, col color.RGBA, dashed bool) {
	a, b := r.point(p), r.point(c)
	st := scene.Style{Stroke: col, StrokeWidth: 3, RoundCaps: true}
	if dashed {
		st.Dash = []float64{5, 5}
		st.StrokeWidth = 2
	}
	if p.Branch == c.Branch || (a.X == b.X || a.Y == b.Y) {
		sc.Add(scene.Line(a.X, a.Y, b.X, b.Y, st))
		return
	}
	// branching out (child on a later lane): bend near the parent;
	// merging back: bend near the child.
	out := r.lane[c.Branch] > r.lane[p.Branch]
	rad := 14.0
	var pts []scene.Point
	switch r.g.Dir {
	case "LR":
		if out {
			pts = []scene.Point{a, {X: a.X, Y: b.Y}, b}
		} else {
			pts = []scene.Point{a, {X: b.X, Y: a.Y}, b}
		}
	default:
		if out {
			pts = []scene.Point{a, {X: b.X, Y: a.Y}, b}
		} else {
			pts = []scene.Point{a, {X: a.X, Y: b.Y}, b}
		}
	}
	sc.Add(scene.NewPath(st).RoundedPolyline(pts, rad))
}

func (r *renderer) drawBranchLabel(sc *scene.Scene, b *Branch, f scene.Font) {
	col := r.color(b.Name)
	w := scene.MeasureText(b.Name, f) + 18
	h := f.Size*1.25 + 8
	a := r.point(&Commit{Branch: b.Name})
	var x, y float64
	switch r.g.Dir {
	case "TB":
		x, y = a.X-w/2, -h-6
	case "BT":
		x, y = a.X-w/2, 6
	default:
		x, y = 0, a.Y-h/2
	}
	sc.Add(scene.RectPath(x, y, w, h, h/2, scene.Style{Fill: col, Shadow: true}))
	sc.Add(scene.NewText(x+w/2, y+h/2, b.Name, f, theme.ContrastText(col), scene.AnchorMiddle, scene.VAlignMiddle))
}

func (r *renderer) drawCommit(sc *scene.Scene, c *Commit, f scene.Font) {
	th := r.th
	col := r.color(c.Branch)
	p := r.point(c)
	rad := 10.0
	switch c.Type {
	case Highlight:
		sc.Add(scene.RectPath(p.X-rad, p.Y-rad, 2*rad, 2*rad, 2, scene.Style{Fill: col, Stroke: theme.Darken(col, 0.3), StrokeWidth: 2}))
		sc.Add(scene.RectPath(p.X-rad/2, p.Y-rad/2, rad, rad, 1, scene.Style{Fill: th.Background}))
	case Reverse:
		sc.Add(scene.Circle(p.X, p.Y, rad, scene.Style{Fill: col, Stroke: theme.Darken(col, 0.3), StrokeWidth: 1.5}))
		s := rad * 0.5
		st := scene.Style{Stroke: theme.ContrastText(col), StrokeWidth: 2.5, RoundCaps: true}
		sc.Add(scene.Line(p.X-s, p.Y-s, p.X+s, p.Y+s, st), scene.Line(p.X-s, p.Y+s, p.X+s, p.Y-s, st))
	case Merge:
		sc.Add(scene.Circle(p.X, p.Y, rad+1, scene.Style{Fill: col, Stroke: theme.Darken(col, 0.3), StrokeWidth: 1.5}))
		sc.Add(scene.Circle(p.X, p.Y, rad*0.45, scene.Style{Fill: th.Background}))
	case CherryPick:
		sc.Add(scene.Circle(p.X, p.Y, rad, scene.Style{Fill: col, Stroke: theme.Darken(col, 0.3), StrokeWidth: 1.5}))
		sc.Add(scene.Circle(p.X-3, p.Y-2, 2.6, scene.Style{Fill: th.Background}))
		sc.Add(scene.Circle(p.X+3, p.Y-2, 2.6, scene.Style{Fill: th.Background}))
	default:
		sc.Add(scene.Circle(p.X, p.Y, rad, scene.Style{Fill: col, Stroke: theme.Darken(col, 0.3), StrokeWidth: 1.5, Shadow: true}))
	}
	if r.showLabels {
		txt := r.label(c)
		w, h := scene.MeasureBlock(txt, f, 0)
		bg := theme.WithAlpha(th.EdgeLabelBg, 220)
		horiz := r.g.Dir == "LR"
		if horiz && r.rotate {
			// rotated -45° label below-left of the commit
			ax, ay := p.X+4, p.Y+rad+6
			sc.Add(&scene.Text{X: ax, Y: ay, S: txt, Font: f, Color: th.TextColor, Anchor: scene.AnchorEnd, VAlign: scene.VAlignMiddle, Rotate: -45})
		} else if horiz {
			sc.Add(scene.RectPath(p.X-w/2-4, p.Y+rad+6, w+8, h+4, 3, scene.Style{Fill: bg}))
			sc.Add(scene.NewText(p.X, p.Y+rad+8+h/2, txt, f, th.TextColor, scene.AnchorMiddle, scene.VAlignMiddle))
		} else {
			sc.Add(scene.RectPath(p.X+rad+8, p.Y-h/2-2, w+8, h+4, 3, scene.Style{Fill: bg}))
			sc.Add(scene.NewText(p.X+rad+12, p.Y, txt, f, th.TextColor, scene.AnchorStart, scene.VAlignMiddle))
		}
	}
	// tags
	off := rad + 8
	for _, tag := range c.Tags {
		w, h := scene.MeasureBlock(tag, f, 0)
		w += 14
		h += 6
		var x, y float64
		if r.g.Dir == "LR" {
			x, y = p.X-w/2, p.Y-off-h
			off += h + 4
		} else {
			x, y = p.X-rad-10-w, p.Y-h/2
		}
		fill := th.TertiaryColor
		sc.Add(scene.NewPath(scene.Style{Fill: fill, Stroke: th.TertiaryBorder, StrokeWidth: 1}).Polygon(
			scene.Pt(x, y+h/2), scene.Pt(x+6, y), scene.Pt(x+w, y), scene.Pt(x+w, y+h), scene.Pt(x+6, y+h)))
		sc.Add(scene.Circle(x+6, y+h/2, 1.8, scene.Style{Fill: th.TertiaryBorder}))
		sc.Add(scene.NewText(x+w/2+3, y+h/2, tag, f, th.TertiaryTextColor, scene.AnchorMiddle, scene.VAlignMiddle))
	}
}
