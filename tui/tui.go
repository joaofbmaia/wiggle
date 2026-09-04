// Package tui is a Bubble Tea viewer for wiggle diagrams. It can be run on its
// own or embedded in another Bubble Tea program.
package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/joaofbmaia/wiggle"
	"github.com/joaofbmaia/wiggle/wavejson"
)

// Block is a piece of content shown by the viewer.
type Block interface {
	render(opts wiggle.Options) string
}

// Diagram is a block re-rendered whenever the view options change.
type Diagram struct {
	D      *wavejson.Diagram
	Indent int // leading columns
}

func (b Diagram) render(opts wiggle.Options) string {
	return indent(wiggle.Render(b.D, opts), b.Indent)
}

// Text is a fixed, pre-rendered block.
type Text string

func (b Text) render(wiggle.Options) string { return string(b) }

func indent(s string, n int) string {
	if n <= 0 {
		return s
	}
	pad := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = pad + l
		}
	}
	return strings.Join(lines, "\n")
}

// KeyMap holds the viewer's key bindings.
type KeyMap struct {
	Quit    key.Binding
	ZoomIn  key.Binding
	ZoomOut key.Binding
	ASCII   key.Binding
	Fill    key.Binding
	Compact key.Binding
	Top     key.Binding
	Bottom  key.Binding
	Help    key.Binding
	Scroll  key.Binding // documentation-only binding for the viewport keys
}

// DefaultKeyMap returns the default bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit:    key.NewBinding(key.WithKeys("q", "esc", "ctrl+c"), key.WithHelp("q", "quit")),
		ZoomIn:  key.NewBinding(key.WithKeys("+", "="), key.WithHelp("+/-", "zoom")),
		ZoomOut: key.NewBinding(key.WithKeys("-", "_")),
		ASCII:   key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "ascii")),
		Fill:    key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "fill")),
		Compact: key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "compact")),
		Top:     key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g/G", "top/bottom")),
		Bottom:  key.NewBinding(key.WithKeys("G", "end")),
		Help:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Scroll:  key.NewBinding(key.WithKeys("up", "down", "left", "right"), key.WithHelp("↑↓←→", "scroll")),
	}
}

// ShortHelp implements help.KeyMap.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Scroll, k.ZoomIn, k.ASCII, k.Fill, k.Help, k.Quit}
}

// FullHelp implements help.KeyMap.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Scroll, k.Top, k.ZoomIn},
		{k.ASCII, k.Fill, k.Compact},
		{k.Help, k.Quit},
	}
}

// Styles holds the chrome styles of the viewer.
type Styles struct {
	Title  lipgloss.Style
	Status lipgloss.Style
	Help   help.Styles
}

// DefaultStyles returns the viewer chrome for a dark or light background.
func DefaultStyles(dark bool) Styles {
	ld := lipgloss.LightDark(dark)
	return Styles{
		Title: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFDF5")).
			Background(ld(lipgloss.Color("#5A56E0"), lipgloss.Color("#7571F9"))).
			Bold(true).Padding(0, 1),
		Status: lipgloss.NewStyle().
			Foreground(ld(lipgloss.Color("#8A8A8A"), lipgloss.Color("#6E6E6E"))).
			Padding(0, 1),
		Help: help.DefaultStyles(dark),
	}
}

// Model is the viewer. Create it with New.
type Model struct {
	KeyMap KeyMap
	Styles Styles

	title    string
	blocks   []Block
	opts     wiggle.Options
	glyphs   *wiggle.Glyphs // glyphs in use before toggling ASCII
	fill     bool
	dark     bool
	ownTheme bool // theme derived from the background; toggles allowed
	vp       viewport.Model
	help     help.Model
	width    int
	height   int
	quitting bool
}

// New creates a viewer. If opts.Theme is nil the theme follows the terminal
// background and can be toggled at runtime.
func New(title string, opts wiggle.Options, blocks ...Block) Model {
	m := Model{
		KeyMap:   DefaultKeyMap(),
		Styles:   DefaultStyles(true),
		title:    title,
		blocks:   blocks,
		opts:     opts,
		dark:     true,
		ownTheme: opts.Theme == nil,
		fill:     true,
		vp:       viewport.New(),
		help:     help.New(),
	}
	if m.opts.Width <= 0 {
		m.opts.Width = wiggle.DefaultWidth
	}
	if m.opts.Glyphs == nil {
		m.opts.Glyphs = &wiggle.Rounded
	}
	m.glyphs = m.opts.Glyphs
	if !m.ownTheme {
		m.fill = opts.Theme.BusFill
	}
	m.applyTheme()
	m.vp.MouseWheelEnabled = true
	m.vp.KeyMap.Left = key.NewBinding(key.WithKeys("left", "h"))
	m.vp.KeyMap.Right = key.NewBinding(key.WithKeys("right", "l"))
	m.refresh()
	return m
}

