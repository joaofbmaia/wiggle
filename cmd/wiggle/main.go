// Command wiggle renders WaveDrom timing diagrams in the terminal.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/fang"
	"github.com/charmbracelet/x/term"
	"github.com/joaofbmaia/wiggle"
	"github.com/joaofbmaia/wiggle/markdown"
	"github.com/joaofbmaia/wiggle/tui"
	"github.com/joaofbmaia/wiggle/wavejson"
	"github.com/spf13/cobra"
)

var version = "dev"

type flags struct {
	width   int
	ascii   bool
	sharp   bool
	flat    bool
	compact bool
	slim    bool
	plain   bool
	wrap    int
	style   string
}

func (f *flags) bind(cmd *cobra.Command) {
	fs := cmd.Flags()
	fs.IntVarP(&f.width, "width", "w", wiggle.DefaultWidth, "columns per cycle")
	fs.BoolVar(&f.ascii, "ascii", false, "draw with ASCII characters only")
	fs.BoolVar(&f.sharp, "sharp", false, "draw square corners")
	fs.BoolVar(&f.flat, "flat", false, "outline bus segments instead of filling them")
	fs.BoolVar(&f.compact, "compact", false, "no blank row between signals")
	fs.BoolVar(&f.slim, "slim", false, "two-row lanes drawn with overline/underline (needs terminal support)")
	fs.BoolVarP(&f.plain, "plain", "p", false, "print instead of opening the interactive viewer")
}

func (f *flags) options(dark, color bool) wiggle.Options {
	o := wiggle.Options{Width: f.width, Compact: f.compact, Slim: f.slim && color && !f.ascii, Glyphs: &wiggle.Rounded}
	switch {
	case f.ascii:
		o.Glyphs = &wiggle.ASCII
	case f.sharp:
		o.Glyphs = &wiggle.Sharp
	}
	var t wiggle.Theme
	switch {
	case !color:
		t = wiggle.PlainTheme()
	case f.flat:
		t = wiggle.FlatTheme(dark)
	default:
		t = wiggle.DefaultTheme(dark)
	}
	o.Theme = &t
	return o
}

func main() {
	var f flags
	root := &cobra.Command{
		Use:   "wiggle [file]",
		Short: "Render WaveDrom timing diagrams in the terminal",
		Long: `Wiggle draws WaveDrom timing diagrams as text, with the same relaxed
WaveJSON syntax WaveDrom accepts. Reads a file or standard input.

In a terminal it opens an interactive viewer; when piped it prints the
diagram.`,
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			src, name, err := input(args)
			if err != nil {
				return err
			}
			d, err := wavejson.Parse(src)
			if err != nil {
				return err
			}
			env := detect(f.plain)
			opts := f.options(env.dark, env.color)
			if !env.interactive {
				return print(wiggle.Render(d, opts))
			}
			return run(tui.New(name, opts, tui.Diagram{D: d}), env)
		},
	}
	f.bind(root)

	md := &cobra.Command{
		Use:   "md [file]",
		Short: "Render Markdown, drawing ```wavedrom code blocks as diagrams",
		Long: `Renders a Markdown document with Glamour. Fenced code blocks tagged
wavedrom or wavejson are drawn as timing diagrams.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			src, name, err := input(args)
			if err != nil {
				return err
			}
			env := detect(f.plain)
			r, err := markdown.NewRenderer(markdown.Options{
				Style:  f.style,
				Dark:   env.dark,
				Plain:  !env.color,
				Wrap:   f.wrap,
				Wiggle: f.options(env.dark, env.color),
			})
			if err != nil {
				return err
			}
			if !env.interactive {
				out, err := r.Render(string(src))
				if err != nil {
					return err
				}
				return print(out)
			}
			var blocks []tui.Block
			for _, seg := range markdown.Split(string(src)) {
				if !seg.IsWave {
					s, err := r.Markdown(seg.Markdown)
					if err != nil {
						return err
					}
					blocks = append(blocks, tui.Text(s))
					continue
				}
				d, err := wavejson.Parse([]byte(seg.Wave))
				if err != nil {
					blocks = append(blocks, tui.Text(r.Error(seg, err)))
					continue
				}
				blocks = append(blocks, tui.Diagram{D: d, Indent: markdown.CodeIndent})
			}
			return run(tui.New(name, r.Options().Wiggle, blocks...), env)
		},
	}
	f.bind(md)
	md.Flags().IntVar(&f.wrap, "wrap", 80, "word-wrap width")
	md.Flags().StringVar(&f.style, "style", os.Getenv("GLAMOUR_STYLE"), "Glamour style name or JSON file")
	root.AddCommand(md)

	if err := fang.Execute(context.Background(), root, fang.WithVersion(version)); err != nil {
		os.Exit(1)
	}
}

func input(args []string) ([]byte, string, error) {
	if len(args) == 1 && args[0] != "-" {
		src, err := os.ReadFile(args[0])
		return src, filepath.Base(args[0]), err
	}
	if term.IsTerminal(os.Stdin.Fd()) {
		return nil, "", errors.New("no input: pass a file or pipe WaveJSON on stdin")
	}
	src, err := io.ReadAll(os.Stdin)
	return src, "stdin", err
}

// environment describes the output terminal.
type environment struct {
	interactive bool // stdout is a terminal and a TTY is available for input
	color       bool
	dark        bool
	tty         *os.File // input for the viewer; nil means stdin
}

func detect(plain bool) environment {
	e := environment{dark: true}
	profile := colorprofile.Detect(os.Stdout, os.Environ())
	e.color = profile > colorprofile.ASCII
	if plain || !term.IsTerminal(os.Stdout.Fd()) {
		return e
	}
	in := os.Stdin
	if !term.IsTerminal(in.Fd()) {
		f, err := os.Open("/dev/tty")
		if err != nil {
			return e
		}
		e.tty = f
		in = f
	}
	e.interactive = true
	e.dark = lipgloss.HasDarkBackground(in, os.Stdout)
	return e
}

func run(m tui.Model, env environment) error {
	var opts []tea.ProgramOption
	if env.tty != nil {
		defer env.tty.Close()
		opts = append(opts, tea.WithInput(env.tty))
	}
	_, err := tea.NewProgram(m, opts...).Run()
	return err
}

func print(s string) error {
	_, err := fmt.Fprintln(lipgloss.Writer, s)
	return err
}
