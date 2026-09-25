package flowchart

import (
	"fmt"
	"maps"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mark3labs/mergo/internal/diagram"
)

// Shape is a node shape name (Mermaid's new shape names are used).
type Shape string

// Classic shapes (the new @{shape: ...} names map onto these or extra ones).
const (
	ShapeRect         Shape = "rect"
	ShapeRounded      Shape = "rounded"
	ShapeStadium      Shape = "stadium"
	ShapeSubroutine   Shape = "subproc"
	ShapeCylinder     Shape = "cyl"
	ShapeCircle       Shape = "circle"
	ShapeDoubleCircle Shape = "dbl-circ"
	ShapeDiamond      Shape = "diamond"
	ShapeHexagon      Shape = "hex"
	ShapeLeanRight    Shape = "lean-r"
	ShapeLeanLeft     Shape = "lean-l"
	ShapeTrapezoid    Shape = "trap-b"
	ShapeTrapezoidAlt Shape = "trap-t"
	ShapeOdd          Shape = "odd"
)

// Node is a flowchart node.
type Node struct {
	ID      string
	Label   string
	Shape   Shape
	Classes []string
	Style   map[string]string
	// Subgraph is the id of the innermost subgraph containing the node.
	Subgraph string
	// labelSet reports whether a label/shape was explicitly given.
	labelSet bool
	order    int
}

// LinkKind is the line style of an edge.
type LinkKind int

const (
	LinkNormal LinkKind = iota
	LinkThick
	LinkDotted
	LinkInvisible
)

// Marker is an edge end decoration.
type Marker byte

const (
	MarkerNone   Marker = 0
	MarkerArrow  Marker = '>'
	MarkerCircle Marker = 'o'
	MarkerCross  Marker = 'x'
)

// Edge is a flowchart link.
type Edge struct {
	From, To   string
	ID         string
	Label      string
	Kind       LinkKind
	Start, End Marker
	Length     int
	Style      map[string]string
	Index      int
}

// Subgraph is a cluster.
type Subgraph struct {
	ID       string
	Title    string
	Dir      string // "" = inherit
	Parent   string
	Children []string // node ids (direct)
	Subs     []string // nested subgraph ids
	Classes  []string
	Style    map[string]string
	order    int
}

// Graph is a parsed flowchart.
type Graph struct {
	Dir       string
	Nodes     map[string]*Node
	NodeOrder []string
	Edges     []*Edge
	Subgraphs map[string]*Subgraph
	SubOrder  []string
	ClassDefs map[string]map[string]string
	// LinkStyles by edge index; -1 is "default".
	LinkStyles map[int]map[string]string
}

type stmt struct {
	text string
	line int
}

// splitStatements splits the source into statements separated by newlines
// or semicolons (outside quotes and brackets).
func splitStatements(src string) []stmt {
	var out []stmt
	line := 1
	var cur strings.Builder
	curLine := 1
	inQuote := false
	depth := 0
	flush := func() {
		t := strings.TrimSpace(cur.String())
		if t != "" {
			out = append(out, stmt{t, curLine})
		}
		cur.Reset()
	}
	for i, r := range src {
		switch {
		case r == '\n':
			line++
			if inQuote {
				// quoted strings may span lines (markdown strings)
				cur.WriteRune(r)
				continue
			}
			flush()
			curLine = line
			depth = 0
			continue
		case r == '"':
			inQuote = !inQuote
		case !inQuote && (r == '[' || r == '(' || r == '{'):
			depth++
		case !inQuote && (r == ']' || r == ')' || r == '}'):
			if depth > 0 {
				depth--
			}
		case r == ';' && !inQuote && depth == 0 && (isStyleStmt(cur.String()) || !diagram.EndsEntity(src, i)):
			flush()
			curLine = line
			continue
		}
		if cur.Len() == 0 {
			curLine = line
		}
		cur.WriteRune(r)
	}
	flush()
	return out
}

// isStyleStmt reports whether a (partial) statement is a style directive,
// where "#333;" is a color followed by a separator rather than an entity
// (Mermaid special-cases these the same way).
func isStyleStmt(s string) bool {
	kw, _, _ := strings.Cut(strings.TrimSpace(s), " ")
	return kw == "style" || kw == "classDef" || kw == "linkStyle"
}

