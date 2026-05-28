package expr

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type tokenKind int

const (
	tokenEOF tokenKind = iota
	tokenNumber
	tokenOperator
	tokenLParen
	tokenRParen
)

type calcToken struct {
	kind tokenKind
	text string
}

func Calc(input string) (string, error) {
	tokens, err := tokenizeCalc(input)
	if err != nil {
		return "", err
	}
	p := calcParser{tokens: tokens}
	value, err := p.parseOr()
	if err != nil {
		return "", err
	}
	if p.peek().kind != tokenEOF {
		return "", fmt.Errorf("unexpected token %q", p.peek().text)
	}
	return value.String(), nil
}

type calcValue struct {
	number  float64
	boolean bool
	isBool  bool
}

func numberValue(v float64) calcValue { return calcValue{number: v} }
func boolValue(v bool) calcValue      { return calcValue{boolean: v, isBool: true} }

func (v calcValue) String() string {
	if v.isBool {
		return strconv.FormatBool(v.boolean)
	}
	if v.number == float64(int64(v.number)) {
		return strconv.FormatInt(int64(v.number), 10)
	}
	return strconv.FormatFloat(v.number, 'f', -1, 64)
}

type calcParser struct {
	tokens []calcToken
	pos    int
}

func (p *calcParser) parseOr() (calcValue, error) {
	left, err := p.parseXor()
	if err != nil {
		return calcValue{}, err
	}
	for p.match("or") {
		right, err := p.parseXor()
		if err != nil {
			return calcValue{}, err
		}
		l, r, err := boolOperands(left, right)
		if err != nil {
			return calcValue{}, err
		}
		left = boolValue(l || r)
	}
	return left, nil
}

func (p *calcParser) parseXor() (calcValue, error) {
	left, err := p.parseAnd()
	if err != nil {
		return calcValue{}, err
	}
	for p.match("xor") {
		right, err := p.parseAnd()
		if err != nil {
			return calcValue{}, err
		}
		l, r, err := boolOperands(left, right)
		if err != nil {
			return calcValue{}, err
		}
		left = boolValue(l != r)
	}
	return left, nil
}

func (p *calcParser) parseAnd() (calcValue, error) {
	left, err := p.parseComparison()
	if err != nil {
		return calcValue{}, err
	}
	for p.match("and") {
		right, err := p.parseComparison()
		if err != nil {
			return calcValue{}, err
		}
		l, r, err := boolOperands(left, right)
		if err != nil {
			return calcValue{}, err
		}
		left = boolValue(l && r)
	}
	return left, nil
}

func (p *calcParser) parseComparison() (calcValue, error) {
	left, err := p.parseAdd()
	if err != nil {
		return calcValue{}, err
	}
	for {
		op := p.peek().text
		switch op {
		case "==", "!=", "<", "<=", ">", ">=":
			p.next()
			right, err := p.parseAdd()
			if err != nil {
				return calcValue{}, err
			}
			l, r, err := numberOperands(left, right)
			if err != nil {
				return calcValue{}, err
			}
			switch op {
			case "==":
				left = boolValue(l == r)
			case "!=":
				left = boolValue(l != r)
			case "<":
				left = boolValue(l < r)
			case "<=":
				left = boolValue(l <= r)
			case ">":
				left = boolValue(l > r)
			case ">=":
				left = boolValue(l >= r)
			}
		default:
			return left, nil
		}
	}
}

func (p *calcParser) parseAdd() (calcValue, error) {
	left, err := p.parseMul()
	if err != nil {
		return calcValue{}, err
	}
	for {
		switch {
		case p.match("+"):
			right, err := p.parseMul()
			if err != nil {
				return calcValue{}, err
			}
			l, r, err := numberOperands(left, right)
			if err != nil {
				return calcValue{}, err
			}
			left = numberValue(l + r)
		case p.match("-"):
			right, err := p.parseMul()
			if err != nil {
				return calcValue{}, err
			}
			l, r, err := numberOperands(left, right)
			if err != nil {
				return calcValue{}, err
			}
			left = numberValue(l - r)
		default:
			return left, nil
		}
	}
}

func (p *calcParser) parseMul() (calcValue, error) {
	left, err := p.parseUnary()
	if err != nil {
		return calcValue{}, err
	}
	for {
		switch {
		case p.match("*"):
			right, err := p.parseUnary()
			if err != nil {
				return calcValue{}, err
			}
			l, r, err := numberOperands(left, right)
			if err != nil {
				return calcValue{}, err
			}
			left = numberValue(l * r)
		case p.match("/"):
			right, err := p.parseUnary()
			if err != nil {
				return calcValue{}, err
			}
			l, r, err := numberOperands(left, right)
			if err != nil {
				return calcValue{}, err
			}
			if r == 0 {
				return calcValue{}, fmt.Errorf("division by zero")
			}
			left = numberValue(l / r)
		case p.match("%"):
			right, err := p.parseUnary()
			if err != nil {
				return calcValue{}, err
			}
			l, r, err := numberOperands(left, right)
			if err != nil {
				return calcValue{}, err
			}
			if l != float64(int64(l)) || r != float64(int64(r)) {
				return calcValue{}, fmt.Errorf("modulo requires integers")
			}
			if r == 0 {
				return calcValue{}, fmt.Errorf("modulo by zero")
			}
			left = numberValue(float64(int64(l) % int64(r)))
		default:
			return left, nil
		}
	}
}

