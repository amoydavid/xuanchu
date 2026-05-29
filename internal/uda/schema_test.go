package uda

import "testing"

func TestUDASchemaValidation(t *testing.T) {
	for _, typ := range []Type{TypeString, TypeNumeric, TypeDate, TypeDuration} {
		if err := ValidateDefinition(Definition{Name: "estimate", Type: typ}); err != nil {
			t.Fatalf("ValidateDefinition(%s) error = %v", typ, err)
		}
	}
	for _, name := range []string{"uuid", "description", "status", "entry", "modified", "end", "due", "start", "wait", "scheduled", "until", "project", "priority", "depends", "annotations", "recur", "parent", "tag", "mask", "imask"} {
		if err := ValidateDefinition(Definition{Name: name, Type: TypeString}); err == nil {
			t.Fatalf("ValidateDefinition(%q) error = nil, want conflict", name)
		}
	}
	if err := ValidateDefinition(Definition{Name: "estimate", Type: TypeNumeric, Values: []string{"1", "2"}}); err != nil {
		t.Fatalf("numeric enum validation error = %v", err)
	}
}

func TestNormalizeValue(t *testing.T) {
	got, err := NormalizeValue(Definition{Name: "estimate", Type: TypeNumeric}, "3.50")
	if err != nil {
		t.Fatalf("NormalizeValue(numeric) error = %v", err)
	}
	if got != "3.5" {
		t.Fatalf("numeric value = %q", got)
	}
	got, err = NormalizeValue(Definition{Name: "reviewed", Type: TypeDate}, "2026-05-28")
	if err != nil {
		t.Fatalf("NormalizeValue(date) error = %v", err)
	}
	if got != "2026-05-28T00:00:00Z" {
		t.Fatalf("date value = %q", got)
	}
	got, err = NormalizeValue(Definition{Name: "effort", Type: TypeDuration}, "1h")
	if err != nil {
		t.Fatalf("NormalizeValue(duration) error = %v", err)
	}
	if got != "3600" {
		t.Fatalf("duration value = %q", got)
	}
}
