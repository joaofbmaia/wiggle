// Package wavejson parses WaveDrom's WaveJSON documents.
//
// WaveDrom accepts a relaxed JSON dialect (unquoted keys, single-quoted
// strings, trailing commas, comments). Parse accepts that dialect as well as
// strict JSON.
package wavejson

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Diagram is a parsed WaveJSON document.
type Diagram struct {
	Signal []Item
	Head   *Marker
	Foot   *Marker
	Config Config
	Edge   []string
}

// Item is one entry of a signal list: a signal, a group, or a blank lane.
// Exactly one of Signal and Group is non-nil; both nil means a blank lane.
type Item struct {
	Signal *Signal
	Group  *Group
}

// Blank reports whether the item is an empty spacer lane.
func (it Item) Blank() bool { return it.Signal == nil && it.Group == nil }

// Group is a named list of items.
type Group struct {
	Name  string
	Items []Item
}

// Signal is a single lane.
type Signal struct {
	Name   string
	Wave   string
	Data   []string
	Period float64 // cycles per wave character; defaults to 1
	Phase  float64 // cycles the lane is shifted left
	Node   string
}

// Marker is a head or foot annotation.
type Marker struct {
	Text  string
	Tick  *Ticks
	Tock  *Ticks
	Every int // label every N cycles; 0 means every cycle
}

// Ticks numbers cycle boundaries (tick) or cycle centers (tock).
// Either Labels is set (explicit labels) or numbering starts at Start.
type Ticks struct {
	Start  int
	Labels []string
}

// Label returns the label for cycle index i.
func (t *Ticks) Label(i int) string {
	if t.Labels != nil {
		if i < len(t.Labels) {
			return t.Labels[i]
		}
		return ""
	}
	return strconv.Itoa(t.Start + i)
}

// Config holds rendering hints from the document.
type Config struct {
	Hscale int // horizontal scale multiplier; defaults to 1
	Skin   string
}

// Cycles returns the total number of cycles spanned by the longest lane.
func (d *Diagram) Cycles() int {
	return itemsCycles(d.Signal)
}

func itemsCycles(items []Item) int {
	n := 0
	for _, it := range items {
		switch {
		case it.Group != nil:
			n = max(n, itemsCycles(it.Group.Items))
		case it.Signal != nil:
			n = max(n, it.Signal.Cycles())
		}
	}
	return n
}

// Cycles returns the number of cycles the signal spans after phase shift.
func (s *Signal) Cycles() int {
	return int(math.Ceil(float64(len([]rune(s.Wave)))*s.Period - s.Phase))
}

// Parse decodes a WaveJSON document.
func Parse(src []byte) (*Diagram, error) {
	strict, err := normalize(src)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Signal []json.RawMessage `json:"signal"`
		Head   *rawMarker        `json:"head"`
		Foot   *rawMarker        `json:"foot"`
		Config *struct {
			Hscale json.Number `json:"hscale"`
			Skin   string      `json:"skin"`
		} `json:"config"`
		Edge []string `json:"edge"`
	}
	dec := json.NewDecoder(strings.NewReader(strict))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("wavejson: %w", err)
	}
	d := &Diagram{Edge: raw.Edge, Config: Config{Hscale: 1}}
	if d.Signal, err = parseItems(raw.Signal); err != nil {
		return nil, err
	}
	if d.Head, err = raw.Head.marker(); err != nil {
		return nil, fmt.Errorf("wavejson: head: %w", err)
	}
	if d.Foot, err = raw.Foot.marker(); err != nil {
		return nil, fmt.Errorf("wavejson: foot: %w", err)
	}
	if raw.Config != nil {
		d.Config.Skin = raw.Config.Skin
		if raw.Config.Hscale != "" {
			f, err := raw.Config.Hscale.Float64()
			if err != nil {
				return nil, fmt.Errorf("wavejson: config.hscale: %w", err)
			}
			d.Config.Hscale = max(1, int(f))
		}
	}
	return d, nil
}

func parseItems(raws []json.RawMessage) ([]Item, error) {
	items := make([]Item, 0, len(raws))
	for i, r := range raws {
		it, err := parseItem(r)
		if err != nil {
			return nil, fmt.Errorf("wavejson: signal[%d]: %w", i, err)
		}
		items = append(items, it)
	}
	return items, nil
}

