package query

import "testing"

func TestAndFlattensNilExpressions(t *testing.T) {
	left := Predicate{Attribute: AttrStatus, Operator: OpEqual, Value: StringValue("pending")}
	got := And(nil, left)
	if got == nil {
		t.Fatal("And() returned nil")
	}
	if got.String() != `status eq "pending"` {
		t.Fatalf("String() = %q", got.String())
	}
}

func TestPredicateStringForTag(t *testing.T) {
	p := Predicate{Attribute: AttrTag, Operator: OpHasTag, Value: StringValue("next")}
	if got := p.String(); got != `tag has "next"` {
		t.Fatalf("String() = %q", got)
	}
}
