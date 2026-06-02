package query

import (
	"fmt"
	"strings"
)

func ParseQuery(input string) (Expr, error) {
	tokens, err := Tokenize(input)
	if err != nil {
		return nil, fmt.Errorf("invalid query: %w", err)
	}
	if len(tokens) == 0 {
		return nil, nil
	}
	p := &queryParser{tokens: tokens}
	expr, err := p.parseOr()
	if err != nil {
		return nil, fmt.Errorf("invalid query: %w", err)
	}
	if p.hasNext() {
		return nil, fmt.Errorf("invalid query: unexpected token %q", p.peek())
	}
	return expr, nil
}

type queryParser struct {
	tokens []Token
	pos    int
}

func (p *queryParser) parseOr() (Expr, error) {
	left, err := p.parseXor()
	if err != nil {
		return nil, err
	}
	for p.match("or") {
		right, err := p.parseXor()
		if err != nil {
			return nil, err
		}
		left = Or(left, right)
	}
	return left, nil
}

func (p *queryParser) parseXor() (Expr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.match("xor") {
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = Xor(left, right)
	}
	return left, nil
}

func (p *queryParser) parseAnd() (Expr, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for {
		if p.match("and") {
			right, err := p.parseNot()
			if err != nil {
				return nil, err
			}
			left = And(left, right)
			continue
		}
		if p.startsExpression() {
			right, err := p.parseNot()
			if err != nil {
				return nil, err
			}
			left = And(left, right)
			continue
		}
		return left, nil
	}
}

func (p *queryParser) parseNot() (Expr, error) {
	if p.match("not") {
		expr, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return Not(expr), nil
	}
	return p.parsePrimary()
}

func (p *queryParser) parsePrimary() (Expr, error) {
	if !p.hasNext() {
		return nil, fmt.Errorf("expected expression")
	}
	if p.match("(") {
		expr, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if !p.match(")") {
			return nil, fmt.Errorf("expected )")
		}
		return expr, nil
	}
	tok := p.next()
	return parsePredicate(tok)
}

func parsePredicate(tok string) (Expr, error) {
	switch {
	case strings.HasPrefix(tok, "+") && len(tok) > 1:
		return Predicate{Attribute: AttrTag, Operator: OpHasTag, Value: StringValue(tok[1:])}, nil
	case strings.HasPrefix(tok, "-") && len(tok) > 1:
		return Predicate{Attribute: AttrTag, Operator: OpMissingTag, Value: StringValue(tok[1:])}, nil
	case strings.HasPrefix(tok, "/") && strings.HasSuffix(tok, "/") && len(tok) >= 2:
		return Predicate{Attribute: AttrDescription, Operator: OpContains, Value: StringValue(strings.Trim(tok, "/"))}, nil
	}

	name, value, ok := strings.Cut(tok, ":")
	if !ok {
		if attr, op, err := parseAttributeOperator(tok); err == nil && op == OpNotNull {
			return Predicate{Attribute: attr, Operator: op}, nil
		}
		if field, ok := parseUDANotNull(tok); ok {
			return Predicate{Attribute: AttrUDA, Field: field, Operator: OpNotNull}, nil
		}
		return Predicate{Attribute: AttrBare, Operator: OpContains, Value: BareValue(tok)}, nil
	}
	attr, op, err := parseAttributeOperator(name)
	if err != nil {
		if field, udaOp, ok := parseUDAAttributeOperator(name); ok {
			attr, op = AttrUDA, udaOp
			name = field
		} else {
			return nil, err
		}
	}
	if value == "" {
		if attr == AttrUDA {
			return Predicate{Attribute: attr, Field: name, Operator: OpIsNull, Value: StringValue("")}, nil
		}
		return Predicate{Attribute: attr, Operator: OpIsNull, Value: StringValue("")}, nil
	}
	if attr == AttrUDA {
		return Predicate{Attribute: attr, Field: name, Operator: op, Value: StringValue(strings.Trim(value, "/"))}, nil
	}
	switch attr {
	case AttrDue, AttrEntry, AttrModified, AttrEnd, AttrStart, AttrWait, AttrScheduled, AttrUntil:
		if op == OpEqual || op == OpBefore || op == OpAfter {
			return Predicate{Attribute: attr, Operator: op, Value: ParseDateValue(value)}, nil
		}
	}
	if attr == AttrAnnotations {
		return Predicate{Attribute: attr, Operator: OpContains, Value: StringValue(value)}, nil
	}
	if attr == AttrDescription && strings.HasPrefix(value, "/") && strings.HasSuffix(value, "/") {
		return Predicate{Attribute: attr, Operator: OpContains, Value: StringValue(strings.Trim(value, "/"))}, nil
	}
	return Predicate{Attribute: attr, Operator: op, Value: StringValue(value)}, nil
}

