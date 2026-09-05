package wiggle

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/joaofbmaia/wiggle/wavejson"
)

// canvas is a grid of styled runes.
type canvas struct {
	w, h  int
	cells []canvasCell
}

type canvasCell struct {
	r  rune
	st *lipgloss.Style
}

func newCanvas(w, h int) *canvas {
	c := &canvas{w: w, h: h, cells: make([]canvasCell, w*h)}
	for i := range c.cells {
		c.cells[i].r = ' '
	}
	return c
}

func (c *canvas) put(x, y int, r rune, st *lipgloss.Style) {
	if x < 0 || y < 0 || x >= c.w || y >= c.h {
		return
	}
	c.cells[y*c.w+x] = canvasCell{r: r, st: st}
}

// at returns the glyph at x, y; a space outside the canvas.
func (c *canvas) at(x, y int) rune {
	if x < 0 || y < 0 || x >= c.w || y >= c.h {
		return ' '
	}
	return c.cells[y*c.w+x].r
}

func (c *canvas) text(x, y int, s string, st *lipgloss.Style) {
	for _, r := range s {
		c.put(x, y, r, st)
		x++
	}
}

func (c *canvas) hline(x0, x1, y int, r rune, st *lipgloss.Style) {
	for x := x0; x <= x1; x++ {
		c.put(x, y, r, st)
	}
}

func (c *canvas) vline(x, y0, y1 int, r rune, st *lipgloss.Style) {
	for y := y0; y <= y1; y++ {
		c.put(x, y, r, st)
	}
}

// String joins rows, merging runs of equally styled cells into one styled
// segment and trimming unstyled trailing blanks.
func (c *canvas) String() string {
	var out strings.Builder
	var run strings.Builder
	for y := range c.h {
		row := c.cells[y*c.w : (y+1)*c.w]
		end := c.w
		for end > 0 && row[end-1].r == ' ' && row[end-1].st == nil {
			end--
		}
		if y > 0 {
			out.WriteByte('\n')
		}
		var cur *lipgloss.Style
		flush := func() {
			if run.Len() == 0 {
				return
			}
			if cur == nil {
				out.WriteString(run.String())
			} else {
				out.WriteString(cur.Render(run.String()))
			}
			run.Reset()
		}
		for x := range end {
			if row[x].st != cur {
				flush()
				cur = row[x].st
			}
			run.WriteRune(row[x].r)
		}
		flush()
	}
	return out.String()
}

// laneRow is a placed lane: its expanded cells and its top canvas row.
type laneRow struct {
	ln  *lane
	top int
}

type renderer struct {
	d        *wavejson.Diagram
	opts     Options
	g        *Glyphs
	t        *Theme
	cw       int // columns per cycle
	x0       int // first lane column
	lanes    []laneRow
	nodes    map[rune]int // node -> lane index
	deferred []deferredText
	c        *canvas
}

func newRenderer(d *wavejson.Diagram, opts Options) *renderer {
	return &renderer{d: d, opts: opts, g: opts.glyphs(), t: opts.theme(), cw: opts.cycleWidth(d), nodes: map[rune]int{}}
}

// rowKind describes what a canvas row holds while laying out.
type rowKind uint8

const (
	rowLane rowKind = iota
	rowGroupHead
	rowGroupFoot
	rowText
	rowTick
	rowTock
)

type rowPlan struct {
	kind  rowKind
	depth int    // group nesting for gutter drawing
	label string // group name or head/foot text
	lane  *lane
	sig   *wavejson.Signal
	tick  *wavejson.Ticks
	every int
	open  bool // group bracket starts here
	close bool // group bracket ends here
	foot  bool // ticks below the lanes
}

