// Package projecttemplate 定义项目模板快照的纯领域契约。
package projecttemplate

import (
	"errors"
)

const (
	SnapshotSchemaV1 = "xuanchu.project-template-snapshot/v1"
	SnapshotSchemaV2 = "xuanchu.project-template-snapshot/v2"
)

type SnapshotV1 struct {
	Schema      string                  `json:"schema"`
	AnchorDate  string                  `json:"anchor_date"`
	Project     ProjectBlueprintV1      `json:"project"`
	Configs     []ConfigBlueprintV1     `json:"configs"`
	Tasks       []TaskBlueprintV1       `json:"tasks"`
	Series      []SeriesBlueprintV1     `json:"series"`
	Automations []AutomationBlueprintV1 `json:"automations"`
}

type ProjectBlueprintV1 struct {
	Description string `json:"description"`
}

type RelativeLocalTimeV1 struct {
	DayOffset int    `json:"day_offset"`
	LocalTime string `json:"local_time"`
}

type TaskDatesV1 struct {
	Due       *RelativeLocalTimeV1 `json:"due,omitempty"`
	Wait      *RelativeLocalTimeV1 `json:"wait,omitempty"`
	Scheduled *RelativeLocalTimeV1 `json:"scheduled,omitempty"`
	Until     *RelativeLocalTimeV1 `json:"until,omitempty"`
}

type UDABlueprintV1 struct {
	Raw  string `json:"raw"`
	Type string `json:"type,omitempty"`
}

type TaskLinkBlueprintV1 struct {
	Type  string `json:"type"`
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
}

type TaskBlueprintV1 struct {
	Ref         string                    `json:"ref"`
	Title       string                    `json:"title"`
	Description *string                   `json:"description,omitempty"`
	Priority    *string                   `json:"priority,omitempty"`
	Tags        []string                  `json:"tags,omitempty"`
	AssigneeIDs []string                  `json:"assignee_ids,omitempty"`
	UDAs        map[string]UDABlueprintV1 `json:"udas,omitempty"`
	Dates       TaskDatesV1               `json:"dates"`
	ParentRef   *string                   `json:"parent_ref,omitempty"`
	DependsRefs []string                  `json:"depends_refs,omitempty"`
	Links       []TaskLinkBlueprintV1     `json:"links,omitempty"`
}

type ConfigBlueprintV1 struct {
	Key              string  `json:"key"`
	Mode             string  `json:"mode"`
	Value            *string `json:"value,omitempty"`
	SecretCiphertext *string `json:"secret_ciphertext,omitempty"`
}

// SnapshotV2 在不可变快照中增加项目创建时的配置输入声明。
// 其余 blueprint 使用独立命名类型，避免把持久协议偷换为 V1 alias。
type SnapshotV2 struct {
	Schema      string                  `json:"schema"`
	AnchorDate  string                  `json:"anchor_date"`
	Project     ProjectBlueprintV2      `json:"project"`
	Configs     []ConfigBlueprintV2     `json:"configs"`
	Tasks       []TaskBlueprintV2       `json:"tasks"`
	Series      []SeriesBlueprintV2     `json:"series"`
	Automations []AutomationBlueprintV2 `json:"automations"`
}

type ProjectBlueprintV2 ProjectBlueprintV1

type ConfigPromptV2 struct {
	Required bool `json:"required"`
}

type ConfigBlueprintV2 struct {
	Key              string          `json:"key"`
	Mode             string          `json:"mode"`
	Value            *string         `json:"value,omitempty"`
	SecretCiphertext *string         `json:"secret_ciphertext,omitempty"`
	Prompt           *ConfigPromptV2 `json:"prompt,omitempty"`
}

// Snapshot 是所有已支持持久版本升级后的 App 内部模型。
// Schema 保留来源版本，便于兼容 v1 secret_input 的既有语义。
type Snapshot struct {
	Schema      string                  `json:"schema"`
	AnchorDate  string                  `json:"anchor_date"`
	Project     ProjectBlueprintV1      `json:"project"`
	Configs     []ConfigBlueprint       `json:"configs"`
	Tasks       []TaskBlueprintV1       `json:"tasks"`
	Series      []SeriesBlueprintV1     `json:"series"`
	Automations []AutomationBlueprintV1 `json:"automations"`
}

