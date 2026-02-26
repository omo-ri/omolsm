package parser

import (
	"fmt"
	"omolsm/internal/invertedindex/engine"
	"strings"
)

// Execute parses a boolean query string and returns the result.
//
// Supported syntax:
//
//	fox
//	fox AND dog
//	fox OR cat
//	fox AND NOT snake
//	(fox OR cat) AND NOT snake
func Execute(e *engine.Engine, input string) (*engine.Result, error) {
	tokens := tokenize(input)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("empty query")
	}

	p := &queryParser{tokens: tokens, engine: e}
	result, err := p.parseOr()
	if err != nil {
		return nil, err
	}

	if p.pos < len(p.tokens) {
		return nil, fmt.Errorf("unexpected token: %q", p.tokens[p.pos])
	}

	return result, nil
}

// ---------------------------------------------------------------------------
// Recursive descent parser
//
// Grammar:
//   expr    → orExpr
//   orExpr  → andExpr ("OR" andExpr)*
//   andExpr → primary ("AND" ["NOT"] primary)*
//   primary → "(" expr ")" | TERM
// ---------------------------------------------------------------------------

type queryParser struct {
	tokens []string
	pos    int
	engine *engine.Engine
}

func (p *queryParser) peek() string {
	if p.pos >= len(p.tokens) {
		return ""
	}
	return p.tokens[p.pos]
}

func (p *queryParser) advance() string {
	t := p.tokens[p.pos]
	p.pos++
	return t
}

// orExpr → andExpr ("OR" andExpr)*
func (p *queryParser) parseOr() (*engine.Result, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}

	for strings.ToUpper(p.peek()) == "OR" {
		p.advance()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = left.OrResult(right)
	}

	return left, nil
}

// andExpr → primary ("AND" ["NOT"] primary)*
func (p *queryParser) parseAnd() (*engine.Result, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}

	for strings.ToUpper(p.peek()) == "AND" {
		p.advance()

		if strings.ToUpper(p.peek()) == "NOT" {
			p.advance()
			right, err := p.parsePrimary()
			if err != nil {
				return nil, err
			}
			left = left.NotResult(right)
		} else {
			right, err := p.parsePrimary()
			if err != nil {
				return nil, err
			}
			left = left.AndResult(right)
		}
	}

	return left, nil
}

// primary → "(" orExpr ")" | TERM
func (p *queryParser) parsePrimary() (*engine.Result, error) {
	if p.peek() == "(" {
		p.advance()
		result, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.peek() != ")" {
			return nil, fmt.Errorf("missing closing parenthesis")
		}
		p.advance()
		return result, nil
	}

	tok := p.peek()
	if tok == "" || tok == ")" {
		return nil, fmt.Errorf("unexpected end of query")
	}

	upper := strings.ToUpper(tok)
	if upper == "AND" || upper == "OR" || upper == "NOT" {
		return nil, fmt.Errorf("unexpected operator: %q", tok)
	}

	p.advance()
	return p.engine.Search(tok), nil
}

// ---------------------------------------------------------------------------
// Tokenizer: splits on whitespace, keeps ( ) as separate tokens.
// ---------------------------------------------------------------------------

func tokenize(input string) []string {
	var tokens []string
	current := strings.Builder{}

	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}

	for _, r := range input {
		switch {
		case r == '(' || r == ')':
			flush()
			tokens = append(tokens, string(r))
		case r == ' ' || r == '\t':
			flush()
		default:
			current.WriteRune(r)
		}
	}
	flush()

	return tokens
}
