// Package diagram contains the diagram type registry, source preprocessing
// (front matter, directives, comments) and helpers shared by the individual
// diagram implementations.
package diagram

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

// Config is passed to diagram renderers.
type Config struct {
	Theme *theme.Theme
	// Title from front matter (or empty).
	Title string
	// Raw merged configuration from front matter `config:` and
	// `%%{init: ...}%%` directives.
	Raw map[string]any
}

// Section returns the configuration sub map for a diagram type (e.g.
// "flowchart", "sequence").
func (c *Config) Section(name string) map[string]any {
	if c == nil || c.Raw == nil {
		return nil
	}
	if m, ok := c.Raw[name].(map[string]any); ok {
		return m
	}
	return nil
}

// Float reads a numeric option from a section with a default.
func (c *Config) Float(section, key string, def float64) float64 {
	m := c.Section(section)
	if m == nil {
		return def
	}
	switch v := m[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

// String reads a string option from a section with a default.
func (c *Config) String(section, key, def string) string {
	m := c.Section(section)
	if m == nil {
		return def
	}
	if s, ok := m[key].(string); ok {
		return s
	}
	return def
}

// Bool reads a boolean option from a section with a default.
func (c *Config) Bool(section, key string, def bool) bool {
	m := c.Section(section)
	if m == nil {
		return def
	}
	if b, ok := m[key].(bool); ok {
		return b
	}
	return def
}

// RenderFunc renders diagram source (already preprocessed: front matter,
// directives and comment lines removed) into a scene.
type RenderFunc func(src string, cfg *Config) (*scene.Scene, error)

// Type describes a diagram type.
type Type struct {
	// Name, e.g. "flowchart".
	Name string
	// Detect reports whether the first significant line (trimmed) declares
	// this diagram type.
	Detect func(header string) bool
	Render RenderFunc
}

var (
	regMu    sync.RWMutex
	registry []Type
)

// Register adds a diagram type.
func Register(t Type) {
	regMu.Lock()
	defer regMu.Unlock()
	registry = append(registry, t)
}

// Types returns the names of all registered diagram types.
func Types() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	var out []string
	for _, t := range registry {
		out = append(out, t.Name)
	}
	sort.Strings(out)
	return out
}

// Keyword returns a Detect function matching a header that starts with any
// of the given keywords (followed by end of line, whitespace or a colon).
func Keyword(words ...string) func(string) bool {
	return func(h string) bool {
		for _, w := range words {
			if strings.HasPrefix(h, w) {
				rest := h[len(w):]
				if rest == "" || rest[0] == ' ' || rest[0] == '\t' || rest[0] == ':' || rest[0] == ';' {
					return true
				}
			}
		}
		return false
	}
}

// Document is preprocessed diagram source.
type Document struct {
	Type   string
	Source string // body without front matter/directives/comments
	Header string // first significant line
	Title  string
	Raw    map[string]any
}

var (
	directiveRe = regexp.MustCompile(`(?s)%%\{(.*?)\}%%`)
)

// Preprocess strips front matter, directives, comments and accessibility
// statements and detects the diagram type.
func Preprocess(src string) (*Document, error) {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.TrimLeft(src, "\ufeff")
	doc := &Document{Raw: map[string]any{}}

	// Front matter
	// Removed regions are replaced by blank lines so that line numbers in
	// error messages still refer to the original source.
	trimmed := strings.TrimLeft(src, " \t\n")
	if strings.HasPrefix(trimmed, "---") {
		lead := src[:len(src)-len(trimmed)]
		rest := trimmed[3:]
		if i := strings.Index(rest, "\n---"); i >= 0 {
			fm := rest[:i]
			after := rest[i+4:]
			// drop the remainder of the closing fence line
			if j := strings.IndexByte(after, '\n'); j >= 0 {
				after = after[j:]
			} else {
				after = ""
			}
			removed := lead + "---" + rest[:i+4]
			src = strings.Repeat("\n", strings.Count(removed, "\n")) + after
			var m map[string]any
			if err := yaml.Unmarshal([]byte(fm), &m); err == nil {
				if t, ok := m["title"].(string); ok {
					doc.Title = t
				}
				if c, ok := m["config"].(map[string]any); ok {
					mergeMaps(doc.Raw, normalize(c).(map[string]any))
				}
			}
		}
	}

	// Directives %%{init: {...}}%%
	src = directiveRe.ReplaceAllStringFunc(src, func(d string) string {
		m := directiveRe.FindStringSubmatch(d)
		body := strings.TrimSpace(m[1])
		blank := strings.Repeat("\n", strings.Count(d, "\n"))
		name, arg, found := strings.Cut(body, ":")
		if !found {
			return blank
		}
		name = strings.TrimSpace(strings.ToLower(name))
		if name != "init" && name != "initialize" {
			return blank
		}
		if v := parseLooseJSON(arg); v != nil {
			mergeMaps(doc.Raw, v)
		}
		return blank
	})

	// Remove comments and accessibility statements.
	var lines []string
	inAccDescr := false
	for line := range strings.SplitSeq(src, "\n") {
		t := strings.TrimSpace(line)
		if inAccDescr {
			if strings.Contains(t, "}") {
				inAccDescr = false
			}
			lines = append(lines, "")
			continue
		}
		if strings.HasPrefix(t, "%%") {
			lines = append(lines, "")
			continue
		}
		if strings.HasPrefix(t, "accTitle") || strings.HasPrefix(t, "accDescr") {
			if strings.HasPrefix(t, "accDescr") && strings.Contains(t, "{") && !strings.Contains(t, "}") {
				inAccDescr = true
			}
			lines = append(lines, "")
			continue
		}
		lines = append(lines, line)
	}
	doc.Source = strings.Join(lines, "\n")
	for _, l := range lines {
		if t := strings.TrimSpace(l); t != "" {
			doc.Header = t
			break
		}
	}
	if doc.Header == "" {
		return nil, fmt.Errorf("empty diagram")
	}
	regMu.RLock()
	defer regMu.RUnlock()
	for _, t := range registry {
		if t.Detect(doc.Header) {
			doc.Type = t.Name
			return doc, nil
		}
	}
	kw := strings.Fields(doc.Header)[0]
	return doc, fmt.Errorf("unsupported or unknown diagram type %q", kw)
}

// Render preprocesses and renders mermaid source with the given theme.
func Render(src string, th *theme.Theme) (*scene.Scene, error) {
	doc, err := Preprocess(src)
	if err != nil {
		return nil, err
	}
	th = th.Clone()
	if tn, ok := doc.Raw["theme"].(string); ok {
		if t2, err := theme.Get(tn); err == nil && tn != "default" {
			th = t2
		}
	}
	if tv, ok := doc.Raw["themeVariables"].(map[string]any); ok {
		ApplyThemeVariables(th, tv)
	}
	cfg := &Config{Theme: th, Title: doc.Title, Raw: doc.Raw}
	regMu.RLock()
	var fn RenderFunc
	for _, t := range registry {
		if t.Name == doc.Type {
			fn = t.Render
		}
	}
	regMu.RUnlock()
	sc, err := safeRender(fn, doc.Source, cfg)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", doc.Type, err)
	}
	return sc, nil
}