var headerRe = regexp.MustCompile(`^(?:graph|flowchart|flowchart-elk)(?:\s+(\S+))?\s*$`)

// Parse parses flowchart source.
func Parse(src string) (*Graph, error) {
	g := &Graph{
		Dir:        "TB",
		Nodes:      map[string]*Node{},
		Subgraphs:  map[string]*Subgraph{},
		ClassDefs:  map[string]map[string]string{},
		LinkStyles: map[int]map[string]string{},
	}
	p := &parser{g: g, edgeIDs: map[string]bool{}}
	stmts := splitStatements(src)
	if len(stmts) == 0 {
		return nil, fmt.Errorf("empty flowchart")
	}
	first := stmts[0]
	m := headerRe.FindStringSubmatch(first.text)
	if m == nil {
		// header may be followed by content on the same line without ';'
		return nil, fmt.Errorf("line %d: expected 'flowchart' or 'graph' header", first.line)
	}
	if m[1] != "" {
		d := strings.ToUpper(m[1])
		switch d {
		case "TB", "TD", "BT", "LR", "RL":
			g.Dir = d
		case "V", "^":
			g.Dir = "BT"
		case ">":
			g.Dir = "LR"
		case "<":
			g.Dir = "RL"
		default:
			return nil, fmt.Errorf("line %d: unknown direction %q", first.line, m[1])
		}
	}
	if g.Dir == "TD" {
		g.Dir = "TB"
	}
	for _, s := range stmts[1:] {
		if err := p.statement(s); err != nil {
			return nil, err
		}
	}
	if len(p.stack) > 0 {
		return nil, fmt.Errorf("line %d: subgraph %q is not closed with 'end'", p.stackLine[len(p.stackLine)-1], p.stack[len(p.stack)-1])
	}
	p.finish()
	return g, nil
}

type parser struct {
	g         *Graph
	stack     []string // open subgraph ids
	stackLine []int
	edgeCount int
	subCount  int
	edgeIDs   map[string]bool
}

func (p *parser) statement(s stmt) error {
	t := s.text
	kw, rest := firstWord(t)
	switch kw {
	case "subgraph":
		return p.subgraph(rest, s.line)
	case "end":
		if rest != "" && !strings.HasPrefix(rest, "%%") {
			break // e.g. "end --> x"? treat as regular statement
		}
		if len(p.stack) == 0 {
			return fmt.Errorf("line %d: 'end' without matching 'subgraph'", s.line)
		}
		p.stack = p.stack[:len(p.stack)-1]
		p.stackLine = p.stackLine[:len(p.stackLine)-1]
		return nil
	case "direction":
		d := strings.ToUpper(strings.TrimSpace(rest))
		if d == "TD" {
			d = "TB"
		}
		switch d {
		case "TB", "BT", "LR", "RL":
		default:
			return fmt.Errorf("line %d: unknown direction %q", s.line, rest)
		}
		if len(p.stack) > 0 {
			p.g.Subgraphs[p.stack[len(p.stack)-1]].Dir = d
		} else {
			p.g.Dir = d
		}
		return nil
	case "classDef":
		names, styles := firstWord(rest)
		st := parseStyle(styles)
		for n := range strings.SplitSeq(names, ",") {
			n = strings.TrimSpace(n)
			if n == "" {
				continue
			}
			if old, ok := p.g.ClassDefs[n]; ok {
				maps.Copy(old, st)
			} else {
				cp := map[string]string{}
				maps.Copy(cp, st)
				p.g.ClassDefs[n] = cp
			}
		}
		return nil
	case "class":
		ids, cls := lastWord(rest)
		if ids == "" {
			return fmt.Errorf("line %d: class statement needs node ids and a class name", s.line)
		}
		for id := range strings.SplitSeq(ids, ",") {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if sg, ok := p.g.Subgraphs[id]; ok {
				sg.Classes = append(sg.Classes, cls)
				continue
			}
			n := p.node(id)
			n.Classes = append(n.Classes, cls)
		}
		return nil
	case "style":
		id, styles := firstWord(rest)
		st := parseStyle(styles)
		if sg, ok := p.g.Subgraphs[id]; ok {
			mergeStyle(&sg.Style, st)
			return nil
		}
		// may reference a subgraph defined later; remember on the node and
		// transfer in finish()
		n := p.node(id)
		mergeStyle(&n.Style, st)
		return nil
	case "linkStyle":
		idxs, styles := firstWord(rest)
		st := parseStyle(styles)
		for is := range strings.SplitSeq(idxs, ",") {
			is = strings.TrimSpace(is)
			if is == "default" {
				mergeLinkStyle(p.g.LinkStyles, -1, st)
				continue
			}
			var i int
			if _, err := fmt.Sscanf(is, "%d", &i); err != nil {
				return fmt.Errorf("line %d: invalid linkStyle index %q", s.line, is)
			}
			mergeLinkStyle(p.g.LinkStyles, i, st)
		}
		return nil
	case "click", "callback", "accTitle", "accDescr", "title":
		return nil
	}
	// edge-id properties: e1@{ animate: true }
	if m := edgePropRe.FindStringSubmatch(t); m != nil && p.edgeIDs[m[1]] {
		return nil
	}
	return p.chain(s)
}

