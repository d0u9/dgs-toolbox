// Package expr reads and evaluates a condition on an Item's keys:
//
//	type == money && category == utility && tags != archived
//
// It knows nothing of rules or Items: what a key holds, and whether a value
// is another (a type below money, a value in the group utility), come from
// an Env.
package expr

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// Env answers what an expression asks of one Item.
type Env interface {
	// Values is what key holds: none when it is missing, several for a
	// list such as tags.
	Values(key string) []string
	// Is reports whether value, held by key, is want: equal, or below it,
	// as a type below its parent or a value in its group.
	Is(key, value, want string) bool
}

// Expr is a parsed condition.
type Expr interface {
	Eval(Env) bool
	// Keys are the keys it reads, once each, in order.
	Keys() []string
}

// Parse reads s. An error names the column where it went wrong.
func Parse(s string) (Expr, error) {
	p := &parser{src: s}
	if err := p.lex(); err != nil {
		return nil, err
	}
	if len(p.toks) == 0 {
		return nil, fmt.Errorf("the condition is empty")
	}
	e, err := p.or()
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.kind != tEOF {
		return nil, p.errAt(t, "expected &&, || or the end, found %q", t.text)
	}
	return e, nil
}

// MustParse is Parse for a condition known to be good.
func MustParse(s string) Expr {
	e, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return e
}

type op int

const (
	opIs op = iota
	opIn
	opContains
	opFilled
)

type cmp struct {
	key  string
	op   op
	want []string
}

func (c cmp) Eval(env Env) bool {
	values := env.Values(c.key)
	if c.op == opFilled {
		return len(values) > 0
	}
	for _, v := range values {
		for _, w := range c.want {
			switch c.op {
			case opContains:
				if strings.Contains(strings.ToLower(v), strings.ToLower(w)) {
					return true
				}
			default:
				if env.Is(c.key, v, w) {
					return true
				}
			}
		}
	}
	return false
}

func (c cmp) Keys() []string { return []string{c.key} }

type not struct{ e Expr }

func (n not) Eval(env Env) bool { return !n.e.Eval(env) }
func (n not) Keys() []string    { return n.e.Keys() }

type both struct {
	and  bool
	a, b Expr
}

func (x both) Eval(env Env) bool {
	if x.and {
		return x.a.Eval(env) && x.b.Eval(env)
	}
	return x.a.Eval(env) || x.b.Eval(env)
}