func (p *calcParser) parseUnary() (calcValue, error) {
	if p.match("-") {
		value, err := p.parseUnary()
		if err != nil {
			return calcValue{}, err
		}
		if value.isBool {
			return calcValue{}, fmt.Errorf("expected number")
		}
		return numberValue(-value.number), nil
	}
	if p.match("not") {
		value, err := p.parseUnary()
		if err != nil {
			return calcValue{}, err
		}
		if !value.isBool {
			return calcValue{}, fmt.Errorf("expected boolean")
		}
		return boolValue(!value.boolean), nil
	}
	return p.parsePow()
}

func (p *calcParser) parsePow() (calcValue, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return calcValue{}, err
	}
	if p.match("^") {
		right, err := p.parseUnary()
		if err != nil {
			return calcValue{}, err
		}
		l, r, err := numberOperands(left, right)
		if err != nil {
			return calcValue{}, err
		}
		return numberValue(math.Pow(l, r)), nil
	}
	return left, nil
}

func (p *calcParser) parsePrimary() (calcValue, error) {
	tok := p.next()
	switch tok.kind {
	case tokenNumber:
		v, err := strconv.ParseFloat(tok.text, 64)
		if err != nil {
			return calcValue{}, err
		}
		return numberValue(v), nil
	case tokenOperator:
		switch tok.text {
		case "true":
			return boolValue(true), nil
		case "false":
			return boolValue(false), nil
		}
	case tokenLParen:
		value, err := p.parseOr()
		if err != nil {
			return calcValue{}, err
		}
		if !p.matchKind(tokenRParen) {
			return calcValue{}, fmt.Errorf("expected )")
		}
		return value, nil
	}
	return calcValue{}, fmt.Errorf("expected expression")
}

func (p *calcParser) match(text string) bool {
	if p.peek().text == text {
		p.next()
		return true
	}
	return false
}

func (p *calcParser) matchKind(kind tokenKind) bool {
	if p.peek().kind == kind {
		p.next()
		return true
	}
	return false
}

func (p *calcParser) peek() calcToken {
	if p.pos >= len(p.tokens) {
		return calcToken{kind: tokenEOF}
	}
	return p.tokens[p.pos]
}

func (p *calcParser) next() calcToken {
	tok := p.peek()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return tok
}

func boolOperands(left, right calcValue) (bool, bool, error) {
	if !left.isBool || !right.isBool {
		return false, false, fmt.Errorf("expected boolean")
	}
	return left.boolean, right.boolean, nil
}

func numberOperands(left, right calcValue) (float64, float64, error) {
	if left.isBool || right.isBool {
		return 0, 0, fmt.Errorf("expected number")
	}
	return left.number, right.number, nil
}

func tokenizeCalc(input string) ([]calcToken, error) {
	var tokens []calcToken
	for i := 0; i < len(input); {
		r, size := utf8.DecodeRuneInString(input[i:])
		if unicode.IsSpace(r) {
			i += size
			continue
		}
		if r == '"' || r == '\'' || r == '[' || r == ']' || r == ',' {
			return nil, fmt.Errorf("unsupported token %q", string(r))
		}
		if unicode.IsDigit(r) || r == '.' {
			start := i
			i += size
			for i < len(input) {
				r, size = utf8.DecodeRuneInString(input[i:])
				if !unicode.IsDigit(r) && r != '.' && r != 'e' && r != 'E' && r != '+' && r != '-' {
					break
				}
				if (r == '+' || r == '-') && !strings.HasSuffix(input[:i], "e") && !strings.HasSuffix(input[:i], "E") {
					break
				}
				i += size
			}
			tokens = append(tokens, calcToken{kind: tokenNumber, text: input[start:i]})
			continue
		}
		if unicode.IsLetter(r) {
			start := i
			i += size
			for i < len(input) {
				r, size = utf8.DecodeRuneInString(input[i:])
				if !unicode.IsLetter(r) {
					break
				}
				i += size
			}
			word := input[start:i]
			switch strings.ToLower(word) {
			case "and", "or", "xor", "not", "true", "false":
				tokens = append(tokens, calcToken{kind: tokenOperator, text: strings.ToLower(word)})
			default:
				return nil, fmt.Errorf("unsupported identifier %q", word)
			}
			continue
		}
		switch input[i] {
		case '(':
			tokens = append(tokens, calcToken{kind: tokenLParen, text: "("})
			i++
		case ')':
			tokens = append(tokens, calcToken{kind: tokenRParen, text: ")"})
			i++
		case '+', '-', '*', '/', '%', '^':
			tokens = append(tokens, calcToken{kind: tokenOperator, text: string(input[i])})
			i++
		case '<', '>', '=', '!':
			if i+1 < len(input) && input[i+1] == '=' {
				tokens = append(tokens, calcToken{kind: tokenOperator, text: input[i : i+2]})
				i += 2
			} else if input[i] == '<' || input[i] == '>' {
				tokens = append(tokens, calcToken{kind: tokenOperator, text: string(input[i])})
				i++
			} else {
				return nil, fmt.Errorf("unsupported operator %q", string(input[i]))
			}
		default:
			return nil, fmt.Errorf("unsupported token %q", string(r))
		}
	}
	tokens = append(tokens, calcToken{kind: tokenEOF})
	return tokens, nil
}