var edgePropRe = regexp.MustCompile(`^([\w-]+)@\{[^}]*\}\s*$`)

func (p *parser) subgraph(rest string, line int) error {
	rest = strings.TrimSpace(rest)
	var id, title string
	switch {
	case rest == "":
		p.subCount++
		id = fmt.Sprintf("subGraph%d", p.subCount)
	case strings.HasPrefix(rest, "\""):
		title = diagram.CleanLabel(rest)
		id = title
	default:
		if i := strings.IndexAny(rest, "[("); i > 0 {
			id = strings.TrimSpace(rest[:i])
			inner := strings.TrimSpace(rest[i+1:])
			inner = strings.TrimRight(inner, "])")
			title = diagram.CleanLabel(inner)
		} else {
			id = rest
			title = diagram.CleanLabel(rest)
		}
	}
	if title == "" {
		title = id
	}
	sg, ok := p.g.Subgraphs[id]
	if !ok {
		sg = &Subgraph{ID: id, order: len(p.g.SubOrder)}
		p.g.Subgraphs[id] = sg
		p.g.SubOrder = append(p.g.SubOrder, id)
	}
	sg.Title = title
	if len(p.stack) > 0 {
		parent := p.stack[len(p.stack)-1]
		if sg.Parent == "" && parent != id {
			sg.Parent = parent
			ps := p.g.Subgraphs[parent]
			ps.Subs = append(ps.Subs, id)
		}
	}
	p.stack = append(p.stack, id)
	p.stackLine = append(p.stackLine, line)
	return nil
}

// node returns (creating if needed) a node and records subgraph membership.
func (p *parser) node(id string) *Node {
	n, ok := p.g.Nodes[id]
	if !ok {
		n = &Node{ID: id, Label: id, Shape: ShapeRect, order: len(p.g.NodeOrder)}
		p.g.Nodes[id] = n
		p.g.NodeOrder = append(p.g.NodeOrder, id)
	}
	if len(p.stack) > 0 && n.Subgraph == "" {
		n.Subgraph = p.stack[len(p.stack)-1]
	}
	return n
}

// chain parses `a --> b & c -- text --> d` style statements.
func (p *parser) chain(s stmt) error {
	sc := &scanner{s: s.text, line: s.line}
	prev, err := p.nodeGroup(sc)
	if err != nil {
		return err
	}
	for {
		sc.skipSpace()
		if sc.eof() {
			return nil
		}
		edgeID := sc.edgeID()
		if edgeID != "" {
			p.edgeIDs[edgeID] = true
		}
		lk, err := sc.link()
		if err != nil {
			return err
		}
		sc.skipSpace()
		if sc.eof() {
			return fmt.Errorf("line %d: link without target node", s.line)
		}
		next, err := p.nodeGroup(sc)
		if err != nil {
			return err
		}
		for _, a := range prev {
			for _, b := range next {
				e := *lk
				e.From, e.To = a, b
				e.ID = edgeID
				e.Index = p.edgeCount
				p.edgeCount++
				p.g.Edges = append(p.g.Edges, &e)
			}
		}
		prev = next
	}
}

