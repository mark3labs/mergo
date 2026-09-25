package tui

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Completion ----------------------------------------------------------------

// diagramHeaders are the keywords that start a diagram, mapped to the type
// they declare.
var diagramHeaders = map[string]string{
	"flowchart": "flowchart", "graph": "flowchart", "flowchart-elk": "flowchart",
	"sequenceDiagram": "sequence",
	"classDiagram":    "class", "classDiagram-v2": "class",
	"stateDiagram": "state", "stateDiagram-v2": "state",
	"erDiagram":     "er",
	"gantt":         "gantt",
	"pie":           "pie",
	"mindmap":       "mindmap",
	"gitGraph":      "gitGraph",
	"timeline":      "timeline",
	"journey":       "journey",
	"quadrantChart": "quadrantChart",
	"xychart-beta":  "xychart", "xychart": "xychart",
}

// headerDirections are completed after the header keyword.
var headerDirections = map[string][]string{
	"flowchart":     {"TD", "TB", "BT", "LR", "RL"},
	"graph":         {"TD", "TB", "BT", "LR", "RL"},
	"flowchart-elk": {"TD", "TB", "BT", "LR", "RL"},
	"gitGraph":      {"LR:", "TB:", "BT:"},
	"xychart-beta":  {"horizontal"},
	"xychart":       {"horizontal"},
	"pie":           {"showData", "title"},
}

// diagramKeywords are the statements, modifiers and options the parsers of
// each diagram type understand.
var diagramKeywords = map[string][]string{
	"flowchart": {"subgraph", "end", "direction", "TB", "TD", "BT", "LR", "RL",
		"classDef", "class", "style", "linkStyle", "click", "callback", "default",
		"accTitle", "accDescr", "title"},
	"sequence": {"participant", "actor", "as", "create", "destroy", "title", "autonumber",
		"off", "activate", "deactivate", "Note", "note", "left of", "right of", "over",
		"loop", "alt", "else", "opt", "par", "and", "critical", "option", "break",
		"rect", "box", "end", "link", "links", "properties", "details",
		"accTitle", "accDescr"},
	"class": {"class", "direction", "TB", "BT", "LR", "RL", "title", "namespace",
		"classDef", "cssClass", "style", "note", "note for", "click", "callback", "link",
		"<<interface>>", "<<abstract>>", "<<enumeration>>", "<<service>>",
		"accTitle", "accDescr"},
	"state": {"state", "direction", "TB", "BT", "LR", "RL", "title", "note",
		"left of", "right of", "end note", "hide", "scale", "click",
		"classDef", "class", "style", "<<fork>>", "<<join>>", "<<choice>>", "[*]",
		"accTitle", "accDescr"},
	"er": {"direction", "TB", "BT", "LR", "RL", "title", "classDef", "class", "style",
		"click", "PK", "FK", "UK", "string", "int", "float", "date", "boolean",
		"accTitle", "accDescr"},
	"gantt": {"title", "dateFormat", "axisFormat", "tickInterval", "excludes",
		"includes", "weekend", "weekends", "todayMarker", "inclusiveEndDates",
		"topAxis", "displayMode", "compact", "section", "active", "done", "crit",
		"milestone", "vert", "after", "until", "click", "accTitle", "accDescr",
		"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"},
	"pie":      {"title", "showData", "accTitle", "accDescr"},
	"mindmap":  {"root", "::icon()"},
	"gitGraph": {"commit", "branch", "checkout", "switch", "merge", "cherry-pick", "id:", "tag:", "msg:", "type:", "order:", "parent:", "NORMAL", "REVERSE", "HIGHLIGHT"},
	"timeline": {"title", "section", "accTitle", "accDescr"},
	"journey":  {"title", "section", "accTitle", "accDescr"},
	"quadrantChart": {"title", "x-axis", "y-axis", "quadrant-1", "quadrant-2",
		"quadrant-3", "quadrant-4", "classDef", "radius", "color", "stroke-color",
		"stroke-width", "accTitle", "accDescr"},
	"xychart": {"title", "x-axis", "y-axis", "bar", "line", "horizontal"},
}

