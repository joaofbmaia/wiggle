package wiggle

import (
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
)

// Slim lanes are two rows tall. Every level runs along a cell edge:
//
//	high   SlimHigh (▔) on the top row
//	z      SlimLow  (▁) on the top row, i.e. the boundary between the rows
//	low    SlimLow  (▁) on the bottom row
//	bus    a box: strokes at both ends, ▔ on top, ▁ along the bottom
//
// Vertical strokes sit at the cell sides (▏ ▕), and where a level line has
// to continue through a stroke's cell the two are drawn as one corner glyph
// (🭽 🭾 🭼 🭿), so lines and strokes meet exactly. A stroke that ends a
// segment is drawn on that segment's last column; one that starts a
// segment on its first.
func (r *renderer) drawSlim(p rowPlan, y, cycles int) {
	g, t := r.g, r.t
	ln := p.lane
	name := p.sig.Name
	r.c.text(r.x0-1-lipgloss.Width(name), y, name, &t.Name)

	total := cycles * r.cw
	hi, lo := y, y+1
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
		lineSt := r.levelStyle(cur.k)
		st := &t.Line
		if ln.marks[x] {
			st = &t.Mark
		}

		// Opening stroke of the new state, or the full swing.
		if prev != cur {
			switch {
			case cl == lvBus:
				bs := r.slimBarStyle(cur)
				r.c.put(cx, hi, g.SlimTL, bs)
				r.c.put(cx, lo, g.SlimBL, bs)
			case pl == lvBus:
				// The bus drew its own closing stroke; the new level
				// simply starts here.
			case pl == lvBottom && cl == lvTop:
				r.c.put(cx, hi, g.SlimTL, st)
				r.c.put(cx, lo, g.SlimLeft, st)
			case pl == lvTop && cl == lvBottom:
				r.c.put(cx, hi, g.SlimLeft, st)
				r.c.put(cx, lo, g.SlimBL, st)
			case pl == lvTop && cl == lvMid:
				r.c.put(cx, hi, g.SlimBL, st)
			case pl == lvMid && cl == lvTop:
				r.c.put(cx, hi, g.SlimTL, st)
			case pl == lvMid && cl == lvBottom:
				r.c.put(cx, lo, g.SlimBL, st)
			case pl == lvBottom && cl == lvMid:
				r.c.put(cx, lo, g.SlimLeft, st)
			}
		}

		switch cl {
		case lvTop:
			r.slimLevel(cx, hi, g.SlimHigh, lineSt)
		case lvBottom:
			r.slimLevel(cx, lo, g.SlimLow, lineSt)
		case lvMid:
			r.slimLevel(cx, hi, g.SlimLow, lineSt)
		case lvBus:
			bs := r.slimBarStyle(cur)
			if next != cur && nl != lvBus {
				// Closing stroke on the segment's last column.
				r.c.put(cx, hi, g.SlimTR, bs)
				r.c.put(cx, lo, g.SlimBR, bs)
				continue
			}
			if prev != cur {
				continue // opening stroke already drawn
			}
			switch {
			case cur.k == kUndef:
				r.c.put(cx, hi, g.Fill, &t.Undefined)
				r.c.put(cx, lo, g.Fill, &t.Undefined)
			case t.BusFill:
				r.c.put(cx, hi, ' ', &t.Bus[ln.items[cur.item].color])
				r.c.put(cx, lo, g.SlimLow, bs)
			default:
				r.c.put(cx, hi, g.SlimHigh, &t.Bus[ln.items[cur.item].color])
				r.c.put(cx, lo, g.SlimLow, &t.Bus[ln.items[cur.item].color])
			}
		}
	}

	// Bus labels on the top row.
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
			end--
		}
		item := ln.items[cur.item]
		if w := end - x - 1; w > 0 && item.label != "" {
			label := fit(item.label, w, g.Ellipsis)
			lx := r.x0 + x + 1 + (w-utf8.RuneCountInString(label))/2
			if !t.BusFill {
				label = " " + label + " "
				lx--
				if w < utf8.RuneCountInString(label) {
					label = strings.TrimSpace(label)
					lx++
				}
			}
			r.c.text(lx, hi, label, &t.Bus[item.color])
		}
		x = e
	}

	for _, gx := range ln.gaps {
		if gx >= 0 && gx < total {
			r.c.put(r.x0+gx, hi, g.Gap, &t.Gap)
			r.c.put(r.x0+gx, lo, g.Gap, &t.Gap)
		}
	}
}

// slimLevel draws a level line glyph unless a stroke already occupies the
// cell.
func (r *renderer) slimLevel(x, y int, glyph rune, st *lipgloss.Style) {
	if x < 0 || y < 0 || x >= r.c.w || y >= r.c.h {
		return
	}
	if c := &r.c.cells[y*r.c.w+x]; c.r == ' ' {
		c.r, c.st = glyph, st
	}
}

// slimBarStyle returns the style for a bus boundary stroke: the line color
// over the bus fill, so filled segments run edge to edge.
func (r *renderer) slimBarStyle(c cell) *lipgloss.Style {
	if c.k != kData || !r.t.BusFill {
		return &r.t.Line
	}
	i := r.lanes[len(r.lanes)-1].ln.items[c.item].color
	if r.slimBars[i] == nil {
		st := r.t.Bus[i].Foreground(r.t.Line.GetForeground())
		r.slimBars[i] = &st
	}
	return r.slimBars[i]
}
