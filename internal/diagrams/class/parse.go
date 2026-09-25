package class

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mark3labs/mergo/internal/diagram"
)

// Class is a UML class.
type Class struct {
	ID          string
	Label       string // display name (generics rendered as <T>)
	Annotations []string
	Attributes  []Member
	Methods     []Member
	Classes     []string
	Style       map[string]string
	Namespace   string
	order       int
}

// Member is an attribute or method line.
type Member struct {
	Text     string
	Static   bool
	Abstract bool
}

// End describes a relation end marker.
type End int

const (
	EndNone End = iota
	EndInheritance
	EndComposition
	EndAggregation
	EndArrow
	EndLollipop
)

// Relation connects two classes.
type Relation struct {
	From, To         string
	FromEnd, ToEnd   End
	Dashed           bool
	FromCard, ToCard string
	Label            string
}

// Note is a note (optionally attached to a class).
type Note struct {
	For  string
	Text string
}

// Namespace groups classes.
type Namespace struct {
	ID      string
	Parent  string
	Classes []string
	Subs    []string
}

// Diagram is a parsed class diagram.
type Diagram struct {
	Dir        string
	Title      string
	Classes    map[string]*Class
	Order      []string
	Relations  []*Relation
	Notes      []*Note
	Namespaces map[string]*Namespace
	NSOrder    []string
	ClassDefs  map[string]map[string]string
}

var (
	classRe   = regexp.MustCompile(`^class\s+([^\s{\[~:<]+)(~[^{<]*~)?\s*(\["([^"]*)"\])?\s*(<<\s*([^>]*?)\s*>>)?\s*(:::\s*(\S+))?\s*(\{)?\s*(.*)$`)
	annotRe   = regexp.MustCompile(`^<<\s*([^>]+?)\s*>>\s*(\S+)?\s*$`)
	noteForRe = regexp.MustCompile(`^note\s+for\s+(\S+)\s+"((?:[^"\\]|\\.)*)"\s*$`)
	noteRe    = regexp.MustCompile(`^note\s+"((?:[^"\\]|\\.)*)"\s*$`)
)

// displayGeneric converts Mermaid's ~T~ generics to <T> (nested too):
// a '~' opens a generic when nothing is open or when it is directly
// followed by a type name; otherwise it closes one.
func displayGeneric(s string) string {
	if !strings.Contains(s, "~") {
		return s
	}
	var b strings.Builder
	depth := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '~' {
			b.WriteByte(c)
			continue
		}
		var next byte
		if i+1 < len(s) {
			next = s[i+1]
		}
		ident := next == '_' || next >= 'a' && next <= 'z' || next >= 'A' && next <= 'Z' || next >= '0' && next <= '9'
		if depth == 0 || ident {
			b.WriteByte('<')
			depth++
		} else {
			b.WriteByte('>')
			depth--
		}
	}
	return b.String()
}

type parser struct {
	d       *Diagram
	nsStack []string
	inClass *Class
	braceNS []bool // for each '{' on the stack: true = namespace
}

// Parse parses class diagram source.
func Parse(src string) (*Diagram, error) {
	d := &Diagram{
		Dir:        "TB",
		Classes:    map[string]*Class{},
		Namespaces: map[string]*Namespace{},
		ClassDefs:  map[string]map[string]string{},
	}
	p := &parser{d: d}
	header := false
	for i, raw := range strings.Split(src, "\n") {
		ln := i + 1
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		if !header {
			if !strings.HasPrefix(line, "classDiagram") {
				return nil, fmt.Errorf("line %d: expected 'classDiagram'", ln)
			}
			header = true
			continue
		}
		if err := p.line(line, ln); err != nil {
			return nil, err
		}
	}
	if p.inClass != nil {
		return nil, fmt.Errorf("class %q is not closed with '}'", p.inClass.ID)
	}
	if len(p.nsStack) > 0 {
		return nil, fmt.Errorf("namespace %q is not closed with '}'", p.nsStack[len(p.nsStack)-1])
	}
	return d, nil
}

