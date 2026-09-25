package main

import (
	"context"
	"fmt"
	"os"

	"charm.land/fang/v2"
	"github.com/spf13/cobra"

	"github.com/mark3labs/mergo/internal/theme"
	"github.com/mark3labs/mergo/internal/tui"
)

var (
	version = "dev"
)

func main() {
	ctx := context.Background()

	var (
		themeFlag     string
		rendererFlag  string
		outputFlag    string
		indexFlag     int
		printFlag     bool
		scaleFlag     float64
		fontFlag      string
		fontBoldFlag  string
		noShadowsFlag bool
	)

	root := &cobra.Command{
		Use:   "mergo [flags] [file ...]",
		Short: "Render Mermaid diagrams natively in the terminal",
		Long: `mergo renders Mermaid diagrams natively in Go with a pure rasterizer,
displaying them in the terminal using the Kitty graphics protocol or half-block rendering.

Supports .mmd/.mermaid files (whole file is one diagram) or .md/.markdown files
(extracts every mermaid fenced block as a separate diagram). Use - or no args
for stdin.`,
		Example: `  mergo examples/pie/basic.mmd
  mergo -t dark examples/flowchart/basic.mmd
  mergo -p examples/sequence/basic.mmd | head -30
  mergo -o /tmp/diagram.png examples/class/basic.mmd
  cat diagram.mmd | mergo -t forest`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Handle non-interactive modes first
			if outputFlag != "" || printFlag {
				return tui.ExportMode(ctx, args, tui.ExportOptions{
					Theme:      themeFlag,
					Output:     outputFlag,
					Scale:      scaleFlag,
					Renderer:   rendererFlag,
					Index:      indexFlag,
					Print:      printFlag,
					Font:       fontFlag,
					FontBold:   fontBoldFlag,
					NoShadows:  noShadowsFlag,
				})
			}

			// Interactive TUI mode
			return tui.Run(ctx, args, tui.Options{
				Theme:     themeFlag,
				Renderer:  rendererFlag,
				Font:      fontFlag,
				FontBold:  fontBoldFlag,
				NoShadows: noShadowsFlag,
			})
		},
	}

	root.Flags().StringVarP(&themeFlag, "theme", "t", "default",
		fmt.Sprintf("diagram theme (%v)", theme.Names()))
	root.Flags().StringVar(&rendererFlag, "renderer", "auto",
		"renderer: auto, kitty, or halfblock")
	root.Flags().StringVarP(&outputFlag, "output", "o", "",
		"write rendered diagram to PNG file and exit (non-interactive)")
	root.Flags().IntVar(&indexFlag, "index", 0,
		"diagram index for export/print (0-indexed)")
	root.Flags().BoolVarP(&printFlag, "print", "p", false,
		"print diagram to stdout sized to terminal width and exit (non-interactive)")
	root.Flags().Float64Var(&scaleFlag, "scale", 2,
		"render scale for export/print (default 2)")
	root.Flags().StringVar(&fontFlag, "font", "",
		"font path (e.g. /path/to/font.ttf)")
	root.Flags().StringVar(&fontBoldFlag, "font-bold", "",
		"bold font path (e.g. /path/to/font-bold.ttf)")
	root.Flags().BoolVar(&noShadowsFlag, "no-shadows", false,
		"disable drop shadows in rendered diagrams")

	if err := fang.Execute(ctx, root, fang.WithVersion(version)); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
