package projecttemplate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

func EncodeV1(in SnapshotV1, limits Limits) ([]byte, string, error) {
	normalized, err := normalizeV1(in)
	if err != nil {
		return nil, "", err
	}
	if err := ValidateSnapshot(normalized, limits); err != nil {
		return nil, "", err
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return nil, "", invalid(fmt.Sprintf("encode snapshot: %v", err))
	}
	if len(raw) > limits.MaxJSONBytes {
		return nil, "", invalid("snapshot exceeds maximum JSON size")
	}
	sum := sha256.Sum256(raw)
	return raw, hex.EncodeToString(sum[:]), nil
}

func Decode(raw []byte, limits Limits) (Snapshot, error) {
	if len(raw) > limits.MaxJSONBytes {
		return Snapshot{}, invalid("snapshot exceeds maximum JSON size")
	}
	var header struct {
		Schema json.RawMessage `json:"schema"`
	}
	if err := json.Unmarshal(raw, &header); err != nil {
		return Snapshot{}, invalid("decode snapshot header")
	}
	if len(header.Schema) == 0 || bytes.Equal(header.Schema, []byte("null")) {
		return Snapshot{}, invalid("snapshot schema is required")
	}
	var schema string
	if err := json.Unmarshal(header.Schema, &schema); err != nil {
		return Snapshot{}, invalid("snapshot schema must be a string")
	}
	if schema != SnapshotSchemaV1 {
		return Snapshot{}, Error{Code: "project_template_snapshot_schema_unsupported", Message: "snapshot schema is unsupported"}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var snapshot SnapshotV1
	if err := decoder.Decode(&snapshot); err != nil {
		return Snapshot{}, invalid("decode snapshot")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Snapshot{}, invalid("snapshot contains trailing JSON")
	}
	if snapshot.Configs == nil || snapshot.Tasks == nil || snapshot.Series == nil || snapshot.Automations == nil {
		return Snapshot{}, invalid("top-level component arrays are required")
	}
	if err := ValidateSnapshot(snapshot, limits); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func normalizeV1(in SnapshotV1) (SnapshotV1, error) {
	out := in
	out.Schema = strings.TrimSpace(out.Schema)
	out.AnchorDate = strings.TrimSpace(out.AnchorDate)
	out.Project.Description = strings.TrimSpace(out.Project.Description)
	out.Configs = append([]ConfigBlueprintV1{}, in.Configs...)
	out.Tasks = append([]TaskBlueprintV1{}, in.Tasks...)
	out.Series = append([]SeriesBlueprintV1{}, in.Series...)
	out.Automations = append([]AutomationBlueprintV1{}, in.Automations...)
	var err error

	for i := range out.Configs {
		out.Configs[i].Key = strings.TrimSpace(out.Configs[i].Key)
		out.Configs[i].Mode = strings.TrimSpace(out.Configs[i].Mode)
		out.Configs[i].Value = trimPtr(out.Configs[i].Value)
	}
	sort.Slice(out.Configs, func(i, j int) bool { return lessConfig(out.Configs[i], out.Configs[j]) })
	out.Configs = uniqueConfigs(out.Configs)
	for i := range out.Tasks {
		out.Tasks[i], err = normalizeTask(out.Tasks[i])
		if err != nil {
			return SnapshotV1{}, err
		}
	}
	for i := range out.Series {
		out.Series[i], err = normalizeSeries(out.Series[i])
		if err != nil {
			return SnapshotV1{}, err
		}
	}
	for i := range out.Automations {
		out.Automations[i] = normalizeAutomation(out.Automations[i])
	}
	return out, nil
}

func lessConfig(left, right ConfigBlueprintV1) bool {
	if left.Key != right.Key {
		return left.Key < right.Key
	}
	if left.Mode != right.Mode {
		return left.Mode < right.Mode
	}
	return configValue(left.Value) < configValue(right.Value)
}

func uniqueConfigs(configs []ConfigBlueprintV1) []ConfigBlueprintV1 {
	if len(configs) < 2 {
		return configs
	}
	out := configs[:0]
	for _, config := range configs {
		if len(out) != 0 && out[len(out)-1].Key == config.Key {
			continue
		}
		out = append(out, config)
	}
	return out
}

func configValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func normalizeTask(in TaskBlueprintV1) (TaskBlueprintV1, error) {
	in.Ref, in.Title = strings.TrimSpace(in.Ref), strings.TrimSpace(in.Title)
	in.Description, in.Priority, in.ParentRef = trimPtr(in.Description), trimPtr(in.Priority), trimPtr(in.ParentRef)
	in.Tags = sortUniqueTrim(in.Tags)
	in.AssigneeIDs = sortUniqueTrim(in.AssigneeIDs)
	in.DependsRefs = sortUniqueTrim(in.DependsRefs)
	in.Dates = normalizeDates(in.Dates)
	in.Links = append([]TaskLinkBlueprintV1{}, in.Links...)
	for i := range in.Links {
		in.Links[i].Type = strings.TrimSpace(in.Links[i].Type)
		in.Links[i].URL = strings.TrimSpace(in.Links[i].URL)
		in.Links[i].Title = strings.TrimSpace(in.Links[i].Title)
	}
	var err error
	in.UDAs, err = normalizeUDAs(in.UDAs)
	return in, err
}

func normalizeSeries(in SeriesBlueprintV1) (SeriesBlueprintV1, error) {
	in.Ref, in.Title = strings.TrimSpace(in.Ref), strings.TrimSpace(in.Title)
	in.Description, in.Priority = trimPtr(in.Description), trimPtr(in.Priority)
	in.Tags, in.AssigneeIDs = sortUniqueTrim(in.Tags), sortUniqueTrim(in.AssigneeIDs)
	in.RecurrenceRule = strings.TrimSpace(in.RecurrenceRule)
	in.FirstDue = normalizeRelativeTime(in.FirstDue)
	if in.Until != nil {
		until := normalizeRelativeTime(*in.Until)
		in.Until = &until
	}
	var err error
	in.UDAs, err = normalizeUDAs(in.UDAs)
	return in, err
}

func normalizeAutomation(in AutomationBlueprintV1) AutomationBlueprintV1 {
	in.Ref, in.Name = strings.TrimSpace(in.Ref), strings.TrimSpace(in.Name)
	in.Description, in.TriggerType = strings.TrimSpace(in.Description), strings.TrimSpace(in.TriggerType)
	in.TriggerConfig.ScheduleType = strings.TrimSpace(in.TriggerConfig.ScheduleType)
	in.TriggerConfig.ScheduleValue = strings.TrimSpace(in.TriggerConfig.ScheduleValue)
	in.TriggerConfig.Timezone = strings.TrimSpace(in.TriggerConfig.Timezone)
	in.TriggerConfig.EventType = strings.TrimSpace(in.TriggerConfig.EventType)
	in.Condition.TaskFilter = strings.TrimSpace(in.Condition.TaskFilter)
	in.Action.Protocol = strings.TrimSpace(in.Action.Protocol)
	in.Action.BaseURLConfigKey = strings.TrimSpace(in.Action.BaseURLConfigKey)
	in.Action.APIKeyConfigKey = strings.TrimSpace(in.Action.APIKeyConfigKey)
	in.Action.ModelConfigKey = strings.TrimSpace(in.Action.ModelConfigKey)
	in.Action.AllowedHostsConfigKey = strings.TrimSpace(in.Action.AllowedHostsConfigKey)
	in.Action.ModelOverride = strings.TrimSpace(in.Action.ModelOverride)
	in.Context.Include = sortUniqueTrim(in.Context.Include)
	in.InstructionTemplate = strings.TrimSpace(in.InstructionTemplate)
	in.SystemPrompt = strings.TrimSpace(in.SystemPrompt)
	return in
}

func normalizeDates(in TaskDatesV1) TaskDatesV1 {
	if in.Due != nil {
		value := normalizeRelativeTime(*in.Due)
		in.Due = &value
	}
	if in.Wait != nil {
		value := normalizeRelativeTime(*in.Wait)
		in.Wait = &value
	}
	if in.Scheduled != nil {
		value := normalizeRelativeTime(*in.Scheduled)
		in.Scheduled = &value
	}
	if in.Until != nil {
		value := normalizeRelativeTime(*in.Until)
		in.Until = &value
	}
	return in
}

func normalizeRelativeTime(in RelativeLocalTimeV1) RelativeLocalTimeV1 {
	in.LocalTime = strings.TrimSpace(in.LocalTime)
	return in
}

func normalizeUDAs(in map[string]UDABlueprintV1) (map[string]UDABlueprintV1, error) {
	if in == nil {
		return nil, nil
	}
	out := make(map[string]UDABlueprintV1, len(in))
	for key, value := range in {
		value.Raw, value.Type = strings.TrimSpace(value.Raw), strings.TrimSpace(value.Type)
		key = strings.TrimSpace(key)
		if _, exists := out[key]; exists {
			return nil, invalid("UDA keys collide after normalization")
		}
		out[key] = value
	}
	return out, nil
}

func trimPtr(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}

func sortUniqueTrim(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		seen[strings.TrimSpace(value)] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
