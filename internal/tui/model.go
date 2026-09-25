package tui

import (
	"fmt"
	"image"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/mark3labs/mergo/internal/diagram"
	"github.com/mark3labs/mergo/internal/scene"
	"github.com/mark3labs/mergo/internal/theme"
)

// Options configures the interactive viewer.
type Options struct {
	Theme string
	// Dark selects the dark variant of the themes (the terminal has a dark
	// background).
	Dark      bool
	Renderer  Renderer
	NoShadows bool
	// Placement selects how kitty images are positioned (auto picks direct
	// placements inside zellij and in terminals without Unicode
	// placeholder support).
	Placement Placement
	// Watch enables auto reload when source files change.
	Watch bool
	// SaveTheme persists a theme picked in the viewer (optional).
	SaveTheme func(name string) error
}

// Messages ------------------------------------------------------------------

type sceneMsg struct {
	key   sceneKey
	entry sceneEntry
}

type renderTickMsg struct{ gen int }

type renderDoneMsg renderResult

type probeTimeoutMsg struct{}

// sixelPaintMsg paints a sixel image once the frame that clears its area
// has been written.
type sixelPaintMsg struct {
	gen int
	sig string
	raw string
}

type pollMsg struct{}

type reloadMsg struct {
	diagrams []*Diagram
	mtimes   map[string]time.Time
	err      error
}

type toastExpireMsg struct{ id int }

type savedMsg struct {
	path string
	err  error
}

const (
	probeImageID   = 31337
	renderDebounce = 25 * time.Millisecond
	pollInterval   = 500 * time.Millisecond
	probeTimeout   = 1500 * time.Millisecond
	// sixelSettle is how long to wait before painting a sixel image over
	// a body that changed in the same update: the renderer writes raw
	// output before the frame, so painting right away would have the
	// frame's blank cells erase the image. A few frames at 60 fps.
	sixelSettle = 40 * time.Millisecond
)

// Model is the bubbletea model of the viewer.
type Model struct {
	opts     Options
	paths    []string
	diagrams []*Diagram
	current  int

	themes   []string
	themeIdx int
	picker   themePicker

	width, height int
	cell          CellSize
	cellFromTerm  bool

	// graphics capability
	probing  bool
	kittyOK  bool
	mode     Renderer // effective renderer (kitty or half-block) once known
	tmux     bool
	idBase   int
	idCursor int
	usedIDs  map[int]bool
	// placement is the resolved kitty placement strategy; shownID is the
	// image currently displayed with a direct placement (0 = none).
	placement Placement
	shownID   int

	// sixel: sixelOK is set when the device attributes advertise sixel
	// graphics, sixelSig is the overlay layout the latest sixel render was
	// requested for and sixelShown whether an image may be on screen.
	sixelOK    bool
	sixelSig   string
	sixelShown bool

	cams    map[int]*camera
	scenes  map[sceneKey]sceneEntry
	pending map[sceneKey]bool
	// lastGood remembers the last successfully parsed scene per diagram so
	// that a broken edit keeps showing the previous render.
	lastGood map[int]*scene.Scene

	gen        int
	appliedGen int
	body       []string
	bodyMode   Renderer
	renderErr  error

	mtimes map[string]time.Time

	dragging     bool
	dragX, dragY int

	showHelp bool
	help     help.Model
	keys     keyMap
	st       styles

	toast    string
	toastErr bool
	toastID  int
}