func (p *parser) class(id string) *Class {
	id = strings.TrimSpace(id)
	generic := ""
	if i := strings.IndexByte(id, '~'); i > 0 {
		generic = id[i:]
		id = id[:i]
	}
	c, ok := p.d.Classes[id]
	if !ok {
		c = &Class{ID: id, Label: id, order: len(p.d.Order)}
		p.d.Classes[id] = c
		p.d.Order = append(p.d.Order, id)
		if len(p.nsStack) > 0 {
			ns := p.nsStack[len(p.nsStack)-1]
			c.Namespace = ns
			p.d.Namespaces[ns].Classes = append(p.d.Namespaces[ns].Classes, id)
		}
	}
	if generic != "" && c.Label == c.ID {
		c.Label = id + displayGeneric(generic)
	}
	return c
}

func parseMember(s string) Member {
	s = strings.TrimSpace(s)
	m := Member{}
	if strings.HasSuffix(s, "$") {
		m.Static = true
		s = strings.TrimSpace(strings.TrimSuffix(s, "$"))
	}
	if strings.HasSuffix(s, "*") {
		m.Abstract = true
		s = strings.TrimSpace(strings.TrimSuffix(s, "*"))
	}
	// static/abstract markers may also follow the parenthesis: foo()$ int
	if i := strings.Index(s, ")$"); i >= 0 {
		m.Static = true
		s = s[:i+1] + s[i+2:]
	}
	if i := strings.Index(s, ")*"); i >= 0 {
		m.Abstract = true
		s = s[:i+1] + s[i+2:]
	}
	m.Text = displayGeneric(diagram.CleanLabel(s))
	return m
}

func (c *Class) addMember(s string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}
	if m := annotRe.FindStringSubmatch(s); m != nil && m[2] == "" {
		c.Annotations = append(c.Annotations, m[1])
		return
	}
	mem := parseMember(s)
	if strings.Contains(s, "(") {
		c.Methods = append(c.Methods, mem)
	} else {
		c.Attributes = append(c.Attributes, mem)
	}
}

