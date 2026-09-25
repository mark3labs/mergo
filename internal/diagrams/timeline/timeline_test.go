package timeline

import "testing"

func TestParse(t *testing.T) {
	tl, err := Parse(`timeline TD
    title History
    section Early
        2002 : LinkedIn
        2004 : Facebook : Google
             : Gmail
    section Late
        2006 : Twitter`)
	if err != nil {
		t.Fatal(err)
	}
	if !tl.Vertical || tl.Title != "History" || len(tl.Sections) != 2 || len(tl.Periods) != 3 {
		t.Fatalf("%+v", tl)
	}
	p := tl.Periods[1]
	if p.Label != "2004" || len(p.Events) != 3 || p.Events[2] != "Gmail" || p.Section != 0 {
		t.Errorf("period %+v", p)
	}
	if tl.Periods[2].Section != 1 {
		t.Error("section index")
	}
	if _, err := Parse("timeline\n: orphan"); err == nil {
		t.Error("expected error for orphan event")
	}
}