// NewModel creates the viewer model.
func NewModel(paths []string, diagrams []*Diagram, opts Options) *Model {
	names := theme.Names()
	themeIdx := 0
	for i, n := range names {
		if n == opts.Theme {
			themeIdx = i
		}
	}
	h := help.New()
	m := &Model{
		opts:      opts,
		paths:     paths,
		diagrams:  diagrams,
		themes:    names,
		themeIdx:  themeIdx,
		cell:      terminalCellSize(),
		tmux:      inTmux() && !inZellij(nil),
		placement: resolvePlacement(opts.Placement, nil),
		idBase:    imageIDBase(),
		usedIDs:   map[int]bool{},
		cams:      map[int]*camera{},
		scenes:    map[sceneKey]sceneEntry{},
		pending:   map[sceneKey]bool{},
		lastGood:  map[int]*scene.Scene{},
		mtimes:    modTimes(paths),
		help:      h,
		keys:      defaultKeys(),
	}
	m.restyle()
	switch opts.Renderer {
	case RendererKitty:
		m.mode = RendererKitty
		m.kittyOK = true
	case RendererSixel:
		m.mode = RendererSixel
		m.sixelOK = true
	case RendererHalfBlock:
		m.mode = RendererHalfBlock
	default:
		m.probing = true
	}
	return m
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		tea.Raw("\x1b[16t"), // request cell size in pixels
		m.ensureScene(),
	}
	if m.probing {
		q := kittyQuery(probeImageID)
		if m.tmux {
			q = wrapTmux(q)
		}
		cmds = append(cmds,
			tea.Raw(q+"\x1b[c"), // kitty query followed by DA1
			tea.Tick(probeTimeout, func(time.Time) tea.Msg { return probeTimeoutMsg{} }),
		)
	}
	if m.opts.Watch {
		cmds = append(cmds, m.poll())
	}
	return tea.Batch(cmds...)
}

func (m *Model) poll() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return pollMsg{} })
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	model, cmd := m.update(msg)
	// A sixel image is part of the cell contents: when overlays appear,
	// move or disappear it has to be painted again around them.
	if m.mode == RendererSixel && !m.probing && m.width > 0 && m.height > 0 {
		if sig := m.overlaySig(); sig != m.sixelSig {
			m.sixelSig = sig
			cmd = tea.Batch(cmd, m.requestRender())
		}
	}
	return model, cmd
}

func (m *Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(msg.Width)
		if !m.cellFromTerm {
			m.cell = terminalCellSize()
		}
		return m, m.requestRender()

	case uv.CellSizeEvent:
		cs := CellSize{W: msg.Width, H: msg.Height}
		if cs.Valid() && cs != m.cell {
			m.cell = cs
			m.cellFromTerm = true
			return m, m.requestRender()
		}
		m.cellFromTerm = cs.Valid()
		return m, nil

	case uv.KittyGraphicsEvent:
		if msg.Options.ID == probeImageID && strings.HasPrefix(string(msg.Payload), "OK") {
			m.kittyOK = true
			if m.probing {
				m.probing = false
				m.mode = RendererKitty
				return m, m.requestRender()
			}
		}
		return m, nil

	case uv.PrimaryDeviceAttributesEvent:
		if hasSixel(msg) {
			m.sixelOK = true
		}
		if m.probing {
			m.finishProbe()
			return m, m.requestRender()
		}
		return m, nil

	case probeTimeoutMsg:
		if m.probing {
			m.finishProbe()
			return m, m.requestRender()
		}
		return m, nil

	case sceneMsg:
		delete(m.pending, msg.key)
		m.scenes[msg.key] = msg.entry
		if msg.entry.err == nil {
			m.lastGood[msg.key.diag] = msg.entry.sc
		}
		if msg.key == m.currentKey() {
			return m, m.requestRender()
		}
		return m, nil

	case renderTickMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		return m, m.startRender()

	case renderDoneMsg:
		if msg.gen <= m.appliedGen {
			return m, nil
		}
		if msg.mode != m.mode {
			return m, nil // rendered for a renderer that was switched away from
		}
		if msg.mode == RendererSixel && msg.err == nil && msg.sig != m.overlaySig() {
			// stale: painting it would cover overlays (a newer render
			// is pending)
			return m, nil
		}
		m.appliedGen = msg.gen
		m.renderErr = msg.err
		if msg.err != nil {
			return m, nil
		}
		if msg.mode == RendererSixel {
			return m, m.applySixel(renderResult(msg))
		}
		m.body = msg.lines
		m.bodyMode = msg.mode
		raw := msg.raw
		if msg.mode == RendererKitty && m.placement == PlacementDirect {
			// the new image is placed; now drop the previous one
			if m.shownID != 0 && m.shownID != msg.imgID {
				raw += kittyDelete(m.shownID, m.tmux)
			}
			m.shownID = msg.imgID
		}
		if raw != "" {
			return m, tea.Raw(raw)
		}
		return m, nil

	case sixelPaintMsg:
		if msg.gen == m.appliedGen && m.mode == RendererSixel && msg.sig == m.overlaySig() {
			m.sixelShown = true
			return m, tea.Raw(msg.raw)
		}
		return m, nil

	case pollMsg:
		return m, tea.Batch(m.checkReload(), m.poll())

	case reloadMsg:
		if msg.err != nil {
			return m, m.showError("reload failed: " + msg.err.Error())
		}
		m.applyReload(msg)
		return m, tea.Batch(m.ensureScene(), m.requestRender())

	case savedMsg:
		if msg.err != nil {
			return m, m.showError("save failed: " + msg.err.Error())
		}
		return m, m.showToast("saved " + msg.path)

	case toastExpireMsg:
		if msg.id == m.toastID {
			m.toast = ""
		}
		return m, nil

	case tea.KeyPressMsg:
		if m.picker.open {
			return m, m.handleThemeKey(msg)
		}
		return m.handleKey(msg)

	case tea.MouseClickMsg, tea.MouseWheelMsg:
		if m.picker.open {
			return m, m.handleThemeMouse(msg)
		}
	}

	switch msg := msg.(type) {

	case tea.MouseWheelMsg:
		return m.handleWheel(msg.Mouse())

	case tea.MouseClickMsg:
		mo := msg.Mouse()
		if mo.Button == tea.MouseLeft {
			m.dragging = true
			m.dragX, m.dragY = mo.X, mo.Y
		}
		return m, nil

	case tea.MouseMotionMsg:
		mo := msg.Mouse()
		if m.dragging && mo.Button == tea.MouseLeft {
			dx, dy := mo.X-m.dragX, mo.Y-m.dragY
			m.dragX, m.dragY = mo.X, mo.Y
			if cam := m.cam(); cam != nil && (dx != 0 || dy != 0) {
				cam.Pan(-float64(dx*m.cell.W), -float64(dy*m.cell.H))
				m.clampCam()
				return m, m.requestRender()
			}
		}
		return m, nil

	case tea.MouseReleaseMsg:
		m.dragging = false
		return m, nil
	}
	return m, nil
}