func (r *renderer) render() string {
	cycles := r.d.Cycles()
	depth := groupDepth(r.d.Signal)
	gutter := depth * 2
	nameW := nameWidth(r.d.Signal)
	r.x0 = gutter + nameW + 1
	width := r.x0 + cycles*r.cw + 2

	var rows []rowPlan
	if h := r.d.Head; h != nil {
		if h.Text != "" {
			rows = append(rows, rowPlan{kind: rowText, label: h.Text})
		}
		if h.Tick != nil {
			rows = append(rows, rowPlan{kind: rowTick, tick: h.Tick, every: h.Every})
		}
		if h.Tock != nil {
			rows = append(rows, rowPlan{kind: rowTock, tick: h.Tock, every: h.Every})
		}
	}
	rows = r.planItems(rows, r.d.Signal, 0)
	if f := r.d.Foot; f != nil {
		if f.Tock != nil {
			rows = append(rows, rowPlan{kind: rowTock, tick: f.Tock, every: f.Every, foot: true})
		}
		if f.Tick != nil {
			rows = append(rows, rowPlan{kind: rowTick, tick: f.Tick, every: f.Every, foot: true})
		}
		if f.Text != "" {
			rows = append(rows, rowPlan{kind: rowText, label: f.Text})
		}
	}
	height := 0
	for _, p := range rows {
		if p.kind == rowLane {
			height += laneRows
		} else {
			height++
		}
	}
	r.c = newCanvas(width, height)

	// Group brackets: track open groups per depth to draw bars.
	bars := make([]bool, depth+1)
	y := 0
	for _, p := range rows {
		rowsHere := 1
		if p.kind == rowLane {
			rowsHere = laneRows
		}
		switch p.kind {
		case rowGroupHead:
			bars[p.depth] = true
			r.c.put(p.depth*2, y, r.g.GroupTop, &r.t.GroupBar)
			r.c.text(p.depth*2+2, y, p.label, &r.t.Group)
		case rowGroupFoot:
			bars[p.depth] = false
			r.c.put(p.depth*2, y, r.g.GroupBottom, &r.t.GroupBar)
		case rowLane:
			r.drawLane(p, y, cycles)
		case rowText:
			r.centered(p.label, y, &r.t.Title)
		case rowTick:
			r.drawTicks(p, y, cycles, true)
		case rowTock:
			r.drawTicks(p, y, cycles, false)
		}
		// Bars for enclosing groups.
		for dpt := 0; dpt <= depth; dpt++ {
			if !bars[dpt] || (p.kind == rowGroupHead && dpt == p.depth) {
				continue
			}
			for yy := y; yy < y+rowsHere; yy++ {
				if p.kind == rowGroupFoot && dpt == p.depth {
					continue
				}
				r.c.put(dpt*2, yy, r.g.GroupBar, &r.t.GroupBar)
			}
		}
		y += rowsHere
	}
	r.drawEdges()
	return r.c.String()
}

func (r *renderer) planItems(rows []rowPlan, items []wavejson.Item, depth int) []rowPlan {
	for _, it := range items {
		switch {
		case it.Group != nil:
			rows = append(rows, rowPlan{kind: rowGroupHead, depth: depth, label: it.Group.Name})
			rows = r.planItems(rows, it.Group.Items, depth+1)
			rows = append(rows, rowPlan{kind: rowGroupFoot, depth: depth})
		case it.Signal != nil:
			rows = append(rows, rowPlan{kind: rowLane, depth: depth, sig: it.Signal, lane: expand(it.Signal, r.cw)})
		default:
			rows = append(rows, rowPlan{kind: rowLane, depth: depth})
		}
	}
	return rows
}

func groupDepth(items []wavejson.Item) int {
	d := 0
	for _, it := range items {
		if it.Group != nil {
			d = max(d, 1+groupDepth(it.Group.Items))
		}
	}
	return d
}

func nameWidth(items []wavejson.Item) int {
	w := 0
	for _, it := range items {
		switch {
		case it.Group != nil:
			w = max(w, nameWidth(it.Group.Items))
		case it.Signal != nil:
			w = max(w, lipgloss.Width(it.Signal.Name))
		}
	}
	return w
}

func (r *renderer) centered(s string, y int, st *lipgloss.Style) {
	w := r.c.w - r.x0
	x := r.x0 + max(0, (w-lipgloss.Width(s))/2)
	r.c.text(x, y, s, st)
}

