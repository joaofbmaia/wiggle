package markdown

import (
	"strings"
	"testing"
)

func TestSplit(t *testing.T) {
	src := "# T\n\n```wavedrom\n{signal:[]}\n```\n\ntext\n\n~~~~ WaveJSON extra\n{a:1}\n~~~~\n\n```md\n```wavedrom\nnot a diagram\n```\n\n   ```wavedrom\n{b:2}\n   ```\nend\n"
	segs := Split(src)
	var got []string
	for _, s := range segs {
		if s.IsWave {
			got = append(got, "W"+strings.TrimSpace(s.Wave))
		} else {
			got = append(got, "M"+strings.TrimSpace(s.Markdown))
		}
	}
	want := []string{
		"M# T",
		"W{signal:[]}",
		"Mtext",
		"W{a:1}",
		"M```md\n```wavedrom\nnot a diagram\n```",
		"W{b:2}",
		"Mend",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if segs[1].Line != 3 || segs[3].Line != 9 {
		t.Errorf("fence lines: %d %d", segs[1].Line, segs[3].Line)
	}
}

func TestSplitUnterminatedFence(t *testing.T) {
	segs := Split("a\n```wavedrom\n{signal:[]}\n")
	if len(segs) != 2 || !segs[1].IsWave || strings.TrimSpace(segs[1].Wave) != "{signal:[]}" {
		t.Errorf("unexpected segments: %+v", segs)
	}
}

func TestRenderPlain(t *testing.T) {
	src := "Intro\n\n```wavedrom\n{signal:[{name:'clk', wave:'p.'}]}\n```\n\n```wavedrom\n{signal:[{name: nope}]}\n```\n"
	out, err := Render(src, Options{Plain: true, Wrap: 40})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Intro") {
		t.Error("markdown missing")
	}
	if !strings.Contains(out, "    clk ") {
		t.Errorf("diagram missing or not indented:\n%s", out)
	}
	if !strings.Contains(out, "unexpected identifier") {
		t.Errorf("parse error not reported inline:\n%s", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Error("plain output contains escapes")
	}
}
