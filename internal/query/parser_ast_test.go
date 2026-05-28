package query

import (
	"strings"
	"testing"
)

func TestParseQueryImplicitAnd(t *testing.T) {
	expr, err := ParseQuery(`+work status:pending`)
	if err != nil {
		t.Fatalf("ParseQuery() error = %v", err)
	}
	if got := expr.String(); got != `(tag has "work" and status eq "pending")` {
		t.Fatalf("String() = %q", got)
	}
}

func TestParseQueryBooleanPrecedence(t *testing.T) {
	expr, err := ParseQuery(`+next or due.before:tomorrow and priority:H`)
	if err != nil {
		t.Fatalf("ParseQuery() error = %v", err)
	}
	want := `(tag has "next" or (due before "tomorrow" and priority eq "H"))`
	if got := expr.String(); got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestParseQueryParentheses(t *testing.T) {
	expr, err := ParseQuery(`(project:work and +urgent) or priority:H`)
	if err != nil {
		t.Fatalf("ParseQuery() error = %v", err)
	}
	want := `((project eq "work" and tag has "urgent") or priority eq "H")`
	if got := expr.String(); got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestParseQueryRecognizesDynamicUDA(t *testing.T) {
	for _, input := range []string{`estimate:3`, `estimate.notnull`, `estimate:`, `uda.reviewed:2026-05-28`} {
		expr, err := ParseQuery(input)
		if err != nil {
			t.Fatalf("ParseQuery(%q) error = %v", input, err)
		}
		if !strings.Contains(expr.String(), "uda.") {
			t.Fatalf("ParseQuery(%q) = %s, want UDA predicate", input, expr.String())
		}
	}
}

func TestParseQueryBareTokenStaysBare(t *testing.T) {
	expr, err := ParseQuery(`abc123`)
	if err != nil {
		t.Fatalf("ParseQuery() error = %v", err)
	}
	if got := expr.String(); got != `bare contains "abc123"` {
		t.Fatalf("String() = %q", got)
	}
}

func TestParseQueryRejectsUnbalancedParens(t *testing.T) {
	if _, err := ParseQuery(`(+work or priority:H`); err == nil {
		t.Fatal("ParseQuery() error = nil, want error")
	}
}

func TestParseQueryNotExpression(t *testing.T) {
	expr, err := ParseQuery(`not +work`)
	if err != nil {
		t.Fatalf("ParseQuery() error = %v", err)
	}
	if got := expr.String(); got != `(not tag has "work")` {
		t.Fatalf("String() = %q", got)
	}
}

func TestParseQueryEmptyInput(t *testing.T) {
	if expr, err := ParseQuery(`   `); err != nil || expr != nil {
		t.Fatalf("ParseQuery(empty) = %#v, %v; want nil, nil", expr, err)
	}
}

func TestParseQueryM2Attributes(t *testing.T) {
	expr, err := ParseQuery(`wait: scheduled.before:eow start.notnull until: depends:abc annotations:note recur:weekly parent:p1`)
	if err != nil {
		t.Fatalf("ParseQuery() error = %v", err)
	}
	got := expr.String()
	for _, part := range []string{
		`wait is_null`,
		`scheduled before "eow"`,
		`start not_null`,
		`until is_null`,
		`depends eq "abc"`,
		`annotations contains "note"`,
		`recur eq "weekly"`,
		`parent eq "p1"`,
	} {
		if !strings.Contains(got, part) {
			t.Fatalf("String() = %q, missing %q", got, part)
		}
	}
}