func (r *renderer) drawTicks(p rowPlan, y, cycles int, boundary bool) {
	n := cycles
	if boundary {
		n++
	}
	for i := range n {
		if p.every > 1 && i%p.every != 0 {
			continue
		}
		label := p.tick.Label(i)
		if label == "" {
			continue
		}
		x := r.x0 + i*r.cw
		if !boundary {
			x += r.cw / 2
		}
		r.c.text(x-utf8.RuneCountInString(label)/2, y, label, &r.t.Tick)
	}
}

// laneRows is the height of a signal lane: high, mid and low rows.
const laneRows = 3

// drawLane renders a lane whose top row is y.
func (r *renderer) drawLane(p rowPlan, y, cycles int) {
	if p.sig == nil {
		return
	}
	ln := p.lane
	li := len(r.lanes)
	r.lanes = append(r.lanes, laneRow{ln: ln, top: y})
	for nd := range ln.nodes {
		r.nodes[nd] = li
	}
	g, t := r.g, r.t
	name := p.sig.Name
	nameX := r.x0 - 1 - lipgloss.Width(name)
	r.c.text(nameX, y+1, name, &t.Name)

	total := cycles * r.cw
	hi, mid, lo := y, y+1, y+2
	put3 := func(x int, a, b, c rune, st *lipgloss.Style) {
		r.c.put(x, hi, a, st)
		r.c.put(x, mid, b, st)
		r.c.put(x, lo, c, st)
	}
	for x := range total {
		cur := ln.state(x)
		if cur.k == kBlank {
			continue
		}
		prev, next := ln.state(x-1), ln.state(x+1)
		if x == 0 {
			prev = cell{k: ln.lead}
		}
		cx := r.x0 + x
		pl, cl, nl := prev.k.level(), cur.k.level(), next.k.level()

		if cl == lvBus {
			switch {
			case prev != cur:
				// Opening boundary; shared with the previous bus item if any.
				a, b, c := g.TL, g.V, g.BL
				switch pl {
				case lvTop:
					a = g.TeeDown
				case lvBottom:
					c = g.TeeUp
				case lvMid:
					b = g.TeeLeft
				case lvBus:
					a, c = g.TeeDown, g.TeeUp
				}
				put3(cx, a, b, c, &t.Line)
			case next != cur && nl != lvBus:
				// Closing boundary.
				a, b, c := g.TR, g.V, g.BR
				switch nl {
				case lvTop:
					a = g.TeeDown
				case lvBottom:
					c = g.TeeUp
				case lvMid:
					b = g.TeeRight
				}
				put3(cx, a, b, c, &t.Line)
			case cur.k == kUndef:
				put3(cx, g.Line, g.Fill, g.Line, &t.Line)
				r.c.put(cx, mid, g.Fill, &t.Undefined)
			default:
				st := &t.Bus[ln.items[cur.item].color]
				if t.BusFill {
					put3(cx, g.Line, ' ', g.Line, &t.Line)
					r.c.put(cx, mid, ' ', st)
				} else {
					put3(cx, g.Line, ' ', g.Line, st)
					r.c.put(cx, mid, ' ', nil)
				}
			}
			continue
		}

		// A clock edge that lands on the level it already had still draws
		// its stroke, as WaveDrom does: a spike from the line to the far
		// level.
		if ln.edges[x] && pl == cl && (cl == lvTop || cl == lvBottom) {
			st, up, down := &t.Line, g.V, g.V
			if ln.marks[x] {
				st, up, down = &t.Mark, g.MarkUp, g.MarkDown
			}
			if cl == lvTop {
				put3(cx, g.TeeDown, up, g.StubUp, st)
			} else {
				put3(cx, g.StubDown, down, g.TeeUp, st)
			}
			continue
		}
		if pl != cl && pl != lvNone && pl != lvBus {
			st := &t.Line
			tl, tr, bl, br, up, down := g.TL, g.TR, g.BL, g.BR, g.V, g.V
			if ln.marks[x] {
				st, up, down = &t.Mark, g.MarkUp, g.MarkDown
			}
			switch {
			case pl == lvBottom && cl == lvTop:
				put3(cx, tl, up, br, st)
			case pl == lvTop && cl == lvBottom:
				put3(cx, tr, down, bl, st)
			case pl == lvTop && cl == lvMid:
				r.c.put(cx, hi, tr, st)
				r.c.put(cx, mid, bl, st)
			case pl == lvMid && cl == lvTop:
				r.c.put(cx, hi, tl, st)
				r.c.put(cx, mid, br, st)
			case pl == lvBottom && cl == lvMid:
				r.c.put(cx, mid, tl, st)
				r.c.put(cx, lo, br, st)
			case pl == lvMid && cl == lvBottom:
				r.c.put(cx, mid, tr, st)
				r.c.put(cx, lo, bl, st)
			}
			continue
		}
		glyph := g.Line
		if cur.k == kWeakHigh || cur.k == kWeakLow {
			glyph = g.Weak
		}
		switch cl {
		case lvTop:
			r.c.put(cx, hi, glyph, r.levelStyle(cur.k))
		case lvMid:
			r.c.put(cx, mid, glyph, &t.HighZ)
		case lvBottom:
			r.c.put(cx, lo, glyph, r.levelStyle(cur.k))
		}
	}

	// Bus labels.
	for x := 0; x < total; {
		cur := ln.state(x)
		if cur.k != kData {
			x++
			continue
		}
		e := x + 1
		for e < total && ln.state(e) == cur {
			e++
		}
		end := e
		if ln.state(e).k.level() != lvBus {
			end-- // closing boundary occupies the last column
		}
		item := ln.items[cur.item]
		if w := end - x - 1; w > 0 && item.label != "" {
			label := fit(item.label, w, g.Ellipsis)
			lx := x + 1 + (w-utf8.RuneCountInString(label))/2
			r.c.text(r.x0+lx, mid, label, &t.Bus[item.color])
		}
		x = e
	}

	for _, gx := range ln.gaps {
		if gx >= 0 && gx < total {
			put3(r.x0+gx, g.Gap, g.Gap, g.Gap, &t.Gap)
		}
	}
}

