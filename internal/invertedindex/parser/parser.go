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

// primary → "(" orExpr ")" | dateExpr | wildcard | TERM
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

	// Date range expressions: DATE:[from,to], VALID:[from,to], APPEARED:[from,to]
	if strings.HasPrefix(upper, "DATE:") || strings.HasPrefix(upper, "VALID:") || strings.HasPrefix(upper, "APPEARED:") {
		return p.parseDateExpr(tok)
	}

	if strings.Contains(tok, "*") {
		// Pure prefix: "fox*" (wildcard only at the end, nothing before it is *)
		if strings.HasSuffix(tok, "*") && !strings.ContainsRune(tok[:len(tok)-1], '*') {
			return p.engine.SearchPrefix(tok[:len(tok)-1]), nil
		}
		// General wildcard: "he*o", "*ello", "h*ll*"
		return p.engine.SearchWildcard(tok), nil
	}

	return p.engine.Search(tok), nil
}

// parseDateExpr parses "DATE:[2024-01-01,2024-12-31]" and similar expressions.
func (p *queryParser) parseDateExpr(tok string) (*engine.Result, error) {
	colonIdx := strings.IndexByte(tok, ':')
	prefix := strings.ToUpper(tok[:colonIdx])
	rangePart := tok[colonIdx+1:]

	if len(rangePart) < 2 || rangePart[0] != '[' || rangePart[len(rangePart)-1] != ']' {
		return nil, fmt.Errorf("date expression must use [from,to] syntax, got %q", rangePart)
	}
	inner := rangePart[1 : len(rangePart)-1]
	parts := strings.SplitN(inner, ",", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("date expression requires two dates: [from,to], got %q", inner)
	}

	from, to := parts[0], parts[1]

	switch prefix {
	case "DATE":
		return p.engine.SearchDateRange(from, to)
	case "VALID":
		return p.engine.SearchValidInRange(from, to)
	case "APPEARED":
		return p.engine.SearchAppearedInRange(from, to)
	default:
		return nil, fmt.Errorf("unknown date prefix: %q", prefix)
	}
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
