package gantt

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mark3labs/mergo/internal/devutil"
	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

var fixedNow = time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestDatesAndDependencies(t *testing.T) {
	c, err := Parse(`gantt
    dateFormat YYYY-MM-DD
    section A
    First      :a1, 2024-01-01, 3d
    Second     :after a1, 2d
    Third      :a3, 2024-01-10, 2024-01-12
    Fourth     :4d
    Until      :u1, 2024-01-01, until a3
    Both       :after a1 a3, 1w
    Half       :0.5d
    Mile       :milestone, m1, after a3, 0d
    Lenient    :idonly, 1d`, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	tk := c.Tasks
	check := func(i int, s, e time.Time) {
		t.Helper()
		if !tk[i].Start.Equal(s) || !tk[i].End.Equal(e) {
			t.Errorf("task %d %q: %v - %v, want %v - %v", i, tk[i].Name, tk[i].Start, tk[i].End, s, e)
		}
	}
	check(0, day(2024, 1, 1), day(2024, 1, 4))
	check(1, day(2024, 1, 4), day(2024, 1, 6))
	check(2, day(2024, 1, 10), day(2024, 1, 12))
	check(3, day(2024, 1, 12), day(2024, 1, 16))
	check(4, day(2024, 1, 1), day(2024, 1, 10))
	check(5, day(2024, 1, 12), day(2024, 1, 19))
	check(6, day(2024, 1, 19), day(2024, 1, 19).Add(12*time.Hour))
	if !tk[7].Milestone || !tk[7].Start.Equal(day(2024, 1, 12)) {
		t.Errorf("milestone %+v", tk[7])
	}
	if tk[8].ID != "idonly" {
		t.Errorf("lenient id: %q", tk[8].ID)
	}
}

func TestExcludesAndInclusive(t *testing.T) {
	c, err := Parse(`gantt
    dateFormat YYYY-MM-DD
    excludes weekends, 2024-01-10
    inclusiveEndDates
    Work :w, 2024-01-05, 3d
    Range :2024-01-01, 2024-01-02`, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	// Fri 5th + 3 working days (Fri, Mon, Tue) skipping the weekend ends at
	// the start of Wed 10th.
	if got := c.Tasks[0].End; !got.Equal(day(2024, 1, 10)) {
		t.Errorf("excludes: end %v", got)
	}
	if got := c.Tasks[1].End; !got.Equal(day(2024, 1, 3)) {
		t.Errorf("inclusive end: %v", got)
	}
	c2, _ := Parse("gantt\nexcludes weekends\nweekend friday\nx :2024-01-01, 1d", fixedNow)
	if !c2.Excluded(day(2024, 1, 5)) || c2.Excluded(day(2024, 1, 7)) {
		t.Error("weekend friday")
	}
}

func TestFormats(t *testing.T) {
	if got := DayjsToGo("YYYY-MM-DD HH:mm:ss"); got != "2006-01-02 15:04:05" {
		t.Errorf("dayjs = %q", got)
	}
	if got := DayjsToGo("DD/MM/YY [at] h:mm A"); got != "02/01/06 at 3:04 PM" {
		t.Errorf("dayjs literal = %q", got)
	}
	tm := time.Date(2024, 3, 5, 14, 7, 9, 0, time.UTC)
	for f, want := range map[string]string{
		"%Y-%m-%d": "2024-03-05", "%b %e": "Mar  5", "%-d %B": "5 March", "%H:%M": "14:07",
		"%I %p": "02 PM", "%a %j": "Tue 065", "%y%%": "24%",
	} {
		if got := Strftime(f, tm); got != want {
			t.Errorf("strftime %q = %q, want %q", f, got, want)
		}
	}
	c, err := Parse("gantt\ndateFormat DD.MM.YYYY HH:mm\nt :01.02.2024 10:30, 90m", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Tasks[0].End.Equal(time.Date(2024, 2, 1, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("custom format: %v", c.Tasks[0].End)
	}
	c, err = Parse("gantt\ndateFormat X\nt :1700000000, 1h", fixedNow)
	if err != nil || c.Tasks[0].Start.Unix() != 1700000000 {
		t.Errorf("unix: %v %v", err, c)
	}
}

func TestErrors(t *testing.T) {
	for _, src := range []string{
		"gantt\nt :x1, 2024-99-99, 1d",
		"gantt\nno colon here",
		"gantt\na :a, after b, 1d\nb :b, after a, 1d",
		"gantt\ntickInterval 3fortnights",
	} {
		if _, err := Parse(src, fixedNow); err == nil {
			t.Errorf("%q: expected error", src)
		}
	}
}

func TestExamples(t *testing.T) {
	Now = func() time.Time { return fixedNow }
	defer func() { Now = time.Now }()
	files, _ := filepath.Glob("../../../examples/gantt/*.mmd")
	if len(files) == 0 {
		t.Fatal("no examples")
	}
	for _, f := range files {
		b, _ := os.ReadFile(f)
		s := string(b)
		for i := 0; i <= len(s); i += 5 {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("%s prefix %d: %v", f, i, r)
					}
				}()
				_, _ = diagram.Render(s[:i], theme.Default())
			}()
		}
		sc, err := diagram.Render(s, theme.MustGet("dark"))
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if sc.Width > 2000 || sc.Width < 400 {
			t.Errorf("%s: width %v", f, sc.Width)
		}
		if testing.Verbose() {
			t.Logf("%s\n%s", f, devutil.ASCII(sc.Render(scene.RenderOptions{Scale: 1}), 140))
		}
	}
}
