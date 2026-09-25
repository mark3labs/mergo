package gitgraph

import (
	"fmt"
	"hash/fnv"
	"image/color"
	"regexp"
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

// Render renders a gitGraph diagram.
func Render(src string, cfg *diagram.Config) (*scene.Scene, error) {
	th := cfg.Theme
	sc := diagram.NewScene(th)

	lines := diagram.Lines(src)
	if len(lines) == 0 {
		return diagram.Finish(sc, th, cfg.Title), nil
	}

	// Parse orientation from first line if present (LR, TB, BT)
	orientation := "LR" // default
	if len(lines) > 0 && (strings.HasPrefix(lines[0], "LR") || strings.HasPrefix(lines[0], "TB") || strings.HasPrefix(lines[0], "BT")) {
		parts := strings.Fields(lines[0])
		if len(parts) > 0 {
			orientation = parts[0]
			lines = lines[1:] // Remove orientation line
		}
	}

	// Parse the git commands
	graph := &GitGraph{
		Commits:      make(map[string]*Commit),
		Branches:     make(map[string]*Branch),
		CommitOrder:  []*Commit{},
		MainBranch:   "main",
		Orientation:  orientation,
		RotateLabels: true,
	}

	// Initialize main branch
	graph.Branches[graph.MainBranch] = &Branch{Name: graph.MainBranch, Order: 0}
	graph.CurrentBranch = graph.MainBranch

	// Parse commands
	for lineIdx, line := range lines {
		if err := parseCommand(graph, line, lineIdx+1); err != nil {
			return nil, err
		}
	}

	// Layout and render
	layoutGitGraph(graph)
	renderGitGraph(sc, graph, th)

	return diagram.Finish(sc, th, cfg.Title), nil
}

type GitGraph struct {
	Commits       map[string]*Commit
	Branches      map[string]*Branch
	CommitOrder   []*Commit // Order of commits as added
	MainBranch    string
	CurrentBranch string
	Orientation   string // LR, TB, BT
	RotateLabels  bool
}

type Commit struct {
	ID         string
	Type       string // NORMAL, REVERSE, HIGHLIGHT
	Tag        string
	Branch     string
	X, Y       float64
	Order      int    // Order in commit sequence
	CommitHash string // Hash for determ ID generation
}

type Branch struct {
	Name      string
	Order     int
	Commits   []*Commit
	X         float64 // x position for the branch lane
	Color     color.RGBA
	TextColor color.RGBA
}

func parseCommand(g *GitGraph, line string, lineNum int) error {
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
	case "commit":
		return parseCommit(g, line)

	case "branch":
		if len(parts) < 2 {
			return fmt.Errorf("line %d: branch requires a name", lineNum)
		}
		branchName := parts[1]
		if strings.HasPrefix(branchName, "\"") && strings.HasSuffix(branchName, "\"") {
			branchName = branchName[1 : len(branchName)-1]
		}
		if _, exists := g.Branches[branchName]; exists {
			return fmt.Errorf("line %d: branch %s already exists", lineNum, branchName)
		}
		order := len(g.Branches)
		g.Branches[branchName] = &Branch{Name: branchName, Order: order}
		g.CurrentBranch = branchName
		return nil

	case "checkout", "switch":
		if len(parts) < 2 {
			return fmt.Errorf("line %d: checkout requires a branch name", lineNum)
		}
		branchName := parts[1]
		if _, exists := g.Branches[branchName]; !exists {
			return fmt.Errorf("line %d: branch %s not found", lineNum, branchName)
		}
		g.CurrentBranch = branchName
		return nil

	case "merge":
		if len(parts) < 2 {
			return fmt.Errorf("line %d: merge requires a branch name", lineNum)
		}
		sourceBranch := parts[1]
		if _, exists := g.Branches[sourceBranch]; !exists {
			return fmt.Errorf("line %d: branch %s not found", lineNum, sourceBranch)
		}
		if sourceBranch == g.CurrentBranch {
			return fmt.Errorf("line %d: cannot merge a branch with itself", lineNum)
		}

		// Extract merge attributes
		id := generateCommitID(g)
		tag := ""
		mergeType := "NORMAL"
		re := regexp.MustCompile(`(\w+):\s*"([^"]*)"`)
		for _, match := range re.FindAllStringSubmatch(line, -1) {
			key, val := match[1], match[2]
			if key == "id" {
				id = val
			} else if key == "tag" {
				tag = val
			} else if key == "type" {
				mergeType = val
			}
		}

		commit := &Commit{
			ID:     id,
			Type:   mergeType,
			Tag:    tag,
			Branch: g.CurrentBranch,
			Order:  len(g.CommitOrder),
		}
		g.Commits[id] = commit
		g.CommitOrder = append(g.CommitOrder, commit)
		g.Branches[g.CurrentBranch].Commits = append(g.Branches[g.CurrentBranch].Commits, commit)
		return nil

	case "cherry-pick":
		if len(parts) < 3 {
			return fmt.Errorf("line %d: cherry-pick requires id", lineNum)
		}
		// Parse: cherry-pick id: "x" [parent: "y"]
		id := ""
		parent := ""
		re := regexp.MustCompile(`(\w+):\s*"([^"]*)"`)
		for _, match := range re.FindAllStringSubmatch(line, -1) {
			key, val := match[1], match[2]
			if key == "id" {
				id = val
			} else if key == "parent" {
				parent = val
			}
		}
		_ = parent // unused in current implementation
		if id == "" {
			return fmt.Errorf("line %d: cherry-pick requires id attribute", lineNum)
		}
		if _, exists := g.Commits[id]; !exists {
			return fmt.Errorf("line %d: commit %s not found for cherry-pick", lineNum, id)
		}
		if len(g.Branches[g.CurrentBranch].Commits) == 0 {
			return fmt.Errorf("line %d: current branch has no commits for cherry-pick", lineNum)
		}

		// Create a new commit representing the cherry-pick
		newID := "cherry-" + id
		commit := &Commit{
			ID:     newID,
			Type:   "NORMAL",
			Tag:    id, // Show the original commit ID as tag
			Branch: g.CurrentBranch,
			Order:  len(g.CommitOrder),
		}
		g.Commits[newID] = commit
		g.CommitOrder = append(g.CommitOrder, commit)
		g.Branches[g.CurrentBranch].Commits = append(g.Branches[g.CurrentBranch].Commits, commit)
		return nil

	default:
		// Ignore unknown commands
		return nil
	}
}

