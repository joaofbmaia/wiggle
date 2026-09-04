package wavejson

import (
	"errors"
	"testing"
)

func TestParseRelaxed(t *testing.T) {
	src := `{ signal: [
	  // clock
	  { name: 'clk', wave: 'p.....|...' },
	  { name: "dat", wave: 'x.345x|=.x', data: ['head', 'body', 'tail', "data"], },
	  { name: 'req', wave: '0.1..0|1.0', period: 2, phase: 0.5 },
	  {},
	  ['grp',
	    { name: 'a', wave: '01.0', data: "A B C" },
	    ['inner', { name: 'b', wave: '=.=', node: '.a.b' }],
	  ],
	  { name: 'ack', wave: '1.....|01.' }
	], head: { text: 'title', tick: 0, every: 2 }, foot: { tock: '1 2 3' },
	config: { hscale: 2.0 }, edge: ['a~>b t1'] /* done */,
	}`
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Signal) != 6 {
		t.Fatalf("want 6 items, got %d", len(d.Signal))
	}
	dat := d.Signal[1].Signal
	if dat.Name != "dat" || len(dat.Data) != 4 || dat.Data[3] != "data" {
		t.Errorf("bad data signal: %+v", dat)
	}
	req := d.Signal[2].Signal
	if req.Period != 2 || req.Phase != 0.5 {
		t.Errorf("period/phase not parsed: %+v", req)
	}
	if !d.Signal[3].Blank() {
		t.Error("expected blank lane")
	}
	g := d.Signal[4].Group
	if g == nil || g.Name != "grp" || len(g.Items) != 2 || g.Items[1].Group == nil || g.Items[1].Group.Name != "inner" {
		t.Fatalf("bad group: %+v", g)
	}
	if got := g.Items[0].Signal.Data; len(got) != 3 || got[2] != "C" {
		t.Errorf("space-separated data: %v", got)
	}
	if d.Head == nil || d.Head.Text != "title" || d.Head.Tick == nil || d.Head.Tick.Label(3) != "3" || d.Head.Every != 2 {
		t.Errorf("bad head: %+v", d.Head)
	}
	if d.Foot == nil || d.Foot.Tock == nil || d.Foot.Tock.Label(1) != "2" || d.Foot.Tock.Label(5) != "" {
		t.Errorf("bad foot: %+v", d.Foot)
	}
	if d.Config.Hscale != 2 || len(d.Edge) != 1 {
		t.Errorf("config/edge: %+v %v", d.Config, d.Edge)
	}
	if d.Cycles() != 20 {
		t.Errorf("cycles: want 20, got %d", d.Cycles())
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		"bare word":       `{signal: [{name: foo}]}`,
		"unterminated":    `{signal: [{name: 'foo}]}`,
		"bad item":        `{signal: [42]}`,
		"negative period": `{signal: [{name:'a', wave:'0', period: -1}]}`,
	}
	for name, src := range cases {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	var se *SyntaxError
	_, err := Parse([]byte("{signal: [\n  {name: 'a', wave: '0'}, @]}"))
	if !errors.As(err, &se) || se.Line != 2 {
		t.Errorf("want SyntaxError on line 2, got %v", err)
	}
}

func TestStringEscapes(t *testing.T) {
	d, err := Parse([]byte(`{signal:[{name:'it\'s "q" \u00e9 \ud83d\ude00', wave:'0'}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := d.Signal[0].Signal.Name, `it's "q" é 😀`; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}
