// Command mmdpng is a small development helper that renders a Mermaid file
// to PNG without the TUI: mmdpng [-theme name] [-scale 2] in.mmd out.png
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/mark3labs/mergo/internal/devutil"
	"github.com/mark3labs/mergo/internal/mermaid"
	"github.com/mark3labs/mergo/internal/theme"
)

func main() {
	th := flag.String("theme", "default", "theme name")
	scale := flag.Float64("scale", 2, "render scale")
	ascii := flag.Int("ascii", 0, "print an ASCII preview with this many columns instead of writing a PNG")
	flag.Parse()
	if *ascii > 0 && flag.NArg() == 1 {
		src, err := os.ReadFile(flag.Arg(0))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		t, _ := theme.Get(*th)
		img, err := mermaid.RenderImage(string(src), t, 1)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		fmt.Print(devutil.ASCII(img, *ascii))
		return
	}
	if flag.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "usage: mmdpng [-theme name] [-scale 2] in.mmd out.png")
		os.Exit(2)
	}
	src, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	t, err := theme.Get(*th)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	b, err := mermaid.RenderPNG(string(src), t, *scale)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(flag.Arg(1), b, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
