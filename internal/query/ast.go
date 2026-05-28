package query

import (
	"fmt"
	"strconv"
)

type Expr interface {
	queryExpr()
	String() string
}

type Attribute string

const (
	AttrUUID        Attribute = "uuid"
	AttrDescription Attribute = "description"
	AttrStatus      Attribute = "status"
	AttrEntry       Attribute = "entry"
	AttrModified    Attribute = "modified"
	AttrEnd         Attribute = "end"
	AttrDue         Attribute = "due"
	AttrStart       Attribute = "start"
	AttrWait        Attribute = "wait"
	AttrScheduled   Attribute = "scheduled"
	AttrUntil       Attribute = "until"
	AttrProject     Attribute = "project"
	AttrPriority    Attribute = "priority"
	AttrDepends     Attribute = "depends"
	AttrAnnotations Attribute = "annotations"
	AttrRecur       Attribute = "recur"
	AttrParent      Attribute = "parent"
	AttrTag         Attribute = "tag"
	AttrBare        Attribute = "bare"
	AttrUDA         Attribute = "uda"
)

type Operator string

const (
	OpEqual      Operator = "eq"
	OpBefore     Operator = "before"
	OpAfter      Operator = "after"
	OpContains   Operator = "contains"
	OpHasTag     Operator = "has"
	OpMissingTag Operator = "missing"
	OpIsNull     Operator = "is_null"
	OpNotNull    Operator = "not_null"
)

type Value struct {
	Raw  string
	Kind ValueKind
	Text string
}

type ValueKind string

const (
	ValueString ValueKind = "string"
	ValueDate   ValueKind = "date"
	ValueBare   ValueKind = "bare"
)

func StringValue(v string) Value {
	return Value{Raw: v, Kind: ValueString, Text: v}
}

func DateValue(raw string) Value {
	return Value{Raw: raw, Kind: ValueDate, Text: raw}
}

func BareValue(raw string) Value {
	return Value{Raw: raw, Kind: ValueBare, Text: raw}
}

type Predicate struct {
	Attribute Attribute
	Operator  Operator
	Value     Value
	Field     string
}

type Binary struct {
	Op          string
	Left, Right Expr
}

type Unary struct {
	Op   string
	Expr Expr
}

func (Predicate) queryExpr() {}
func (Binary) queryExpr()    {}
func (Unary) queryExpr()     {}

func And(left, right Expr) Expr {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	return Binary{Op: "and", Left: left, Right: right}
}

func Or(left, right Expr) Expr {
	return Binary{Op: "or", Left: left, Right: right}
}

func Xor(left, right Expr) Expr {
	return Binary{Op: "xor", Left: left, Right: right}
}

func Not(expr Expr) Expr {
	return Unary{Op: "not", Expr: expr}
}

func (p Predicate) String() string {
	attr := string(p.Attribute)
	if p.Attribute == AttrUDA && p.Field != "" {
		attr = "uda." + p.Field
	}
	if p.Operator == OpIsNull || p.Operator == OpNotNull {
		return fmt.Sprintf("%s %s", attr, p.Operator)
	}
	value := strconv.Quote(p.Value.Raw)
	return fmt.Sprintf("%s %s %s", attr, p.Operator, value)
}

func (b Binary) String() string {
	return fmt.Sprintf("(%s %s %s)", b.Left.String(), b.Op, b.Right.String())
}

func (u Unary) String() string {
	return fmt.Sprintf("(%s %s)", u.Op, u.Expr.String())
}