func safeRender(fn RenderFunc, src string, cfg *Config) (sc *scene.Scene, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("internal error: %v", r)
		}
	}()
	return fn(src, cfg)
}

// ApplyThemeVariables maps Mermaid themeVariables onto a theme.
func ApplyThemeVariables(th *theme.Theme, vars map[string]any) {
	for k, v := range vars {
		s, ok := v.(string)
		if !ok {
			if f, ok := v.(float64); ok && strings.EqualFold(k, "fontSize") {
				th.FontSize = f
			}
			continue
		}
		if strings.EqualFold(k, "fontSize") {
			if f, err := strconv.ParseFloat(strings.TrimSuffix(s, "px"), 64); err == nil {
				th.FontSize = f
			}
			continue
		}
		if strings.EqualFold(k, "darkMode") {
			th.Dark = s == "true"
			continue
		}
		c, err := theme.ParseColor(s)
		if err != nil {
			continue
		}
		switch k {
		case "background":
			th.Background = c
		case "primaryColor", "mainBkg", "nodeBkg":
			th.PrimaryColor = c
			th.ActorBkg = c
			th.LabelBoxBkg = c
		case "primaryTextColor", "nodeTextColor":
			th.PrimaryTextColor = c
			th.ActorText = c
		case "primaryBorderColor", "nodeBorder":
			th.PrimaryBorderColor = c
			th.ActorBorder = c
			th.LabelBoxBorder = c
		case "secondaryColor":
			th.SecondaryColor = c
		case "secondaryTextColor":
			th.SecondaryTextColor = c
		case "secondaryBorderColor":
			th.SecondaryBorder = c
		case "tertiaryColor":
			th.TertiaryColor = c
		case "tertiaryTextColor":
			th.TertiaryTextColor = c
		case "tertiaryBorderColor":
			th.TertiaryBorder = c
		case "textColor":
			th.TextColor = c
			th.SignalText = c
		case "lineColor", "defaultLinkColor":
			th.LineColor = c
			th.SignalColor = c
		case "edgeLabelBackground":
			th.EdgeLabelBg = c
		case "clusterBkg":
			th.ClusterBkg = c
		case "clusterBorder":
			th.ClusterBorder = c
		case "titleColor":
			th.TitleColor = c
		case "noteBkgColor":
			th.NoteBkg = c
		case "noteBorderColor":
			th.NoteBorder = c
		case "noteTextColor":
			th.NoteText = c
		case "actorBkg":
			th.ActorBkg = c
		case "actorBorder":
			th.ActorBorder = c
		case "actorTextColor":
			th.ActorText = c
		case "actorLineColor":
			th.ActorLine = c
		case "signalColor":
			th.SignalColor = c
		case "signalTextColor":
			th.SignalText = c
		case "labelBoxBkgColor":
			th.LabelBoxBkg = c
		case "labelBoxBorderColor":
			th.LabelBoxBorder = c
		case "labelTextColor":
			th.LabelText = c
		case "loopTextColor":
			th.LoopText = c
		case "activationBkgColor":
			th.ActivationBkg = c
		case "activationBorderColor":
			th.ActivationBorder = c
		case "sequenceNumberColor":
			th.SequenceNumberText = c
		case "taskBkgColor":
			th.TaskBkg = c
		case "taskBorderColor":
			th.TaskBorder = c
		case "taskTextColor", "taskTextLightColor":
			th.TaskText = c
		case "activeTaskBkgColor":
			th.ActiveTask = c
		case "activeTaskBorderColor":
			th.ActiveBorder = c
		case "doneTaskBkgColor":
			th.DoneTask = c
		case "doneTaskBorderColor":
			th.DoneBorder = c
		case "critBkgColor":
			th.CritTask = c
		case "critBorderColor":
			th.CritBorder = c
		case "gridColor":
			th.GridColor = c
		case "todayLineColor":
			th.TodayLine = c
		default:
			// pie1..pie12, cScale0..11, git0..7
			for _, pfx := range []string{"pie", "cScale", "git"} {
				if strings.HasPrefix(k, pfx) {
					if n, err := strconv.Atoi(k[len(pfx):]); err == nil {
						idx := n
						if pfx == "pie" {
							idx = n - 1
						}
						if idx >= 0 && idx < 32 {
							for len(th.Palette) <= idx {
								th.Palette = append(th.Palette, th.PrimaryColor)
							}
							th.Palette[idx] = c
							th.PaletteText = nil
							for _, pc := range th.Palette {
								th.PaletteText = append(th.PaletteText, theme.ContrastText(pc))
							}
						}
					}
				}
			}
		}
	}
}

