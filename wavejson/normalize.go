package wavejson

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// SyntaxError reports a problem in the relaxed JSON source.
type SyntaxError struct {
	Line, Col int
	Msg       string
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("wavejson: line %d, column %d: %s", e.Line, e.Col, e.Msg)
}

// normalize rewrites WaveDrom's relaxed JSON into strict JSON: it quotes bare
// keys, converts single-quoted strings, strips comments and trailing commas.
func normalize(src []byte) (string, error) {
	n := normalizer{src: string(src), line: 1, col: 1}
	return n.run()
}

type normalizer struct {
	src       string
	pos       int
	line, col int
	out       strings.Builder
	lastComma int // index in out just past the last emitted ',' or -1
}

func (n *normalizer) errf(format string, args ...any) error {
	return &SyntaxError{Line: n.line, Col: n.col, Msg: fmt.Sprintf(format, args...)}
}

func (n *normalizer) peek() rune {
	if n.pos >= len(n.src) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(n.src[n.pos:])
	return r
}

func (n *normalizer) next() rune {
	r, size := utf8.DecodeRuneInString(n.src[n.pos:])
	n.pos += size
	if r == '\n' {
		n.line++
		n.col = 1
	} else {
		n.col++
	}
	return r
}

func (n *normalizer) skipSpace() error {
	for n.pos < len(n.src) {
		switch r := n.peek(); {
		case unicode.IsSpace(r):
			n.next()
		case r == '/' && strings.HasPrefix(n.src[n.pos:], "//"):
			for n.pos < len(n.src) && n.peek() != '\n' {
				n.next()
			}
		case r == '/' && strings.HasPrefix(n.src[n.pos:], "/*"):
			start := *n
			n.next()
			n.next()
			for {
				if n.pos >= len(n.src) {
					return start.errf("unterminated comment")
				}
				if strings.HasPrefix(n.src[n.pos:], "*/") {
					n.next()
					n.next()
					break
				}
				n.next()
			}
		default:
			return nil
		}
	}
	return nil
}

func (n *normalizer) emit(s string) {
	n.out.WriteString(s)
	n.lastComma = -1
}

func (n *normalizer) run() (string, error) {
	n.lastComma = -1
	for {
		if err := n.skipSpace(); err != nil {
			return "", err
		}
		if n.pos >= len(n.src) {
			break
		}
		start := *n
		r := n.peek()
		switch {
		case r == '{' || r == '[' || r == ':':
			n.next()
			n.emit(string(r))
		case r == ',':
			n.next()
			n.out.WriteByte(',')
			n.lastComma = n.out.Len()
		case r == '}' || r == ']':
			n.next()
			if n.lastComma == n.out.Len() {
				s := n.out.String()
				n.out.Reset()
				n.out.WriteString(s[:len(s)-1])
			}
			n.emit(string(r))
		case r == '"' || r == '\'':
			s, err := n.readString()
			if err != nil {
				return "", err
			}
			n.emit(s)
		case r == '-' || r == '+' || r == '.' || unicode.IsDigit(r):
			s, err := n.readNumber()
			if err != nil {
				return "", err
			}
			n.emit(s)
		case r == '_' || r == '$' || unicode.IsLetter(r):
			word := n.readWord()
			if err := n.skipSpace(); err != nil {
				return "", err
			}
			if n.peek() == ':' {
				n.emit(quote(word))
				continue
			}
			switch word {
			case "true", "false", "null":
				n.emit(word)
			default:
				return "", start.errf("unexpected identifier %q", word)
			}
		default:
			return "", start.errf("unexpected character %q", r)
		}
	}
	return n.out.String(), nil
}

func (n *normalizer) readWord() string {
	start := n.pos
	for n.pos < len(n.src) {
		r := n.peek()
		if r == '_' || r == '$' || unicode.IsLetter(r) || unicode.IsDigit(r) {
			n.next()
			continue
		}
		break
	}
	return n.src[start:n.pos]
}

func (n *normalizer) readNumber() (string, error) {
	start := *n
	var b strings.Builder
	if r := n.peek(); r == '+' {
		n.next()
	} else if r == '-' {
		n.next()
		b.WriteByte('-')
	}
	digits := func() int {
		c := 0
		for n.pos < len(n.src) && unicode.IsDigit(n.peek()) {
			b.WriteRune(n.next())
			c++
		}
		return c
	}
	intDigits := digits()
	if intDigits == 0 {
		if n.peek() != '.' {
			return "", start.errf("malformed number")
		}
		b.WriteByte('0')
	}
	if n.peek() == '.' {
		n.next()
		b.WriteByte('.')
		if digits() == 0 {
			b.WriteByte('0')
		}
	}
	if r := n.peek(); r == 'e' || r == 'E' {
		b.WriteRune(n.next())
		if r := n.peek(); r == '+' || r == '-' {
			b.WriteRune(n.next())
		}
		if digits() == 0 {
			return "", start.errf("malformed exponent")
		}
	}
	return b.String(), nil
}

func (n *normalizer) readString() (string, error) {
	start := *n
	q := n.next()
	var b strings.Builder
	for {
		if n.pos >= len(n.src) {
			return "", start.errf("unterminated string")
		}
		r := n.next()
		switch {
		case r == q:
			return quote(b.String()), nil
		case r == '\\':
			if n.pos >= len(n.src) {
				return "", start.errf("unterminated string")
			}
			e := n.next()
			switch e {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case 'b':
				b.WriteByte('\b')
			case 'f':
				b.WriteByte('\f')
			case '\n': // line continuation
			case 'u':
				r, err := n.readHex4()
				if err != nil {
					return "", start.errf("%v", err)
				}
				if utf16.IsSurrogate(r) && strings.HasPrefix(n.src[n.pos:], `\u`) {
					n.next()
					n.next()
					lo, err := n.readHex4()
					if err != nil {
						return "", start.errf("%v", err)
					}
					r = utf16.DecodeRune(r, lo)
				}
				b.WriteRune(r)
			default:
				b.WriteRune(e)
			}
		case r == '\n':
			return "", start.errf("unterminated string")
		default:
			b.WriteRune(r)
		}
	}
}

func (n *normalizer) readHex4() (rune, error) {
	if n.pos+4 > len(n.src) {
		return 0, errors.New("malformed \\u escape")
	}
	v, err := strconv.ParseUint(n.src[n.pos:n.pos+4], 16, 32)
	if err != nil {
		return 0, errors.New("malformed \\u escape")
	}
	for range 4 {
		n.next()
	}
	return rune(v), nil
}

// quote produces a strict JSON string literal.
func quote(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
