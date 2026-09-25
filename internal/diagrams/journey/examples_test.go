package journey

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mergo/internal/devutil"
	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

func TestExamples(t *testing.T) {
	files, _ := filepath.Glob("../../../examples/journey/*.mmd")
	if len(files) == 0 {
		t.Fatal("no examples")
	}
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
		for _, name := range theme.Names() {
			sc, err := diagram.Render(s, theme.MustGet(name))
			if err != nil {
				t.Fatalf("%s (%s): %v", f, name, err)
			}
			if testing.Verbose() && name == "default" {
				t.Logf("%s\n%s", f, devutil.ASCII(sc.Render(scene.RenderOptions{Scale: 1}), 120))
			}
		}
	}
}
