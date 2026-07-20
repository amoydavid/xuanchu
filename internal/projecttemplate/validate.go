package projecttemplate

import (
	"fmt"
	"regexp"
	"time"
)

var localRefPattern = regexp.MustCompile(`^(task|series|automation)-[1-9][0-9]*$`)

func ValidateSnapshot(snapshot Snapshot, limits Limits) error {
	if snapshot.Schema != SnapshotSchemaV1 {
		return invalid("snapshot schema must be v1")
	}
	if _, err := time.Parse("2006-01-02", snapshot.AnchorDate); err != nil {
		return invalid("anchor date must be YYYY-MM-DD")
	}
	if snapshot.Configs == nil || snapshot.Tasks == nil || snapshot.Series == nil || snapshot.Automations == nil {
		return invalid("top-level component arrays are required")
	}
	if len(snapshot.Tasks) > limits.MaxTasks || len(snapshot.Series) > limits.MaxSeries || len(snapshot.Configs) > limits.MaxConfigs || len(snapshot.Automations) > limits.MaxAutomations {
		return invalid("snapshot component count exceeds limit")
	}
	for _, config := range snapshot.Configs {
		if config.Key == "" || (config.Mode != "literal" && config.Mode != "secret_input") {
			return invalid("config key or mode is invalid")
		}
		if (config.Mode == "literal" && config.Value == nil) || (config.Mode == "secret_input" && config.Value != nil) {
			return invalid("config value does not match mode")
		}
		if err := checkTextLimit(config.Key, limits, "config key"); err != nil {
			return err
		}
		if config.Value != nil {
			if err := checkTextLimit(*config.Value, limits, "config value"); err != nil {
				return err
			}
		}
	}
	return validateReferencesAndText(snapshot, limits)
}

func checkTextLimit(value string, limits Limits, field string) error {
	if len(value) > limits.MaxTextBytes {
		return invalid(fmt.Sprintf("%s exceeds maximum text size", field))
	}
	return nil
}

func validateReferencesAndText(snapshot Snapshot, limits Limits) error {
	taskRefs := make(map[string]struct{}, len(snapshot.Tasks))
	for _, task := range snapshot.Tasks {
		if !validLocalRef(task.Ref, "task") || task.Title == "" {
			return invalid("task ref or title is invalid")
		}
		if _, exists := taskRefs[task.Ref]; exists {
			return invalid("task refs must be unique")
		}
		taskRefs[task.Ref] = struct{}{}
		if err := validateTaskText(task, limits); err != nil {
			return err
		}
	}
	for _, series := range snapshot.Series {
		if !validLocalRef(series.Ref, "series") || series.Title == "" || series.RecurrenceRule == "" {
			return invalid("series ref, title, or recurrence rule is invalid")
		}
		if err := validateSeriesText(series, limits); err != nil {
			return err
		}
	}
	for _, automation := range snapshot.Automations {
		if !validLocalRef(automation.Ref, "automation") || automation.Name == "" || automation.TriggerType == "" || automation.InstructionTemplate == "" {
			return invalid("automation required field is invalid")
		}
		if err := validateAutomationText(automation, limits); err != nil {
			return err
		}
	}
	for _, task := range snapshot.Tasks {
		for _, ref := range append(append([]string{}, task.DependsRefs...), ptrString(task.ParentRef)...) {
			if _, exists := taskRefs[ref]; !exists {
				return Error{Code: "project_template_dependency_missing", Message: "task reference target is not selected"}
			}
		}
	}
	if hasTaskReferenceCycle(snapshot.Tasks) {
		return Error{Code: "project_template_ref_cycle", Message: "task parent or dependency graph contains a cycle"}
	}
	return nil
}

func validLocalRef(ref, prefix string) bool {
	return localRefPattern.MatchString(ref) && len(ref) > len(prefix)+1 && ref[:len(prefix)+1] == prefix+"-"
}

func ptrString(value *string) []string {
	if value == nil {
		return nil
	}
	return []string{*value}
}

func hasTaskReferenceCycle(tasks []TaskBlueprintV1) bool {
	edges := make(map[string][]string, len(tasks))
	for _, task := range tasks {
		edges[task.Ref] = append(append([]string{}, task.DependsRefs...), ptrString(task.ParentRef)...)
	}
	state := make(map[string]uint8, len(tasks))
	var visit func(string) bool
	visit = func(ref string) bool {
		switch state[ref] {
		case 1:
			return true
		case 2:
			return false
		}
		state[ref] = 1
		for _, target := range edges[ref] {
			if visit(target) {
				return true
			}
		}
		state[ref] = 2
		return false
	}
	for _, task := range tasks {
		if visit(task.Ref) {
			return true
		}
	}
	return false
}

func validateTaskText(task TaskBlueprintV1, limits Limits) error {
	for field, value := range map[string]string{"task ref": task.Ref, "task title": task.Title} {
		if err := checkTextLimit(value, limits, field); err != nil {
			return err
		}
	}
	if task.Description != nil {
		if err := checkTextLimit(*task.Description, limits, "task description"); err != nil {
			return err
		}
		if err := validateMarkdownReferences(*task.Description); err != nil {
			return err
		}
	}
	return nil
}

func validateSeriesText(series SeriesBlueprintV1, limits Limits) error {
	if err := checkTextLimit(series.Title, limits, "series title"); err != nil {
		return err
	}
	if series.Description != nil {
		if err := checkTextLimit(*series.Description, limits, "series description"); err != nil {
			return err
		}
		if err := validateMarkdownReferences(*series.Description); err != nil {
			return err
		}
	}
	return nil
}

func validateAutomationText(automation AutomationBlueprintV1, limits Limits) error {
	for field, value := range map[string]string{
		"automation name": automation.Name, "automation description": automation.Description,
		"automation instruction": automation.InstructionTemplate, "automation system prompt": automation.SystemPrompt,
	} {
		if err := checkTextLimit(value, limits, field); err != nil {
			return err
		}
	}
	return nil
}