// identifierTypes are the diagram types whose names (nodes, participants,
// classes, states, entities, task ids, branches) are worth completing.
var identifierTypes = map[string]bool{
	"flowchart": true, "sequence": true, "class": true, "state": true,
	"er": true, "gantt": true, "gitGraph": true,
}

// keywordSet returns the words to highlight for a diagram type.
func keywordSet(dtype string) map[string]bool {
	set := map[string]bool{}
	for h := range diagramHeaders {
		set[h] = true
	}
	for _, kw := range diagramKeywords[dtype] {
		for w := range strings.FieldsSeq(kw) {
			if w = strings.Trim(w, "<>:[]*()"); w != "" {
				set[w] = true
			}
		}
	}
	return set
}

// compItem is a completion candidate.
type compItem struct {
	text string
	kind string // "diagram", "keyword" or "name"
	// n is the number of runes before the cursor the candidate replaces.
	n int
}

// completion is the state of the completion popup.
type completion struct {
	open  bool
	items []compItem
	sel   int
	top   int
	// engaged is set when the popup was opened explicitly or navigated:
	// only then does enter accept (tab always does).
	engaged bool
}

const compRows = 8

func (c *completion) close() { *c = completion{} }

func (c *completion) move(d int) {
	if len(c.items) == 0 {
		return
	}
	c.sel = (c.sel + d + len(c.items)) % len(c.items)
	if c.sel < c.top {
		c.top = c.sel
	}
	if c.sel >= c.top+compRows {
		c.top = c.sel - compRows + 1
	}
	c.engaged = true
}

var (
	// wordRe matches the word being typed at the end of the text before
	// the cursor; keywords may contain dashes (x-axis, stateDiagram-v2).
	wordRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_\-]*$`)
	// symbolRe matches a symbolic keyword being typed (<<fork>>, [*]).
	symbolRe = regexp.MustCompile(`(?:<<[A-Za-z]*|\[\*?)$`)
	// identRe finds names in the source.
	identRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*(?:-[A-Za-z0-9_]+)*`)
)

// prefixes returns the word and the symbolic token before the cursor.
func prefixes(before string) (word, sym string) {
	word = wordRe.FindString(before)
	if strings.Contains(word, "--") {
		word = "" // an arrow, not a word
	}
	sym = symbolRe.FindString(before)
	return word, sym
}

// headerInfo locates the diagram header: the first line that isn't blank,
// a comment, a directive or front matter.
func headerInfo(lines [][]rune) (row int, words []string) {
	inFront := false
	for i, l := range lines {
		t := strings.TrimSpace(string(l))
		switch {
		case i == 0 && t == "---":
			inFront = true
			continue
		case inFront:
			if t == "---" {
				inFront = false
			}
			continue
		case t == "" || strings.HasPrefix(t, "%%"):
			continue
		}
		return i, strings.Fields(t)
	}
	return len(lines), nil
}

// diagramTypeOf returns the diagram type declared by the header words.
func diagramTypeOf(words []string) string {
	if len(words) == 0 {
		return ""
	}
	return diagramHeaders[strings.TrimRight(words[0], ":;")]
}

// complete computes the candidates at the cursor of e. With manual set,
// an empty prefix lists everything that fits.
func complete(e *editor, manual bool) []compItem {
	l := e.line()
	before := string(l[:e.col])
	if !manual && e.col < len(l) && isWordRune(l[e.col]) {
		return nil // in the middle of a word
	}
	if s := strings.TrimSpace(before); strings.HasPrefix(s, "%%") || strings.HasPrefix(s, "%%{") {
		return nil
	}
	if strings.Count(before, `"`)%2 == 1 {
		return nil // inside a string
	}
	word, sym := prefixes(before)
	if word == "" && sym == "" && !manual {
		return nil
	}
	hrow, hwords := headerInfo(e.lines)
	var cands []compItem
	lead := strings.Fields(strings.TrimSuffix(before, word))
	switch {
	case e.row < hrow || e.row == hrow && len(lead) == 0:
		for h := range diagramHeaders {
			cands = append(cands, compItem{text: h, kind: "diagram"})
		}
	case e.row == hrow:
		if len(lead) == 1 {
			for _, d := range headerDirections[lead[0]] {
				cands = append(cands, compItem{text: d, kind: "keyword"})
			}
		}
	default:
		dtype := diagramTypeOf(hwords)
		seen := map[string]bool{}
		for _, kw := range diagramKeywords[dtype] {
			if !seen[kw] {
				seen[kw] = true
				cands = append(cands, compItem{text: kw, kind: "keyword"})
			}
		}
		if identifierTypes[dtype] {
			for _, id := range identifiers(e, hrow, dtype) {
				if !seen[id] {
					seen[id] = true
					cands = append(cands, compItem{text: id, kind: "name"})
				}
			}
		}
	}
	// statements start lines; further in, names are the likelier target
	return filterCandidates(cands, word, sym, len(lead) > 0 && e.row > hrow)
}

