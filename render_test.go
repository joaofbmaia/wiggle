package wiggle

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joaofbmaia/wiggle/wavejson"
)

var update = flag.Bool("update", false, "rewrite golden files")

func plain() Options {
	t := PlainTheme()
	return Options{Theme: &t}
}

func TestGolden(t *testing.T) {
	files, _ := filepath.Glob("testdata/*.json5")
	if len(files) == 0 {
		t.Fatal("no fixtures")
	}
	variants := map[string]func(Options) Options{
		"":        func(o Options) Options { return o },
		".ascii":  func(o Options) Options { o.Glyphs = &ASCII; return o },
		".narrow": func(o Options) Options { o.Width = 2; return o },
	}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for suffix, mod := range variants {
			name := strings.TrimSuffix(filepath.Base(f), ".json5") + suffix
			t.Run(name, func(t *testing.T) {
				got, err := RenderSource(src, mod(plain()))
				if err != nil {
					t.Fatal(err)
				}
				golden := filepath.Join("testdata", name+".golden")
				if *update {
					if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
						t.Fatal(err)
					}
					return
				}
				want, err := os.ReadFile(golden)
				if err != nil {
					t.Fatalf("%v (run with -update)", err)
				}
				if got != string(want) {
					t.Errorf("mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
				}
			})
		}
	}
}

func render(t *testing.T, src string, opts Options) []string {
	t.Helper()
	out, err := RenderSource([]byte(src), opts)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(out, "\n")
}

func TestNoTrailingWhitespaceOrColor(t *testing.T) {
	for _, l := range render(t, `{signal:[{name:'a', wave:'01.0x=', data:['d']}]}`, plain()) {
		if strings.TrimRight(l, " ") != l {
			t.Errorf("trailing spaces in %q", l)
		}
		if strings.Contains(l, "\x1b") {
			t.Errorf("escape codes with plain theme in %q", l)
		}
	}
}

func TestStyledOutputCarriesColor(t *testing.T) {
	out, err := RenderSource([]byte(`{signal:[{name:'a', wave:'01'}]}`), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "\x1b[") {
		t.Error("default theme produced no styling")
	}
}

func TestPhaseShiftsLeft(t *testing.T) {
	o := plain()
	o.Width = 4
	base := render(t, `{signal:[{name:'a', wave:'0.1.'}]}`, o)
	shifted := render(t, `{signal:[{name:'a', wave:'0.1.', phase:1}]}`, o)
	// Rising edge moves from column 8 to column 4 relative to lane start.
	x0 := strings.Index(base[2], "─")
	if got := strings.IndexRune(base[0], '╭') - x0; got != 8 {
		t.Fatalf("unshifted edge at %d", got)
	}
	if got := strings.IndexRune(shifted[0], '╭') - x0; got != 4 {
		t.Errorf("phase 1 edge at %d, want 4", got)
	}
	if len(strings.TrimRight(shifted[0], " ")) >= len(strings.TrimRight(base[0], " ")) {
		t.Error("phase-shifted lane should end earlier")
	}
}

func TestDataLabelsConsumedOnlyByNewItems(t *testing.T) {
	o := plain()
	o.Width = 8
	lines := render(t, `{signal:[{name:'d', wave:'=.=|=', data:['one','two','three']}]}`, o)
	top := lines[1]
	for _, want := range []string{"one", "two", "three"} {
		if !strings.Contains(top, want) {
			t.Errorf("label %q missing in %q", want, top)
		}
	}
	if strings.Index(top, "one") > strings.Index(top, "two") || strings.Index(top, "two") > strings.Index(top, "three") {
		t.Errorf("labels out of order: %q", top)
	}
}

func TestLongLabelTruncated(t *testing.T) {
	o := plain()
	o.Width = 6
	lines := render(t, `{signal:[{name:'d', wave:'=', data:['abcdefgh']}]}`, o)
	if !strings.Contains(lines[1], "abc…") || strings.Contains(lines[1], "abcd") {
		t.Errorf("want truncated label with ellipsis, got %q", lines[1])
	}
}

func TestEdgeNeedsBothNodes(t *testing.T) {
	src := `{signal:[{name:'a', wave:'01', node:'.a'}], edge:['a->z nope']}`
	for _, l := range render(t, src, plain()) {
		if strings.Contains(l, "nope") || strings.Contains(l, "▶") {
			t.Errorf("edge with unknown node drawn: %q", l)
		}
	}
}

func TestHscaleMultipliesWidth(t *testing.T) {
	o := plain()
	o.Width = 4
	d, _ := wavejson.Parse([]byte(`{signal:[{name:'a', wave:'0.'}], config:{hscale:3}}`))
	if got := o.cycleWidth(d); got != 12 {
		t.Errorf("cycle width %d, want 12", got)
	}
}