func (m *Model) finishProbe() {
	m.probing = false
	switch {
	case m.kittyOK:
		m.mode = RendererKitty
	case m.sixelOK:
		m.mode = RendererSixel
	default:
		m.mode = RendererHalfBlock
	}
}

// applySixel shows a finished sixel render. The image is painted over a
// grid of blank cells; if the body showed something else until now, the
// frame replacing it with blanks must reach the terminal first.
func (m *Model) applySixel(res renderResult) tea.Cmd {
	blank := m.bodyMode == RendererSixel && len(m.body) == len(res.lines) &&
		(len(m.body) == 0 || m.body[0] == res.lines[0])
	m.body = res.lines
	m.bodyMode = RendererSixel
	if blank {
		m.sixelShown = true
		return tea.Raw(res.raw)
	}
	gen, sig, raw := res.gen, res.sig, res.raw
	return tea.Tick(sixelSettle, func(time.Time) tea.Msg { return sixelPaintMsg{gen: gen, sig: sig, raw: raw} })
}

// eraseSixel removes a sixel image from the body (the cell renderer
// doesn't know about it), sparing the overlays.
func (m *Model) eraseSixel() tea.Cmd {
	if !m.sixelShown {
		return nil
	}
	m.sixelShown = false
	cols, rows := m.bodySize()
	return tea.Raw(eraseCells(m.bodyTop(), cols, rows, m.overlayRects()))
}