func parseUDANotNull(tok string) (string, bool) {
	if strings.HasPrefix(tok, "uda.") {
		body := strings.TrimPrefix(tok, "uda.")
		field, suffix, ok := strings.Cut(body, ".")
		return field, ok && suffix == "notnull" && field != ""
	}
	field, suffix, ok := strings.Cut(tok, ".")
	if !ok || suffix != "notnull" || isBuiltInAttribute(field) {
		return "", false
	}
	return strings.TrimPrefix(field, "uda."), true
}

func parseUDAAttributeOperator(name string) (string, Operator, bool) {
	field := name
	op := OpEqual
	if strings.HasPrefix(field, "uda.") {
		field = strings.TrimPrefix(field, "uda.")
	} else if isBuiltInAttribute(strings.Split(field, ".")[0]) {
		return "", "", false
	}
	if base, suffix, ok := strings.Cut(field, "."); ok {
		field = base
		switch suffix {
		case "before":
			op = OpBefore
		case "after":
			op = OpAfter
		case "notnull":
			op = OpNotNull
		default:
			return "", "", false
		}
	}
	if field == "" {
		return "", "", false
	}
	return field, op, true
}

func isBuiltInAttribute(name string) bool {
	_, _, err := parseAttributeOperator(name)
	return err == nil
}

func parseAttributeOperator(name string) (Attribute, Operator, error) {
	base, suffix, hasSuffix := strings.Cut(name, ".")
	attr := map[string]Attribute{
		"uuid": AttrUUID, "description": AttrDescription,
		"status": AttrStatus, "entry": AttrEntry, "modified": AttrModified,
		"end": AttrEnd, "due": AttrDue, "start": AttrStart, "wait": AttrWait,
		"scheduled": AttrScheduled, "until": AttrUntil, "project": AttrProject,
		"priority": AttrPriority, "depends": AttrDepends, "annotations": AttrAnnotations,
		"recur": AttrRecur, "parent": AttrParent, "assignee": AttrAssignee,
	}[base]
	if attr == "" {
		return "", "", fmt.Errorf("unknown attribute %q", base)
	}
	if !hasSuffix {
		return attr, OpEqual, nil
	}
	switch suffix {
	case "before":
		return attr, OpBefore, nil
	case "after":
		return attr, OpAfter, nil
	case "notnull":
		return attr, OpNotNull, nil
	default:
		return "", "", fmt.Errorf("unknown modifier %q", suffix)
	}
}

func (p *queryParser) startsExpression() bool {
	if !p.hasNext() {
		return false
	}
	switch p.peek() {
	case ")", "or", "xor":
		return false
	default:
		return true
	}
}

func (p *queryParser) match(text string) bool {
	if p.hasNext() && p.peek() == text {
		p.pos++
		return true
	}
	return false
}

func (p *queryParser) hasNext() bool { return p.pos < len(p.tokens) }
func (p *queryParser) peek() string  { return p.tokens[p.pos].Text }
func (p *queryParser) next() string {
	text := p.peek()
	p.pos++
	return text
}
