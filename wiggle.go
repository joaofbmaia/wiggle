// Package wiggle renders WaveDrom timing diagrams as terminal text styled with
// Lip Gloss.
//
// The rendering is a plain string; colors are emitted as truecolor ANSI and
// are expected to be downsampled by the writer (Bubble Tea does this
// automatically; standalone programs can use lipgloss.Print).
package wiggle

import (
	"image/color"

	"charm.land/lipgloss/v2"
	"github.com/joaofbmaia/wiggle/wavejson"
)

// DefaultWidth is the default number of columns per cycle.
const DefaultWidth = 6

// Options controls rendering.
type Options struct {
	// Width is the number of columns per cycle before the document's hscale
	// multiplier is applied. Zero means DefaultWidth.
	Width int
	// Compact removes the blank spacer row between lanes.
	Compact bool
	// Glyphs selects the character set. Nil means Rounded.
	Glyphs *Glyphs
	// Theme selects the styles. Nil means DefaultTheme(true).
	Theme *Theme
}

func (o Options) cycleWidth(d *wavejson.Diagram) int {
	w := o.Width
	if w <= 0 {
		w = DefaultWidth
	}
	return max(2, w*max(1, d.Config.Hscale))
}

func (o Options) glyphs() *Glyphs {
	if o.Glyphs == nil {
		return &Rounded
	}
	return o.Glyphs
}

func (o Options) theme() *Theme {
	if o.Theme == nil {
		return &defaultDark
	}
	return o.Theme
}

// Glyphs is the character set used to draw waveforms.
//
// Lanes are two rows tall: the top row carries the high level, the bottom row
// the low level. Corners join the two rows on transitions.
type Glyphs struct {
	Line rune // level line
	Weak rune // weak (pull-up/pull-down) level line
	Mid  rune // high-impedance line, drawn on the top row along its bottom edge
	Fill rune // undefined fill
	Gap  rune // time break marker

	// Corners. TL is ╭: joins down and right. Rising edges use TL over BR,
	// falling edges use TR over BL.
	TL, TR, BL, BR rune
	// Emphasized corners for marked clock edges (P, N, H, L).
	MarkTL, MarkTR, MarkBL, MarkBR rune
	// Diagonals for bus boundaries: Up is ╱, Down is ╲.
	Up, Down rune

	// Group bracket.
	GroupTop, GroupBar, GroupBottom rune

	// Edge annotations.
	EdgeH, EdgeV                              rune
	ArrowRight, ArrowLeft, ArrowUp, ArrowDown rune
	Anchor                                    rune // marks an edge end without an arrow head

	Ellipsis rune
}

// Rounded is the default glyph set with rounded corners.
var Rounded = Glyphs{
	Line: '─', Weak: '╌', Mid: '▁', Fill: '░', Gap: '┆',
	TL: '╭', TR: '╮', BL: '╰', BR: '╯',
	MarkTL: '┎', MarkTR: '┒', MarkBL: '┖', MarkBR: '┚',
	Up: '╱', Down: '╲',
	GroupTop: '╭', GroupBar: '│', GroupBottom: '╰',
	EdgeH: '─', EdgeV: '│',
	ArrowRight: '▶', ArrowLeft: '◀', ArrowUp: '▲', ArrowDown: '▼',
	Anchor: '╵', Ellipsis: '…',
}

// Sharp is a glyph set with square corners.
var Sharp = Glyphs{
	Line: '─', Weak: '╌', Mid: '▁', Fill: '░', Gap: '┆',
	TL: '┌', TR: '┐', BL: '└', BR: '┘',
	MarkTL: '┎', MarkTR: '┒', MarkBL: '┖', MarkBR: '┚',
	Up: '╱', Down: '╲',
	GroupTop: '┌', GroupBar: '│', GroupBottom: '└',
	EdgeH: '─', EdgeV: '│',
	ArrowRight: '▶', ArrowLeft: '◀', ArrowUp: '▲', ArrowDown: '▼',
	Anchor: '╵', Ellipsis: '…',
}

// ASCII is a glyph set restricted to 7-bit ASCII.
var ASCII = Glyphs{
	Line: '-', Weak: '.', Mid: '_', Fill: 'x', Gap: '~',
	TL: '.', TR: '.', BL: '\'', BR: '\'',
	MarkTL: '+', MarkTR: '+', MarkBL: '+', MarkBR: '+',
	Up: '/', Down: '\\',
	GroupTop: '.', GroupBar: '|', GroupBottom: '\'',
	EdgeH: '-', EdgeV: '|',
	ArrowRight: '>', ArrowLeft: '<', ArrowUp: '^', ArrowDown: 'v',
	Anchor: '\'', Ellipsis: '~',
}