func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := m.keys
	cam := m.cam()
	pw, ph := m.viewPx()
	switch {
	case key.Matches(msg, k.Quit):
		return m, tea.Sequence(tea.Raw(m.cleanup()), tea.Quit)
	case key.Matches(msg, k.Help):
		m.showHelp = !m.showHelp
		return m, nil
	case key.Matches(msg, k.Next):
		return m, m.switchDiagram(1)
	case key.Matches(msg, k.Prev):
		return m, m.switchDiagram(-1)
	case key.Matches(msg, k.Theme):
		return m, m.openThemePicker()
	case key.Matches(msg, k.Renderer):
		return m, m.toggleRenderer()
	case key.Matches(msg, k.Reload):
		return m, m.forceReload()
	case key.Matches(msg, k.Save):
		return m, m.save()
	}
	if cam == nil {
		return m, nil
	}
	step := 0.12
	switch {
	case key.Matches(msg, k.ZoomIn):
		cam.Zoom(zoomStep, pw/2, ph/2, pw, ph)
	case key.Matches(msg, k.ZoomOut):
		cam.Zoom(1/zoomStep, pw/2, ph/2, pw, ph)
	case key.Matches(msg, k.Fit):
		if sc := m.currentScene(); sc != nil {
			cam.Fit(sc.Width, sc.Height, pw, ph)
		}
	case key.Matches(msg, k.Actual):
		cam.SetZoom(1)
	case key.Matches(msg, k.Left):
		cam.Pan(-pw*step, 0)
	case key.Matches(msg, k.Right):
		cam.Pan(pw*step, 0)
	case key.Matches(msg, k.Up):
		cam.Pan(0, -ph*step)
	case key.Matches(msg, k.Down):
		cam.Pan(0, ph*step)
	default:
		return m, nil
	}
	m.clampCam()
	return m, m.requestRender()
}

func (m *Model) handleWheel(mo tea.Mouse) (tea.Model, tea.Cmd) {
	cam := m.cam()
	if cam == nil {
		return m, nil
	}
	pw, ph := m.viewPx()
	top := m.bodyTop()
	ax := float64(mo.X*m.cell.W) + float64(m.cell.W)/2
	ay := float64((mo.Y-top)*m.cell.H) + float64(m.cell.H)/2
	switch mo.Button {
	case tea.MouseWheelUp:
		if mo.Mod.Contains(tea.ModShift) {
			cam.Pan(0, -ph*0.08)
		} else {
			cam.Zoom(1.1, ax, ay, pw, ph)
		}
	case tea.MouseWheelDown:
		if mo.Mod.Contains(tea.ModShift) {
			cam.Pan(0, ph*0.08)
		} else {
			cam.Zoom(1/1.1, ax, ay, pw, ph)
		}
	case tea.MouseWheelLeft:
		cam.Pan(-pw*0.08, 0)
	case tea.MouseWheelRight:
		cam.Pan(pw*0.08, 0)
	default:
		return m, nil
	}
	m.clampCam()
	return m, m.requestRender()
}

// ---------------------------------------------------------------------------
// state helpers

func (m *Model) themeName() string { return m.themes[m.themeIdx] }

// restyle rebuilds the chrome styles from the active theme's UI palette.
func (m *Model) restyle() {
	p, err := theme.Palette(m.themeName(), m.opts.Dark)
	if err != nil {
		p, _ = theme.Palette(theme.DefaultName, m.opts.Dark)
	}
	m.st = newStyles(p)
	m.help.Styles = m.st.help
}

func (m *Model) currentDiagram() *Diagram {
	if m.current < 0 || m.current >= len(m.diagrams) {
		return nil
	}
	return m.diagrams[m.current]
}

func (m *Model) currentKey() sceneKey {
	d := m.currentDiagram()
	if d == nil {
		return sceneKey{}
	}
	return sceneKey{diag: m.current, hash: hashSource(d.Source), theme: m.themeName()}
}

// currentScene returns the scene to display: the current parse if it
// succeeded, otherwise the last good scene of this diagram.
func (m *Model) currentScene() *scene.Scene {
	if e, ok := m.scenes[m.currentKey()]; ok && e.err == nil {
		return e.sc
	}
	return m.lastGood[m.current]
}

// currentErr returns the parse error of the current diagram (if any).
func (m *Model) currentErr() error {
	if e, ok := m.scenes[m.currentKey()]; ok {
		return e.err
	}
	return nil
}

func (m *Model) cam() *camera {
	if m.currentDiagram() == nil {
		return nil
	}
	c, ok := m.cams[m.current]
	if !ok {
		c = &camera{}
		m.cams[m.current] = c
	}
	return c
}

// bodyTop is the first screen row of the image area.
func (m *Model) bodyTop() int { return 1 }

