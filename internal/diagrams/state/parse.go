package state

import (
	"fmt"
	"maps"
	"regexp"
	"strings"

	"github.com/mark3labs/mergo/internal/diagram"
)

// Kind is the kind of a state node.
type Kind int

const (
	KindNormal Kind = iota
	KindStart
	KindEnd
	KindFork
	KindJoin
	KindChoice
)

// State is a node (simple or composite).
type State struct {
	ID      string
	Label   string
	Descs   []string
	Kind    Kind
	Parent  string // composite id ("" = root)
	Region  int    // region index inside the parent composite
	Regions [][]string
	Dir     string
	Classes []string
	Style   map[string]string
	order   int
}

// IsComposite reports whether the state has children.
func (s *State) IsComposite() bool {
	for _, r := range s.Regions {
		if len(r) > 0 {
			return true
		}
	}
	return false
}

// Transition is an edge.
type Transition struct {
	From, To string
	Label    string
}

// Note is a note attached to a state.
type Note struct {
	Target string
	Left   bool
	Text   string
}

// Diagram is a parsed state diagram.
type Diagram struct {
	Dir         string
	Title       string
	States      map[string]*State
	Order       []string
	Root        [][]string // regions of the root scope (usually one)
	Transitions []*Transition
	Notes       []*Note
	ClassDefs   map[string]map[string]string
}

var (
	stateAsRe  = regexp.MustCompile(`^state\s+"([^"]*)"\s+as\s+(\S+?)\s*(\{)?\s*$`)
	stateRe    = regexp.MustCompile(`^state\s+([^\s{"]+)\s*(<<\s*(fork|join|choice)\s*>>)?\s*(\{)?\s*$`)
	arrowRe    = regexp.MustCompile(`^(\S+?)\s*-->\s*(\S+?)\s*(?::\s*(.*))?$`)
	noteRe     = regexp.MustCompile(`^note\s+(left|right)\s+of\s+(\S+)\s*(?::\s*(.*))?$`)
	classAppRe = regexp.MustCompile(`^(\S+?):::(\S+)$`)
)

type scope struct {
	id     string // composite id, "" for root
	region int
}

type parser struct {
	d      *Diagram
	stack  []scope
	inNote *Note
	noteLn int
}

// Parse parses a state diagram.
func Parse(src string) (*Diagram, error) {
	d := &Diagram{Dir: "TB", States: map[string]*State{}, Root: [][]string{nil}, ClassDefs: map[string]map[string]string{}}
	p := &parser{d: d}
	header := false
	for i, raw := range strings.Split(src, "\n") {
		ln := i + 1
		line := strings.TrimSpace(raw)
		if p.inNote != nil {
			if strings.EqualFold(line, "end note") {
				p.inNote.Text = strings.TrimSpace(p.inNote.Text)
				d.Notes = append(d.Notes, p.inNote)
				p.inNote = nil
				continue
			}
			p.inNote.Text += diagram.CleanLabel(line) + "\n"
			continue
		}
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		if !header {
			if !strings.HasPrefix(line, "stateDiagram") {
				return nil, fmt.Errorf("line %d: expected 'stateDiagram' or 'stateDiagram-v2'", ln)
			}
			header = true
			continue
		}
		if err := p.line(strings.TrimSuffix(line, ";"), ln); err != nil {
			return nil, err
		}
	}
	if p.inNote != nil {
		return nil, fmt.Errorf("line %d: note is not closed with 'end note'", p.noteLn)
	}
	if len(p.stack) > 0 {
		return nil, fmt.Errorf("composite state %q is not closed with '}'", p.stack[len(p.stack)-1].id)
	}
	return d, nil
}

func (p *parser) cur() scope {
	if len(p.stack) == 0 {
		return scope{}
	}
	return p.stack[len(p.stack)-1]
}

// regionList returns a pointer to the member list of the current region.
func (p *parser) addToScope(id string) {
	sc := p.cur()
	if sc.id == "" {
		p.d.Root[sc.region] = append(p.d.Root[sc.region], id)
		return
	}
	par := p.d.States[sc.id]
	for len(par.Regions) <= sc.region {
		par.Regions = append(par.Regions, nil)
	}
	par.Regions[sc.region] = append(par.Regions[sc.region], id)
}

// state returns (creating in the current scope) a state.
func (p *parser) state(id string) *State {
	if s, ok := p.d.States[id]; ok {
		return s
	}
	sc := p.cur()
	s := &State{ID: id, Label: id, Parent: sc.id, Region: sc.region, order: len(p.d.Order)}
	p.d.States[id] = s
	p.d.Order = append(p.d.Order, id)
	p.addToScope(id)
	return s
}

// pseudo returns the start or end pseudo state of the current scope.
func (p *parser) pseudo(start bool) string {
	sc := p.cur()
	kind, tag := KindEnd, "end"
	if start {
		kind, tag = KindStart, "start"
	}
	id := fmt.Sprintf("[*]%s:%s:%d", tag, sc.id, sc.region)
	s := p.state(id)
	s.Kind = kind
	s.Label = ""
	return id
}