func (r *renderer) levelStyle(k kind) *lipgloss.Style {
	switch k {
	case kWeakHigh, kWeakLow:
		return &r.t.Weak
	case kHighZ:
		return &r.t.HighZ
	}
	return &r.t.Line
}

// fit truncates s to w cells, ending with an ellipsis when cut.
func fit(s string, w int, ellipsis rune) string {
	rs := []rune(s)
	if len(rs) <= w {
		return s
	}
	if w < 2 {
		return string(rs[:w])
	}
	return string(rs[:w-1]) + string(ellipsis)
}

var edgeRe = regexp.MustCompile(`^\s*(\S)\s*([-~|/#<>+]+)\s*(\S)\s*(.*?)\s*$`)

// deferredText is text painted after all edge lines so lines never cover it.
type deferredText struct {
	x, y int
	s    string
	st   *lipgloss.Style
}

// drawEdges overlays node-to-node arrows, then edge labels, then the names
// of visible nodes. Per the WaveJSON spec, [A-Z] are invisible markers and
// any other character is shown.
func (r *renderer) drawEdges() {
	for _, e := range r.d.Edge {
		m := edgeRe.FindStringSubmatch(e)
		if m == nil {
			continue
		}
		a, _ := utf8.DecodeRuneInString(m[1])
		b, _ := utf8.DecodeRuneInString(m[3])
		la, oka := r.nodes[a]
		lb, okb := r.nodes[b]
		if !oka || !okb {
			continue
		}
		shape := m[2]
		headB := strings.Contains(shape, ">") || strings.Contains(shape, "+")
		headA := strings.Contains(shape, "<") || strings.Contains(shape, "+")
		r.drawEdge(r.lanes[la], r.lanes[la].ln.nodes[a], r.lanes[lb], r.lanes[lb].ln.nodes[b], headA, headB, m[4])
	}
	for _, d := range r.deferred {
		r.c.text(d.x, d.y, d.s, d.st)
	}
	for _, l := range r.lanes {
		for name, x := range l.ln.nodes {
			if unicode.IsUpper(name) {
				continue
			}
			r.c.put(r.x0+x, l.top+1, name, &r.t.EdgeLabel)
		}
	}
}

