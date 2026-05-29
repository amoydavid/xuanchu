package taskrc

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Entry struct {
	Key    string `json:"key"`
	Target string `json:"target,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type Report struct {
	Imported []Entry           `json:"imported"`
	Skipped  []Entry           `json:"skipped"`
	Unknown  []Entry           `json:"unknown"`
	DryRun   bool              `json:"dry_run"`
	Values   map[string]string `json:"-"`
}

func ParseFile(path string) (Report, error) {
	report := Report{Values: map[string]string{}}
	seen := map[string]bool{}
	if err := parseFile(path, &report, seen); err != nil {
		return Report{}, err
	}
	return report, nil
}

func parseFile(path string, report *Report, seen map[string]bool) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if seen[abs] {
		return nil
	}
	seen[abs] = true
	file, err := os.Open(abs)
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	dir := filepath.Dir(abs)
	for scanner.Scan() {
		line := stripComment(strings.TrimSpace(scanner.Text()))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "include ") {
			includePath := strings.TrimSpace(strings.TrimPrefix(line, "include "))
			includePath = strings.Trim(includePath, `"'`)
			if !filepath.IsAbs(includePath) {
				includePath = filepath.Join(dir, includePath)
			}
			if err := parseFile(includePath, report, seen); err != nil {
				return err
			}
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("invalid taskrc line %q", line)
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"`)
		target, class, reason := classify(key)
		entry := Entry{Key: key, Target: target, Reason: reason}
		switch class {
		case "imported":
			report.Imported = append(report.Imported, entry)
			report.Values[target] = value
		case "skipped":
			report.Skipped = append(report.Skipped, entry)
		default:
			report.Unknown = append(report.Unknown, entry)
		}
	}
	return scanner.Err()
}

func stripComment(line string) string {
	if idx := strings.Index(line, "#"); idx >= 0 {
		return strings.TrimSpace(line[:idx])
	}
	return line
}

func classify(key string) (target, class, reason string) {
	switch key {
	case "data.location":
		return "database.path", "skipped", "database.path is read-only in M3; use --db, TASKG_DB, --data-dir, or TOML"
	case "color":
		return "color", "imported", ""
	case "dateformat":
		return "date.format", "imported", ""
	}
	if strings.HasPrefix(key, "context.") {
		return key, "imported", ""
	}
	if strings.HasPrefix(key, "uda.") {
		parts := strings.Split(key, ".")
		field := parts[len(parts)-1]
		switch field {
		case "type", "label", "values", "default":
			return key, "imported", ""
		}
	}
	if strings.HasPrefix(key, "urgency.uda.") {
		return key, "imported", ""
	}
	for _, prefix := range []string{"report.", "calendar.", "burndown.", "news.", "sync.", "hooks."} {
		if strings.HasPrefix(key, prefix) {
			return key, "skipped", "not supported in M3"
		}
	}
	return key, "unknown", "unknown key"
}