func (x both) Keys() []string {
	out := x.a.Keys()
	for _, k := range x.b.Keys() {
		if !contains(out, k) {
			out = append(out, k)
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

type tokKind int

const (
	tEOF tokKind = iota
	tWord
	tString
	tLParen
	tRParen
	tLBrack
	tRBrack
	tComma
	tOp // && || ! == != ~
)

type tok struct {
	kind tokKind
	text string
	col  int
}

type parser struct {
	src  string
	toks []tok
	pos  int
}

func (p *parser) errAt(t tok, format string, args ...any) error {
	return fmt.Errorf("column %d: %s", t.col, fmt.Sprintf(format, args...))
}

func (p *parser) lex() error {
	runes := []rune(p.src)
	for i := 0; i < len(runes); {
		r, col := runes[i], i+1
		switch {
		case unicode.IsSpace(r):
			i++
		case strings.ContainsRune("()[],", r):
			kind := map[rune]tokKind{'(': tLParen, ')': tRParen, '[': tLBrack, ']': tRBrack, ',': tComma}[r]
			p.toks = append(p.toks, tok{kind, string(r), col})
			i++
		case strings.ContainsRune("&|!=~", r):
			op := string(r)
			if i+1 < len(runes) && slices.Contains([]string{"&&", "||", "==", "!="}, string(runes[i:i+2])) {
				op = string(runes[i : i+2])
			}
			if op == "&" || op == "|" || op == "=" {
				return fmt.Errorf("column %d: %s is not an operator; write %s%s", col, op, op, op)
			}
			p.toks = append(p.toks, tok{tOp, op, col})
			i += len(op)
		case r == '"':
			j := i + 1
			for j < len(runes) && runes[j] != '"' {
				j++
			}
			if j == len(runes) {
				return fmt.Errorf("column %d: a quote is not closed", col)
			}
			p.toks = append(p.toks, tok{tString, string(runes[i+1 : j]), col})
			i = j + 1
		default:
			j := i
			for j < len(runes) && !unicode.IsSpace(runes[j]) && !strings.ContainsRune("()[],\"&|!=~", runes[j]) {
				j++
			}
			p.toks = append(p.toks, tok{tWord, string(runes[i:j]), col})
			i = j
		}
	}
	return nil
}

func (p *parser) peek() tok {
	if p.pos < len(p.toks) {
		return p.toks[p.pos]
	}
	return tok{kind: tEOF, col: len([]rune(p.src)) + 1}
}

func (p *parser) next() tok {
	t := p.peek()
	if p.pos < len(p.toks) {
		p.pos++
	}
	return t
}

func (p *parser) isOp(o string) bool {
	t := p.peek()
	return t.kind == tOp && t.text == o
}

func (p *parser) isWord(w string) bool {
	t := p.peek()
	return t.kind == tWord && t.text == w
}

func (p *parser) or() (Expr, error) {
	a, err := p.and()
	for err == nil && p.isOp("||") {
		p.next()
		var b Expr
		if b, err = p.and(); err == nil {
			a = both{false, a, b}
		}
	}
	return a, err
}

func (p *parser) and() (Expr, error) {
	a, err := p.unary()
	for err == nil && p.isOp("&&") {
		p.next()
		var b Expr
		if b, err = p.unary(); err == nil {
			a = both{true, a, b}
		}
	}
	return a, err
}

var keywords = map[string]bool{"in": true, "has": true}

func (p *parser) unary() (Expr, error) {
	if p.isOp("!") {
		p.next()
		e, err := p.unary()
		return not{e}, err
	}
	if p.peek().kind == tLParen {
		p.next()
		e, err := p.or()
		if err != nil {
			return nil, err
		}
		if t := p.next(); t.kind != tRParen {
			return nil, p.errAt(t, "a bracket ( is not closed")
		}
		return e, nil
	}
	if p.isWord("has") {
		p.next()
		if t := p.next(); t.kind != tLParen {
			return nil, p.errAt(t, "has takes a key in brackets: has(due)")
		}
		key, err := p.key()
		if err != nil {
			return nil, err
		}
		if t := p.next(); t.kind != tRParen {
			return nil, p.errAt(t, "a bracket ( is not closed")
		}
		return cmp{key: key, op: opFilled}, nil
	}
	key, err := p.key()
	if err != nil {
		return nil, err
	}
	t := p.next()
	switch {
	case t.kind == tOp && (t.text == "==" || t.text == "~"):
		o := map[string]op{"==": opIs, "~": opContains}[t.text]
		v, err := p.value()
		return cmp{key, o, []string{v}}, err
	case t.kind == tOp && t.text == "!=":
		v, err := p.value()
		return not{cmp{key, opIs, []string{v}}}, err
	case t.kind != tWord || t.text != "in":
		return nil, p.errAt(t, "after %s, expected ==, !=, ~ or in", key)
	}
	if t := p.next(); t.kind != tLBrack {
		return nil, p.errAt(t, "in takes a list: [a, b]")
	}
	var list []string
	for {
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		list = append(list, v)
		t := p.next()
		if t.kind == tRBrack {
			return cmp{key, opIn, list}, nil
		}
		if t.kind != tComma {
			return nil, p.errAt(t, "expected , or ] in the list")
		}
	}
}

func (p *parser) key() (string, error) {
	t := p.next()
	if t.kind != tWord || keywords[t.text] {
		return "", p.errAt(t, "expected a key, such as type or about.type, found %q", t.text)
	}
	return t.text, nil
}

func (p *parser) value() (string, error) {
	t := p.next()
	if t.kind == tString || t.kind == tWord && !keywords[t.text] {
		return t.text, nil
	}
	return "", p.errAt(t, "expected a value, found %q; quote one that is a word such as \"in\"", t.text)
}
