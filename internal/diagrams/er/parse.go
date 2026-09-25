// Package er implements Mermaid entity relationship diagrams.
package er

import (
	"fmt"
	"maps"
	"regexp"
	"strings"

	"github.com/mark3labs/mergo/internal/diagram"
)

// Card is a cardinality.
type Card int

const (
	ZeroOrOne Card = iota
	ExactlyOne
	ZeroOrMore
	OneOrMore
)

// Attribute is an entity attribute row.
type Attribute struct {
	Type, Name string
	Keys       []string
	Comment    string
}

// Entity is an ER entity.
type Entity struct {
	ID      string
	Label   string
	Attrs   []Attribute
	Classes []string
	Style   map[string]string
	order   int
}

// Relationship connects two entities.
type Relationship struct {
	From, To         string
	FromCard, ToCard Card
	Identifying      bool
	Label            string
}

// Diagram is a parsed ER diagram.
type Diagram struct {
	Dir       string
	Title     string
	Entities  map[string]*Entity
	Order     []string
	Rels      []*Relationship
	ClassDefs map[string]map[string]string
}

const namePat = `("[^"]+"|[\p{L}\p{N}_\-.*]+)`

var (
	symRelRe  = regexp.MustCompile(`^` + namePat + `\s*([|}][o|])(--|\.\.)([o|][|{])\s*` + namePat + `\s*(?::\s*(.*))?$`)
	cardWord  = `(one or zero|zero or one|one or more|one or many|many\(1\)|1\+|zero or more|zero or many|many\(0\)|0\+|only one|1)`
	wordRelRe = regexp.MustCompile(`^` + namePat + `\s+` + cardWord + `\s+(to|optionally to)\s+` + cardWord + `\s+` + namePat + `\s*(?::\s*(.*))?$`)
	entityRe  = regexp.MustCompile(`^` + namePat + `\s*(?:\[\s*"?([^"\]]*)"?\s*\])?\s*(?::::\s*(\S+))?\s*(\{)?\s*$`)
	attrRe    = regexp.MustCompile(`^(\S+)\s+(\S+)((?:\s+(?:PK|FK|UK)\s*,?)*)\s*(?:"([^"]*)")?\s*$`)
)

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

func wordCard(w string) Card {
	switch w {
	case "one or zero", "zero or one":
		return ZeroOrOne
	case "one or more", "one or many", "many(1)", "1+":
		return OneOrMore
	case "zero or more", "zero or many", "many(0)", "0+":
		return ZeroOrMore
	}
	return ExactlyOne
}

func leftCard(s string) Card {
	switch s {
	case "|o":
		return ZeroOrOne
	case "}o":
		return ZeroOrMore
	case "}|":
		return OneOrMore
	}
	return ExactlyOne
}

func rightCard(s string) Card {
	switch s {
	case "o|":
		return ZeroOrOne
	case "o{":
		return ZeroOrMore
	case "|{":
		return OneOrMore
	}
	return ExactlyOne
}

// Parse parses an ER diagram.
func Parse(src string) (*Diagram, error) {
	d := &Diagram{Dir: "TB", Entities: map[string]*Entity{}, ClassDefs: map[string]map[string]string{}}
	header := false
	var cur *Entity
	entity := func(id string) *Entity {
		id = unquote(id)
		if e, ok := d.Entities[id]; ok {
			return e
		}
		e := &Entity{ID: id, Label: id, order: len(d.Order)}
		d.Entities[id] = e
		d.Order = append(d.Order, id)
		return e
	}
	for i, raw := range strings.Split(src, "\n") {
		ln := i + 1
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		if !header {
			if !strings.HasPrefix(line, "erDiagram") {
				return nil, fmt.Errorf("line %d: expected 'erDiagram'", ln)
			}
			header = true
			continue
		}
		if cur != nil {
			if line == "}" {
				cur = nil
				continue
			}
			m := attrRe.FindStringSubmatch(line)
			if m == nil {
				return nil, fmt.Errorf("line %d: invalid attribute %q (expected 'type name [PK|FK|UK] [\"comment\"]')", ln, line)
			}
			a := Attribute{Type: displayType(m[1]), Name: m[2], Comment: diagram.CleanLabel(m[4])}
			a.Keys = append(a.Keys, strings.FieldsFunc(m[3], func(r rune) bool { return r == ',' || r == ' ' })...)
			cur.Attrs = append(cur.Attrs, a)
			continue
		}
		kw, rest := cutWord(line)
		switch kw {
		case "direction":
			dd := strings.ToUpper(rest)
			if dd == "TD" {
				dd = "TB"
			}
			switch dd {
			case "TB", "BT", "LR", "RL":
				d.Dir = dd
				continue
			}
			return nil, fmt.Errorf("line %d: unknown direction %q", ln, rest)
		case "title":
			d.Title = diagram.CleanLabel(rest)
			continue
		case "classDef":
			names, css := cutWord(rest)
			st := diagram.ParseCSS(css)
			for n := range strings.SplitSeq(names, ",") {
				if n = strings.TrimSpace(n); n != "" {
					if d.ClassDefs[n] == nil {
						d.ClassDefs[n] = map[string]string{}
					}
					maps.Copy(d.ClassDefs[n], st)
				}
			}
			continue
		case "class":
			ids, cls := lastWord(rest)
			for id := range strings.SplitSeq(ids, ",") {
				if id = strings.TrimSpace(id); id != "" {
					e := entity(id)
					e.Classes = append(e.Classes, cls)
				}
			}
			continue
		case "style":
			id, css := cutWord(rest)
			e := entity(id)
			if e.Style == nil {
				e.Style = map[string]string{}
			}
			maps.Copy(e.Style, diagram.ParseCSS(css))
			continue
		case "accTitle", "accDescr", "click":
			continue
		}
		if m := symRelRe.FindStringSubmatch(line); m != nil {
			a, b := entity(m[1]), entity(m[5])
			d.Rels = append(d.Rels, &Relationship{
				From: a.ID, To: b.ID,
				FromCard: leftCard(m[2]), ToCard: rightCard(m[4]),
				Identifying: m[3] == "--",
				Label:       diagram.CleanLabel(m[6]),
			})
			continue
		}
		if m := wordRelRe.FindStringSubmatch(line); m != nil {
			a, b := entity(m[1]), entity(m[5])
			d.Rels = append(d.Rels, &Relationship{
				From: a.ID, To: b.ID,
				FromCard: wordCard(m[2]), ToCard: wordCard(m[4]),
				Identifying: m[3] == "to",
				Label:       diagram.CleanLabel(m[6]),
			})
			continue
		}
		if m := entityRe.FindStringSubmatch(line); m != nil {
			e := entity(m[1])
			if m[2] != "" {
				e.Label = diagram.CleanLabel(m[2])
			}
			if m[3] != "" {
				e.Classes = append(e.Classes, m[3])
			}
			if m[4] == "{" {
				cur = e
			}
			continue
		}
		return nil, fmt.Errorf("line %d: unrecognized statement %q", ln, line)
	}
	if cur != nil {
		return nil, fmt.Errorf("entity %q is not closed with '}'", cur.ID)
	}
	return d, nil
}

// displayType renders generic markers: `list~int~` → `list<int>`.
func displayType(t string) string {
	if strings.Count(t, "~") >= 2 {
		i := strings.IndexByte(t, '~')
		j := strings.LastIndexByte(t, '~')
		return t[:i] + "<" + t[i+1:j] + ">" + t[j+1:]
	}
	return t
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