func parseCommit(g *GitGraph, line string) error {
	// Parse: commit [id: "x"] [type: TYPE] [tag: "y"]
	id := generateCommitID(g)
	commitType := "NORMAL"
	tag := ""

	// Extract attributes
	re := regexp.MustCompile(`(\w+):\s*"([^"]*)"|\b(NORMAL|REVERSE|HIGHLIGHT)\b`)
	for _, match := range re.FindAllStringSubmatch(line, -1) {
		if match[1] != "" {
			key, val := match[1], match[2]
			if key == "id" {
				id = val
			} else if key == "tag" {
				tag = val
			}
		} else if match[3] != "" {
			commitType = match[3]
		}
	}

	commit := &Commit{
		ID:     id,
		Type:   commitType,
		Tag:    tag,
		Branch: g.CurrentBranch,
		Order:  len(g.CommitOrder),
	}

	g.Commits[id] = commit
	g.CommitOrder = append(g.CommitOrder, commit)
	g.Branches[g.CurrentBranch].Commits = append(g.Branches[g.CurrentBranch].Commits, commit)

	return nil
}

func generateCommitID(g *GitGraph) string {
	// Generate a deterministic ID based on the order
	h := fnv.New32a()
	h.Write([]byte(strconv.Itoa(len(g.CommitOrder))))
	hash := fmt.Sprintf("%08x", h.Sum32())
	return fmt.Sprintf("%d-%s", len(g.CommitOrder), hash[:4])
}

func layoutGitGraph(g *GitGraph) {
	// Assign x positions to branches
	branchGap := 80.0

	branchOrder := make([]string, 0, len(g.Branches))
	for name := range g.Branches {
		branchOrder = append(branchOrder, name)
	}

	// Sort by order
	for i := 0; i < len(branchOrder)-1; i++ {
		for j := i + 1; j < len(branchOrder); j++ {
			if g.Branches[branchOrder[i]].Order > g.Branches[branchOrder[j]].Order {
				branchOrder[i], branchOrder[j] = branchOrder[j], branchOrder[i]
			}
		}
	}

	for i, name := range branchOrder {
		branch := g.Branches[name]
		branch.X = float64(i) * branchGap
	}
	// Colors will be set during rendering with theme

	// Assign y positions to commits
	commitHeight := 40.0
	for commitIdx, commit := range g.CommitOrder {
		y := float64(commitIdx) * commitHeight
		commit.Y = y
		commit.X = g.Branches[commit.Branch].X
	}
}

func (b *Branch) getColor(index int, th *theme.Theme) color.RGBA {
	// Return a color from theme palette based on branch index
	return th.PaletteColor(index)
}

func (b *Branch) getTextColor(index int, th *theme.Theme) color.RGBA {
	return th.PaletteTextColor(index)
}

