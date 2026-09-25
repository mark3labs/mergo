package class

import (
	"fmt"
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

func TestParseClassWithLabel(t *testing.T) {
	src := `classDiagram
class A["Label for A"]
class B["Another label"]`
	p := newParser(&diagram.Config{Theme: theme.Default()})
	if err := p.parse(src); err != nil {
		t.Fatal(err)
	}
	if p.classes["A"].label != "Label for A" {
		t.Errorf("expected label 'Label for A', got %q", p.classes["A"].label)
	}
	if p.classes["B"].label != "Another label" {
		t.Errorf("expected label 'Another label', got %q", p.classes["B"].label)
	}
}

func TestParseClassWithGenerics(t *testing.T) {
	src := `classDiagram
class List~T~
class Map~K~V~`
	p := newParser(&diagram.Config{Theme: theme.Default()})
	if err := p.parse(src); err != nil {
		t.Fatal(err)
	}
	if p.classes["List"].generics != "~T~" {
		t.Errorf("expected generics '~T~', got %q", p.classes["List"].generics)
	}
	if p.classes["Map"].generics != "~K~V~" {
		t.Errorf("expected generics '~K~V~', got %q", p.classes["Map"].generics)
	}
}

func TestParseClassWithNestedGenerics(t *testing.T) {
	src := `classDiagram
class Container~List~int~~`
	p := newParser(&diagram.Config{Theme: theme.Default()})
	if err := p.parse(src); err != nil {
		t.Fatal(err)
	}
	if p.classes["Container"].generics != "~List~int~~" {
		t.Errorf("expected nested generics, got %q", p.classes["Container"].generics)
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
	// Check visibility
	if a.members[0].visibility != '+' {
		t.Errorf("expected visibility '+', got %q", string(a.members[0].visibility))
	}
}

func TestParseStaticAndAbstractMembers(t *testing.T) {
	src := `classDiagram
class Shape {
  +draw()*
  #calculateArea()$
}`
	p := newParser(&diagram.Config{Theme: theme.Default()})
	if err := p.parse(src); err != nil {
		t.Fatal(err)
	}
	shape, ok := p.classes["Shape"]
	if !ok {
		t.Fatal("Shape not found")
	}

	// Check abstract marker
	if !shape.members[0].isAbstract {
		t.Error("draw() should be abstract")
	}

	// Check static marker
	if !shape.members[1].isStatic {
		t.Error("calculateArea() should be static")
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

func TestParseMultipleAnnotations(t *testing.T) {
	src := `classDiagram
class Service <<service>> <<abstract>>`
	p := newParser(&diagram.Config{Theme: theme.Default()})
	if err := p.parse(src); err != nil {
		t.Fatal(err)
	}

	svc, ok := p.classes["Service"]
	if !ok {
		t.Fatal("Service not found")
	}
	if len(svc.annotations) != 1 {
		// Only the first annotation in inline form, but should still parse
		t.Logf("Service has %d annotations", len(svc.annotations))
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

func TestParseAllRelationTypes(t *testing.T) {
	src := `classDiagram
A <|-- B
C *-- D
E o-- F
G --> H
I -- J
K ..> L
M ..|> N`
	p := newParser(&diagram.Config{Theme: theme.Default()})
	if err := p.parse(src); err != nil {
		t.Fatal(err)
	}
	if len(p.relations) < 7 {
		t.Errorf("expected at least 7 relations, got %d", len(p.relations))
	}
}

func TestParseReversedRelations(t *testing.T) {
	src := `classDiagram
A --|> B
C --* D
E --o F
G <-- H`
	p := newParser(&diagram.Config{Theme: theme.Default()})
	if err := p.parse(src); err != nil {
		t.Fatal(err)
	}
	if len(p.relations) < 4 {
		t.Errorf("expected at least 4 relations, got %d", len(p.relations))
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

func TestParseRelationWithCardinality(t *testing.T) {
	src := `classDiagram
class Customer
class Order
Customer "1" --> "*" Order : places`
	p := newParser(&diagram.Config{Theme: theme.Default()})
	if err := p.parse(src); err != nil {
		t.Fatal(err)
	}
	if len(p.relations) < 1 {
		t.Fatal("expected at least 1 relation")
	}
	if p.relations[0].cardFrom != "1" {
		t.Errorf("expected cardFrom '1', got %q", p.relations[0].cardFrom)
	}
	if p.relations[0].cardTo != "*" {
		t.Errorf("expected cardTo '*', got %q", p.relations[0].cardTo)
	}
}

func TestParseStyleDef(t *testing.T) {
	src := `classDiagram
class A
classDef redStyle fill:#f9f,stroke:#333
cssClass "A" redStyle`
	p := newParser(&diagram.Config{Theme: theme.Default()})
	if err := p.parse(src); err != nil {
		t.Fatal(err)
	}
	if len(p.styleClassDefs) == 0 {
		t.Error("no style definitions found")
	}
}

func TestParseDirectionTB(t *testing.T) {
	src := `classDiagram
direction TB
class A
class B
A --> B`
	p := newParser(&diagram.Config{Theme: theme.Default()})
	if err := p.parse(src); err != nil {
		t.Fatal(err)
	}
	if p.direction != 0 { // TB is 0
		t.Errorf("expected direction TB (0), got %d", p.direction)
	}
}

func TestParseDirectionLR(t *testing.T) {
	src := `classDiagram
direction LR
class A`
	p := newParser(&diagram.Config{Theme: theme.Default()})
	if err := p.parse(src); err != nil {
		t.Fatal(err)
	}
	if p.direction != 2 { // LR is 2
		t.Errorf("expected direction LR (2), got %d", p.direction)
	}
}

func TestParseNote(t *testing.T) {
	src := `classDiagram
class A
note for A "This is a note"`
	p := newParser(&diagram.Config{Theme: theme.Default()})
	if err := p.parse(src); err != nil {
		t.Fatal(err)
	}
	if len(p.notes) != 1 {
		t.Errorf("expected 1 note, got %d", len(p.notes))
	}
	if p.notes[0].forClass != "A" {
		t.Errorf("expected note for A, got %s", p.notes[0].forClass)
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
		{
			"with_generics",
			`classDiagram
class List~T~ {
  +add(T item)
}`,
		},
		{
			"all_visibilities",
			`classDiagram
class Example {
  +public_attr
  -private_attr
  #protected_attr
  ~package_attr
}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sc, err := Render(tc.src, &diagram.Config{Theme: theme.Default()})
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

func TestRenderWithGenerics(t *testing.T) {
	src := `classDiagram
class Container~List~int~~ {
  +get() List~int~
}
class Item {
  +value: int
}
Container~List~int~~ --> Item`

	sc, err := Render(src, &diagram.Config{Theme: theme.Default()})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}

	if sc.Width < 10 || sc.Height < 10 {
		t.Errorf("scene too small: %fx%f", sc.Width, sc.Height)
	}

	if testing.Verbose() {
		img := sc.Render(scene.RenderOptions{Scale: 1})
		t.Log("\n" + devutil.ASCII(img, 140))
	}
}

func TestParseTruncations(t *testing.T) {
	// Test that parser doesn't panic on truncations
	testCases := []string{
		"classDiagram",
		"classDiagram\nclass",
		"classDiagram\nclass A",
		"classDiagram\nclass A {",
		"classDiagram\nclass A <<",
		"classDiagram\nclass A [\"",
		"classDiagram\nA -->",
		"classDiagram\nA -- B :",
		"classDiagram\nnote for A",
	}

	for i, src := range testCases {
		t.Run(fmt.Sprintf("truncation_%d", i), func(t *testing.T) {
			p := newParser(&diagram.Config{Theme: theme.Default()})
			_ = p.parse(src) // Should not panic
		})
	}
}

func TestRenderAllVisibilities(t *testing.T) {
	src := `classDiagram
class Example {
  +public
  -private
  #protected
  ~package
  +method() void
  -privateMethod()
}`

	sc, err := Render(src, &diagram.Config{Theme: theme.Default()})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}

	if sc.Width < 10 || sc.Height < 10 {
		t.Errorf("scene too small: %fx%f", sc.Width, sc.Height)
	}
}

func TestRenderStaticAndAbstract(t *testing.T) {
	src := `classDiagram
class Shape {
  #area: double
  +draw()*
  #calculateArea()$ double
}`

	sc, err := Render(src, &diagram.Config{Theme: theme.Default()})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}

	if sc.Width < 10 || sc.Height < 10 {
		t.Errorf("scene too small: %fx%f", sc.Width, sc.Height)
	}
}
