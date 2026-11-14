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
	"charm.land/tone"
	"github.com/joaofbmaia/wiggle/wavejson"
)

// DefaultWidth is the default number of columns per cycle.
const DefaultWidth = 6

// Options controls rendering.
type Options struct {
	// Width is the number of columns per cycle before the document's hscale
	// multiplier is applied. Zero means DefaultWidth.
	Width int
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
// Lanes are three rows tall: high, mid (high-impedance, bus labels) and
// low. Corners and verticals join the rows on transitions.
type Glyphs struct {
	Line rune // level line
	Weak rune // weak (pull-up/pull-down) level line
	Fill rune // undefined fill
	Gap  rune // time break marker
	V    rune // vertical stroke of a full swing

	// Corners. TL is ╭: joins down and right. Rising edges use TL, V, BR;
	// falling edges use TR, V, BL.
	TL, TR, BL, BR rune
	// Arrow heads replacing the vertical stroke of marked clock edges
	// (P, N, H, L): MarkUp on rising edges, MarkDown on falling ones.
	MarkUp, MarkDown rune
	// Tees joining level lines to bus outlines: ┬ ┴ ┤ ├.
	TeeDown, TeeUp, TeeLeft, TeeRight rune

	// Group bracket.
	GroupTop, GroupBar, GroupBottom rune

	// Edge annotations.
	EdgeH, EdgeV                              rune
	ArrowRight, ArrowLeft, ArrowUp, ArrowDown rune
	// Stubs ending a clock edge that meets a line already at that level:
	// StubUp hangs from the top row, StubDown rises from the bottom row.
	StubUp, StubDown rune

	Ellipsis rune
}

// Rounded is the default glyph set with rounded corners.
var Rounded = Glyphs{
	Line: '─', Weak: '╌', Fill: '╱', Gap: '┆', V: '│',
	TL: '╭', TR: '╮', BL: '╰', BR: '╯',
	MarkUp: '▲', MarkDown: '▼',
	TeeDown: '┬', TeeUp: '┴', TeeLeft: '┤', TeeRight: '├',
	GroupTop: '╭', GroupBar: '│', GroupBottom: '╰',
	EdgeH: '─', EdgeV: '│',
	ArrowRight: '▶', ArrowLeft: '◀', ArrowUp: '▲', ArrowDown: '▼',
	Ellipsis: '…', StubUp: '╵', StubDown: '╷',
}

// Sharp is a glyph set with square corners.
var Sharp = Glyphs{
	Line: '─', Weak: '╌', Fill: '╱', Gap: '┆', V: '│',
	TL: '┌', TR: '┐', BL: '└', BR: '┘',
	MarkUp: '▲', MarkDown: '▼',
	TeeDown: '┬', TeeUp: '┴', TeeLeft: '┤', TeeRight: '├',
	GroupTop: '┌', GroupBar: '│', GroupBottom: '└',
	EdgeH: '─', EdgeV: '│',
	ArrowRight: '▶', ArrowLeft: '◀', ArrowUp: '▲', ArrowDown: '▼',
	Ellipsis: '…', StubUp: '╵', StubDown: '╷',
}

// ASCII is a glyph set restricted to 7-bit ASCII.
var ASCII = Glyphs{
	Line: '-', Weak: '.', Fill: '/', Gap: ':', V: '|',
	TL: '.', TR: '.', BL: '\'', BR: '\'',
	MarkUp: '^', MarkDown: 'v',
	TeeDown: '+', TeeUp: '+', TeeLeft: '+', TeeRight: '+',
	GroupTop: '.', GroupBar: '|', GroupBottom: '\'',
	EdgeH: '-', EdgeV: '|',
	ArrowRight: '>', ArrowLeft: '<', ArrowUp: '^', ArrowDown: 'v',
	Ellipsis: '~', StubUp: '\'', StubDown: '.',
}

// Theme holds the styles applied to each element of a diagram.
type Theme struct {
	Name      lipgloss.Style // signal names
	Line      lipgloss.Style // signal lines and bus outlines
	Mark      lipgloss.Style // marked clock edges (P, N, H, L)
	Weak      lipgloss.Style // pull-up/pull-down levels
	HighZ     lipgloss.Style // high-impedance level
	Undefined lipgloss.Style // undefined fill
	Gap       lipgloss.Style // time break markers
	// Bus styles indexed by WaveDrom color: '=' and '2' use Bus[0], '3'
	// uses Bus[1], and so on up to '9'. In the default themes Bus[0] is the
	// plain line style, so '=' segments are outlined but not coloured.
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
// terminal background. Bus segments are filled with the CharmTone palette.
func DefaultTheme(dark bool) Theme {
	ld := lipgloss.LightDark(dark)
	t := baseTheme(ld)
	t.BusFill = true
	fills := [8][2]tone.Key{
		{tone.Sash, tone.Iron},      // neutral
		{tone.Zest, tone.Zest},      // yellow
		{tone.Yam, tone.Tang},       // orange
		{tone.Sardine, tone.Malibu}, // blue
		{tone.Lichen, tone.Turtle},  // cyan
		{tone.Bok, tone.Julep},      // green
		{tone.Blush, tone.Dolly},    // pink
		{tone.Hazy, tone.Charple},   // purple
	}
	for i, f := range fills {
		fg := color.Color(tone.Pepper)
		if i == 7 {
			fg = tone.Butter
		}
		t.Bus[i] = lipgloss.NewStyle().Foreground(fg).Background(ld(f[0], f[1]))
	}
	// '=' and '2' are WaveDrom's uncoloured default: outline only.
	t.Bus[0] = t.Line
	return t
}

// FlatTheme returns the Charm-flavoured theme with outlined, unfilled bus
// segments. Suited to terminals where backgrounds look heavy.
func FlatTheme(dark bool) Theme {
	ld := lipgloss.LightDark(dark)
	t := baseTheme(ld)
	inks := [8][2]tone.Key{
		{tone.Char, tone.Sash},     // neutral
		{tone.Cumin, tone.Zest},    // yellow
		{tone.Paprika, tone.Tang},  // orange
		{tone.Damson, tone.Malibu}, // blue
		{tone.Zinc, tone.Turtle},   // cyan
		{tone.Guac, tone.Julep},    // green
		{tone.Chili, tone.Dolly},   // pink
		{tone.Grape, tone.Hazy},    // purple
	}
	for i, c := range inks {
		t.Bus[i] = lipgloss.NewStyle().Foreground(ld(c[0], c[1]))
	}
	t.Bus[0] = t.Line
	return t
}

// PlainTheme returns a theme without any styling.
func PlainTheme() Theme { return Theme{} }

func baseTheme(ld lipgloss.LightDarkFunc) Theme {
	purple := ld(tone.Charple, tone.Charple)
	pink := ld(tone.Macaron, tone.Dolly)
	ink := ld(tone.Char, tone.Sash)
	muted := ld(tone.Squid, tone.Oyster)
	return Theme{
		Name:      lipgloss.NewStyle().Foreground(purple).Bold(true),
		Line:      lipgloss.NewStyle().Foreground(ink),
		Mark:      lipgloss.NewStyle().Foreground(pink).Bold(true),
		Weak:      lipgloss.NewStyle().Foreground(muted),
		HighZ:     lipgloss.NewStyle().Foreground(muted),
		Undefined: lipgloss.NewStyle().Foreground(muted),
		Gap:       lipgloss.NewStyle().Foreground(pink),
		Group:     lipgloss.NewStyle().Foreground(purple).Bold(true),
		GroupBar:  lipgloss.NewStyle().Foreground(purple),
		Edge:      lipgloss.NewStyle().Foreground(pink),
		EdgeLabel: lipgloss.NewStyle().Foreground(pink).Italic(true),
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
