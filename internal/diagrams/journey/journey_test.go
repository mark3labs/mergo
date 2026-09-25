package journey

import "testing"

func TestParse(t *testing.T) {
	j, err := Parse(`journey
    title My day
    section Morning
      Make tea: 5: Me
      Commute: 2: Me, Cat
    section Evening
      Relax: 9: Cat`)
	if err != nil {
		t.Fatal(err)
	}
	if j.Title != "My day" || len(j.Tasks) != 3 || len(j.Actors) != 2 || len(j.Sections) != 2 {
		t.Fatalf("%+v", j)
	}
	if j.Tasks[1].Score != 2 || len(j.Tasks[1].Actors) != 2 || j.Tasks[2].Score != 5 || j.Tasks[2].Section != 1 {
		t.Errorf("tasks %+v %+v", j.Tasks[1], j.Tasks[2])
	}
	for _, bad := range []string{"Task without score", "Task: high: Me"} {
		if _, err := Parse("journey\n" + bad); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}
