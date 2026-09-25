// Package tui provides the terminal user interface for mergo.
package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/mermaid"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

// Options configures the TUI.
type Options struct {
	Theme     string
	Renderer  string
	Font      string
	FontBold  string
	NoShadows bool
}

// Model is the bubbletea model for the TUI.
type Model struct {
	// Input files and diagrams.
	files    []string
	diagrams []*diagram.Document // parsed diagrams
	index    int                  // currently displayed diagram

	// Rendering state.
	theme         *theme.Theme
	themeIndex    int // index into theme.Names()
	renderer      RendererType
	lastError     string
	lastErrorLine string

	// Scene caching.
	sceneCache map[string]*scene.Scene // keyed by (filename, diagram index, theme name)

	// Viewport and graphics.
	viewport Viewport
	graphics Graphics
	cellSize CellSize

	// File watching.
	lastModTime map[string]time.Time
	nextReload  time.Time

	// Rendering generation counter.
	renderGen    int
	pendingRender *RenderRequest

	// UI state.
	showHelp bool

	// Fonts.
	fontPath     string
	fontBoldPath string
	noShadows    bool
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	// Detect terminal capabilities and load diagrams.
	return tea.Batch(
		m.cmdDetectGraphics,
		m.cmdLoadDiagrams,
		m.cmdPollReload,
	)
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.viewport.Width = int(msg.Width)
		m.viewport.Height = int(msg.Height) - 3 // Leave room for status bar and tab bar
		return m, m.cmdRender()

	case CellSizeMsg:
		m.cellSize = CellSize(msg)
		m.graphics.UpdateCellSize(m.cellSize)
		return m, m.cmdRender()

	case GraphicsCapMsg:
		m.graphics.SetCapability(GraphicsCapability(msg))
		return m, m.cmdRender()

	case RenderResultMsg:
		if msg.Gen == m.renderGen {
			// This is the current generation, use the result
			m.graphics.SetImage(msg.PNG)
		}
		// Otherwise it's stale, ignore
		return m, nil

	case DiagramsLoadedMsg:
		m.diagrams = msg.Diagrams
		if len(m.diagrams) > 0 {
			m.index = 0
			return m, m.cmdRender()
		}
		m.lastError = "No diagrams found in input files"
		return m, nil

	case DiagramErrorMsg:
		m.lastError = msg.Error
		m.lastErrorLine = msg.Line
		return m, nil

	case ReloadMsg:
		// Check for file modifications
		changed := false
		for _, f := range m.files {
			if fi, err := os.Stat(f); err == nil {
				if lastMod, ok := m.lastModTime[f]; ok {
					if fi.ModTime().After(lastMod) {
						changed = true
						m.lastModTime[f] = fi.ModTime()
					}
				} else {
					m.lastModTime[f] = fi.ModTime()
				}
			}
		}
		if changed {
			return m, m.cmdLoadDiagrams
		}
		return m, m.cmdPollReload

	case tea.QuitMsg:
		return m, tea.Quit
	}

	return m, nil
}

// View implements tea.Model.
func (m *Model) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion

	// Render the diagram or error.
	content := m.graphics.Render(m.renderDiagram())

	// Build the full view with tabs, content, and status bar.
	// TODO: Implement full view layout
	v.Content = content

	return v
}

// handleKey processes keyboard input.
func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "+", "equal":
		m.viewport.ZoomIn()
		return m, m.cmdRender()

	case "-", "minus":
		m.viewport.ZoomOut()
		return m, m.cmdRender()

	case "0":
		m.viewport.Fit()
		return m, m.cmdRender()

	case "1":
		m.viewport.SetScale(1)
		return m, m.cmdRender()

	case "f":
		m.viewport.Fit()
		return m, m.cmdRender()

	case "tab":
		if len(m.diagrams) > 1 {
			m.index = (m.index + 1) % len(m.diagrams)
			return m, m.cmdRender()
		}

	case "shift+tab":
		if len(m.diagrams) > 1 {
			m.index = (m.index - 1 + len(m.diagrams)) % len(m.diagrams)
			return m, m.cmdRender()
		}

	case "t":
		m.themeIndex = (m.themeIndex + 1) % len(theme.Names())
		themeName := theme.Names()[m.themeIndex]
		th, err := theme.Get(themeName)
		if err == nil {
			m.theme = th
			m.sceneCache = make(map[string]*scene.Scene) // Clear cache
		}
		return m, m.cmdRender()

	case "r":
		m.graphics.ToggleRenderer()
		return m, m.cmdRender()

	case "R":
		return m, m.cmdLoadDiagrams

	case "s":
		return m, m.cmdSaveDiagram()

	case "?":
		m.showHelp = !m.showHelp
		return m, nil

	case "h", "left":
		m.viewport.PanLeft(10)
		return m, m.cmdRender()

	case "j", "down":
		m.viewport.PanDown(10)
		return m, m.cmdRender()

	case "k", "up":
		m.viewport.PanUp(10)
		return m, m.cmdRender()

	case "l", "right":
		m.viewport.PanRight(10)
		return m, m.cmdRender()
	}

	return m, nil
}

// renderDiagram returns the rendered PNG for the current diagram.
func (m *Model) renderDiagram() []byte {
	if m.index < 0 || m.index >= len(m.diagrams) {
		return nil
	}

	doc := m.diagrams[m.index]
	sceneKey := fmt.Sprintf("%s:%d:%s", doc.Source, m.index, m.theme.Name)

	// Check cache
	if sc, ok := m.sceneCache[sceneKey]; ok {
		// Render from cached scene
		opts := scene.RenderOptions{
			Scale:     1, // Will be scaled by graphics layer
			Viewport:  m.viewport.Rect(),
			NoShadows: m.noShadows,
		}
		return scene.RenderToPNG(sc.Render(opts))
	}

	// Need to render: this should be done off-thread via cmdRender
	return nil
}

