package wiggle

import (
	"math"

	"github.com/joaofbmaia/wiggle/wavejson"
)

// kind is the logical state of a lane at one column.
type kind uint8

const (
	kBlank kind = iota
	kHigh
	kLow
	kWeakHigh
	kWeakLow
	kHighZ
	kUndef
	kData
)

// level is the vertical position a kind is drawn at.
type level uint8

const (
	lvNone level = iota
	lvTop
	lvBottom
	lvMid
	lvBus
)

func (k kind) level() level {
	switch k {
	case kHigh, kWeakHigh:
		return lvTop
	case kLow, kWeakLow:
		return lvBottom
	case kHighZ:
		return lvMid
	case kUndef, kData:
		return lvBus
	}
	return lvNone
}

type cell struct {
	k    kind
	item int32 // bus item index for kData
}

type busItem struct {
	label string
	color int // 0..7
}

// lane is a signal expanded to column resolution.
type lane struct {
	sig   *wavejson.Signal
	cells []cell
	marks []bool // emphasized transition at this column
	edges []bool // clock edge at this column, drawn even without a level change
	gaps  []int  // columns carrying a time break
	items []busItem
	nodes map[rune]int // node name -> column
	lead  kind         // state assumed before column 0, so clocks open with an edge
}

// expand converts a signal's wave string into per-column cells, cw columns
// per cycle.
func expand(sig *wavejson.Signal, cw int) *lane {
	wave := []rune(sig.Wave)
	shift := int(math.Round(sig.Phase * float64(cw)))
	colOf := func(i int) int {
		return int(math.Round(float64(i)*sig.Period*float64(cw))) - shift
	}
	total := max(0, colOf(len(wave)))
	ln := &lane{
		sig:   sig,
		cells: make([]cell, total),
		marks: make([]bool, total),
		edges: make([]bool, total),
		nodes: map[rune]int{},
	}
	set := func(from, to int, c cell) {
		for x := max(from, 0); x < min(to, total); x++ {
			ln.cells[x] = c
		}
	}
	mark := func(x int) {
		if x >= 0 && x < total {
			ln.marks[x] = true
		}
	}

	var (
		prev     rune // last non-repeat wave char
		prevCell cell // state at the end of the previous char
		dataIdx  int  // next label to consume
	)
	for i, ch := range wave {
		s, e := colOf(i), colOf(i+1)
		if ch == '.' || ch == '|' {
			if ch == '|' {
				ln.gaps = append(ln.gaps, min(s+(e-s)/2+1, e-1))
			}
			switch prev {
			case 'p', 'P', 'n', 'N':
				clock(ln, s, e, prev, set, mark)
			case 0:
			default:
				set(s, e, prevCell)
			}
			continue
		}
		prev = ch
		switch ch {
		case 'p', 'P', 'n', 'N':
			prevCell = clock(ln, s, e, ch, set, mark)
		case '1', 'h', 'H':
			prevCell = cell{k: kHigh}
			set(s, e, prevCell)
			if ch == 'H' {
				mark(s)
			}
		case '0', 'l', 'L':
			prevCell = cell{k: kLow}
			set(s, e, prevCell)
			if ch == 'L' {
				mark(s)
			}
		case 'u':
			prevCell = cell{k: kWeakHigh}
			set(s, e, prevCell)
		case 'd':
			prevCell = cell{k: kWeakLow}
			set(s, e, prevCell)
		case 'z':
			prevCell = cell{k: kHighZ}
			set(s, e, prevCell)
		case 'x':
			prevCell = cell{k: kUndef}
			set(s, e, prevCell)
		case '=', '2', '3', '4', '5', '6', '7', '8', '9':
			color := 0
			if ch != '=' {
				color = int(ch - '2')
			}
			label := ""
			if dataIdx < len(sig.Data) {
				label = sig.Data[dataIdx]
			}
			dataIdx++
			ln.items = append(ln.items, busItem{label: label, color: color})
			prevCell = cell{k: kData, item: int32(len(ln.items) - 1)}
			set(s, e, prevCell)
		default:
			prev = 0
			prevCell = cell{}
		}
	}
	for i, ch := range []rune(sig.Node) {
		if ch == '.' || ch == ' ' || i >= len(wave) {
			continue
		}
		if x := colOf(i); x >= 0 && x <= total {
			ln.nodes[ch] = x
		}
	}
	return ln
}

// clock fills one clock period and returns the state at its end.
func clock(ln *lane, s, e int, ch rune, set func(int, int, cell), mark func(int)) cell {
	half := s + (e-s)/2
	first, second := cell{k: kHigh}, cell{k: kLow}
	if ch == 'n' || ch == 'N' {
		first, second = second, first
	}
	set(s, half, first)
	set(half, e, second)
	if s == 0 {
		ln.lead = second.k
	}
	if s >= 0 && s < len(ln.edges) {
		ln.edges[s] = true
	}
	if ch == 'P' || ch == 'N' {
		mark(s)
	}
	return second
}

// state returns the cell at column x, blank outside the lane.
func (ln *lane) state(x int) cell {
	if x < 0 || x >= len(ln.cells) {
		return cell{}
	}
	return ln.cells[x]
}