// Theme holds the styles applied to each element of a diagram.
type Theme struct {
	Name      lipgloss.Style // signal names
	Line      lipgloss.Style // signal lines and bus outlines
	Mark      lipgloss.Style // emphasized clock edges
	Weak      lipgloss.Style // pull-up/pull-down levels
	HighZ     lipgloss.Style // high-impedance level
	Undefined lipgloss.Style // undefined fill
	Gap       lipgloss.Style // time break markers
	// Bus styles indexed by WaveDrom color: '=' and '2' use Bus[0], '3'
	// uses Bus[1], and so on up to '9'.
	Bus [8]lipgloss.Style
	// BusFill paints the inside of bus segments with the Bus style instead of
	// drawing an outline. Bus styles should then set a background.
	BusFill   bool
	Group     lipgloss.Style // group names
	GroupBar  lipgloss.Style // group brackets
	Edge      lipgloss.Style // edge arrows
	EdgeLabel lipgloss.Style // edge labels
	Title     lipgloss.Style // head and foot text
	Tick      lipgloss.Style // tick and tock numbers
}

var (
	defaultDark = DefaultTheme(true)
)

// DefaultTheme returns the Charm-flavoured theme for a dark or light
// terminal background. Bus segments are filled.
func DefaultTheme(dark bool) Theme {
	ld := lipgloss.LightDark(dark)
	t := baseTheme(ld)
	t.BusFill = true
	fills := [8][2]string{
		{"#EAEAEA", "#3B3B4F"}, // neutral
		{"#FFF1A8", "#6B5E14"}, // yellow
		{"#FFDDB5", "#7A4A1F"}, // orange
		{"#B9DEFF", "#1F4A7A"}, // blue
		{"#C4F5F7", "#1F6466"}, // cyan
		{"#CDF7C4", "#1F6B3F"}, // green
		{"#F9CCF6", "#7A2F6A"}, // pink
		{"#D8D4FF", "#463A8A"}, // purple
	}
	fg := ld(lipgloss.Color("#1C1C1C"), lipgloss.Color("#F5F5F5"))
	for i, f := range fills {
		t.Bus[i] = lipgloss.NewStyle().Foreground(fg).Background(ld(lipgloss.Color(f[0]), lipgloss.Color(f[1])))
	}
	return t
}

// FlatTheme returns the Charm-flavoured theme with outlined, unfilled bus
// segments. Suited to terminals where backgrounds look heavy.
func FlatTheme(dark bool) Theme {
	ld := lipgloss.LightDark(dark)
	t := baseTheme(ld)
	inks := [8][2]string{
		{"#3C3C3C", "#DDDDDD"}, // neutral
		{"#9A7B00", "#F2D45C"}, // yellow
		{"#B85C00", "#FFAB5E"}, // orange
		{"#1D5FA8", "#7CB8FF"}, // blue
		{"#0D7C80", "#5FE0E6"}, // cyan
		{"#1F7A3A", "#6DE38F"}, // green
		{"#B0308F", "#F78FE0"}, // pink
		{"#5A4BC2", "#A79BFF"}, // purple
	}
	for i, c := range inks {
		t.Bus[i] = lipgloss.NewStyle().Foreground(ld(lipgloss.Color(c[0]), lipgloss.Color(c[1])))
	}
	return t
}

// PlainTheme returns a theme without any styling.
func PlainTheme() Theme { return Theme{} }

func baseTheme(ld lipgloss.LightDarkFunc) Theme {
	c := func(light, dark string) color.Color {
		return ld(lipgloss.Color(light), lipgloss.Color(dark))
	}
	indigo := c("#5A56E0", "#7571F9")
	fuchsia := c("#F25D94", "#F780E2")
	ink := c("#3C3C3C", "#D9D9D9")
	muted := c("#8A8A8A", "#6E6E6E")
	return Theme{
		Name:      lipgloss.NewStyle().Foreground(indigo).Bold(true),
		Line:      lipgloss.NewStyle().Foreground(ink),
		Mark:      lipgloss.NewStyle().Foreground(fuchsia).Bold(true),
		Weak:      lipgloss.NewStyle().Foreground(muted),
		HighZ:     lipgloss.NewStyle().Foreground(muted),
		Undefined: lipgloss.NewStyle().Foreground(muted),
		Gap:       lipgloss.NewStyle().Foreground(fuchsia),
		Group:     lipgloss.NewStyle().Foreground(indigo).Bold(true),
		GroupBar:  lipgloss.NewStyle().Foreground(indigo),
		Edge:      lipgloss.NewStyle().Foreground(fuchsia),
		EdgeLabel: lipgloss.NewStyle().Foreground(fuchsia).Italic(true),
		Title:     lipgloss.NewStyle().Foreground(ink).Bold(true),
		Tick:      lipgloss.NewStyle().Foreground(muted),
	}
}

// Render draws a diagram.
func Render(d *wavejson.Diagram, opts Options) string {
	return newRenderer(d, opts).render()
}

// RenderSource parses and draws a WaveJSON document.
func RenderSource(src []byte, opts Options) (string, error) {
	d, err := wavejson.Parse(src)
	if err != nil {
		return "", err
	}
	return Render(d, opts), nil
}
