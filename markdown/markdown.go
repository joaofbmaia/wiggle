// Package markdown renders Markdown with Glamour, drawing fenced
// ```wavedrom code blocks as wiggle diagrams.
package markdown

import (
	"strconv"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
	"github.com/joaofbmaia/wiggle"
	"github.com/joaofbmaia/wiggle/wavejson"
)

// CodeIndent is the column Glamour's standard styles place code block text
// at: the document margin plus the code block margin.
const CodeIndent = 4

// Segment is a run of Markdown or the body of a WaveDrom fence.
type Segment struct {
	Markdown string
	Wave     string
	IsWave   bool
	Line     int // 1-based line of the fence in the source
}

// Split cuts Markdown at ```wavedrom and ```wavejson fences.
func Split(src string) []Segment {
	var (
		segs    []Segment
		md      strings.Builder
		wave    strings.Builder
		inFence bool   // inside any fence
		inWave  bool   // inside a wavedrom fence
		fence   string // opening fence marker, e.g. "```"
		start   int
	)
	flushMD := func() {
		if md.Len() > 0 {
			segs = append(segs, Segment{Markdown: md.String()})
			md.Reset()
		}
	}
	lines := strings.SplitAfter(src, "\n")
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		if len(line)-len(trimmed) > 3 {
			trimmed = line
		}
		if !inFence {
			if marker, info, ok := openFence(trimmed); ok {
				inFence = true
				fence = marker
				lang := strings.ToLower(strings.Fields(info + " ")[0])
				if lang == "wavedrom" || lang == "wavejson" {
					inWave = true
					start = i + 1
					flushMD()
					continue
				}
			}
			md.WriteString(line)
			continue
		}
		if closesFence(trimmed, fence) {
			inFence = false
			if inWave {
				inWave = false
				segs = append(segs, Segment{Wave: wave.String(), IsWave: true, Line: start})
				wave.Reset()
				continue
			}
		}
		if inWave {
			wave.WriteString(line)
		} else {
			md.WriteString(line)
		}
	}
	if inWave { // unterminated fence: keep the body as a diagram
		segs = append(segs, Segment{Wave: wave.String(), IsWave: true, Line: start})
	}
	flushMD()
	return segs
}

func openFence(line string) (marker, info string, ok bool) {
	for _, ch := range []byte{'`', '~'} {
		n := 0
		for n < len(line) && line[n] == ch {
			n++
		}
		if n >= 3 {
			info = strings.TrimSpace(strings.TrimRight(line[n:], "\n"))
			if ch == '`' && strings.Contains(info, "`") {
				return "", "", false
			}
			return line[:n], info, true
		}
	}
	return "", "", false
}

func closesFence(line, marker string) bool {
	line = strings.TrimRight(line, " \t\r\n")
	if !strings.HasPrefix(line, marker) {
		return false
	}
	return strings.Trim(line, string(marker[0])) == ""
}

// Options configures Render.
type Options struct {
	// Style is a Glamour style name or JSON file path. Empty picks dark or
	// light from Dark.
	Style string
	// Dark selects the built-in style when Style is empty.
	Dark bool
	// Plain disables color: Glamour's notty style and an unstyled theme.
	Plain bool
	// Wrap is the word-wrap width; zero means 80.
	Wrap int
	// Wiggle configures the diagrams.
	Wiggle wiggle.Options
}

// Renderer renders Markdown segments consistently.
type Renderer struct {
	opts    Options
	glamour *glamour.TermRenderer
	errSt   lipgloss.Style
}

// NewRenderer builds a renderer.
func NewRenderer(opts Options) (*Renderer, error) {
	if opts.Wrap <= 0 {
		opts.Wrap = 80
	}
	style := opts.Style
	switch {
	case style != "":
	case opts.Plain:
		style = styles.NoTTYStyle
	case opts.Dark:
		style = styles.DarkStyle
	default:
		style = styles.LightStyle
	}
	g, err := glamour.NewTermRenderer(glamour.WithStylePath(style), glamour.WithWordWrap(opts.Wrap))
	if err != nil {
		return nil, err
	}
	if opts.Wiggle.Theme == nil {
		var t wiggle.Theme
		if opts.Plain {
			t = wiggle.PlainTheme()
		} else {
			t = wiggle.DefaultTheme(opts.Dark)
		}
		opts.Wiggle.Theme = &t
	}
	r := &Renderer{opts: opts, glamour: g}
	if !opts.Plain {
		r.errSt = lipgloss.NewStyle().Foreground(lipgloss.Color("#ED567A"))
	}
	return r, nil
}

// Options returns the effective options, including the derived theme.
func (r *Renderer) Options() Options { return r.opts }

// Markdown renders a Markdown segment with Glamour.
func (r *Renderer) Markdown(src string) (string, error) {
	return r.glamour.Render(src)
}

// Diagram renders a WaveJSON segment as an indented waveform.
func (r *Renderer) Diagram(d *wavejson.Diagram) string {
	return indent(wiggle.Render(d, r.opts.Wiggle), CodeIndent)
}

// Error renders a parse failure in place of a diagram.
func (r *Renderer) Error(seg Segment, err error) string {
	return indent(r.errSt.Render("wavedrom (line "+strconv.Itoa(seg.Line)+"): "+err.Error()), CodeIndent-2)
}

// Render renders a whole document.
func (r *Renderer) Render(src string) (string, error) {
	var out strings.Builder
	for _, seg := range Split(src) {
		if !seg.IsWave {
			s, err := r.Markdown(seg.Markdown)
			if err != nil {
				return "", err
			}
			out.WriteString(s)
			continue
		}
		d, err := wavejson.Parse([]byte(seg.Wave))
		if err != nil {
			out.WriteString(r.Error(seg, err) + "\n")
			continue
		}
		out.WriteString(r.Diagram(d) + "\n")
	}
	return out.String(), nil
}

// Render renders a whole document with the given options.
func Render(src string, opts Options) (string, error) {
	r, err := NewRenderer(opts)
	if err != nil {
		return "", err
	}
	return r.Render(src)
}

func indent(s string, n int) string {
	pad := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = pad + l
		}
	}
	return strings.Join(lines, "\n")
}