// bodySize returns the image area in cells.
func (m *Model) bodySize() (int, int) {
	return max(m.width, 0), max(m.height-2, 0)
}

// viewPx returns the image area in terminal pixels.
func (m *Model) viewPx() (float64, float64) {
	c, r := m.bodySize()
	return viewPixels(c, r, m.cell)
}

func (m *Model) clampCam() {
	cam := m.cam()
	sc := m.currentScene()
	if cam == nil || sc == nil {
		return
	}
	pw, ph := m.viewPx()
	cam.Clamp(sc.Width, sc.Height, pw, ph)
}

// ensureScene starts parsing the current diagram if needed.
func (m *Model) ensureScene() tea.Cmd {
	d := m.currentDiagram()
	if d == nil {
		return nil
	}
	k := m.currentKey()
	if _, ok := m.scenes[k]; ok || m.pending[k] {
		return nil
	}
	m.pending[k] = true
	src, th, dark := d.Source, k.theme, m.opts.Dark
	return func() tea.Msg {
		sc, err := parseScene(src, th, dark)
		return sceneMsg{key: k, entry: sceneEntry{sc: sc, err: err}}
	}
}

// requestRender schedules a (debounced) re-render.
func (m *Model) requestRender() tea.Cmd {
	m.gen++
	g := m.gen
	return tea.Tick(renderDebounce, func(time.Time) tea.Msg { return renderTickMsg{gen: g} })
}

func (m *Model) nextImageID() int {
	// rotate through 3 ids so the id being replaced is never the one on screen
	m.idCursor = (m.idCursor + 1) % 3
	id := m.idBase + m.idCursor
	m.usedIDs[id] = true
	return id
}

func (m *Model) startRender() tea.Cmd {
	if m.probing || m.width == 0 || m.height == 0 {
		return nil
	}
	sc := m.currentScene()
	cam := m.cam()
	if sc == nil || cam == nil {
		// nothing to show: a direct placement would otherwise linger over
		// the error panel / placeholder text
		m.body = nil
		return tea.Batch(m.hideDirect(), m.eraseSixel())
	}
	cols, rows := m.bodySize()
	pw, ph := viewPixels(cols, rows, m.cell)
	if !cam.init || cam.fit {
		cam.Fit(sc.Width, sc.Height, pw, ph)
	}
	cam.Clamp(sc.Width, sc.Height, pw, ph)
	job := renderJob{
		gen:       m.gen,
		sc:        sc,
		mode:      m.mode,
		cols:      cols,
		rows:      rows,
		cell:      m.cell,
		cam:       *cam,
		noShadows: m.opts.NoShadows,
		tmux:      m.tmux,
		placement: m.placement,
		top:       m.bodyTop(),
	}
	switch m.mode {
	case RendererKitty:
		job.imgID = m.nextImageID()
	case RendererSixel:
		job.sig = m.overlaySig()
		m.sixelSig = job.sig
		top := m.bodyTop()
		for _, r := range m.overlayRects() {
			job.mask = append(job.mask, image.Rect(
				r.Min.X*m.cell.W, (r.Min.Y-top)*m.cell.H,
				r.Max.X*m.cell.W, (r.Max.Y-top)*m.cell.H))
		}
	}
	return func() tea.Msg { return renderDoneMsg(job.run()) }
}

func (m *Model) switchDiagram(delta int) tea.Cmd {
	if len(m.diagrams) < 2 {
		return nil
	}
	m.current = (m.current + delta + len(m.diagrams)) % len(m.diagrams)
	return tea.Batch(m.ensureScene(), m.requestRender())
}

// toggleRenderer cycles through the renderers the terminal supports:
// kitty, sixel and half blocks.
func (m *Model) toggleRenderer() tea.Cmd {
	if m.probing {
		return nil
	}
	var modes []Renderer
	if m.kittyOK {
		modes = append(modes, RendererKitty)
	}
	if m.sixelOK {
		modes = append(modes, RendererSixel)
	}
	modes = append(modes, RendererHalfBlock)
	if len(modes) == 1 {
		return m.showError("no terminal graphics support (kitty or sixel)")
	}
	next := modes[(slices.Index(modes, m.mode)+1)%len(modes)]
	var cmds []tea.Cmd
	switch m.mode {
	case RendererKitty:
		cmds = append(cmds, tea.Raw(m.cleanup()))
	case RendererSixel:
		cmds = append(cmds, m.eraseSixel())
	}
	m.mode = next
	return tea.Batch(append(cmds, m.requestRender(), m.showToast("renderer: "+next.String()))...)
}