// identifiers collects the names used in the source (outside of labels,
// strings and comments), excluding the word being typed.
func identifiers(e *editor, hrow int, dtype string) []string {
	kw := keywordSet(dtype)
	count := map[string]int{}
	var order []string
	for i := hrow + 1; i < len(e.lines); i++ {
		s := stripLabels(string(e.lines[i]), dtype)
		if strings.HasPrefix(strings.TrimSpace(s), "%%") {
			continue
		}
		for _, loc := range identRe.FindAllStringIndex(s, -1) {
			w := s[loc[0]:loc[1]]
			if kw[w] {
				continue
			}
			if i == e.row && utf8.RuneCountInString(s[:loc[1]]) == e.col {
				continue // the word being typed
			}
			if count[w] == 0 {
				order = append(order, w)
			}
			count[w]++
		}
	}
	return order
}

// stripLabels blanks out the parts of a line that hold free text: strings,
// bracketed node labels, |edge labels| and (for most types) text after a
// colon. Positions are preserved.
func stripLabels(s string, dtype string) string {
	r := []rune(s)
	depth := 0
	inStr, inPipe := false, false
	colon := false
	for i, c := range r {
		blank := colon || inStr || inPipe || depth > 0
		switch {
		case c == '"':
			inStr = !inStr
			blank = true
		case inStr:
		case c == '[' || c == '(' || c == '{':
			depth++
			blank = true
		case c == ']' || c == ')' || c == '}':
			depth = max(depth-1, 0)
			blank = true
		case c == '|' && dtype == "flowchart":
			inPipe = !inPipe
			blank = true
		case c == ':' && depth == 0 && dtype != "gantt" && dtype != "gitGraph":
			colon = true
		}
		if blank {
			r[i] = ' '
		}
	}
	return string(r)
}

// filterCandidates keeps the candidates matching the typed prefix
// (ignoring case), best matches first. Symbolic candidates (<<fork>>, [*])
// match the symbolic token, the others the word. namesFirst ranks names
// above keywords.
func filterCandidates(cands []compItem, word, sym string, namesFirst bool) []compItem {
	var out []compItem
	for _, c := range cands {
		prefix := word
		if r := rune(c.text[0]); !unicode.IsLetter(r) && r != '_' {
			prefix = sym
		}
		if c.text == prefix || !strings.HasPrefix(strings.ToLower(c.text), strings.ToLower(prefix)) {
			continue
		}
		c.n = len([]rune(prefix))
		out = append(out, c)
	}
	rank := func(c compItem) int {
		r := 0
		if c.n == 0 {
			r += 4 // not matching anything typed (manual completion)
		}
		if (c.kind == "name") != namesFirst {
			r++
		}
		return r
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if ra, rb := rank(a), rank(b); ra != rb {
			return ra < rb
		}
		if len(a.text) != len(b.text) {
			return len(a.text) < len(b.text)
		}
		return a.text < b.text
	})
	return out
}

// isCompletionTrigger reports whether typing s should (re)open completion.
func isCompletionTrigger(s string) bool {
	for _, r := range s {
		if !isWordRune(r) && !strings.ContainsRune("-<[*", r) {
			return false
		}
	}
	return s != ""
}
