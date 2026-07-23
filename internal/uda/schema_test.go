package uda

import (
	"reflect"
	"strings"
	"testing"
)

func TestUDASchemaValidation(t *testing.T) {
	for _, typ := range []Type{TypeString, TypeNumeric, TypeDate, TypeDuration} {
		if err := ValidateDefinition(Definition{Name: "estimate", Type: typ}); err != nil {
			t.Fatalf("ValidateDefinition(%s) error = %v", typ, err)
		}
	}
	for _, name := range []string{"uuid", "title", "description", "status", "entry", "modified", "end", "due", "start", "wait", "scheduled", "until", "project", "priority", "depends", "annotations", "recur", "parent", "tag", "mask", "imask"} {
		if err := ValidateDefinition(Definition{Name: name, Type: TypeString}); err == nil {
			t.Fatalf("ValidateDefinition(%q) error = nil, want conflict", name)
		}
	}
	if err := ValidateDefinition(Definition{Name: "estimate", Type: TypeNumeric, Values: []string{"1", "2"}}); err != nil {
		t.Fatalf("numeric enum validation error = %v", err)
	}
}

func TestUDASchemaRejectsTaskSlugReservedNames(t *testing.T) {
	for _, name := range []string{"task_slug", "project_seq"} {
		if err := ValidateDefinition(Definition{Name: name, Type: TypeString}); err == nil || !strings.Contains(err.Error(), "built-in attribute") {
			t.Fatalf("ValidateDefinition(%s) error = %v, want reserved", name, err)
		}
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

func TestNormalizeDurationValueVariants(t *testing.T) {
	cases := map[string]string{
		"30min": "1800",
		"7d":    "604800",
		"2days": "172800",
		"42":    "42",
	}
	for raw, want := range cases {
		got, err := NormalizeValue(Definition{Name: "effort", Type: TypeDuration}, raw)
		if err != nil {
			t.Fatalf("NormalizeValue(%q) error = %v", raw, err)
		}
		if got != want {
			t.Fatalf("NormalizeValue(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestNormalizeDurationValueRejectsInvalid(t *testing.T) {
	if _, err := NormalizeValue(Definition{Name: "effort", Type: TypeDuration}, "oops"); err == nil {
		t.Fatal("NormalizeValue(invalid duration) error = nil, want error")
	}
}

func TestValuesJSONAndParseValuesCSV(t *testing.T) {
	jsonValue, err := ValuesJSON([]string{"1", "2", "3"})
	if err != nil {
		t.Fatalf("ValuesJSON() error = %v", err)
	}
	if jsonValue != `["1","2","3"]` {
		t.Fatalf("ValuesJSON() = %q, want JSON array", jsonValue)
	}
	emptyJSON, err := ValuesJSON(nil)
	if err != nil {
		t.Fatalf("ValuesJSON(nil) error = %v", err)
	}
	if emptyJSON != "" {
		t.Fatalf("ValuesJSON(nil) = %q, want empty string", emptyJSON)
	}

	got := ParseValuesCSV("1, 2, ,3,, 5 ")
	want := []string{"1", "2", "3", "5"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseValuesCSV() = %#v, want %#v", got, want)
	}
	if got := ParseValuesCSV("   "); got != nil {
		t.Fatalf("ParseValuesCSV(blank) = %#v, want nil", got)
	}
}

func TestNormalizeStringValueRejectsNewlines(t *testing.T) {
	if _, err := NormalizeValue(Definition{Name: "notes", Type: TypeString}, "line1\nline2"); err == nil {
		t.Fatal("NormalizeValue(string with newline) error = nil, want error")
	}
}

func TestNormalizeDefinitionCanonicalizesValuesAndDefault(t *testing.T) {
	got, err := NormalizeDefinition(Definition{
		Name: " estimate ", Type: TypeNumeric, Label: " 工作量 ",
		Values: []string{"1.0", "2", "1"}, Default: "2.0",
	})
	if err != nil {
		t.Fatalf("NormalizeDefinition() error = %v", err)
	}
	wantValues := []string{"1", "2"}
	if got.Name != "estimate" || got.Label != "工作量" || got.Default != "2" || !reflect.DeepEqual(got.Values, wantValues) {
		t.Fatalf("NormalizeDefinition() = %#v, want values=%#v default=2", got, wantValues)
	}
}

func TestNormalizeDefinitionRejectsDefaultOutsideCanonicalEnum(t *testing.T) {
	_, err := NormalizeDefinition(Definition{Name: "estimate", Type: TypeNumeric, Values: []string{"1.0", "2"}, Default: "3"})
	if err == nil {
		t.Fatal("NormalizeDefinition() error = nil, want enum error")
	}
}