// drawEdge routes an arrow from node a to node b. Horizontal runs use the
// source lane's middle row; vertical runs drop or rise at b's column and
// land on b's lane with an arrow head.
func (r *renderer) drawEdge(la laneRow, xa int, lb laneRow, xb int, headA, headB bool, label string) {
	g, t := r.g, r.t
	st, lst := &t.Edge, &t.EdgeLabel
	xa += r.x0
	xb += r.x0
	y := la.top + 1 // middle row of a
	right := xb > xa

	putLabel := func(x0, x1, y int) bool {
		lo, hi := min(x0, x1), max(x0, x1)
		w := hi - lo - 1
		n := utf8.RuneCountInString(label)
		if label == "" || n > w {
			return false
		}
		r.deferred = append(r.deferred, deferredText{lo + 1 + (w-n)/2, y, label, lst})
		return true
	}
	// start joins the run to the transition stroke at a, if any.
	joinA := r.c.at(xa, y) == g.V
	start := func() {
		if !joinA {
			return
		}
		if right {
			r.c.put(xa, y, g.TeeRight, st)
		} else {
			r.c.put(xa, y, g.TeeLeft, st)
		}
	}

	switch {
	case la.top == lb.top:
		r.c.hline(min(xa, xb), max(xa, xb), y, g.EdgeH, st)
		start()
		if headB {
			r.c.put(xb, y, arrowH(g, right), st)
		}
		if headA {
			r.c.put(xa, y, arrowH(g, !right), st)
		}
		putLabel(xa, xb, y)
	case la.top < lb.top:
		// Down: run along a's middle row, drop onto b's top row.
		if xa == xb {
			r.c.vline(xa, y+1, lb.top-1, g.EdgeV, st)
			if headA {
				r.c.put(xa, y+1, g.ArrowUp, st)
			}
		} else {
			r.c.hline(min(xa, xb), max(xa, xb), y, g.EdgeH, st)
			start()
			if right {
				r.c.put(xb, y, g.TR, st)
			} else {
				r.c.put(xb, y, g.TL, st)
			}
			r.c.vline(xb, y+1, lb.top-1, g.EdgeV, st)
			if headA {
				r.c.put(xa, y, arrowH(g, !right), st)
			}
		}
		if headB {
			r.c.put(xb, lb.top, g.ArrowDown, st)
		}
		if !putLabel(xa, xb, y) {
			r.deferred = append(r.deferred, deferredText{xb + 1, (y + lb.top) / 2, label, lst})
		}
	default:
		// Up: run along a's middle row, rise onto b's bottom row.
		bot := lb.top + laneRows - 1
		if xa == xb {
			r.c.vline(xa, bot+1, y-1, g.EdgeV, st)
			if headA {
				r.c.put(xa, y-1, g.ArrowDown, st)
			}
		} else {
			r.c.hline(min(xa, xb), max(xa, xb), y, g.EdgeH, st)
			start()
			if right {
				r.c.put(xb, y, g.BR, st)
			} else {
				r.c.put(xb, y, g.BL, st)
			}
			r.c.vline(xb, bot+1, y-1, g.EdgeV, st)
			if headA {
				r.c.put(xa, y, arrowH(g, !right), st)
			}
		}
		if headB {
			r.c.put(xb, bot, g.ArrowUp, st)
		}
		if !putLabel(xa, xb, y) {
			r.deferred = append(r.deferred, deferredText{xb + 1, (y + bot) / 2, label, lst})
		}
	}
}

func arrowH(g *Glyphs, right bool) rune {
	if right {
		return g.ArrowRight
	}
	return g.ArrowLeft
}