func (p *parser) nodeGroup(sc *scanner) ([]string, error) {
	var ids []string
	for {
		sc.skipSpace()
		id, err := p.nodeRef(sc)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
		sc.skipSpace()
		if sc.peek() == '&' {
			sc.i++
			continue
		}
		return ids, nil
	}
}

// nodeRef parses one node reference with optional shape/label and class.
func (p *parser) nodeRef(sc *scanner) (string, error) {
	id := sc.ident()
	if id == "" {
		if sc.eof() {
			return "", fmt.Errorf("line %d: expected node", sc.line)
		}
		return "", fmt.Errorf("line %d: unexpected %q", sc.line, sc.rest(12))
	}
	n := p.node(id)
	if sc.hasPrefix("@{") {
		body, err := sc.braced()
		if err != nil {
			return "", err
		}
		applyShapeData(n, body)
	} else if shape, label, ok, err := sc.shape(); err != nil {
		return "", err
	} else if ok {
		n.Shape = shape
		n.Label = label
		n.labelSet = true
	}
	for sc.hasPrefix(":::") {
		sc.i += 3
		cls := sc.ident()
		if cls != "" {
			n.Classes = append(n.Classes, cls)
		}
	}
	return id, nil
}

// applyShapeData handles `@{ shape: rounded, label: "x" }`.
func applyShapeData(n *Node, body string) {
	for k, v := range parseProps(body) {
		switch k {
		case "shape":
			n.Shape = normalizeShape(v)
			n.labelSet = true
		case "label":
			n.Label = diagram.CleanLabel(v)
			n.labelSet = true
		}
	}
}

