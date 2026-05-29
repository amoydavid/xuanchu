package app

import (
	"strings"

	taskrcparser "github.com/dajee/taskg/internal/taskrc"
)

func (s *Service) ImportTaskRC(path string, dryRun bool) (taskrcparser.Report, error) {
	report, err := taskrcparser.ParseFile(path)
	if err != nil {
		return taskrcparser.Report{}, err
	}
	report.DryRun = dryRun
	if dryRun {
		return report, nil
	}
	var activeContextValue *string
	for key, value := range report.Values {
		if key == "context.active" {
			v := value
			activeContextValue = &v
			continue
		}
		if strings.HasPrefix(key, "context.") {
			name := strings.TrimPrefix(key, "context.")
			if err := s.DefineContext(name, value); err != nil {
				return report, err
			}
			continue
		}
		if key == "database.path" {
			continue
		}
		if err := s.SetConfig(key, value); err != nil {
			return report, err
		}
	}
	if activeContextValue != nil {
		value := strings.TrimSpace(*activeContextValue)
		if value == "" || strings.EqualFold(value, "none") {
			if err := s.ContextNone(); err != nil {
				return report, err
			}
		} else if err := s.UseContext(value); err != nil {
			return report, err
		}
	}
	return report, nil
}
