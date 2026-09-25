package mindmap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mergo/internal/devutil"
	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

func TestParse(t *testing.T) {
	root, err := Parse(`mindmap
  root((Central))
    a[Square]
      ::icon(fa fa-book)
      b(Rounded)
      c))Bang((
    d)Cloud(
    :::urgent large
    e{{Hex}}
    Plain text with <br>break`)
	if err != nil {
		t.Fatal(err)
	}
	if root.Label != "Central" || root.Shape != ShapeCircle || len(root.Children) != 4 {
		t.Fatalf("root %+v", root)
	}
	a := root.Children[0]
	if a.Shape != ShapeSquare || a.Icon != "fa fa-book" || len(a.Children) != 2 {
		t.Errorf("a %+v", a)
	}
	if a.Children[1].Shape != ShapeBang || root.Children[1].Shape != ShapeCloud || root.Children[2].Shape != ShapeHexagon {
		t.Error("shapes")
	}
	if strings.Join(root.Children[1].Classes, ",") != "urgent,large" {
		t.Errorf("classes %v", root.Children[1].Classes)
	}
	if root.Children[3].Label != "Plain text with\nbreak" {
		t.Errorf("label %q", root.Children[3].Label)
	}
	if _, err := Parse("mindmap\n  a\n b"); err == nil {
		t.Error("expected multiple roots error")
	}
}

func collect(n *Node, out *[]*Node) {
	*out = append(*out, n)
	for _, c := range n.Children {
		collect(c, out)
	}
}

func TestLargeTreeNoOverlap(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("mindmap\n  root((Root))\n")
	for i := 0; i < 7; i++ {
		fmt.Fprintf(&sb, "    Branch %d\n", i)
		for j := 0; j < i%4+1; j++ {
			fmt.Fprintf(&sb, "      Leaf %d.%d with a longer label\n", i, j)
			for k := 0; k < j%3; k++ {
				fmt.Fprintf(&sb, "        Deep %d.%d.%d\n", i, j, k)
			}
		}
	}
	root, err := Parse(sb.String())
	if err != nil {
		t.Fatal(err)
	}
	th := theme.Default()
	measure(root, 0, th)
	var right, left []*Node
	var rh, lh float64
	for i, c := range root.Children {
		assignBranch(c, i)
		if rh <= lh {
			right = append(right, c)
			rh += c.subH + vGap
		} else {
			left = append(left, c)
			lh += c.subH + vGap
		}
	}
	place(right, root, 1, th)
	place(left, root, -1, th)
	var all []*Node
	collect(root, &all)
	for i := range all {
		for j := i + 1; j < len(all); j++ {
			a, b := all[i], all[j]
			if lt(a.x-a.w/2, b.x+b.w/2) && lt(b.x-b.w/2, a.x+a.w/2) && lt(a.y-a.h/2, b.y+b.h/2) && lt(b.y-b.h/2, a.y+a.h/2) {
				t.Errorf("%q overlaps %q", a.Label, b.Label)
			}
		}
	}
}

func lt(a, b float64) bool { return a < b-0.5 }

func TestExamples(t *testing.T) {
	files, _ := filepath.Glob("../../../examples/mindmap/*.mmd")
	for _, f := range files {
		b, _ := os.ReadFile(f)
		s := string(b)
		for i := 0; i <= len(s); i += 3 {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("%s prefix %d: %v", f, i, r)
					}
				}()
				_, _ = diagram.Render(s[:i], theme.Default())
			}()
		}
		sc, err := diagram.Render(s, theme.MustGet("charm"))
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if testing.Verbose() {
			t.Logf("%s\n%s", f, devutil.ASCII(sc.Render(scene.RenderOptions{Scale: 1}), 120))
		}
	}
}