// cleanup returns escape sequences deleting all transmitted images.
func (m *Model) cleanup() string {
	var sb strings.Builder
	for id := range m.usedIDs {
		sb.WriteString(kittyDelete(id, m.tmux))
	}
	m.usedIDs = map[int]bool{}
	m.shownID = 0
	return sb.String()
}

// hideDirect removes the currently shown direct placement (if any).
func (m *Model) hideDirect() tea.Cmd {
	if m.shownID == 0 {
		return nil
	}
	id := m.shownID
	m.shownID = 0
	return tea.Raw(kittyDelete(id, m.tmux))
}

func (m *Model) showToast(s string) tea.Cmd { return m.toastMsg(s, false) }

// showError shows a toast in the error style.
func (m *Model) showError(s string) tea.Cmd { return m.toastMsg(s, true) }

func (m *Model) toastMsg(s string, isErr bool) tea.Cmd {
	m.toastID++
	m.toast, m.toastErr = s, isErr
	id := m.toastID
	return tea.Tick(2500*time.Millisecond, func(time.Time) tea.Msg { return toastExpireMsg{id: id} })
}

func (m *Model) save() tea.Cmd {
	d := m.currentDiagram()
	sc := m.currentScene()
	if d == nil || sc == nil {
		return m.showToast("nothing to save")
	}
	path := savePath(d)
	noShadows := m.opts.NoShadows
	return func() tea.Msg {
		img := sc.Render(scene.RenderOptions{Scale: 2, NoShadows: noShadows})
		data, err := encodePNG(img)
		if err == nil {
			err = os.WriteFile(path, data, 0o644)
		}
		return savedMsg{path: path, err: err}
	}
}

