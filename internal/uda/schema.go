package uda

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Type string

const (
	TypeString   Type = "string"
	TypeNumeric  Type = "numeric"
	TypeDate     Type = "date"
	TypeDuration Type = "duration"
)

type Definition struct {
	Name          string
	Type          Type
	Label         string
	Values        []string
	Default       string
	OrphanAllowed bool
}

func ValidateDefinition(def Definition) error {
	if strings.TrimSpace(def.Name) == "" {
		return fmt.Errorf("UDA name is required")
	}
	if isReservedName(def.Name) {
		return fmt.Errorf("UDA name %q conflicts with built-in attribute", def.Name)
	}
	switch def.Type {
	case TypeString, TypeNumeric, TypeDate, TypeDuration:
	default:
		return fmt.Errorf("invalid UDA type %q", def.Type)
	}
	for _, value := range def.Values {
		if _, err := NormalizeValue(Definition{Name: def.Name, Type: def.Type}, value); err != nil {
			return err
		}
	}
	return nil
}

func NormalizeValue(def Definition, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	var value string
	switch def.Type {
	case TypeString:
		if strings.ContainsAny(raw, "\n\r") {
			return "", fmt.Errorf("UDA %s must not contain newlines", def.Name)
		}
		value = raw
	case TypeNumeric:
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return "", fmt.Errorf("invalid numeric UDA %s: %w", def.Name, err)
		}
		value = strconv.FormatFloat(f, 'f', -1, 64)
	case TypeDate:
		t, err := parseDate(raw)
		if err != nil {
			return "", fmt.Errorf("invalid date UDA %s: %w", def.Name, err)
		}
		value = t.UTC().Format(time.RFC3339)
	case TypeDuration:
		sec, err := parseDurationSeconds(raw)
		if err != nil {
			return "", fmt.Errorf("invalid duration UDA %s: %w", def.Name, err)
		}
		value = strconv.FormatInt(sec, 10)
	default:
		return "", fmt.Errorf("invalid UDA type %q", def.Type)
	}
	if len(def.Values) > 0 && !contains(def.Values, value) && !contains(def.Values, raw) {
		return "", fmt.Errorf("UDA %s value %q is not in enum", def.Name, raw)
	}
	return value, nil
}

func ValuesJSON(values []string) (string, error) {
	if values == nil {
		return "", nil
	}
	data, err := json.Marshal(values)
	return string(data), err
}

func ParseValuesCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func isReservedName(name string) bool {
	switch name {
	case "uuid", "description", "status", "entry", "modified", "end", "due", "start", "wait", "scheduled", "until", "project", "priority", "depends", "annotations", "recur", "parent", "tag":
		return true
	default:
		return false
	}
}

func parseDate(raw string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	return time.ParseInLocation("2006-01-02", raw, time.UTC)
}

func parseDurationSeconds(raw string) (int64, error) {
	switch {
	case strings.HasSuffix(raw, "min"):
		n, err := strconv.ParseInt(strings.TrimSuffix(raw, "min"), 10, 64)
		return n * 60, err
	case strings.HasSuffix(raw, "h"):
		n, err := strconv.ParseInt(strings.TrimSuffix(raw, "h"), 10, 64)
		return n * 3600, err
	case strings.HasSuffix(raw, "days"):
		n, err := strconv.ParseInt(strings.TrimSuffix(raw, "days"), 10, 64)
		return n * 86400, err
	case strings.HasSuffix(raw, "d"):
		n, err := strconv.ParseInt(strings.TrimSuffix(raw, "d"), 10, 64)
		return n * 86400, err
	case strings.HasSuffix(raw, "w"):
		n, err := strconv.ParseInt(strings.TrimSuffix(raw, "w"), 10, 64)
		return n * 7 * 86400, err
	default:
		return strconv.ParseInt(raw, 10, 64)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
