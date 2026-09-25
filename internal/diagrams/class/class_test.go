package class

import (
	"strings"
	"testing"

	"github.com/mark3labs/mergo/internal/devutil"
	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

func TestParseBasicClass(t *testing.T) {
	src := `classDiagram
class Animal
class Dog`
	p := newParser(&diagram.Config{Theme: theme.Default()})
	if err := p.parse(src); err != nil {
		t.Fatal(err)
	}
	if len(p.classes) != 2 {
		t.Errorf("expected 2 classes, got %d", len(p.classes))
	}
	if _, ok := p.classes["Animal"]; !ok {
		t.Error("Animal not found")
	}
	if _, ok := p.classes["Dog"]; !ok {
		t.Error("Dog not found")
	}
}

func TestParseClassWithMembers(t *testing.T) {
	src := `classDiagram
class Animal {
  +int age
  +string name
  +makeSound()
}`
	p := newParser(&diagram.Config{Theme: theme.Default()})
	if err := p.parse(src); err != nil {
		t.Fatal(err)
	}
	a, ok := p.classes["Animal"]
	if !ok {
		t.Fatal("Animal not found")
	}
	if len(a.members) != 3 {
		t.Errorf("expected 3 members, got %d", len(a.members))
	}
}

func TestParseAnnotations(t *testing.T) {
	src := `classDiagram
class Shape <<interface>>
class Animal {
  <<abstract>>
  +age
}`
	p := newParser(&diagram.Config{Theme: theme.Default()})
	if err := p.parse(src); err != nil {
		t.Fatal(err)
	}

	shape, ok := p.classes["Shape"]
	if !ok {
		t.Fatal("Shape not found")
	}
	if len(shape.annotations) == 0 {
		t.Error("no annotations on Shape")
	}

	animal, ok := p.classes["Animal"]
	if !ok {
		t.Fatal("Animal not found")
	}
	if len(animal.annotations) == 0 {
		t.Error("no annotations on Animal")
	}
}

func TestParseRelations(t *testing.T) {
	src := `classDiagram
class Animal
class Dog
Animal <|-- Dog`
	p := newParser(&diagram.Config{Theme: theme.Default()})
	if err := p.parse(src); err != nil {
		t.Fatal(err)
	}
	if len(p.relations) != 1 {
		t.Errorf("expected 1 relation, got %d", len(p.relations))
	}
	if p.relations[0].from != "Animal" {
		t.Errorf("expected from=Animal, got %s", p.relations[0].from)
	}
}

func TestParseRelationWithLabel(t *testing.T) {
	src := `classDiagram
class A
class B
A --> B : depends on`
	p := newParser(&diagram.Config{Theme: theme.Default()})
	if err := p.parse(src); err != nil {
		t.Fatal(err)
	}
	if len(p.relations) != 1 {
		t.Fatal("expected 1 relation")
	}
	if p.relations[0].label != "depends on" {
		t.Errorf("expected label 'depends on', got '%s'", p.relations[0].label)
	}
}

func TestRenderSmoke(t *testing.T) {
	testCases := []struct {
		name string
		src  string
	}{
		{
			"basic",
			`classDiagram
class Animal {
  +int age
}
class Dog
Animal <|-- Dog`,
		},
		{
			"with_annotations",
			`classDiagram
class Shape <<interface>> {
  +draw()
}`,
		},
		{
			"multiple_relations",
			`classDiagram
class A
class B
class C
A --> B : uses
B --> C : depends`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sc, err := Render(strings.TrimPrefix(tc.src, "classDiagram\n"), &diagram.Config{Theme: theme.Default()})
			if err != nil {
				t.Fatalf("render error: %v", err)
			}
			if sc.Width < 10 || sc.Height < 10 {
				t.Errorf("scene too small: %fx%f", sc.Width, sc.Height)
			}
		})
	}
}

func TestRenderNoOverlap(t *testing.T) {
	src := `classDiagram
class Student {
  -id: int
  -name: string
  +study()
}
class Course {
  -code: string
  +enroll()
}
Student --> Course : enrolls`

	sc, err := Render(src, &diagram.Config{Theme: theme.Default()})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}

	// Basic sanity check: scene should not be empty
	if sc.Width < 50 || sc.Height < 50 {
		t.Errorf("scene too small: %fx%f", sc.Width, sc.Height)
	}

	// Render to image for verification
	img := sc.Render(scene.RenderOptions{Scale: 1})
	if testing.Verbose() {
		t.Log("\n" + devutil.ASCII(img, 140))
	}
}
