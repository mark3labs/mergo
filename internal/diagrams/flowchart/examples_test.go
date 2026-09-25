package flowchart

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/theme"
)

// TestRenderAllExamples verifies all example flowchart files render without error
func TestRenderAllExamples(t *testing.T) {
	examplesDir := "examples/flowchart"
	entries, err := os.ReadDir(examplesDir)
	if err != nil {
		t.Skipf("skipping: examples directory not found")
	}

	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".mmd" {
			t.Run(entry.Name(), func(t *testing.T) {
				src, err := os.ReadFile(filepath.Join(examplesDir, entry.Name()))
				if err != nil {
					t.Fatalf("failed to read %s: %v", entry.Name(), err)
				}

				th := theme.Default()
				cfg := &diagram.Config{Theme: th}
				sc, err := Render(string(src), cfg)
				if err != nil {
					t.Fatalf("failed to render %s: %v", entry.Name(), err)
				}

				if sc == nil {
					t.Fatalf("scene is nil for %s", entry.Name())
				}

				// Verify dimensions are reasonable
				if sc.Width <= 0 || sc.Height <= 0 {
					t.Errorf("invalid scene dimensions: %fx%f", sc.Width, sc.Height)
				}
			})
		}
	}
}