// parseLooseJSON parses the JSON-ish objects Mermaid accepts in directives
// (single quotes, unquoted keys, trailing commas).
func parseLooseJSON(s string) map[string]any {
	s = strings.TrimSpace(s)
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err == nil {
		return normalize(m).(map[string]any)
	}
	// YAML is a superset of JSON and handles single quotes and bare keys.
	if err := yaml.Unmarshal([]byte(s), &m); err == nil {
		return normalize(m).(map[string]any)
	}
	fixed := strings.ReplaceAll(s, "'", "\"")
	if err := json.Unmarshal([]byte(fixed), &m); err == nil {
		return normalize(m).(map[string]any)
	}
	return nil
}

// normalize converts yaml maps and numeric types into JSON-like values.
func normalize(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, vv := range t {
			out[k] = normalize(vv)
		}
		return out
	case map[any]any:
		out := map[string]any{}
		for k, vv := range t {
			out[fmt.Sprint(k)] = normalize(vv)
		}
		return out
	case []any:
		for i := range t {
			t[i] = normalize(t[i])
		}
		return t
	case int:
		return float64(t)
	case int64:
		return float64(t)
	}
	return v
}

func mergeMaps(dst, src map[string]any) {
	for k, v := range src {
		if sm, ok := v.(map[string]any); ok {
			if dm, ok := dst[k].(map[string]any); ok {
				mergeMaps(dm, sm)
				continue
			}
		}
		dst[k] = v
	}
}

