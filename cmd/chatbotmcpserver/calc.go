package main

import (
	"fmt"
	"strconv"
)

// evalArithmetic evaluates a basic arithmetic expression (+, -, *, /, parentheses, decimals,
// unary minus) via a small recursive-descent parser — no shell/process execution, no external
// expression-evaluation library, just a hand-rolled evaluator over the tool's own string input.
func evalArithmetic(expr string) (float64, error) {
	p := &exprParser{input: expr}
	p.skipSpace()
	v, err := p.parseExpr()
	if err != nil {
		return 0, err
	}
	p.skipSpace()
	if p.pos != len(p.input) {
		return 0, fmt.Errorf("unexpected character %q at position %d", p.input[p.pos], p.pos)
	}
	return v, nil
}

type exprParser struct {
	input string
	pos   int
}

func (p *exprParser) skipSpace() {
	for p.pos < len(p.input) && (p.input[p.pos] == ' ' || p.input[p.pos] == '\t') {
		p.pos++
	}
}

func (p *exprParser) peek() byte {
	if p.pos >= len(p.input) {
		return 0
	}
	return p.input[p.pos]
}

// parseExpr handles + and - (lowest precedence).
func (p *exprParser) parseExpr() (float64, error) {
	v, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpace()
		switch p.peek() {
		case '+':
			p.pos++
			rhs, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			v += rhs
		case '-':
			p.pos++
			rhs, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			v -= rhs
		default:
			return v, nil
		}
	}
}

// parseTerm handles * and / (higher precedence than + -).
func (p *exprParser) parseTerm() (float64, error) {
	v, err := p.parseUnary()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpace()
		switch p.peek() {
		case '*':
			p.pos++
			rhs, err := p.parseUnary()
			if err != nil {
				return 0, err
			}
			v *= rhs
		case '/':
			p.pos++
			rhs, err := p.parseUnary()
			if err != nil {
				return 0, err
			}
			if rhs == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			v /= rhs
		default:
			return v, nil
		}
	}
}

// parseUnary handles a leading + or - before a primary expression.
func (p *exprParser) parseUnary() (float64, error) {
	p.skipSpace()
	switch p.peek() {
	case '-':
		p.pos++
		v, err := p.parseUnary()
		if err != nil {
			return 0, err
		}
		return -v, nil
	case '+':
		p.pos++
		return p.parseUnary()
	default:
		return p.parsePrimary()
	}
}

// parsePrimary handles a number literal or a parenthesized expression.
func (p *exprParser) parsePrimary() (float64, error) {
	p.skipSpace()
	if p.peek() == '(' {
		p.pos++
		v, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		p.skipSpace()
		if p.peek() != ')' {
			return 0, fmt.Errorf("missing closing parenthesis")
		}
		p.pos++
		return v, nil
	}
	start := p.pos
	for p.pos < len(p.input) && (isDigit(p.input[p.pos]) || p.input[p.pos] == '.') {
		p.pos++
	}
	if p.pos == start {
		return 0, fmt.Errorf("expected a number at position %d", p.pos)
	}
	numStr := p.input[start:p.pos]
	v, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number %q: %w", numStr, err)
	}
	return v, nil
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}