func (p *parser) line(line string, ln int) error {
	if p.inClass != nil {
		if line == "}" {
			p.inClass = nil
			return nil
		}
		// a member line may end the block: "+foo() }" is unusual; handle
		if strings.HasSuffix(line, "}") && !strings.HasSuffix(line, "{}") {
			p.inClass.addMember(strings.TrimSuffix(line, "}"))
			p.inClass = nil
			return nil
		}
		p.inClass.addMember(line)
		return nil
	}
	kw, rest := cutWord(line)
	switch kw {
	case "direction":
		d := strings.ToUpper(strings.TrimSpace(rest))
		if d == "TD" {
			d = "TB"
		}
		switch d {
		case "TB", "BT", "LR", "RL":
			p.d.Dir = d
			return nil
		}
		return fmt.Errorf("line %d: unknown direction %q", ln, rest)
	case "title":
		p.d.Title = diagram.CleanLabel(rest)
		return nil
	case "namespace":
		name := strings.TrimSpace(strings.TrimSuffix(rest, "{"))
		if name == "" {
			return fmt.Errorf("line %d: namespace needs a name", ln)
		}
		ns, ok := p.d.Namespaces[name]
		if !ok {
			ns = &Namespace{ID: name}
			p.d.Namespaces[name] = ns
			p.d.NSOrder = append(p.d.NSOrder, name)
		}
		if len(p.nsStack) > 0 && ns.Parent == "" {
			ns.Parent = p.nsStack[len(p.nsStack)-1]
			par := p.d.Namespaces[ns.Parent]
			par.Subs = append(par.Subs, name)
		}
		p.nsStack = append(p.nsStack, name)
		return nil
	case "}":
		if len(p.nsStack) == 0 {
			return fmt.Errorf("line %d: unexpected '}'", ln)
		}
		p.nsStack = p.nsStack[:len(p.nsStack)-1]
		return nil
	case "class":
		m := classRe.FindStringSubmatch(line)
		if m == nil {
			return fmt.Errorf("line %d: invalid class declaration", ln)
		}
		c := p.class(m[1] + m[2])
		if m[4] != "" {
			c.Label = diagram.CleanLabel(m[4])
		}
		if m[6] != "" {
			c.Annotations = append(c.Annotations, m[6])
		}
		if m[8] != "" {
			c.Classes = append(c.Classes, m[8])
		}
		if m[9] == "{" {
			body := strings.TrimSpace(m[10])
			if strings.HasSuffix(body, "}") {
				// single line: class A { +x }
				for _, part := range strings.Split(strings.TrimSuffix(body, "}"), ";") {
					c.addMember(part)
				}
				return nil
			}
			if body != "" {
				c.addMember(body)
			}
			p.inClass = c
		}
		return nil
	case "classDef":
		names, css := cutWord(rest)
		st := diagram.ParseCSS(css)
		for _, n := range strings.Split(names, ",") {
			if n = strings.TrimSpace(n); n != "" {
				if p.d.ClassDefs[n] == nil {
					p.d.ClassDefs[n] = map[string]string{}
				}
				for k, v := range st {
					p.d.ClassDefs[n][k] = v
				}
			}
		}
		return nil
	case "cssClass":
		// cssClass "A,B" name
		q := strings.Index(rest, `"`)
		q2 := strings.LastIndex(rest, `"`)
		if q < 0 || q2 <= q {
			return fmt.Errorf("line %d: cssClass expects \"ids\" className", ln)
		}
		name := strings.TrimSpace(rest[q2+1:])
		for _, id := range strings.Split(rest[q+1:q2], ",") {
			if id = strings.TrimSpace(id); id != "" {
				c := p.class(id)
				c.Classes = append(c.Classes, name)
			}
		}
		return nil
	case "style":
		id, css := cutWord(rest)
		c := p.class(id)
		if c.Style == nil {
			c.Style = map[string]string{}
		}
		for k, v := range diagram.ParseCSS(css) {
			c.Style[k] = v
		}
		return nil
	case "click", "callback", "link", "accTitle", "accDescr":
		return nil
	case "note":
		if m := noteForRe.FindStringSubmatch(line); m != nil {
			p.class(m[1])
			p.d.Notes = append(p.d.Notes, &Note{For: m[1], Text: diagram.CleanLabel(unescape(m[2]))})
			return nil
		}
		if m := noteRe.FindStringSubmatch(line); m != nil {
			p.d.Notes = append(p.d.Notes, &Note{Text: diagram.CleanLabel(unescape(m[1]))})
			return nil
		}
		return fmt.Errorf("line %d: invalid note", ln)
	}
	if m := annotRe.FindStringSubmatch(line); m != nil && m[2] != "" {
		c := p.class(m[2])
		c.Annotations = append(c.Annotations, m[1])
		return nil
	}
	if rel, ok, err := parseRelation(line, ln); err != nil {
		return err
	} else if ok {
		p.class(rel.From)
		p.class(rel.To)
		rel.From = baseID(rel.From)
		rel.To = baseID(rel.To)
		p.d.Relations = append(p.d.Relations, rel)
		return nil
	}
	// member statement: A : +int x   /   A:::cls
	if i := strings.Index(line, ":::"); i > 0 && !strings.Contains(line[:i], " ") {
		c := p.class(line[:i])
		c.Classes = append(c.Classes, strings.TrimSpace(line[i+3:]))
		return nil
	}
	if id, mem, ok := strings.Cut(line, ":"); ok && !strings.ContainsAny(strings.TrimSpace(id), " \t") && strings.TrimSpace(id) != "" {
		c := p.class(strings.TrimSpace(id))
		c.addMember(mem)
		return nil
	}
	// bare class name
	if !strings.ContainsAny(line, " \t{}\"") {
		p.class(line)
		return nil
	}
	return fmt.Errorf("line %d: unrecognized statement %q", ln, line)
}