// checkReload compares file modification times.
func (m *Model) checkReload() tea.Cmd {
	cur := modTimes(m.paths)
	changed := false
	for p, t := range cur {
		if old, ok := m.mtimes[p]; !ok || !t.Equal(old) {
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return m.loadCmd(cur)
}

func (m *Model) forceReload() tea.Cmd {
	return tea.Batch(m.loadCmd(modTimes(m.paths)), m.showToast("reloaded"))
}

func (m *Model) loadCmd(mt map[string]time.Time) tea.Cmd {
	var files []string
	for _, p := range m.paths {
		if p != "-" {
			files = append(files, p)
		}
	}
	if len(files) == 0 {
		return nil
	}
	return func() tea.Msg {
		ds, err := LoadInputs(files, nil)
		return reloadMsg{diagrams: ds, mtimes: mt, err: err}
	}
}

// applyReload replaces diagrams from files that were re-read. Diagrams
// from stdin are kept as they are.
func (m *Model) applyReload(msg reloadMsg) {
	m.mtimes = msg.mtimes
	var out []*Diagram
	byPath := map[string][]*Diagram{}
	for _, d := range msg.diagrams {
		byPath[d.Path] = append(byPath[d.Path], d)
	}
	for _, p := range m.paths {
		if p == "-" {
			for _, d := range m.diagrams {
				if d.Path == "-" {
					out = append(out, d)
				}
			}
			continue
		}
		out = append(out, byPath[p]...)
	}
	m.diagrams = out
	if m.current >= len(out) {
		m.current = max(len(out)-1, 0)
	}
}

// ---------------------------------------------------------------------------
// View

// View implements tea.Model.
func (m *Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = m.windowTitle()
	return v
}

func (m *Model) windowTitle() string {
	if d := m.currentDiagram(); d != nil {
		return "mergo — " + d.Name()
	}
	return "mergo"
}

func (m *Model) render() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	cols, rows := m.bodySize()
	layers := []*lipgloss.Layer{
		lipgloss.NewLayer(m.headerView()).X(0).Y(0),
		lipgloss.NewLayer(m.bodyView(cols, rows)).X(0).Y(m.bodyTop()),
		lipgloss.NewLayer(m.statusView()).X(0).Y(m.height - 1),
	}
	for _, o := range m.overlays() {
		layers = append(layers, lipgloss.NewLayer(o.content).X(o.x).Y(o.y).Z(o.z))
	}
	canvas := lipgloss.NewCanvas(m.width, m.height)
	return canvas.Compose(lipgloss.NewCompositor(layers...)).Render()
}

// overlay is a panel drawn over the body.
type overlay struct {
	content string
	x, y, z int
}

// overlays returns the panels currently drawn over the body: error panel,
// toast, help and theme picker.
func (m *Model) overlays() []overlay {
	cols, rows := m.bodySize()
	var out []overlay
	if err := m.displayError(); err != nil {
		box := m.errorView(err, cols)
		bh := lipgloss.Height(box)
		out = append(out, overlay{box, max((cols-lipgloss.Width(box))/2, 0), m.bodyTop() + max(rows-bh-1, 0), 2})
	}
	if m.toast != "" {
		st := m.st.toast
		if m.toastErr {
			st = m.st.toastErr
		}
		t := st.Render(m.toast)
		out = append(out, overlay{t, max(cols-lipgloss.Width(t)-1, 0), m.bodyTop() + 1, 3})
	}
	if m.showHelp {
		h := m.st.helpBox.Render(fillPanel(m.help.FullHelpView(m.keys.FullHelp()), m.st.panelBg))
		out = append(out, overlay{h,
			max((cols-lipgloss.Width(h))/2, 0),
			m.bodyTop() + max((rows-lipgloss.Height(h))/2, 0), 4})
	}
	if m.picker.open {
		box := m.themePickerView(rows)
		bw, bh := lipgloss.Width(box), lipgloss.Height(box)
		x, y := max((cols-bw)/2, 0), m.bodyTop()+max((rows-bh)/2, 0)
		m.placeThemePicker(x, y, bw, bh)
		out = append(out, overlay{box, x, y, 5})
	}
	return out
}

// overlayRects returns the screen cells covered by overlays, clipped to
// the body.
func (m *Model) overlayRects() []image.Rectangle {
	cols, rows := m.bodySize()
	body := image.Rect(0, m.bodyTop(), cols, m.bodyTop()+rows)
	var out []image.Rectangle
	for _, o := range m.overlays() {
		r := image.Rect(o.x, o.y, o.x+lipgloss.Width(o.content), o.y+lipgloss.Height(o.content)).Intersect(body)
		if !r.Empty() {
			out = append(out, r)
		}
	}
	return out
}

// overlaySig identifies the overlay layout (and the body size), so that a
// sixel image is repainted whenever the cells it must spare change.
func (m *Model) overlaySig() string {
	cols, rows := m.bodySize()
	var sb strings.Builder
	fmt.Fprintf(&sb, "%dx%d", cols, rows)
	for _, r := range m.overlayRects() {
		fmt.Fprintf(&sb, ";%d,%d,%d,%d", r.Min.X, r.Min.Y, r.Max.X, r.Max.Y)
	}
	return sb.String()
}

func (m *Model) displayError() error {
	if err := m.currentErr(); err != nil {
		return err
	}
	return m.renderErr
}

func (m *Model) bodyView(cols, rows int) string {
	if m.currentDiagram() == nil {
		return lipgloss.Place(cols, rows, lipgloss.Center, lipgloss.Center,
			m.st.placeholder.Render("no mermaid diagrams found"))
	}
	if m.probing {
		return lipgloss.Place(cols, rows, lipgloss.Center, lipgloss.Center,
			m.st.placeholder.Render("detecting terminal graphics…"))
	}
	if len(m.body) == 0 {
		msg := "rendering…"
		if m.currentScene() == nil && m.currentErr() != nil {
			msg = ""
		}
		return lipgloss.Place(cols, rows, lipgloss.Center, lipgloss.Center, m.st.placeholder.Render(msg))
	}
	return strings.Join(m.body, "\n")
}

func (m *Model) headerView() string {
	logo := m.st.logo.Render("mergo")
	var parts []string
	used := lipgloss.Width(logo) + 1
	if len(m.diagrams) > 1 {
		for i, d := range m.diagrams {
			label := fmt.Sprintf("%d %s", i+1, d.Name())
			st := m.st.tab
			if i == m.current {
				st = m.st.tabActive
			}
			t := st.Render(label)
			if used+lipgloss.Width(t) > m.width-4 && i != m.current {
				if i > m.current {
					parts = append(parts, m.st.tab.Render("…"))
					break
				}
				continue
			}
			used += lipgloss.Width(t)
			parts = append(parts, t)
		}
	} else if d := m.currentDiagram(); d != nil {
		parts = append(parts, m.st.tabActive.Render(d.Name()))
	}
	line := logo + m.st.bar.Render(" ") + strings.Join(parts, "")
	return m.st.bar.Width(m.width).MaxWidth(m.width).Render(line)
}

func (m *Model) statusView() string {
	st := m.st
	sep := st.statusDim.Render(" │ ")
	var left []string
	if d := m.currentDiagram(); d != nil {
		kind := d.Kind()
		left = append(left, st.kindPill.Render(kind))
		left = append(left, st.statusVal.Render(" "+d.Name()))
		if len(m.diagrams) > 1 {
			left = append(left, st.statusKey.Render(fmt.Sprintf("  %d/%d", m.current+1, len(m.diagrams))))
		}
	}
	var right []string
	if cam := m.cam(); cam != nil && cam.init {
		right = append(right, st.statusKey.Render("zoom ")+st.statusVal.Render(fmt.Sprintf("%d%%", int(cam.zoom*100+0.5))))
	}
	mode := m.mode.String()
	if m.mode == RendererKitty && m.placement == PlacementDirect {
		mode = "kitty·direct"
	}
	if m.probing {
		mode = "probing"
	}
	modePill := st.kittyPill
	if m.mode == RendererHalfBlock {
		modePill = st.blockPill
	}
	right = append(right, st.statusKey.Render("theme ")+st.statusVal.Render(m.themeName()))
	right = append(right, modePill.Render(mode))
	right = append(right, st.statusKey.Render("? help"))
	l := strings.Join(left, "")
	r := strings.Join(right, sep) + st.bar.Render(" ")
	gap := m.width - lipgloss.Width(l) - lipgloss.Width(r)
	if gap < 1 {
		// drop the right side progressively when narrow
		r = modePill.Render(mode)
		gap = max(m.width-lipgloss.Width(l)-lipgloss.Width(r), 0)
	}
	return st.bar.Width(m.width).MaxWidth(m.width).Render(l + st.bar.Render(strings.Repeat(" ", gap)) + r)
}

var lineRe = regexp.MustCompile(`(?i)\bline (\d+)`)

func (m *Model) errorView(err error, cols int) string {
	st := m.st
	maxW := max(min(cols-8, 90), 20)
	msg := st.errMsg.Width(maxW).Render(err.Error())
	lines := []string{st.errTitle.Render("✗ Diagram error"), "", msg}
	if d := m.currentDiagram(); d != nil {
		if mm := lineRe.FindStringSubmatch(err.Error()); mm != nil {
			n, _ := strconv.Atoi(mm[1])
			src := strings.Split(d.Source, "\n")
			if n >= 1 && n <= len(src) {
				fileLine := d.Line + n - 1
				code := strings.TrimRight(src[n-1], " \t")
				if lipgloss.Width(code) > maxW-8 {
					code = code[:max(maxW-9, 1)] + "…"
				}
				lines = append(lines, "",
					st.errLineNo.Render(fmt.Sprintf("%4d │ ", fileLine))+st.errCode.Render(code))
			}
		}
	}
	if m.lastGood[m.current] != nil && m.currentErr() != nil {
		lines = append(lines, "", st.errLineNo.Render("showing last good render"))
	}
	return st.errBox.Render(fillPanel(lipgloss.JoinVertical(lipgloss.Left, lines...), st.panelBg))
}

// ensure the diagram package is linked for Preprocess (used by Kind).
var _ = diagram.Preprocess
