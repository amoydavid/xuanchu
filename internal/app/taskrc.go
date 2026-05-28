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
	for key, value := range report.Values {
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
	return report, nil
}
