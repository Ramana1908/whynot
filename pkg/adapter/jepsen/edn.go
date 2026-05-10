// Package jepsen converts Jepsen-style EDN history logs into a whynot
// History. The parser handles the subset of EDN that real Jepsen
// histories use:
//
//   - vectors  [ ... ]         (top-level container)
//   - lists    ( ... )         (treated like vectors)
//   - maps     { :k v, ... }
//   - keywords :read, :write, :ok, :invoke, :fail, :info, :nemesis ...
//   - integers (signed)
//   - nil
//   - strings  "..."
//   - commas as whitespace (EDN treats them as such)
//
// Anything outside this subset returns a parse error rather than being
// silently skipped — better to fail loudly than mis-interpret a real
// log.
package jepsen

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"
)

// ednValue is the parser's tagged-union for parsed EDN values.
// Concrete types: ednKeyword, int64, nil, string, []ednValue, ednMap.
type ednValue interface{}

type ednKeyword string

type ednMap map[ednKeyword]ednValue

type ednParser struct {
	src []rune
	pos int
}

func parseEDN(r io.Reader) ([]ednValue, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	p := &ednParser{src: []rune(string(b))}
	var out []ednValue
	for {
		p.skipSpace()
		if p.pos >= len(p.src) {
			break
		}
		v, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	// If the top-level was a single vector/list, flatten it: that's the
	// canonical Jepsen layout. Otherwise return the sequence as-is so a
	// line-delimited log of bare maps still works.
	if len(out) == 1 {
		if vec, ok := out[0].([]ednValue); ok {
			return vec, nil
		}
	}
	return out, nil
}

func (p *ednParser) skipSpace() {
	for p.pos < len(p.src) {
		r := p.src[p.pos]
		switch {
		case unicode.IsSpace(r), r == ',':
			p.pos++
		case r == ';': // line comment
			for p.pos < len(p.src) && p.src[p.pos] != '\n' {
				p.pos++
			}
		default:
			return
		}
	}
}

func (p *ednParser) parseValue() (ednValue, error) {
	p.skipSpace()
	if p.pos >= len(p.src) {
		return nil, fmt.Errorf("unexpected EOF")
	}
	r := p.src[p.pos]
	switch {
	case r == '[':
		return p.parseSeq('[', ']')
	case r == '(':
		return p.parseSeq('(', ')')
	case r == '{':
		return p.parseMap()
	case r == ':':
		return p.parseKeyword()
	case r == '"':
		return p.parseString()
	case r == '-' || (r >= '0' && r <= '9'):
		return p.parseNumber()
	case isSymbolStart(r):
		return p.parseSymbol()
	}
	return nil, fmt.Errorf("unexpected character %q at pos %d", r, p.pos)
}

func (p *ednParser) parseSeq(open, close rune) ([]ednValue, error) {
	if p.src[p.pos] != open {
		return nil, fmt.Errorf("expected %q at pos %d", open, p.pos)
	}
	p.pos++
	var out []ednValue
	for {
		p.skipSpace()
		if p.pos >= len(p.src) {
			return nil, fmt.Errorf("unclosed %q", open)
		}
		if p.src[p.pos] == close {
			p.pos++
			return out, nil
		}
		v, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
}

func (p *ednParser) parseMap() (ednMap, error) {
	if p.src[p.pos] != '{' {
		return nil, fmt.Errorf("expected '{' at pos %d", p.pos)
	}
	p.pos++
	out := ednMap{}
	for {
		p.skipSpace()
		if p.pos >= len(p.src) {
			return nil, fmt.Errorf("unclosed '{'")
		}
		if p.src[p.pos] == '}' {
			p.pos++
			return out, nil
		}
		key, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		k, ok := key.(ednKeyword)
		if !ok {
			return nil, fmt.Errorf("map key must be keyword (got %T at pos %d)", key, p.pos)
		}
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		out[k] = val
	}
}

func (p *ednParser) parseKeyword() (ednKeyword, error) {
	if p.src[p.pos] != ':' {
		return "", fmt.Errorf("expected ':' at pos %d", p.pos)
	}
	p.pos++
	start := p.pos
	for p.pos < len(p.src) && isSymbolChar(p.src[p.pos]) {
		p.pos++
	}
	if p.pos == start {
		return "", fmt.Errorf("empty keyword at pos %d", start)
	}
	return ednKeyword(p.src[start:p.pos]), nil
}

func (p *ednParser) parseString() (string, error) {
	if p.src[p.pos] != '"' {
		return "", fmt.Errorf("expected '\"' at pos %d", p.pos)
	}
	p.pos++
	var b strings.Builder
	for p.pos < len(p.src) {
		r := p.src[p.pos]
		if r == '"' {
			p.pos++
			return b.String(), nil
		}
		if r == '\\' && p.pos+1 < len(p.src) {
			p.pos++
			esc := p.src[p.pos]
			p.pos++
			switch esc {
			case 'n':
				b.WriteRune('\n')
			case 't':
				b.WriteRune('\t')
			case '"':
				b.WriteRune('"')
			case '\\':
				b.WriteRune('\\')
			default:
				b.WriteRune(esc)
			}
			continue
		}
		b.WriteRune(r)
		p.pos++
	}
	return "", fmt.Errorf("unterminated string")
}

func (p *ednParser) parseNumber() (int64, error) {
	start := p.pos
	if p.src[p.pos] == '-' {
		p.pos++
	}
	for p.pos < len(p.src) && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
		p.pos++
	}
	if p.pos == start || (p.pos == start+1 && p.src[start] == '-') {
		return 0, fmt.Errorf("invalid number at pos %d", start)
	}
	return strconv.ParseInt(string(p.src[start:p.pos]), 10, 64)
}

func (p *ednParser) parseSymbol() (ednValue, error) {
	start := p.pos
	for p.pos < len(p.src) && isSymbolChar(p.src[p.pos]) {
		p.pos++
	}
	sym := string(p.src[start:p.pos])
	switch sym {
	case "nil":
		return nil, nil
	case "true":
		return int64(1), nil
	case "false":
		return int64(0), nil
	}
	return nil, fmt.Errorf("unsupported symbol %q at pos %d (parser handles only nil/true/false; Jepsen logs rarely use bare symbols elsewhere)", sym, start)
}

func isSymbolStart(r rune) bool {
	return r == '_' || r == '/' || r == '-' || r == '.' || r == '*' || r == '!' || r == '?' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isSymbolChar(r rune) bool {
	if isSymbolStart(r) {
		return true
	}
	return (r >= '0' && r <= '9')
}