// parseProps parses a YAML-ish flat map `key: value, key: "v, x"`.
func parseProps(s string) map[string]string {
	out := map[string]string{}
	var parts []string
	var cur strings.Builder
	inQ := false
	for _, r := range s {
		switch {
		case r == '"':
			inQ = !inQ
			cur.WriteRune(r)
		case (r == ',' || r == '\n') && !inQ:
			parts = append(parts, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	parts = append(parts, cur.String())
	for _, part := range parts {
		k, v, ok := strings.Cut(part, ":")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		}
		out[k] = v
	}
	return out
}

// shapeAliases maps Mermaid's v11 shape names (and aliases) to the
// canonical names we draw.
var shapeAliases = map[string]Shape{
	"rect": ShapeRect, "rectangle": ShapeRect, "proc": ShapeRect, "process": ShapeRect, "square": ShapeRect,
	"rounded": ShapeRounded, "event": ShapeRounded,
	"stadium": ShapeStadium, "pill": ShapeStadium, "terminal": ShapeStadium,
	"subproc": ShapeSubroutine, "subprocess": ShapeSubroutine, "subroutine": ShapeSubroutine, "fr-rect": ShapeSubroutine, "framed-rectangle": ShapeSubroutine,
	"cyl": ShapeCylinder, "cylinder": ShapeCylinder, "database": ShapeCylinder, "db": ShapeCylinder,
	"circle": ShapeCircle, "circ": ShapeCircle,
	"dbl-circ": ShapeDoubleCircle, "double-circle": ShapeDoubleCircle,
	"diam": ShapeDiamond, "diamond": ShapeDiamond, "decision": ShapeDiamond, "question": ShapeDiamond,
	"hex": ShapeHexagon, "hexagon": ShapeHexagon, "prepare": ShapeHexagon,
	"lean-r": ShapeLeanRight, "lean-right": ShapeLeanRight, "in-out": ShapeLeanRight,
	"lean-l": ShapeLeanLeft, "lean-left": ShapeLeanLeft, "out-in": ShapeLeanLeft,
	"trap-b": ShapeTrapezoid, "trapezoid-bottom": ShapeTrapezoid, "priority": ShapeTrapezoid, "trapezoid": ShapeTrapezoid,
	"trap-t": ShapeTrapezoidAlt, "trapezoid-top": ShapeTrapezoidAlt, "manual": ShapeTrapezoidAlt, "inv-trapezoid": ShapeTrapezoidAlt,
	"odd": ShapeOdd,
	"doc": "doc", "document": "doc",
	"docs": "docs", "documents": "docs", "st-doc": "docs", "stacked-document": "docs",
	"notch-rect": "notch-rect", "card": "notch-rect", "notched-rectangle": "notch-rect",
	"delay": "delay", "half-rounded-rectangle": "delay",
	"h-cyl": "h-cyl", "das": "h-cyl", "horizontal-cylinder": "h-cyl",
	"lin-cyl": "lin-cyl", "disk": "lin-cyl", "lined-cylinder": "lin-cyl",
	"curv-trap": "curv-trap", "display": "curv-trap", "curved-trapezoid": "curv-trap",
	"div-rect": "div-rect", "div-proc": "div-rect", "divided-rectangle": "div-rect", "divided-process": "div-rect",
	"tri": "tri", "extract": "tri", "triangle": "tri",
	"flip-tri": "flip-tri", "manual-file": "flip-tri", "flipped-triangle": "flip-tri",
	"win-pane": "win-pane", "internal-storage": "win-pane", "window-pane": "win-pane",
	"f-circ": "f-circ", "junction": "f-circ", "filled-circle": "f-circ",
	"sm-circ": "sm-circ", "start": "sm-circ", "small-circle": "sm-circ",
	"fr-circ": "fr-circ", "stop": "fr-circ", "framed-circle": "fr-circ",
	"fork": "fork", "join": "fork",
	"hourglass": "hourglass", "collate": "hourglass",
	"brace": "brace", "comment": "brace", "brace-l": "brace",
	"brace-r": "brace-r", "braces": "braces",
	"text": "text",
	"bolt": "bolt", "com-link": "bolt", "lightning-bolt": "bolt",
	"flag": "flag", "paper-tape": "flag",
	"sl-rect": "sl-rect", "manual-input": "sl-rect", "sloped-rectangle": "sl-rect",
	"st-rect": "st-rect", "procs": "st-rect", "processes": "st-rect", "stacked-rectangle": "st-rect",
	"bow-rect": "bow-rect", "stored-data": "bow-rect", "bow-tie-rectangle": "bow-rect",
	"cross-circ": "cross-circ", "summary": "cross-circ", "crossed-circle": "cross-circ",
	"tag-doc": "tag-doc", "tagged-document": "tag-doc",
	"tag-rect": "tag-rect", "tag-proc": "tag-rect", "tagged-rectangle": "tag-rect", "tagged-process": "tag-rect",
	"lin-rect": "lin-rect", "lin-proc": "lin-rect", "lined-rectangle": "lin-rect", "lined-process": "lin-rect", "shaded-process": "lin-rect",
	"notch-pent": "notch-pent", "loop-limit": "notch-pent", "notched-pentagon": "notch-pent",
	"lin-doc": "lin-doc", "lined-document": "lin-doc",
}

func normalizeShape(s string) Shape {
	s = strings.ToLower(strings.TrimSpace(s))
	if v, ok := shapeAliases[s]; ok {
		return v
	}
	return ShapeRect
}

func (p *parser) finish() {
	g := p.g
	// References to subgraph ids are cluster references, not nodes.
	for id, sg := range g.Subgraphs {
		if n, ok := g.Nodes[id]; ok {
			if n.Style != nil {
				mergeStyle(&sg.Style, n.Style)
			}
			sg.Classes = append(sg.Classes, n.Classes...)
			delete(g.Nodes, id)
			for i, oid := range g.NodeOrder {
				if oid == id {
					g.NodeOrder = append(g.NodeOrder[:i], g.NodeOrder[i+1:]...)
					break
				}
			}
		}
	}
	for _, id := range g.NodeOrder {
		n := g.Nodes[id]
		if n.Subgraph != "" {
			if sg, ok := g.Subgraphs[n.Subgraph]; ok {
				sg.Children = append(sg.Children, id)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// scanner

type scanner struct {
	s    string
	i    int
	line int
}

func (sc *scanner) eof() bool { return sc.i >= len(sc.s) }

func (sc *scanner) peek() byte {
	if sc.i >= len(sc.s) {
		return 0
	}
	return sc.s[sc.i]
}

func (sc *scanner) at(k int) byte {
	if sc.i+k >= len(sc.s) || sc.i+k < 0 {
		return 0
	}
	return sc.s[sc.i+k]
}

func (sc *scanner) hasPrefix(p string) bool { return strings.HasPrefix(sc.s[sc.i:], p) }

func (sc *scanner) rest(n int) string {
	r := sc.s[sc.i:]
	if len(r) > n {
		r = r[:n] + "…"
	}
	return r
}

func (sc *scanner) skipSpace() {
	for sc.i < len(sc.s) && (sc.s[sc.i] == ' ' || sc.s[sc.i] == '\t') {
		sc.i++
	}
}

func isIdentRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// ident reads a node id. Dashes and dots are allowed inside ids as long as
// they don't start a link.
func (sc *scanner) ident() string {
	start := sc.i
	for sc.i < len(sc.s) {
		r, size := decodeRune(sc.s[sc.i:])
		if isIdentRune(r) {
			sc.i += size
			continue
		}
		if (r == '-' || r == '.') && sc.i > start {
			// part of the id if followed by an identifier char (a-b, v1.2)
			nr, _ := decodeRune(sc.s[sc.i+1:])
			if isIdentRune(nr) && (r != '-' || (nr != 'x' && nr != 'o') || !sc.linkAhead()) {
				sc.i += size
				continue
			}
		}
		break
	}
	return sc.s[start:sc.i]
}

// linkAhead reports whether a link token starts at the current position
// (used to disambiguate ids like "a-b" from "a--xb").
func (sc *scanner) linkAhead() bool {
	save := sc.i
	defer func() { sc.i = save }()
	_, err := sc.link()
	return err == nil
}

func decodeRune(s string) (rune, int) {
	if s == "" {
		return 0, 0
	}
	return utf8.DecodeRuneInString(s)
}

// edgeID parses an optional `id@` prefix before a link.
func (sc *scanner) edgeID() string {
	save := sc.i
	id := sc.ident()
	if id != "" && sc.peek() == '@' && strings.ContainsRune("-=.<ox~", rune(sc.at(1))) {
		sc.i++
		return id
	}
	sc.i = save
	return ""
}

// braced reads a `@{ ... }` block and returns its body.
func (sc *scanner) braced() (string, error) {
	sc.i += 2
	start := sc.i
	inQ := false
	for sc.i < len(sc.s) {
		c := sc.s[sc.i]
		if c == '"' {
			inQ = !inQ
		} else if c == '}' && !inQ {
			body := sc.s[start:sc.i]
			sc.i++
			return body, nil
		}
		sc.i++
	}
	return "", fmt.Errorf("line %d: unterminated '@{'", sc.line)
}

type shapeSyntax struct {
	open   string
	closes []string
	shapes []Shape
}

// Order matters: longer openers first.
var shapeSyntaxes = []shapeSyntax{
	{"(((", []string{")))"}, []Shape{ShapeDoubleCircle}},
	{"((", []string{"))"}, []Shape{ShapeCircle}},
	{"([", []string{"])"}, []Shape{ShapeStadium}},
	{"[[", []string{"]]"}, []Shape{ShapeSubroutine}},
	{"[(", []string{")]"}, []Shape{ShapeCylinder}},
	{"[/", []string{"/]", "\\]"}, []Shape{ShapeLeanRight, ShapeTrapezoid}},
	{"[\\", []string{"\\]", "/]"}, []Shape{ShapeLeanLeft, ShapeTrapezoidAlt}},
	{"[", []string{"]"}, []Shape{ShapeRect}},
	{"(", []string{")"}, []Shape{ShapeRounded}},
	{"{{", []string{"}}"}, []Shape{ShapeHexagon}},
	{"{", []string{"}"}, []Shape{ShapeDiamond}},
	{">", []string{"]"}, []Shape{ShapeOdd}},
}

// shape parses an optional shape+label following a node id.
func (sc *scanner) shape() (Shape, string, bool, error) {
	for _, ss := range shapeSyntaxes {
		if !sc.hasPrefix(ss.open) {
			continue
		}
		// `A-->B` : '>' after an id is only a shape when not part of a link
		if ss.open == ">" && sc.i > 0 && (sc.s[sc.i-1] == '-' || sc.s[sc.i-1] == '=') {
			return "", "", false, nil
		}
		start := sc.i
		sc.i += len(ss.open)
		body := sc.s[sc.i:]
		// quoted label
		trimmed := strings.TrimLeft(body, " ")
		if strings.HasPrefix(trimmed, "\"") {
			q := sc.i + (len(body) - len(trimmed))
			end := strings.IndexByte(sc.s[q+1:], '"')
			if end >= 0 {
				after := sc.s[q+1+end+1:]
				afterT := strings.TrimLeft(after, " ")
				for k, c := range ss.closes {
					if strings.HasPrefix(afterT, c) {
						label := sc.s[q : q+1+end+1]
						sc.i = q + 1 + end + 1 + (len(after) - len(afterT)) + len(c)
						return ss.shapes[k], diagram.CleanLabel(label), true, nil
					}
				}
			}
		}
		// unquoted: earliest closer
		best, bestK := -1, 0
		for k, c := range ss.closes {
			if j := indexClose(body, c, ss.open); j >= 0 && (best < 0 || j < best) {
				best, bestK = j, k
			}
		}
		if best < 0 {
			sc.i = start
			return "", "", false, fmt.Errorf("line %d: unclosed node shape %q", sc.line, ss.open)
		}
		label := body[:best]
		sc.i += best + len(ss.closes[bestK])
		return ss.shapes[bestK], diagram.CleanLabel(label), true, nil
	}
	return "", "", false, nil
}

// indexClose finds closer c in body, allowing balanced nested brackets of
// the simple single-char kinds.
func indexClose(body, c, open string) int {
	if len(open) != 1 || len(c) != 1 || open == ">" {
		return strings.Index(body, c)
	}
	depth := 0
	for i := 0; i < len(body); i++ {
		switch body[i] {
		case open[0]:
			depth++
		case c[0]:
			if depth == 0 {
				return i
			}
			depth--
		}
	}
	return -1
}

var (
	// label end tokens for "-- text -->", "== text ==>", "-. text .->"
	normalEndRe = regexp.MustCompile(`--+[-xo>]`)
	thickEndRe  = regexp.MustCompile(`==+[=xo>]`)
	dottedEndRe = regexp.MustCompile(`\.+-[xo>]?`)
)

func isMarkerEnd(c byte, next byte) bool {
	switch c {
	case '>':
		return true
	case 'x', 'o':
		// only a marker if not followed by an identifier character
		return next != '_' && (next < 'a' || next > 'z') && (next < 'A' || next > 'Z') && (next < '0' || next > '9')
	}
	return false
}

// link parses a link token (with optional inline or pipe label).
func (sc *scanner) link() (*Edge, error) {
	start := sc.i
	e := &Edge{Length: 1}
	if c := sc.peek(); (c == 'x' || c == 'o' || c == '<') && strings.ContainsRune("-=.", rune(sc.at(1))) {
		if c == '<' {
			e.Start = MarkerArrow
		} else {
			e.Start = Marker(c)
		}
		sc.i++
	}
	switch sc.peek() {
	case '~':
		n := sc.count('~')
		if n < 3 {
			break
		}
		e.Kind = LinkInvisible
		e.Length = n - 2
		sc.pipeLabel(e)
		return e, nil
	case '=':
		n := sc.count('=')
		if n >= 2 {
			e.Kind = LinkThick
			if c := sc.peek(); c != 0 && isMarkerEnd(c, sc.at(1)) {
				e.End = Marker(c)
				sc.i++
				e.Length = max(n-1, 1)
				sc.pipeLabel(e)
				return e, nil
			}
			if n >= 3 {
				e.Length = n - 2
				sc.pipeLabel(e)
				return e, nil
			}
			// text form: == text ==>
			return sc.textLink(e, thickEndRe, start)
		}
	case '-':
		if sc.at(1) == '.' {
			sc.i++
			dots := sc.count('.')
			if sc.peek() == '-' {
				sc.i++
				e.Kind = LinkDotted
				e.Length = dots
				if c := sc.peek(); c != 0 && isMarkerEnd(c, sc.at(1)) {
					e.End = Marker(c)
					sc.i++
				}
				sc.pipeLabel(e)
				return e, nil
			}
			if dots == 1 {
				e.Kind = LinkDotted
				return sc.textLink(e, dottedEndRe, start)
			}
			break
		}
		n := sc.count('-')
		if n >= 2 {
			if c := sc.peek(); c != 0 && isMarkerEnd(c, sc.at(1)) {
				e.End = Marker(c)
				sc.i++
				e.Length = max(n-1, 1)
				sc.pipeLabel(e)
				return e, nil
			}
			if n >= 3 {
				e.Length = n - 2
				sc.pipeLabel(e)
				return e, nil
			}
			return sc.textLink(e, normalEndRe, start)
		}
	}
	sc.i = start
	return nil, fmt.Errorf("line %d: expected a link (e.g. -->) but found %q", sc.line, sc.rest(12))
}

// textLink parses the "-- text -->" form after the opening token.
func (sc *scanner) textLink(e *Edge, endRe *regexp.Regexp, start int) (*Edge, error) {
	rest := sc.s[sc.i:]
	loc := endRe.FindStringIndex(rest)
	if loc == nil {
		sc.i = start
		return nil, fmt.Errorf("line %d: unterminated link label %q", sc.line, sc.rest(20))
	}
	e.Label = diagram.CleanLabel(rest[:loc[0]])
	tok := rest[loc[0]:loc[1]]
	sc.i += loc[1]
	last := tok[len(tok)-1]
	switch e.Kind {
	case LinkDotted:
		e.Length = strings.Count(tok, ".")
		if last == 'x' || last == 'o' || last == '>' {
			e.End = Marker(last)
		}
	case LinkThick:
		if last == '=' {
			e.Length = strings.Count(tok, "=") - 2
		} else {
			e.End = Marker(last)
			e.Length = strings.Count(tok, "=") - 1
		}
	default:
		if last == '-' {
			e.Length = strings.Count(tok, "-") - 2
		} else {
			e.End = Marker(last)
			e.Length = strings.Count(tok, "-") - 1
		}
	}
	if e.Length < 1 {
		e.Length = 1
	}
	return e, nil
}

func (sc *scanner) count(c byte) int {
	n := 0
	for sc.i < len(sc.s) && sc.s[sc.i] == c {
		sc.i++
		n++
	}
	return n
}

// pipeLabel parses an optional |label| after a link.
func (sc *scanner) pipeLabel(e *Edge) {
	save := sc.i
	sc.skipSpace()
	if sc.peek() != '|' {
		sc.i = save
		return
	}
	end := strings.IndexByte(sc.s[sc.i+1:], '|')
	if end < 0 {
		sc.i = save
		return
	}
	e.Label = diagram.CleanLabel(sc.s[sc.i+1 : sc.i+1+end])
	sc.i += end + 2
}

// ---------------------------------------------------------------------------
// helpers

func firstWord(s string) (string, string) {
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

// parseStyle parses "fill:#f9f,stroke:#333,stroke-width:4px".
func parseStyle(s string) map[string]string {
	out := map[string]string{}
	depth := 0
	var parts []string
	var cur strings.Builder
	for _, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',', ';':
			if depth == 0 {
				parts = append(parts, cur.String())
				cur.Reset()
				continue
			}
		}
		cur.WriteRune(r)
	}
	parts = append(parts, cur.String())
	for _, p := range parts {
		k, v, ok := strings.Cut(p, ":")
		if !ok {
			continue
		}
		k = strings.ToLower(strings.TrimSpace(k))
		v = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(v), "!important"))
		if k != "" {
			out[k] = v
		}
	}
	return out
}

func mergeStyle(dst *map[string]string, src map[string]string) {
	if *dst == nil {
		*dst = map[string]string{}
	}
	maps.Copy((*dst), src)
}

func mergeLinkStyle(m map[int]map[string]string, i int, st map[string]string) {
	cur := m[i]
	mergeStyle(&cur, st)
	m[i] = cur
}
