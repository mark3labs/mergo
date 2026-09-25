package tui

import (
	"testing"
)

func TestExtractMermaidBlocks(t *testing.T) {
	tests := []struct {
		name string
		md   string
		want []string
	}{
		{
			name: "single block",
			md: `# Title

` + "```mermaid\n" + `pie
  "A": 30
  "B": 70
` + "```\n" + `
Some text.`,
			want: []string{"pie\n  \"A\": 30\n  \"B\": 70"},
		},
		{
			name: "multiple blocks",
			md: "```mermaid\n" + `flowchart TD
  A --> B
` + "```\n\n```mermaid\n" + `pie
  "X": 50
` + "```",
			want: []string{
				"flowchart TD\n  A --> B",
				"pie\n  \"X\": 50",
			},
		},
		{
			name: "no blocks",
			md:   "Just markdown with no code blocks.",
			want: []string{},
		},
		{
			name: "non-mermaid blocks ignored",
			md: "```go\n" + `func main() {}
` + "```\n\n```mermaid\n" + `pie
  "A": 100
` + "```",
			want: []string{"pie\n  \"A\": 100"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractMermaidBlocks(tt.md)
			if len(got) != len(tt.want) {
				t.Errorf("length: got %d, want %d", len(got), len(tt.want))
				return
			}
			for i, block := range got {
				if block != tt.want[i] {
					t.Errorf("block %d: got %q, want %q", i, block, tt.want[i])
				}
			}
		})
	}
}