func unescape(s string) string { return strings.ReplaceAll(s, `\"`, `"`) }

func baseID(id string) string {
	id = strings.TrimSpace(id)
	if i := strings.IndexByte(id, '~'); i > 0 {
		return id[:i]
	}
	return id
}

func cutWord(s string) (string, string) {
	s = strings.TrimSpace(s)
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimSpace(s[i+1:])
}

// parseRelation parses `A "1" <|-- "*" B : label`.
func parseRelation(line string, ln int) (*Relation, bool, error) {
	// locate the line operator outside quotes
	op, opIdx := "", -1
	inQ := false
	for i := 0; i+1 < len(line); i++ {
		c := line[i]
		if c == '"' {
			inQ = !inQ
			continue
		}
		if inQ {
			continue
		}
		if (c == '-' && line[i+1] == '-') || (c == '.' && line[i+1] == '.') {
			op, opIdx = line[i:i+2], i
			break
		}
		if c == ':' {
			break
		}
	}
	if opIdx < 0 {
		return nil, false, nil
	}
	left := line[:opIdx]
	right := line[opIdx+2:]
	r := &Relation{Dashed: op == ".."}

	// left marker
	lt := strings.TrimRight(left, " ")
	switch {
	case strings.HasSuffix(lt, "<|"):
		r.FromEnd, lt = EndInheritance, lt[:len(lt)-2]
	case strings.HasSuffix(lt, "()"):
		r.FromEnd, lt = EndLollipop, lt[:len(lt)-2]
	case strings.HasSuffix(lt, "*"):
		r.FromEnd, lt = EndComposition, lt[:len(lt)-1]
	case strings.HasSuffix(lt, "<"):
		r.FromEnd, lt = EndArrow, lt[:len(lt)-1]
	case strings.HasSuffix(lt, " o") || lt == "o" || strings.HasSuffix(lt, "\"o"):
		r.FromEnd, lt = EndAggregation, lt[:len(lt)-1]
	}
	// right marker
	rt := strings.TrimLeft(right, " ")
	switch {
	case strings.HasPrefix(rt, "|>"):
		r.ToEnd, rt = EndInheritance, rt[2:]
	case strings.HasPrefix(rt, "()"):
		r.ToEnd, rt = EndLollipop, rt[2:]
	case strings.HasPrefix(rt, "*"):
		r.ToEnd, rt = EndComposition, rt[1:]
	case strings.HasPrefix(rt, ">"):
		r.ToEnd, rt = EndArrow, rt[1:]
	case strings.HasPrefix(rt, "o ") || rt == "o" || strings.HasPrefix(rt, "o\""):
		r.ToEnd, rt = EndAggregation, rt[1:]
	}
	// label
	if i := strings.LastIndex(rt, ":"); i >= 0 && !strings.Contains(rt[i:], "\"") {
		r.Label = displayGeneric(diagram.CleanLabel(rt[i+1:]))
		rt = rt[:i]
	}
	// cardinalities
	lt = strings.TrimSpace(lt)
	if strings.HasSuffix(lt, "\"") {
		if q := strings.LastIndex(lt[:len(lt)-1], "\""); q >= 0 {
			r.FromCard = lt[q+1 : len(lt)-1]
			lt = strings.TrimSpace(lt[:q])
		}
	}
	rt = strings.TrimSpace(rt)
	if strings.HasPrefix(rt, "\"") {
		if q := strings.Index(rt[1:], "\""); q >= 0 {
			r.ToCard = rt[1 : q+1]
			rt = strings.TrimSpace(rt[q+2:])
		}
	}
	r.From, r.To = lt, rt
	if r.From == "" || r.To == "" || strings.ContainsAny(r.From, " \t") || strings.ContainsAny(r.To, " \t") {
		return nil, false, fmt.Errorf("line %d: invalid relation %q", ln, line)
	}
	return r, true, nil
}