func renderGitGraph(sc *scene.Scene, g *GitGraph, th *theme.Theme) {
	// Draw branch lanes
	maxY := float64(0)
	if len(g.CommitOrder) > 0 {
		maxY = g.CommitOrder[len(g.CommitOrder)-1].Y + 40
	}

	// Set branch colors during rendering
	branchOrder := make([]string, 0, len(g.Branches))
	for name := range g.Branches {
		branchOrder = append(branchOrder, name)
	}
	for i := 0; i < len(branchOrder)-1; i++ {
		for j := i + 1; j < len(branchOrder); j++ {
			if g.Branches[branchOrder[i]].Order > g.Branches[branchOrder[j]].Order {
				branchOrder[i], branchOrder[j] = branchOrder[j], branchOrder[i]
			}
		}
	}
	for i, name := range branchOrder {
		branch := g.Branches[name]
		branch.Color = branch.getColor(i, th)
		branch.TextColor = branch.getTextColor(i, th)
	}

	for _, branch := range g.Branches {
		// Draw branch line
		line := scene.NewPath(scene.Style{
			Stroke:      branch.Color,
			StrokeWidth: 2,
		})
		line.MoveTo(branch.X, 0)
		line.LineTo(branch.X, maxY)
		sc.Add(line)

		// Draw branch label
		labelF := scene.Font{Size: th.FontSize * 0.9, Bold: true}
		label := scene.NewText(branch.X, -15, branch.Name, labelF, branch.TextColor, scene.AnchorMiddle, scene.VAlignMiddle)
		sc.Add(label)
	}

	// Draw commits
	for _, commit := range g.CommitOrder {
		branch := g.Branches[commit.Branch]
		renderCommit(sc, commit, branch, g, th)
	}
}

func renderCommit(sc *scene.Scene, c *Commit, b *Branch, g *GitGraph, th *theme.Theme) {
	x, y := c.X, c.Y
	radius := 6.0

	// Draw commit marker based on type
	switch c.Type {
	case "HIGHLIGHT":
		// Square
		s := scene.NewPath(scene.Style{
			Fill:        b.Color,
			Stroke:      b.Color,
			StrokeWidth: 2,
		})
		s.Rect(x-8, y-8, 16, 16, 0)
		sc.Add(s)

	case "REVERSE":
		// Circle with cross
		circle := scene.NewPath(scene.Style{
			Fill:        th.Background,
			Stroke:      b.Color,
			StrokeWidth: 2,
		})
		circle.Circle(x, y, radius)
		sc.Add(circle)

		// Draw cross
		cross := scene.NewPath(scene.Style{
			Stroke:      b.Color,
			StrokeWidth: 1.5,
		})
		cross.MoveTo(x-4, y-4)
		cross.LineTo(x+4, y+4)
		cross.MoveTo(x+4, y-4)
		cross.LineTo(x-4, y+4)
		sc.Add(cross)

	default: // NORMAL
		// Filled circle
		circle := scene.NewPath(scene.Style{
			Fill:        b.Color,
			Stroke:      b.Color,
			StrokeWidth: 1,
		})
		circle.Circle(x, y, radius)
		sc.Add(circle)
	}

	// Draw commit ID label
	if c.ID != "" {
		labelF := scene.Font{Size: th.FontSize * 0.8}
		labelY := y + 20
		labelRotate := 0.0
		if g.RotateLabels {
			labelRotate = -45
		}
		label := scene.NewText(x, labelY, c.ID, labelF, th.TextColor, scene.AnchorMiddle, scene.VAlignMiddle)
		label.Rotate = labelRotate
		sc.Add(label)
	}

	// Draw merge connector if this is a merge commit
	if c.Type == "NORMAL" && len(g.Branches[c.Branch].Commits) > 1 {
		// Check if we came from another branch
		idx := -1
		for i, commit := range g.Branches[c.Branch].Commits {
			if commit.ID == c.ID {
				idx = i
				break
			}
		}
		if idx > 0 {
			prevCommit := g.Branches[c.Branch].Commits[idx-1]
			if prevCommit.X != c.X {
				// Draw a curve from previous position to current branch line
				connector := scene.NewPath(scene.Style{
					Stroke:      b.Color,
					StrokeWidth: 1,
					Dash:        []float64{2, 2},
				})
				midX := (prevCommit.X + c.X) / 2
				_ = (prevCommit.Y + c.Y) / 2 // midY would be used for arc adjustment
				connector.MoveTo(prevCommit.X, prevCommit.Y)
				connector.CubicTo(midX, prevCommit.Y, midX, c.Y, c.X, c.Y)
				sc.Add(connector)
			}
		}
	}

	// Draw tag label if present
	if c.Tag != "" {
		tagF := scene.Font{Size: th.FontSize * 0.7, Bold: true}
		// Draw tag shape above commit
		tagW := float64(len(c.Tag))*6 + 4
		tagH := 12.0
		tagX := x
		tagY := y - 25

		tagBg := scene.NewPath(scene.Style{
			Fill:        th.PrimaryColor,
			Stroke:      th.PrimaryBorderColor,
			StrokeWidth: 1,
		})
		tagBg.Rect(tagX-tagW/2, tagY-tagH/2, tagW, tagH, 3)
		sc.Add(tagBg)

		tagText := scene.NewText(tagX, tagY, c.Tag, tagF, th.PrimaryTextColor, scene.AnchorMiddle, scene.VAlignMiddle)
		sc.Add(tagText)
	}
}