type ConfigBlueprint struct {
	Key              string
	Mode             string
	Value            *string
	SecretCiphertext *string
	Prompt           *ConfigPromptV2
}

type SeriesBlueprintV1 struct {
	Ref            string                    `json:"ref"`
	Title          string                    `json:"title"`
	Description    *string                   `json:"description,omitempty"`
	Priority       *string                   `json:"priority,omitempty"`
	Tags           []string                  `json:"tags,omitempty"`
	AssigneeIDs    []string                  `json:"assignee_ids,omitempty"`
	UDAs           map[string]UDABlueprintV1 `json:"udas,omitempty"`
	RecurrenceRule string                    `json:"recurrence_rule"`
	FirstDue       RelativeLocalTimeV1       `json:"first_due"`
	Until          *RelativeLocalTimeV1      `json:"until,omitempty"`
}

type TaskBlueprintV2 TaskBlueprintV1

type SeriesBlueprintV2 SeriesBlueprintV1

// AutomationTriggerV1 与公开自动化规则的触发字段等价，但属于持久快照契约。
type AutomationTriggerV1 struct {
	ScheduleType  string `json:"schedule_type,omitempty"`
	ScheduleValue string `json:"schedule_value,omitempty"`
	Timezone      string `json:"timezone,omitempty"`
	EventType     string `json:"event_type,omitempty"`
}

type AutomationConditionV1 struct {
	TaskFilter         string `json:"task_filter,omitempty"`
	MaxTasks           int    `json:"max_tasks,omitempty"`
	OnlyAddedAssignees bool   `json:"only_added_assignees,omitempty"`
}

type AutomationActionV1 struct {
	Protocol              string  `json:"protocol"`
	BaseURLConfigKey      string  `json:"base_url_config_key"`
	APIKeyConfigKey       string  `json:"api_key_config_key"`
	ModelConfigKey        string  `json:"model_config_key"`
	AllowedHostsConfigKey string  `json:"allowed_hosts_config_key,omitempty"`
	ModelOverride         string  `json:"model_override,omitempty"`
	Temperature           float64 `json:"temperature"`
	MaxAttempts           int     `json:"max_attempts,omitempty"`
	AttachMetadata        bool    `json:"attach_metadata,omitempty"`
}

type AutomationContextV1 struct {
	Include []string `json:"include"`
}

type AutomationBlueprintV1 struct {
	Ref                 string                `json:"ref"`
	Name                string                `json:"name"`
	Description         string                `json:"description,omitempty"`
	TriggerType         string                `json:"trigger_type"`
	TriggerConfig       AutomationTriggerV1   `json:"trigger_config"`
	Condition           AutomationConditionV1 `json:"condition"`
	Action              AutomationActionV1    `json:"action"`
	Context             AutomationContextV1   `json:"context"`
	InstructionTemplate string                `json:"instruction_template"`
	SystemPrompt        string                `json:"system_prompt,omitempty"`
}

type AutomationBlueprintV2 AutomationBlueprintV1

type Limits struct {
	MaxTasks, MaxSeries, MaxConfigs, MaxAutomations int
	MaxJSONBytes, MaxTextBytes                      int
}

var DefaultLimits = Limits{
	MaxTasks: 1000, MaxSeries: 200, MaxConfigs: 500, MaxAutomations: 200,
	MaxJSONBytes: 8 << 20, MaxTextBytes: 512 << 10,
}

type Error struct {
	Code    string
	Message string
}

func (e Error) Error() string { return e.Message }

func ErrorCode(err error) string {
	var templateErr Error
	if errors.As(err, &templateErr) {
		return templateErr.Code
	}
	return ""
}

func invalid(message string) Error {
	return Error{Code: "project_template_snapshot_invalid", Message: message}
}
