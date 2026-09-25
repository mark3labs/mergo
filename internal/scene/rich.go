package scene

import "strings"

// Inline style markers. Labels may embed these private-use runes to switch
// bold/italic on and off within a line of text; MeasureText, WrapText and the
// rasterizer honor them. Markers never carry across '\n': every line starts
// in the Text's base font, so producers must re-open styles on each line
// (StyleLines does that).
const (
	BoldOn    = '\uE000'
	BoldOff   = '\uE001'
	ItalicOn  = '\uE002'
	ItalicOff = '\uE003'
)

const markerChars = "\uE000\uE001\uE002\uE003"

// HasStyle reports whether s contains inline style markers.
func HasStyle(s string) bool { return strings.ContainsAny(s, markerChars) }

// StripStyle removes inline style markers from s.
func StripStyle(s string) string {
	if !HasStyle(s) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if isMarker(r) {
			return -1
		}
		return r
	}, s)
}

func isMarker(r rune) bool { return r >= BoldOn && r <= ItalicOff }

// styleState is the nesting depth of bold and italic spans.
type styleState struct{ bold, italic int }

func (st *styleState) apply(r rune) {
	switch r {
	case BoldOn:
		st.bold++
	case BoldOff:
		st.bold = max(st.bold-1, 0)
	case ItalicOn:
		st.italic++
	case ItalicOff:
		st.italic = max(st.italic-1, 0)
	}
}

func (st styleState) openers() string {
	var b strings.Builder
	if st.bold > 0 {
		b.WriteRune(BoldOn)
	}
	if st.italic > 0 {
		b.WriteRune(ItalicOn)
	}
	return b.String()
}

func (st styleState) closers() string {
	var b strings.Builder
	if st.italic > 0 {
		b.WriteRune(ItalicOff)
	}
	if st.bold > 0 {
		b.WriteRune(BoldOff)
	}
	return b.String()
}

func (st styleState) font(f Font) Font {
	f.Bold = f.Bold || st.bold > 0
	f.Italic = f.Italic || st.italic > 0
	return f
}

// StyleLines rewrites a multi-line string whose style spans may cross line
// breaks so that every line is self-contained: open spans are closed at the
// end of a line and re-opened at the start of the next. Lines that contain
// only markers become empty.
func StyleLines(s string) string {
	if !HasStyle(s) {
		return s
	}
	lines := strings.Split(s, "\n")
	var st styleState
	for i, l := range lines {
		pre := st.openers()
		for _, r := range l {
			st.apply(r)
		}
		l = pre + l + st.closers()
		if strings.TrimSpace(StripStyle(l)) == "" {
			l = ""
		}
		lines[i] = l
	}
	return strings.Join(lines, "\n")
}

// textRun is a piece of a line drawn with a single font.
type textRun struct {
	s string
	f Font
	w float64 // advance width, filled in by the rasterizer
}

// runs splits a single line into runs of uniform style, starting from base.
func runs(s string, base Font) []textRun {
	if !HasStyle(s) {
		return []textRun{{s: s, f: base}}
	}
	var out []textRun
	var st styleState
	start := 0
	for i, r := range s {
		if !isMarker(r) {
			continue
		}
		if i > start {
			out = append(out, textRun{s: s[start:i], f: st.font(base)})
		}
		st.apply(r)
		start = i + len(string(r))
	}
	if start < len(s) {
		out = append(out, textRun{s: s[start:], f: st.font(base)})
	}
	return out
}