// Command functions for off-thread work.

func (m *Model) cmdDetectGraphics() tea.Msg {
	// TODO: Implement graphics detection
	// Send query, wait for response or timeout
	return GraphicsCapMsg(CapAuto)
}

func (m *Model) cmdLoadDiagrams() tea.Msg {
	// TODO: Load diagrams from files
	return DiagramsLoadedMsg{Diagrams: []*diagram.Document{}}
}

func (m *Model) cmdPollReload() tea.Msg {
	time.Sleep(500 * time.Millisecond)
	return ReloadMsg{}
}

func (m *Model) cmdRender() tea.Cmd {
	return func() tea.Msg {
		// TODO: Render off-thread
		return nil
	}
}

func (m *Model) cmdSaveDiagram() tea.Cmd {
	return func() tea.Msg {
		// TODO: Save diagram as PNG
		return nil
	}
}

// Message types for the TUI event loop.

type GraphicsCapMsg int

type CellSizeMsg struct {
	Width  int
	Height int
}

type RenderRequest struct {
	DiagramIndex int
	Viewport     Viewport
	Gen          int
}

type RenderResultMsg struct {
	PNG []byte
	Gen int
}

type DiagramsLoadedMsg struct {
	Diagrams []*diagram.Document
}

type DiagramErrorMsg struct {
	Error string
	Line  string
}

type ReloadMsg struct{}

// Run starts the TUI.
func Run(ctx context.Context, args []string, opts Options) error {
	// Load files
	files, err := getInputFiles(args)
	if err != nil {
		return err
	}

	// Create initial model
	th, err := theme.Get(opts.Theme)
	if err != nil {
		th = theme.Default()
	}

	m := &Model{
		files:         files,
		theme:         th,
		themeIndex:    findThemeIndex(opts.Theme),
		sceneCache:    make(map[string]*scene.Scene),
		viewport:      NewViewport(80, 24),
		cellSize:      CellSize{Width: 8, Height: 16},
		lastModTime:   make(map[string]time.Time),
		fontPath:      opts.Font,
		fontBoldPath:  opts.FontBold,
		noShadows:     opts.NoShadows,
	}

	// Create and run the program
	p := tea.NewProgram(m)
	_, err = p.Run()
	return err
}

// ExportMode handles non-interactive export/print modes.
func ExportMode(ctx context.Context, args []string, opts ExportOptions) error {
	files, err := getInputFiles(args)
	if err != nil {
		return err
	}

	th, err := theme.Get(opts.Theme)
	if err != nil {
		th = theme.Default()
	}

	// Load diagrams
	diagrams, err := loadDiagrams(files)
	if err != nil {
		return err
	}

	if len(diagrams) == 0 {
		return fmt.Errorf("no diagrams found")
	}

	if opts.Index >= len(diagrams) {
		return fmt.Errorf("diagram index %d out of range", opts.Index)
	}

	doc := diagrams[opts.Index]

	// Render diagram to scene
	sc, err := mermaid.Render(doc.Source, th)
	if err != nil {
		return fmt.Errorf("render error: %w", err)
	}

	// Rasterize
	renderOpts := scene.RenderOptions{
		Scale: opts.Scale,
	}
	img := sc.Render(renderOpts)

	if opts.Output != "" {
		// Write PNG to file
		if err := writePNG(opts.Output, img); err != nil {
			return fmt.Errorf("write PNG: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Wrote %s\n", opts.Output)
	} else if opts.Print {
		// Print to stdout with half-block rendering
		// TODO: Implement printing
		fmt.Fprintf(os.Stderr, "Print mode not yet implemented\n")
	}

	return nil
}

// Helper functions.

func getInputFiles(args []string) ([]string, error) {
	if len(args) == 0 {
		// Read from stdin
		return []string{"-"}, nil
	}
	return args, nil
}

func findThemeIndex(name string) int {
	names := theme.Names()
	for i, n := range names {
		if n == name {
			return i
		}
	}
	return 0
}

func loadDiagrams(files []string) ([]*diagram.Document, error) {
	var diagrams []*diagram.Document

	for _, file := range files {
		var data []byte
		var err error

		if file == "-" {
			data, err = io.ReadAll(os.Stdin)
		} else {
			data, err = os.ReadFile(file)
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", file, err)
		}

		ext := filepath.Ext(file)
		if ext == ".md" || ext == ".markdown" {
			// Extract mermaid blocks
			blocks := ExtractMermaidBlocks(string(data))
			for _, block := range blocks {
				doc, err := diagram.Preprocess(block)
				if err != nil {
					// Skip invalid diagrams
					continue
				}
				diagrams = append(diagrams, doc)
			}
		} else {
			// Treat whole file as one diagram
			doc, err := diagram.Preprocess(string(data))
			if err != nil {
				return nil, fmt.Errorf("preprocess %s: %w", file, err)
			}
			diagrams = append(diagrams, doc)
		}
	}

	return diagrams, nil
}

// ExportOptions configures export/print modes.
type ExportOptions struct {
	Theme     string
	Output    string
	Scale     float64
	Renderer  string
	Index     int
	Print     bool
	Font      string
	FontBold  string
	NoShadows bool
}