// ---------------------------------------------------------------------------
// Label helpers

var (
	brRe     = regexp.MustCompile(`(?i)<br\s*/?>`)
	tagRe    = regexp.MustCompile(`</?[a-zA-Z][^>]*>`)
	entityRe = regexp.MustCompile(`#([a-zA-Z]+|\d+);`)
	faRe     = regexp.MustCompile(`\bfa[bklrs]?:fa-[\w-]+\s*`)
)

// CleanLabel normalizes a label: strips surrounding quotes and markdown
// backticks, turns <br> into newlines, decodes Mermaid (#quot;) and HTML
// entities, drops other HTML tags and font-awesome icons, and converts the
// literal sequence "\n" into a newline.
func CleanLabel(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	if len(s) >= 2 && s[0] == '`' && s[len(s)-1] == '`' {
		s = s[1 : len(s)-1]
		s = strings.ReplaceAll(s, "**", "")
		s = strings.ReplaceAll(s, "__", "")
	}
	s = brRe.ReplaceAllString(s, "\n")
	s = tagRe.ReplaceAllString(s, "")
	s = faRe.ReplaceAllString(s, "")
	s = entityRe.ReplaceAllStringFunc(s, func(e string) string {
		name := e[1 : len(e)-1]
		if n, err := strconv.Atoi(name); err == nil {
			return string(rune(n))
		}
		return html.UnescapeString("&" + name + ";")
	})
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, `\n`, "\n")
	// trim each line
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	return strings.Join(lines, "\n")
}

// Lines splits source into trimmed, non-empty lines. Semicolons are NOT
// treated as separators (callers that need that should split themselves).
func Lines(src string) []string {
	var out []string
	for l := range strings.SplitSeq(src, "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Scene helpers

// Pad is the default outer padding of diagrams.
const Pad = 20

// Font returns the regular font at the theme's size scaled by f.
func Font(th *theme.Theme, f float64) scene.Font {
	return scene.Font{Size: th.FontSize * f}
}

// BoldFont returns a bold font at the theme's size scaled by f.
func BoldFont(th *theme.Theme, f float64) scene.Font {
	return scene.Font{Size: th.FontSize * f, Bold: true}
}

// NewScene creates a scene configured from the theme.
func NewScene(th *theme.Theme) *scene.Scene {
	sc := scene.New(th.Background)
	sc.ShadowColor = th.ShadowColor
	return sc
}

// Finish adds an optional title above the content and fits the scene.
func Finish(sc *scene.Scene, th *theme.Theme, title string) *scene.Scene {
	title = CleanLabel(title)
	if title != "" {
		b := sc.Bounds()
		f := scene.Font{Size: th.FontSize * 1.125, Bold: true}
		_, h := scene.MeasureBlock(title, f, 0)
		sc.Add(scene.NewText(b.X+b.W/2, b.Y-16-h/2, title, f, th.TitleColor, scene.AnchorMiddle, scene.VAlignMiddle))
	}
	sc.Fit(Pad)
	return sc
}
