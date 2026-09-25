// Command mergo renders Mermaid diagrams natively in the terminal.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"charm.land/fang/v2"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/mark3labs/mergo/internal/mermaid"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
	"github.com/mark3labs/mergo/internal/tui"
)

var version = "dev"

type flags struct {
	theme     string
	renderer  string
	output    string
	index     int
	print     bool
	scale     float64
	width     int
	font      string
	fontBold  string
	noShadows bool
	noWatch   bool
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := fang.Execute(ctx, rootCmd(), fang.WithVersion(version)); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	var f flags
	cmd := &cobra.Command{
		Use:   "mergo [file ...]",
		Short: "Beautiful Mermaid diagrams in your terminal",
		Long: `mergo renders Mermaid diagrams natively in Go — no browser, no Node.js —
and displays them as real graphics using the kitty graphics protocol, falling
back to true-color half blocks on other terminals.

Inputs can be Mermaid files (.mmd, .mermaid), Markdown files (every
` + "```mermaid" + ` block becomes a diagram) or standard input.`,
		Example: `  # interactive viewer (tab switches between diagrams)
  mergo docs/architecture.md

  # pipe a diagram in
  echo 'graph LR; A-->B-->C' | mergo

  # print inline, like cat
  mergo -p flow.mmd

  # export a PNG
  mergo -o flow.png -t dark flow.mmd`,
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), args, f)
		},
	}
	fl := cmd.Flags()
	fl.StringVarP(&f.theme, "theme", "t", "default", "diagram theme ("+strings.Join(theme.Names(), ", ")+")")
	fl.StringVarP(&f.renderer, "renderer", "r", "auto", "image renderer: auto, kitty or halfblock")
	fl.StringVarP(&f.output, "output", "o", "", "export the diagram as PNG to this path and exit (- for stdout)")
	fl.BoolVarP(&f.print, "print", "p", false, "print the diagram inline and exit")
	fl.IntVarP(&f.index, "index", "i", 1, "which diagram to print/export when the input has several (1-based)")
	fl.Float64Var(&f.scale, "scale", 2, "pixel scale for PNG export")
	fl.IntVarP(&f.width, "width", "w", 0, "width in cells for --print (default: terminal width)")
	fl.StringVar(&f.font, "font", "", "TrueType/OpenType font file to use for labels")
	fl.StringVar(&f.fontBold, "font-bold", "", "bold font file (defaults to --font)")
	fl.BoolVar(&f.noShadows, "no-shadows", false, "disable drop shadows")
	fl.BoolVar(&f.noWatch, "no-watch", false, "don't reload when input files change")

	_ = cmd.RegisterFlagCompletionFunc("theme", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return theme.Names(), cobra.ShellCompDirectiveNoFileComp
	})
	_ = cmd.RegisterFlagCompletionFunc("renderer", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"auto", "kitty", "halfblock"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.ValidArgsFunction = func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"mmd", "mermaid", "md", "markdown"}, cobra.ShellCompDirectiveFilterFileExt
	}
	cmd.AddCommand(typesCmd())
	return cmd
}

func typesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "types",
		Short: "List supported diagram types and themes",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintln(cmd.OutOrStdout(), "Diagram types:")
			for _, t := range mermaid.Types() {
				fmt.Fprintln(cmd.OutOrStdout(), "  •", t)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "\nThemes:")
			for _, t := range theme.Names() {
				fmt.Fprintln(cmd.OutOrStdout(), "  •", t)
			}
		},
	}
}

func run(ctx context.Context, args []string, f flags) error {
	if _, err := theme.Get(f.theme); err != nil {
		return err
	}
	rend, ok := tui.ParseRenderer(f.renderer)
	if !ok {
		return fmt.Errorf("unknown renderer %q (want auto, kitty or halfblock)", f.renderer)
	}
	if f.font != "" || f.fontBold != "" {
		if err := scene.LoadFonts(f.font, f.fontBold); err != nil {
			return err
		}
	}

	paths := args
	if len(paths) == 0 {
		if term.IsTerminal(os.Stdin.Fd()) {
			return errors.New("no input: pass a .mmd/.md file or pipe a diagram on stdin (see --help)")
		}
		paths = []string{"-"}
	}
	diagrams, err := tui.LoadInputs(paths, os.Stdin)
	if err != nil {
		return err
	}
	if len(diagrams) == 0 {
		return errors.New("no mermaid diagrams found in input")
	}

	if f.output != "" || f.print {
		if f.index < 1 || f.index > len(diagrams) {
			return fmt.Errorf("--index %d out of range (input has %d diagram(s))", f.index, len(diagrams))
		}
		d := diagrams[f.index-1]
		if f.output != "" {
			if err := tui.Export(f.output, d, f.theme, f.scale, f.noShadows); err != nil {
				return err
			}
			if f.output != "-" {
				fmt.Fprintln(os.Stderr, "wrote", f.output)
			}
		}
		if f.print {
			return tui.Print(os.Stdout, d, tui.PrintOptions{
				Theme: f.theme, Renderer: rend, NoShadows: f.noShadows, Width: f.width,
			})
		}
		return nil
	}

	return tui.Run(ctx, paths, diagrams, tui.Options{
		Theme:     f.theme,
		Renderer:  rend,
		NoShadows: f.noShadows,
		Watch:     !f.noWatch,
	})
}
