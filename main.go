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

	"github.com/mark3labs/mergo/internal/config"
	"github.com/mark3labs/mergo/internal/mermaid"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
	"github.com/mark3labs/mergo/internal/tui"
)

var version = "dev"

type flags struct {
	theme      string
	background string
	renderer   string
	placement  string
	output     string
	index      int
	print      bool
	scale      float64
	width      int
	font       string
	fontBold   string
	noShadows  bool
	noWatch    bool
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
			name, err := resolveTheme(cmd, f.theme, loadSettings(cmd))
			if err != nil {
				return err
			}
			f.theme = name
			return run(cmd.Context(), args, f)
		},
	}
	fl := cmd.Flags()
	fl.StringVarP(&f.theme, "theme", "t", "", "diagram theme for this run ("+strings.Join(theme.Names(), ", ")+"); the saved theme is used by default")
	fl.StringVar(&f.background, "background", "auto", "theme variant: auto (follow the terminal background), dark or light")
	fl.StringVarP(&f.renderer, "renderer", "r", "auto", "image renderer: auto, kitty or halfblock")
	fl.StringVar(&f.placement, "kitty-placement", "auto", "kitty image placement: auto, unicode (placeholders) or direct (zellij, WezTerm, Konsole)")
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
	_ = cmd.RegisterFlagCompletionFunc("background", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"auto", "dark", "light"}, cobra.ShellCompDirectiveNoFileComp
	})
	_ = cmd.RegisterFlagCompletionFunc("renderer", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"auto", "kitty", "halfblock"}, cobra.ShellCompDirectiveNoFileComp
	})
	_ = cmd.RegisterFlagCompletionFunc("kitty-placement", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"auto", "unicode", "direct"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.ValidArgsFunction = func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"mmd", "mermaid", "md", "markdown"}, cobra.ShellCompDirectiveFilterFileExt
	}
	cmd.AddCommand(typesCmd(), themesCmd())
	return cmd
}

func typesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "types",
		Short: "List supported diagram types and themes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var sb strings.Builder
			sb.WriteString("Diagram types:\n")
			for _, t := range mermaid.Types() {
				sb.WriteString("  • " + t + "\n")
			}
			sb.WriteString("\nThemes:\n")
			for _, t := range theme.Names() {
				sb.WriteString("  • " + t + "\n")
			}
			_, err := fmt.Fprint(cmd.OutOrStdout(), sb.String())
			return err
		},
	}
}

// defaultTheme is used when neither --theme nor a saved theme is set.
const defaultTheme = theme.DefaultName

// loadSettings reads the saved settings. Errors are only reported, so a
// broken config never prevents mergo from starting.
func loadSettings(cmd *cobra.Command) config.Settings {
	s, err := config.Load()
	if err != nil {
		cmd.PrintErrf("mergo: reading settings: %v\n", err)
		return config.Settings{}
	}
	return s
}

// resolveTheme picks the diagram theme: the --theme flag, else the saved
// one, else the default. A bad flag is an error; a bad saved value only a
// warning, so a stale config never prevents mergo from starting.
func resolveTheme(cmd *cobra.Command, flag string, s config.Settings) (string, error) {
	if flag != "" {
		if !theme.Valid(flag) {
			return "", fmt.Errorf("unknown theme %q (available: %s)", flag, strings.Join(theme.Names(), ", "))
		}
		return strings.ToLower(strings.TrimSpace(flag)), nil
	}
	if s.Theme == "" {
		return defaultTheme, nil
	}
	if !theme.Valid(s.Theme) {
		cmd.PrintErrf("mergo: unknown saved theme %q, using %s\n", s.Theme, defaultTheme)
		return defaultTheme, nil
	}
	return s.Theme, nil
}

// saveTheme persists the theme picked in the viewer.
func saveTheme(name string) error {
	return config.Update(func(s *config.Settings) { s.Theme = name })
}

func themesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "themes",
		Short: "List the available diagram themes",
		Long: "List the available diagram themes. The active one is marked with *.\n\n" +
			"Pick a theme in the viewer with t; the choice is saved to\n" +
			"$XDG_CONFIG_HOME/mergo/config.json. --theme overrides it for one run.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			current, err := resolveTheme(cmd, "", loadSettings(cmd))
			if err != nil {
				return err
			}
			var b strings.Builder
			for _, n := range theme.Names() {
				mark := "  "
				if n == current {
					mark = "* "
				}
				b.WriteString(mark + n + "\n")
			}
			if p, err := config.Path(); err == nil {
				b.WriteString("\nsettings: " + p + "\n")
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), b.String())
			return err
		},
	}
}

// darkBackground resolves --background. auto asks the terminal for its
// background color, like gopyter; without a terminal to ask it assumes dark.
func darkBackground(mode string) (bool, error) {
	switch mode {
	case "dark":
		return true, nil
	case "light":
		return false, nil
	case "auto", "":
	default:
		return false, fmt.Errorf("unknown background %q (want auto, dark or light)", mode)
	}
	return tui.DarkBackground(), nil
}

func run(ctx context.Context, args []string, f flags) error {
	// Probe before the TUI takes over the terminal, so the query can't race
	// the program's input reader.
	dark, err := darkBackground(f.background)
	if err != nil {
		return err
	}
	rend, ok := tui.ParseRenderer(f.renderer)
	if !ok {
		return fmt.Errorf("unknown renderer %q (want auto, kitty or halfblock)", f.renderer)
	}
	placement, ok := tui.ParsePlacement(f.placement)
	if !ok {
		return fmt.Errorf("unknown kitty placement %q (want auto, unicode or direct)", f.placement)
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
			if err := tui.Export(f.output, d, f.theme, dark, f.scale, f.noShadows); err != nil {
				return err
			}
			if f.output != "-" {
				fmt.Fprintln(os.Stderr, "wrote", f.output)
			}
		}
		if f.print {
			return tui.Print(os.Stdout, d, tui.PrintOptions{
				Theme: f.theme, Dark: dark, Renderer: rend, NoShadows: f.noShadows, Width: f.width,
			})
		}
		return nil
	}

	return tui.Run(ctx, paths, diagrams, tui.Options{
		Theme:     f.theme,
		Dark:      dark,
		Renderer:  rend,
		NoShadows: f.noShadows,
		Placement: placement,
		Watch:     !f.noWatch,
		SaveTheme: saveTheme,
	})
}
