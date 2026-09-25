package er

import (
	"testing"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/theme"
)

func TestParseSimpleEntity(t *testing.T) {
	src := `erDiagram
	CUSTOMER
	`
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(doc.Entities) != 1 {
		t.Errorf("expected 1 entity, got %d", len(doc.Entities))
	}
	ent, ok := doc.Entities["CUSTOMER"]
	if !ok {
		t.Fatal("CUSTOMER entity not found")
	}
	if ent.Label != "CUSTOMER" {
		t.Errorf("expected label CUSTOMER, got %q", ent.Label)
	}
}

func TestParseEntityWithLabel(t *testing.T) {
	src := `erDiagram
	CUSTOMER["Customer Entity"]
	`
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	ent, ok := doc.Entities["CUSTOMER"]
	if !ok {
		t.Fatal("CUSTOMER entity not found")
	}
	if ent.Label != "Customer Entity" {
		t.Errorf("expected label 'Customer Entity', got %q", ent.Label)
	}
}

func TestParseEntityWithAttributes(t *testing.T) {
	src := `erDiagram
	CUSTOMER { int id PK, string name, string email }
	`
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	ent, ok := doc.Entities["CUSTOMER"]
	if !ok {
		t.Fatal("CUSTOMER entity not found")
	}
	if len(ent.Attrs) != 3 {
		t.Errorf("expected 3 attributes, got %d", len(ent.Attrs))
	}
	if ent.Attrs[0].Type != "int" || ent.Attrs[0].Name != "id" {
		t.Errorf("first attribute wrong: %v", ent.Attrs[0])
	}
}

func TestParseRelationship(t *testing.T) {
	src := `erDiagram
	CUSTOMER
	ORDER
	CUSTOMER ||--o{ ORDER : "places"
	`
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(doc.Relationships) != 1 {
		t.Errorf("expected 1 relationship, got %d", len(doc.Relationships))
	}
	rel := doc.Relationships[0]
	if rel.From != "CUSTOMER" || rel.To != "ORDER" {
		t.Errorf("relationship endpoints wrong: %s -> %s", rel.From, rel.To)
	}
	if rel.FromCard != CardExactlyOne || rel.ToCard != CardZeroOrMore {
		t.Errorf("cardinalities wrong: from=%d, to=%d", rel.FromCard, rel.ToCard)
	}
	if rel.Label != "places" {
		t.Errorf("label wrong: %q", rel.Label)
	}
}

func TestParseCardinality(t *testing.T) {
	tests := []struct {
		s    string
		want Cardinality
	}{
		{"||", CardExactlyOne},
		{"|o", CardZeroOrOne},
		{"o|", CardZeroOrOne},
		{"}|", CardOneOrMore},
		{"|{", CardOneOrMore},
		{"}o", CardZeroOrMore},
		{"o{", CardZeroOrMore},
	}
	for _, tt := range tests {
		got := parseCardinality(tt.s)
		if got != tt.want {
			t.Errorf("parseCardinality(%q) = %d, want %d", tt.s, got, tt.want)
		}
	}
}

func TestParseCardinalityWord(t *testing.T) {
	tests := []struct {
		s    string
		want Cardinality
	}{
		{"only one", CardExactlyOne},
		{"one or zero", CardZeroOrOne},
		{"zero or more", CardZeroOrMore},
		{"one or more", CardOneOrMore},
	}
	for _, tt := range tests {
		got := parseCardinalityWord(tt.s)
		if got != tt.want {
			t.Errorf("parseCardinalityWord(%q) = %d, want %d", tt.s, got, tt.want)
		}
	}
}

func TestParseDirection(t *testing.T) {
	src := `erDiagram
	direction LR
	CUSTOMER
	ORDER
	`
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if doc.Direction != "LR" {
		t.Errorf("direction = %q, want LR", doc.Direction)
	}
}

func TestParseTitle(t *testing.T) {
	src := `erDiagram
	title Customer Order Relationship
	CUSTOMER
	`
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if doc.Title != "Customer Order Relationship" {
		t.Errorf("title = %q, want 'Customer Order Relationship'", doc.Title)
	}
}

func TestRenderBasicER(t *testing.T) {
	src := `erDiagram
	CUSTOMER
	ORDER
	CUSTOMER ||--o{ ORDER : "places"
	`
	cfg := &diagram.Config{Theme: theme.Default()}
	sc, err := Render(src, cfg)
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	if sc.Width <= 0 || sc.Height <= 0 {
		t.Logf("scene dimensions: %fx%f", sc.Width, sc.Height)
		t.Logf("items: %d", len(sc.Items))
		t.Errorf("scene dimensions invalid")
	}
	if len(sc.Items) == 0 {
		t.Error("no items rendered")
	}
}

func TestRenderERWithAttributes(t *testing.T) {
	src := `erDiagram
	CUSTOMER { int id PK, string name }
	ORDER { int id PK, int customer_id FK }
	CUSTOMER ||--o{ ORDER : ""
	`
	cfg := &diagram.Config{Theme: theme.Default()}
	sc, err := Render(src, cfg)
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	if sc.Width <= 0 || sc.Height <= 0 {
		t.Logf("scene dimensions: %fx%f", sc.Width, sc.Height)
		t.Logf("items: %d", len(sc.Items))
		t.Errorf("scene dimensions invalid")
	}
}
