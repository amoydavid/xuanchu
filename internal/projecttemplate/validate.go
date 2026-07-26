package projecttemplate

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var localRefPattern = regexp.MustCompile(`^(task|series|automation)-[1-9][0-9]*$`)

func ValidateSnapshot(snapshot SnapshotV1, limits Limits) error {
	return validateSnapshot(snapshotFromV1(snapshot), limits)
}

func ValidateSnapshotV2(snapshot SnapshotV2, limits Limits) error {
	return validateSnapshot(snapshotFromV2(snapshot), limits)
}

func validateSnapshot(snapshot Snapshot, limits Limits) error {
	if err := validatePersistedText(snapshot, limits); err != nil {
		return err
	}
	if snapshot.Schema != SnapshotSchemaV1 && snapshot.Schema != SnapshotSchemaV2 {
		return invalid("snapshot schema is unsupported")
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
	configKeys := make(map[string]struct{}, len(snapshot.Configs))
	for _, config := range snapshot.Configs {
		if config.Key == "" {
			return invalid("config key or mode is invalid")
		}
		if snapshot.Schema == SnapshotSchemaV2 {
			if _, exists := configKeys[config.Key]; exists {
				return invalid("config keys must be unique")
			}
			configKeys[config.Key] = struct{}{}
			if config.Mode != "literal" && config.Mode != "secret_copy" && config.Mode != "inherit" && config.Mode != "prompt" {
				return invalid("config key or mode is invalid")
			}
			if (config.Mode == "literal" && (config.Value == nil || config.SecretCiphertext != nil || config.Prompt != nil)) ||
				(config.Mode == "secret_copy" && (config.Value != nil || config.SecretCiphertext == nil || strings.TrimSpace(*config.SecretCiphertext) == "" || config.Prompt != nil)) ||
				(config.Mode == "inherit" && (config.Value != nil || config.SecretCiphertext != nil || config.Prompt != nil)) ||
				(config.Mode == "prompt" && (config.Value != nil || config.SecretCiphertext != nil || config.Prompt == nil)) {
				return invalid("config value does not match mode")
			}
			continue
		}
		if config.Mode != "literal" && config.Mode != "secret_input" && config.Mode != "secret_copy" {
			return invalid("config key or mode is invalid")
		}
		if config.Prompt != nil ||
			(config.Mode == "literal" && (config.Value == nil || config.SecretCiphertext != nil)) ||
			(config.Mode == "secret_input" && (config.Value != nil || config.SecretCiphertext != nil)) ||
			(config.Mode == "secret_copy" && (config.Value != nil || config.SecretCiphertext == nil || strings.TrimSpace(*config.SecretCiphertext) == "")) {
			return invalid("config value does not match mode")
		}
	}
	return validateReferencesAndText(snapshot)
}

func checkTextLimit(value string, limits Limits, field string) error {
	if len(value) > limits.MaxTextBytes {
		return invalid(fmt.Sprintf("%s exceeds maximum text size", field))
	}
	return nil
}

func validateReferencesAndText(snapshot Snapshot) error {
	taskRefs := make(map[string]struct{}, len(snapshot.Tasks))
	for _, task := range snapshot.Tasks {
		if !validLocalRef(task.Ref, "task") || task.Title == "" {
			return invalid("task ref or title is invalid")
		}
		if _, exists := taskRefs[task.Ref]; exists {
			return invalid("task refs must be unique")
		}
		taskRefs[task.Ref] = struct{}{}
	}
	seriesRefs := make(map[string]struct{}, len(snapshot.Series))
	for _, series := range snapshot.Series {
		if !validLocalRef(series.Ref, "series") || series.Title == "" || series.RecurrenceRule == "" {
			return invalid("series ref, title, or recurrence rule is invalid")
		}
		if _, exists := seriesRefs[series.Ref]; exists {
			return invalid("series refs must be unique")
		}
		seriesRefs[series.Ref] = struct{}{}
		if err := validateRelativeLocalTime(series.FirstDue, "series first due"); err != nil {
			return err
		}
		if series.Until != nil {
			if err := validateRelativeLocalTime(*series.Until, "series until"); err != nil {
				return err
			}
		}
	}
	automationRefs := make(map[string]struct{}, len(snapshot.Automations))
	for _, automation := range snapshot.Automations {
		if !validLocalRef(automation.Ref, "automation") || automation.Name == "" || automation.TriggerType == "" || automation.InstructionTemplate == "" {
			return invalid("automation required field is invalid")
		}
		if automation.Context.Include == nil {
			return invalid("automation context include is required")
		}
		if _, exists := automationRefs[automation.Ref]; exists {
			return invalid("automation refs must be unique")
		}
		automationRefs[automation.Ref] = struct{}{}
	}
	for _, task := range snapshot.Tasks {
		if err := validateTaskDates(task.Dates); err != nil {
			return err
		}
		for _, ref := range append(append([]string{}, task.DependsRefs...), ptrString(task.ParentRef)...) {
			if _, exists := taskRefs[ref]; !exists {
				return Error{Code: "project_template_dependency_missing", Message: "task reference target is not selected"}
			}
		}
		if task.Description != nil {
			if err := validateMarkdownReferences(*task.Description, taskRefs); err != nil {
				return err
			}
		}
	}
	for _, series := range snapshot.Series {
		if series.Description != nil {
			if err := validateMarkdownReferences(*series.Description, taskRefs); err != nil {
				return err
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

func validateTaskDates(dates TaskDatesV1) error {
	values := []struct {
		name  string
		value *RelativeLocalTimeV1
	}{
		{"task due", dates.Due}, {"task wait", dates.Wait},
		{"task scheduled", dates.Scheduled}, {"task until", dates.Until},
	}
	for _, item := range values {
		if item.value != nil {
			if err := validateRelativeLocalTime(*item.value, item.name); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateRelativeLocalTime(value RelativeLocalTimeV1, field string) error {
	clock, err := time.Parse("15:04:05", value.LocalTime)
	if err != nil || clock.Format("15:04:05") != value.LocalTime {
		return invalid(fmt.Sprintf("%s local time must be HH:MM:SS", field))
	}
	return nil
}

func validatePersistedText(snapshot Snapshot, limits Limits) error {
	check := func(field, value string) error { return checkTextLimit(value, limits, field) }
	checkPtr := func(field string, value *string) error {
		if value == nil {
			return nil
		}
		return check(field, *value)
	}
	if err := check("schema", snapshot.Schema); err != nil {
		return err
	}
	if err := check("anchor date", snapshot.AnchorDate); err != nil {
		return err
	}
	if err := check("project description", snapshot.Project.Description); err != nil {
		return err
	}
	for _, config := range snapshot.Configs {
		for field, value := range map[string]string{"config key": config.Key, "config mode": config.Mode} {
			if err := check(field, value); err != nil {
				return err
			}
		}
		if err := checkPtr("config value", config.Value); err != nil {
			return err
		}
	}
	for _, task := range snapshot.Tasks {
		for field, value := range map[string]string{"task ref": task.Ref, "task title": task.Title} {
			if err := check(field, value); err != nil {
				return err
			}
		}
		if err := checkPtr("task description", task.Description); err != nil {
			return err
		}
		if err := checkPtr("task priority", task.Priority); err != nil {
			return err
		}
		if err := checkPtr("task parent ref", task.ParentRef); err != nil {
			return err
		}
		if err := checkStrings(check, "task tag", task.Tags); err != nil {
			return err
		}
		if err := checkStrings(check, "task assignee ID", task.AssigneeIDs); err != nil {
			return err
		}
		if err := checkStrings(check, "task dependency ref", task.DependsRefs); err != nil {
			return err
		}
		if err := checkUDAText(check, task.UDAs); err != nil {
			return err
		}
		for _, link := range task.Links {
			for field, value := range map[string]string{"task link type": link.Type, "task link URL": link.URL, "task link title": link.Title} {
				if err := check(field, value); err != nil {
					return err
				}
			}
		}
		if err := checkTaskDateText(check, task.Dates); err != nil {
			return err
		}
	}
	for _, series := range snapshot.Series {
		for field, value := range map[string]string{"series ref": series.Ref, "series title": series.Title, "series recurrence rule": series.RecurrenceRule} {
			if err := check(field, value); err != nil {
				return err
			}
		}
		if err := checkPtr("series description", series.Description); err != nil {
			return err
		}
		if err := checkPtr("series priority", series.Priority); err != nil {
			return err
		}
		if err := checkStrings(check, "series tag", series.Tags); err != nil {
			return err
		}
		if err := checkStrings(check, "series assignee ID", series.AssigneeIDs); err != nil {
			return err
		}
		if err := checkUDAText(check, series.UDAs); err != nil {
			return err
		}
		if err := check("series first due local time", series.FirstDue.LocalTime); err != nil {
			return err
		}
		if series.Until != nil {
			if err := check("series until local time", series.Until.LocalTime); err != nil {
				return err
			}
		}
	}
	for _, automation := range snapshot.Automations {
		values := map[string]string{
			"automation ref": automation.Ref, "automation name": automation.Name,
			"automation description": automation.Description, "automation trigger type": automation.TriggerType,
			"automation schedule type":            automation.TriggerConfig.ScheduleType,
			"automation schedule value":           automation.TriggerConfig.ScheduleValue,
			"automation timezone":                 automation.TriggerConfig.Timezone,
			"automation event type":               automation.TriggerConfig.EventType,
			"automation task filter":              automation.Condition.TaskFilter,
			"automation protocol":                 automation.Action.Protocol,
			"automation base URL config key":      automation.Action.BaseURLConfigKey,
			"automation API key config key":       automation.Action.APIKeyConfigKey,
			"automation model config key":         automation.Action.ModelConfigKey,
			"automation allowed hosts config key": automation.Action.AllowedHostsConfigKey,
			"automation model override":           automation.Action.ModelOverride,
			"automation instruction template":     automation.InstructionTemplate,
			"automation system prompt":            automation.SystemPrompt,
		}
		for field, value := range values {
			if err := check(field, value); err != nil {
				return err
			}
		}
		if err := checkStrings(check, "automation context include", automation.Context.Include); err != nil {
			return err
		}
	}
	return nil
}

func checkStrings(check func(string, string) error, field string, values []string) error {
	for _, value := range values {
		if err := check(field, value); err != nil {
			return err
		}
	}
	return nil
}

func checkUDAText(check func(string, string) error, udas map[string]UDABlueprintV1) error {
	keys := make(map[string]struct{}, len(udas))
	for key, value := range udas {
		trimmedKey := strings.TrimSpace(key)
		if _, exists := keys[trimmedKey]; exists {
			return invalid("UDA keys collide after normalization")
		}
		keys[trimmedKey] = struct{}{}
		for field, text := range map[string]string{"UDA key": key, "UDA raw": value.Raw, "UDA type": value.Type} {
			if err := check(field, text); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkTaskDateText(check func(string, string) error, dates TaskDatesV1) error {
	for field, value := range map[string]*RelativeLocalTimeV1{
		"task due local time": dates.Due, "task wait local time": dates.Wait,
		"task scheduled local time": dates.Scheduled, "task until local time": dates.Until,
	} {
		if value != nil {
			if err := check(field, value.LocalTime); err != nil {
				return err
			}
		}
	}
	return nil
}
