package expr

import "testing"

func TestCalcArithmetic(t *testing.T) {
	got, err := Calc("1 + 2 * 3")
	if err != nil {
		t.Fatalf("Calc() error = %v", err)
	}
	if got != "7" {
		t.Fatalf("got %q, want 7", got)
	}
}

func TestCalcBoolean(t *testing.T) {
	got, err := Calc("3 > 2 and 1 < 2")
	if err != nil {
		t.Fatalf("Calc() error = %v", err)
	}
	if got != "true" {
		t.Fatalf("got %q, want true", got)
	}
}

func TestCalcFloatOutput(t *testing.T) {
	got, err := Calc("1 / 2")
	if err != nil {
		t.Fatalf("Calc() error = %v", err)
	}
	if got != "0.5" {
		t.Fatalf("got %q, want 0.5", got)
	}
}

func TestCalcM1Operators(t *testing.T) {
	tests := map[string]string{
		"2 ^ 3":            "8",
		"5 % 2":            "1",
		"true xor false":   "true",
		"true xor true":    "false",
		"1e3 + 2":          "1002",
		"(1 + 2) * 3 >= 9": "true",
	}
	for input, want := range tests {
		got, err := Calc(input)
		if err != nil {
			t.Fatalf("Calc(%q) error = %v", input, err)
		}
		if got != want {
			t.Fatalf("Calc(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCalcRejectsFunctionsStringsAndIndexes(t *testing.T) {
	for _, input := range []string{`len("abc")`, `"abc"`, `[1, 2][0]`, "5.5 % 2"} {
		if _, err := Calc(input); err == nil {
			t.Fatalf("Calc(%q) error = nil, want error", input)
		}
	}
}

func TestCalcRejectsInvalidExpression(t *testing.T) {
	if _, err := Calc("1 +"); err == nil {
		t.Fatal("Calc() error = nil, want error")
	}
}
