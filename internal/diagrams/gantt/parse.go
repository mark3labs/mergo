package gantt

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mergo/internal/diagram"
)

// Task is a gantt task (or milestone / vertical marker).
type Task struct {
	Name      string
	ID        string
	Section   int
	Start     time.Time
	End       time.Time
	Done      bool
	Active    bool
	Crit      bool
	Milestone bool
	Vert      bool
	Line      int
	order     int

	rawStart, rawEnd string
	resolved         bool
	resolving        bool
}

// Chart is a parsed gantt chart.
type Chart struct {
	Title          string
	DateFormat     string
	AxisFormat     string
	TickInterval   string
	Excludes       []string
	Includes       []string
	WeekendFriday  bool
	TodayMarker    string
	Inclusive      bool
	TopAxis        bool
	Compact        bool
	Sections       []string
	Tasks          []*Task
	byID           map[string]*Task
	excludeDates   map[string]bool
	excludeWeekday map[time.Weekday]bool
	includeDates   map[string]bool
}

var (
	tagNames = map[string]bool{"active": true, "done": true, "crit": true, "milestone": true, "vert": true}
	durRe    = regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*(ms|s|m|h|d|w|M|y)$`)
	idRe     = regexp.MustCompile(`^[A-Za-z_][\w-]*$`)
	tickRe   = regexp.MustCompile(`^([1-9][0-9]*)(millisecond|second|minute|hour|day|week|month)$`)
)

// Parse parses gantt source. now is used for tasks without any date.
func Parse(src string, now time.Time) (*Chart, error) {
	c := &Chart{DateFormat: "YYYY-MM-DD", byID: map[string]*Task{}}
	header := false
	section := -1
	for i, raw := range strings.Split(src, "\n") {
		ln := i + 1
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		if !header {
			if !strings.HasPrefix(line, "gantt") {
				return nil, fmt.Errorf("line %d: expected 'gantt'", ln)
			}
			header = true
			continue
		}
		kw, rest := cutWord(line)
		switch strings.ToLower(kw) {
		case "title":
			c.Title = diagram.CleanLabel(rest)
			continue
		case "dateformat":
			c.DateFormat = rest
			continue
		case "axisformat":
			c.AxisFormat = rest
			continue
		case "tickinterval":
			if !tickRe.MatchString(rest) {
				return nil, fmt.Errorf("line %d: invalid tickInterval %q", ln, rest)
			}
			c.TickInterval = rest
			continue
		case "excludes":
			c.Excludes = append(c.Excludes, splitList(rest)...)
			continue
		case "includes":
			c.Includes = append(c.Includes, splitList(rest)...)
			continue
		case "weekend":
			c.WeekendFriday = strings.EqualFold(strings.TrimSpace(rest), "friday")
			continue
		case "todaymarker":
			c.TodayMarker = rest
			continue
		case "inclusiveenddates":
			c.Inclusive = true
			continue
		case "topaxis":
			c.TopAxis = true
			continue
		case "displaymode":
			c.Compact = strings.EqualFold(strings.TrimSpace(rest), "compact")
			continue
		case "section":
			c.Sections = append(c.Sections, diagram.CleanLabel(rest))
			section = len(c.Sections) - 1
			continue
		case "click", "accTitle", "accTitle:", "accDescr", "accDescr:":
			continue
		}
		name, data, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("line %d: expected 'Task name : data'", ln)
		}
		if section < 0 {
			c.Sections = append(c.Sections, "")
			section = 0
		}
		t := &Task{Name: diagram.CleanLabel(name), Section: section, Line: ln, order: len(c.Tasks)}
		items := splitList(data)
		for len(items) > 0 && tagNames[items[0]] {
			switch items[0] {
			case "active":
				t.Active = true
			case "done":
				t.Done = true
			case "crit":
				t.Crit = true
			case "milestone":
				t.Milestone = true
			case "vert":
				t.Vert = true
			}
			items = items[1:]
		}
		switch len(items) {
		case 0:
			return nil, fmt.Errorf("line %d: task %q has no duration or end", ln, t.Name)
		case 1:
			t.rawEnd = items[0]
		case 2:
			t.rawStart, t.rawEnd = items[0], items[1]
			// lenient: "id, 3d" (an id instead of a start date)
			if !strings.HasPrefix(strings.ToLower(items[0]), "after ") {
				if _, err := c.parseDate(items[0]); err != nil && idRe.MatchString(items[0]) {
					t.ID, t.rawStart = items[0], ""
				}
			}
		default:
			t.ID, t.rawStart, t.rawEnd = items[0], items[1], items[2]
		}
		if t.ID == "" {
			t.ID = fmt.Sprintf("task%d", len(c.Tasks)+1)
		}
		c.byID[t.ID] = t
		c.Tasks = append(c.Tasks, t)
	}
	c.prepareExcludes()
	for _, t := range c.Tasks {
		if err := c.resolve(t, now); err != nil {
			return nil, err
		}
	}
	return c, nil
}

func splitList(s string) []string {
	var out []string
	for p := range strings.SplitSeq(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func cutWord(s string) (string, string) {
	s = strings.TrimSpace(s)
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimSpace(s[i+1:])
}

var weekdays = map[string]time.Weekday{
	"sunday": time.Sunday, "monday": time.Monday, "tuesday": time.Tuesday, "wednesday": time.Wednesday,
	"thursday": time.Thursday, "friday": time.Friday, "saturday": time.Saturday,
}

func (c *Chart) prepareExcludes() {
	c.excludeDates = map[string]bool{}
	c.excludeWeekday = map[time.Weekday]bool{}
	c.includeDates = map[string]bool{}
	for _, e := range c.Excludes {
		l := strings.ToLower(e)
		if l == "weekends" {
			if c.WeekendFriday {
				c.excludeWeekday[time.Friday] = true
				c.excludeWeekday[time.Saturday] = true
			} else {
				c.excludeWeekday[time.Saturday] = true
				c.excludeWeekday[time.Sunday] = true
			}
			continue
		}
		if wd, ok := weekdays[l]; ok {
			c.excludeWeekday[wd] = true
			continue
		}
		if t, err := c.parseDate(e); err == nil {
			c.excludeDates[t.Format("2006-01-02")] = true
		}
	}
	for _, e := range c.Includes {
		if t, err := c.parseDate(e); err == nil {
			c.includeDates[t.Format("2006-01-02")] = true
		}
	}
}

// Excluded reports whether a day is excluded.
func (c *Chart) Excluded(t time.Time) bool {
	key := t.Format("2006-01-02")
	if c.includeDates[key] {
		return false
	}
	return c.excludeDates[key] || c.excludeWeekday[t.Weekday()]
}

func (c *Chart) hasExcludes() bool { return len(c.excludeDates) > 0 || len(c.excludeWeekday) > 0 }

func (c *Chart) resolve(t *Task, now time.Time) error {
	if t.resolved {
		return nil
	}
	if t.resolving {
		return fmt.Errorf("line %d: circular dependency involving task %q", t.Line, t.ID)
	}
	t.resolving = true
	defer func() { t.resolving = false }()

	// start
	switch {
	case t.rawStart == "":
		if t.order > 0 {
			prev := c.Tasks[t.order-1]
			if err := c.resolve(prev, now); err != nil {
				return err
			}
			t.Start = prev.End
		} else {
			t.Start = startOfDay(now)
		}
	case strings.HasPrefix(strings.ToLower(t.rawStart), "after "):
		ids := strings.Fields(t.rawStart[6:])
		var latest time.Time
		found := false
		for _, id := range ids {
			dep, ok := c.byID[id]
			if !ok {
				continue
			}
			if err := c.resolve(dep, now); err != nil {
				return err
			}
			if !found || dep.End.After(latest) {
				latest = dep.End
				found = true
			}
		}
		if !found {
			latest = startOfDay(now)
		}
		t.Start = latest
	default:
		st, err := c.parseDate(t.rawStart)
		if err != nil {
			return fmt.Errorf("line %d: invalid start date %q for format %q", t.Line, t.rawStart, c.DateFormat)
		}
		t.Start = st
	}

	// end
	raw := t.rawEnd
	switch {
	case strings.HasPrefix(strings.ToLower(raw), "until "):
		ids := strings.Fields(raw[6:])
		var earliest time.Time
		found := false
		for _, id := range ids {
			dep, ok := c.byID[id]
			if !ok {
				continue
			}
			if err := c.resolve(dep, now); err != nil {
				return err
			}
			if !found || dep.Start.Before(earliest) {
				earliest = dep.Start
				found = true
			}
		}
		if !found {
			return fmt.Errorf("line %d: unknown task in %q", t.Line, raw)
		}
		t.End = earliest
	default:
		if d, ok := parseDuration(raw, t.Start); ok {
			t.End = d
			if c.hasExcludes() && !t.Milestone {
				t.End = c.skipExcluded(t.Start, t.End)
			}
		} else if e, err := c.parseDate(raw); err == nil {
			t.End = e
			if c.Inclusive && isDateOnly(c.DateFormat) {
				t.End = t.End.AddDate(0, 0, 1)
			}
		} else {
			return fmt.Errorf("line %d: invalid end date or duration %q", t.Line, raw)
		}
	}
	if t.End.Before(t.Start) {
		t.End = t.Start
	}
	t.resolved = true
	return nil
}

// skipExcluded extends [start, end) so that excluded days don't count.
func (c *Chart) skipExcluded(start, end time.Time) time.Time {
	cur := start
	for i := 0; cur.Before(end) && i < 20000; i++ {
		if c.Excluded(cur) {
			end = end.AddDate(0, 0, 1)
		}
		cur = cur.AddDate(0, 0, 1)
	}
	// don't end right after an excluded stretch that starts at end
	return end
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func isDateOnly(f string) bool {
	return !strings.ContainsAny(f, "HhmsSAaXx")
}

func parseDuration(s string, from time.Time) (time.Time, bool) {
	m := durRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return time.Time{}, false
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	switch m[2] {
	case "ms":
		return from.Add(time.Duration(v * float64(time.Millisecond))), true
	case "s":
		return from.Add(time.Duration(v * float64(time.Second))), true
	case "m":
		return from.Add(time.Duration(v * float64(time.Minute))), true
	case "h":
		return from.Add(time.Duration(v * float64(time.Hour))), true
	case "d":
		whole := math.Floor(v)
		return from.AddDate(0, 0, int(whole)).Add(time.Duration((v - whole) * 24 * float64(time.Hour))), true
	case "w":
		days := v * 7
		whole := math.Floor(days)
		return from.AddDate(0, 0, int(whole)).Add(time.Duration((days - whole) * 24 * float64(time.Hour))), true
	case "M":
		whole := math.Floor(v)
		t := from.AddDate(0, int(whole), 0)
		return t.Add(time.Duration((v - whole) * 30 * 24 * float64(time.Hour))), true
	case "y":
		whole := math.Floor(v)
		t := from.AddDate(int(whole), 0, 0)
		return t.Add(time.Duration((v - whole) * 365 * 24 * float64(time.Hour))), true
	}
	return time.Time{}, false
}

// parseDate parses a date using the chart's dayjs-style dateFormat.
func (c *Chart) parseDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	f := strings.TrimSpace(c.DateFormat)
	switch f {
	case "X":
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return time.Time{}, err
		}
		return time.Unix(int64(v), 0).UTC(), nil
	case "x":
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return time.Time{}, err
		}
		return time.UnixMilli(v).UTC(), nil
	}
	layout := DayjsToGo(f)
	t, err := time.ParseInLocation(layout, s, time.UTC)
	if err == nil {
		return t, nil
	}
	// be lenient: accept ISO dates regardless of dateFormat
	for _, l := range []string{"2006-01-02", "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02 15:04"} {
		if t, err2 := time.ParseInLocation(l, s, time.UTC); err2 == nil {
			return t, nil
		}
	}
	return time.Time{}, err
}

// DayjsToGo converts a dayjs format string into a Go time layout.
func DayjsToGo(f string) string {
	repl := []struct{ from, to string }{
		{"YYYY", "2006"}, {"YY", "06"},
		{"MMMM", "January"}, {"MMM", "Jan"}, {"MM", "01"}, {"M", "1"},
		{"DD", "02"}, {"Do", "2"}, {"D", "2"},
		{"dddd", "Monday"}, {"ddd", "Mon"},
		{"HH", "15"}, {"H", "15"}, {"hh", "03"}, {"h", "3"},
		{"mm", "04"}, {"m", "4"},
		{"ss", "05"}, {"s", "5"},
		{"SSS", "000"}, {"SS", "00"}, {"S", "0"},
		{"A", "PM"}, {"a", "pm"},
		{"ZZ", "-0700"}, {"Z", "-07:00"},
	}
	var b strings.Builder
	for i := 0; i < len(f); {
		if f[i] == '[' {
			if j := strings.IndexByte(f[i:], ']'); j > 0 {
				b.WriteString(f[i+1 : i+j])
				i += j + 1
				continue
			}
		}
		matched := false
		for _, r := range repl {
			if strings.HasPrefix(f[i:], r.from) {
				b.WriteString(r.to)
				i += len(r.from)
				matched = true
				break
			}
		}
		if !matched {
			b.WriteByte(f[i])
			i++
		}
	}
	return b.String()
}

// Strftime formats t with a d3 strftime-style format.
func Strftime(f string, t time.Time) string {
	var b strings.Builder
	for i := 0; i < len(f); i++ {
		if f[i] != '%' || i+1 >= len(f) {
			b.WriteByte(f[i])
			continue
		}
		i++
		// d3 padding modifiers: %-d, %_d, %0d
		pad := byte('0')
		if f[i] == '-' || f[i] == '_' || f[i] == '0' {
			pad = f[i]
			i++
			if i >= len(f) {
				break
			}
		}
		num := func(v, w int) string {
			s := strconv.Itoa(v)
			switch pad {
			case '-':
				return s
			case '_':
				return strings.Repeat(" ", max(w-len(s), 0)) + s
			}
			return strings.Repeat("0", max(w-len(s), 0)) + s
		}
		switch f[i] {
		case 'Y':
			b.WriteString(strconv.Itoa(t.Year()))
		case 'y':
			b.WriteString(num(t.Year()%100, 2))
		case 'm':
			b.WriteString(num(int(t.Month()), 2))
		case 'd':
			b.WriteString(num(t.Day(), 2))
		case 'e':
			s := strconv.Itoa(t.Day())
			if pad != '-' && len(s) < 2 {
				s = " " + s
			}
			b.WriteString(s)
		case 'b', 'h':
			b.WriteString(t.Month().String()[:3])
		case 'B':
			b.WriteString(t.Month().String())
		case 'a':
			b.WriteString(t.Weekday().String()[:3])
		case 'A':
			b.WriteString(t.Weekday().String())
		case 'H':
			b.WriteString(num(t.Hour(), 2))
		case 'I':
			h := t.Hour() % 12
			if h == 0 {
				h = 12
			}
			b.WriteString(num(h, 2))
		case 'M':
			b.WriteString(num(t.Minute(), 2))
		case 'S':
			b.WriteString(num(t.Second(), 2))
		case 'L':
			b.WriteString(num(t.Nanosecond()/1e6, 3))
		case 'p':
			if t.Hour() < 12 {
				b.WriteString("AM")
			} else {
				b.WriteString("PM")
			}
		case 'j':
			b.WriteString(num(t.YearDay(), 3))
		case 'U':
			b.WriteString(num((t.YearDay()+6-int(t.Weekday()))/7, 2))
		case 'W':
			wd := (int(t.Weekday()) + 6) % 7
			b.WriteString(num((t.YearDay()+6-wd)/7, 2))
		case 'w':
			b.WriteString(strconv.Itoa(int(t.Weekday())))
		case 'u':
			wd := int(t.Weekday())
			if wd == 0 {
				wd = 7
			}
			b.WriteString(strconv.Itoa(wd))
		case 'V':
			_, w := t.ISOWeek()
			b.WriteString(num(w, 2))
		case 'q':
			b.WriteString(strconv.Itoa((int(t.Month())-1)/3 + 1))
		case 'Z':
			b.WriteString(t.Format("-0700"))
		case '%':
			b.WriteByte('%')
		default:
			b.WriteByte('%')
			b.WriteByte(f[i])
		}
	}
	return b.String()
}