func (p *parser) line(line string, ln int) error {
	switch {
	case strings.HasPrefix(line, "direction "):
		dir := strings.ToUpper(strings.TrimSpace(line[10:]))
		if dir == "TD" {
			dir = "TB"
		}
		switch dir {
		case "TB", "BT", "LR", "RL":
		default:
			return fmt.Errorf("line %d: unknown direction %q", ln, line[10:])
		}
		if sc := p.cur(); sc.id != "" {
			p.d.States[sc.id].Dir = dir
		} else {
			p.d.Dir = dir
		}
		return nil
	case strings.HasPrefix(line, "title "):
		p.d.Title = diagram.CleanLabel(line[6:])
		return nil
	case line == "}":
		if len(p.stack) == 0 {
			return fmt.Errorf("line %d: unexpected '}'", ln)
		}
		p.stack = p.stack[:len(p.stack)-1]
		return nil
	case line == "--":
		sc := p.cur()
		if sc.id == "" {
			return nil // concurrency separators only apply inside composites
		}
		p.stack[len(p.stack)-1].region++
		par := p.d.States[sc.id]
		par.Regions = append(par.Regions, nil)
		return nil
	case strings.HasPrefix(line, "hide ") || strings.HasPrefix(line, "scale ") ||
		strings.HasPrefix(line, "click ") || strings.HasPrefix(line, "accTitle") || strings.HasPrefix(line, "accDescr"):
		return nil
	case strings.HasPrefix(line, "classDef "):
		names, css := cutWord(line[9:])
		st := diagram.ParseCSS(css)
		for n := range strings.SplitSeq(names, ",") {
			if n = strings.TrimSpace(n); n != "" {
				if p.d.ClassDefs[n] == nil {
					p.d.ClassDefs[n] = map[string]string{}
				}
				maps.Copy(p.d.ClassDefs[n], st)
			}
		}
		return nil
	case strings.HasPrefix(line, "class "):
		ids, cls := lastWord(line[6:])
		for id := range strings.SplitSeq(ids, ",") {
			if id = strings.TrimSpace(id); id != "" {
				s := p.state(id)
				s.Classes = append(s.Classes, cls)
			}
		}
		return nil
	case strings.HasPrefix(line, "style "):
		id, css := cutWord(line[6:])
		s := p.state(id)
		if s.Style == nil {
			s.Style = map[string]string{}
		}
		maps.Copy(s.Style, diagram.ParseCSS(css))
		return nil
	case strings.HasPrefix(strings.ToLower(line), "note "):
		m := noteRe.FindStringSubmatch(line)
		if m == nil {
			return fmt.Errorf("line %d: invalid note (use 'note left of X : text' or 'note right of X')", ln)
		}
		p.state(m[2])
		n := &Note{Target: m[2], Left: m[1] == "left"}
		if strings.Contains(line, ":") {
			n.Text = diagram.CleanLabel(m[3])
			p.d.Notes = append(p.d.Notes, n)
			return nil
		}
		p.inNote, p.noteLn = n, ln
		return nil
	case strings.HasPrefix(line, "state "):
		if m := stateAsRe.FindStringSubmatch(line); m != nil {
			s := p.state(m[2])
			s.Label = diagram.CleanLabel(m[1])
			if m[3] == "{" {
				p.stack = append(p.stack, scope{id: s.ID})
			}
			return nil
		}
		if m := stateRe.FindStringSubmatch(line); m != nil {
			s := p.state(m[1])
			switch m[3] {
			case "fork":
				s.Kind = KindFork
			case "join":
				s.Kind = KindJoin
			case "choice":
				s.Kind = KindChoice
			}
			if m[4] == "{" {
				p.stack = append(p.stack, scope{id: s.ID})
			}
			return nil
		}
		// state Name : description  /  state "desc" as X : ...
		if id, desc, ok := strings.Cut(line[6:], ":"); ok {
			s := p.state(strings.TrimSpace(id))
			s.Descs = append(s.Descs, diagram.CleanLabel(desc))
			return nil
		}
		return fmt.Errorf("line %d: invalid state declaration", ln)
	}
	if m := arrowRe.FindStringSubmatch(line); m != nil {
		from := p.endpoint(m[1], true)
		to := p.endpoint(m[2], false)
		p.d.Transitions = append(p.d.Transitions, &Transition{From: from, To: to, Label: diagram.CleanLabel(m[3])})
		return nil
	}
	if m := classAppRe.FindStringSubmatch(line); m != nil {
		s := p.state(m[1])
		s.Classes = append(s.Classes, m[2])
		return nil
	}
	// S : description
	if id, desc, ok := strings.Cut(line, ":"); ok {
		id = strings.TrimSpace(id)
		if id != "" && !strings.ContainsAny(id, " \t") {
			s := p.state(id)
			s.Descs = append(s.Descs, diagram.CleanLabel(desc))
			return nil
		}
	}
	// bare state name
	if !strings.ContainsAny(line, " \t{}") {
		p.state(line)
		return nil
	}
	return fmt.Errorf("line %d: unrecognized statement %q", ln, line)
}

// endpoint resolves a transition endpoint, handling [*] and :::class.
func (p *parser) endpoint(tok string, source bool) string {
	cls := ""
	if i := strings.Index(tok, ":::"); i > 0 {
		tok, cls = tok[:i], tok[i+3:]
	}
	if tok == "[*]" {
		return p.pseudo(source)
	}
	s := p.state(tok)
	if cls != "" {
		s.Classes = append(s.Classes, cls)
	}
	return s.ID
}

func cutWord(s string) (string, string) {
	s = strings.TrimSpace(s)
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimSpace(s[i+1:])
}

func lastWord(s string) (string, string) {
	s = strings.TrimSpace(s)
	i := strings.LastIndexAny(s, " \t")
	if i < 0 {
		return "", s
	}
	return strings.TrimSpace(s[:i]), s[i+1:]
}