// Options returns the rendering options currently in effect.
func (m Model) Options() wiggle.Options { return m.opts }

// SetSize sets the space available to the viewer, chrome included.
func (m *Model) SetSize(w, h int) {
	m.width, m.height = w, h
	m.help.SetWidth(w)
	m.vp.SetWidth(w)
	m.vp.SetHeight(max(1, h-lipgloss.Height(m.footer())))
}

// SetDark switches between the dark and light themes. It is called
// automatically when the terminal reports its background color.
func (m *Model) SetDark(dark bool) {
	m.dark = dark
	m.Styles = DefaultStyles(dark)
	m.help.Styles = m.Styles.Help
	m.applyTheme()
	m.refresh()
}

func (m *Model) applyTheme() {
	if !m.ownTheme {
		return
	}
	var t wiggle.Theme
	if m.fill {
		t = wiggle.DefaultTheme(m.dark)
	} else {
		t = wiggle.FlatTheme(m.dark)
	}
	m.opts.Theme = &t
}

func (m *Model) refresh() {
	parts := make([]string, len(m.blocks))
	for i, b := range m.blocks {
		parts[i] = b.render(m.opts)
	}
	m.vp.SetContent(strings.Join(parts, "\n"))
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return tea.RequestBackgroundColor
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
	case tea.BackgroundColorMsg:
		m.SetDark(msg.IsDark())
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.KeyMap.Quit):
			m.quitting = true
			return m, tea.Quit
		case key.Matches(msg, m.KeyMap.ZoomIn):
			m.opts.Width = min(m.opts.Width+2, 40)
			m.refresh()
		case key.Matches(msg, m.KeyMap.ZoomOut):
			m.opts.Width = max(m.opts.Width-2, 2)
			m.refresh()
		case key.Matches(msg, m.KeyMap.ASCII):
			if m.opts.Glyphs == &wiggle.ASCII {
				m.opts.Glyphs = m.glyphs
			} else {
				m.opts.Glyphs = &wiggle.ASCII
			}
			m.refresh()
		case key.Matches(msg, m.KeyMap.Fill):
			m.fill = !m.fill
			m.applyTheme()
			m.refresh()
		case key.Matches(msg, m.KeyMap.Compact):
			m.opts.Compact = !m.opts.Compact
			m.refresh()
		case key.Matches(msg, m.KeyMap.Top):
			m.vp.GotoTop()
		case key.Matches(msg, m.KeyMap.Bottom):
			m.vp.GotoBottom()
		case key.Matches(msg, m.KeyMap.Help):
			m.help.ShowAll = !m.help.ShowAll
			m.SetSize(m.width, m.height)
		}
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

// View implements tea.Model.
func (m Model) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	if m.title != "" {
		v.WindowTitle = "wiggle · " + m.title
	}
	if m.quitting {
		return v
	}
	v.SetContent(m.vp.View() + "\n" + m.footer())
	return v
}

func (m Model) footer() string {
	status := fmt.Sprintf("width %d", m.opts.Width)
	if m.vp.TotalLineCount() > m.vp.VisibleLineCount() {
		status = fmt.Sprintf("%d%%  ·  %s", int(m.vp.ScrollPercent()*100), status)
	}
	left := m.Styles.Title.Render(m.title)
	if m.title == "" {
		left = m.Styles.Title.Render("wiggle")
	}
	bar := left + m.Styles.Status.Render(status)
	if m.help.ShowAll {
		return bar + "\n" + m.Styles.Status.Render(m.help.View(m.KeyMap))
	}
	gap := m.width - lipgloss.Width(bar) - 2
	h := m.help.ShortHelpView(m.KeyMap.ShortHelp())
	if gap < lipgloss.Width(h) {
		return bar
	}
	return bar + strings.Repeat(" ", gap-lipgloss.Width(h)) + h + " "
}