func parseItem(r json.RawMessage) (Item, error) {
	t := strings.TrimSpace(string(r))
	switch {
	case strings.HasPrefix(t, "["):
		var parts []json.RawMessage
		if err := json.Unmarshal(r, &parts); err != nil {
			return Item{}, err
		}
		g := &Group{}
		if len(parts) > 0 {
			if err := json.Unmarshal(parts[0], &g.Name); err != nil {
				return Item{}, fmt.Errorf("group name must be a string: %w", err)
			}
			parts = parts[1:]
		}
		var err error
		if g.Items, err = parseItems(parts); err != nil {
			return Item{}, err
		}
		return Item{Group: g}, nil
	case strings.HasPrefix(t, "{"):
		var raw struct {
			Name   string          `json:"name"`
			Wave   string          `json:"wave"`
			Data   json.RawMessage `json:"data"`
			Period json.Number     `json:"period"`
			Phase  json.Number     `json:"phase"`
			Node   string          `json:"node"`
		}
		dec := json.NewDecoder(strings.NewReader(t))
		dec.UseNumber()
		if err := dec.Decode(&raw); err != nil {
			return Item{}, err
		}
		if raw.Name == "" && raw.Wave == "" && raw.Node == "" {
			return Item{}, nil
		}
		s := &Signal{Name: raw.Name, Wave: raw.Wave, Node: raw.Node, Period: 1}
		var err error
		if s.Data, err = parseData(raw.Data); err != nil {
			return Item{}, fmt.Errorf("data: %w", err)
		}
		if raw.Period != "" {
			if s.Period, err = raw.Period.Float64(); err != nil || s.Period <= 0 {
				return Item{}, fmt.Errorf("period must be a positive number, got %s", raw.Period)
			}
		}
		if raw.Phase != "" {
			if s.Phase, err = raw.Phase.Float64(); err != nil {
				return Item{}, fmt.Errorf("phase: %w", err)
			}
		}
		return Item{Signal: s}, nil
	default:
		return Item{}, fmt.Errorf("expected object or array, got %s", abbreviate(t))
	}
}

func parseData(r json.RawMessage) ([]string, error) {
	if len(r) == 0 {
		return nil, nil
	}
	var v any
	dec := json.NewDecoder(strings.NewReader(string(r)))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	switch v := v.(type) {
	case string:
		return strings.Fields(v), nil
	case []any:
		out := make([]string, len(v))
		for i, e := range v {
			out[i] = stringify(e)
		}
		return out, nil
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("must be a string or array, got %s", abbreviate(string(r)))
	}
}

func stringify(v any) string {
	switch v := v.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case nil:
		return ""
	case []any: // JsonML: [tag, {attrs}?, children...]; keep the text.
		var b strings.Builder
		for i, e := range v {
			if i == 0 {
				if _, isTag := e.(string); isTag {
					continue
				}
			}
			b.WriteString(stringify(e))
		}
		return b.String()
	case map[string]any:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

type rawMarker struct {
	Text  json.RawMessage `json:"text"`
	Tick  json.RawMessage `json:"tick"`
	Tock  json.RawMessage `json:"tock"`
	Every json.Number     `json:"every"`
}

func (r *rawMarker) marker() (*Marker, error) {
	if r == nil {
		return nil, nil
	}
	m := &Marker{}
	if len(r.Text) > 0 {
		var v any
		if err := json.Unmarshal(r.Text, &v); err != nil {
			return nil, fmt.Errorf("text: %w", err)
		}
		m.Text = stringify(v)
	}
	var err error
	if m.Tick, err = parseTicks(r.Tick); err != nil {
		return nil, fmt.Errorf("tick: %w", err)
	}
	if m.Tock, err = parseTicks(r.Tock); err != nil {
		return nil, fmt.Errorf("tock: %w", err)
	}
	if r.Every != "" {
		f, err := r.Every.Float64()
		if err != nil {
			return nil, fmt.Errorf("every: %w", err)
		}
		m.Every = int(f)
	}
	return m, nil
}

func parseTicks(r json.RawMessage) (*Ticks, error) {
	if len(r) == 0 {
		return nil, nil
	}
	var v any
	dec := json.NewDecoder(strings.NewReader(string(r)))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	switch v := v.(type) {
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return nil, err
		}
		return &Ticks{Start: int(f)}, nil
	case bool:
		if v {
			return &Ticks{}, nil
		}
		return nil, nil
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return &Ticks{Start: n}, nil
		}
		return &Ticks{Labels: strings.Fields(v)}, nil
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("must be a number, boolean or string, got %s", abbreviate(string(r)))
	}
}

func abbreviate(s string) string {
	if len(s) > 24 {
		return s[:24] + "…"
	}
	return s
}
