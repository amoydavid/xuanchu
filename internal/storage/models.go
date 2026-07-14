package storage

type Meta struct {
	Key   string `gorm:"primaryKey"`
	Value string `gorm:"not null"`
}

type Workspace struct {
	ID              string `gorm:"primaryKey"`
	Slug            string `gorm:"not null;uniqueIndex"`
	Name            string `gorm:"not null"`
	CreatedByUserID *string
	Description     string
	Visibility      string `gorm:"not null;default:'private'"`
	SettingsJSON    string `gorm:"not null;default:'{}'"`
	ArchivedAt      *int64
	CreatedAt       int64 `gorm:"not null"`
	ModifiedAt      int64 `gorm:"not null"`
}

type User struct {
	ID                 string  `gorm:"primaryKey"`
	Name               string  `gorm:"not null;uniqueIndex"`
	DisplayName        string  `gorm:"not null;default:''"`
	Email              *string `gorm:"uniqueIndex"`
	DefaultWorkspaceID *string
	CreatedAt          int64 `gorm:"not null"`
	ModifiedAt         int64 `gorm:"not null"`
}

type Membership struct {
	UserID      string `gorm:"primaryKey;not null"`
	WorkspaceID string `gorm:"primaryKey;not null;index"`
	Role        string `gorm:"not null;index"`
	JoinedAt    int64  `gorm:"not null"`
	ModifiedAt  int64  `gorm:"not null"`
}

type AuditLog struct {
	ID                      int64   `gorm:"primaryKey;autoIncrement"`
	ActorType               string  `gorm:"not null;default:'';index"`
	ActorUserID             *string `gorm:"index"`
	ActorTokenID            *string `gorm:"index"`
	ActorTokenName          *string
	ActorTokenPrefix        *string
	WorkspaceID             *string `gorm:"index;index:idx_audit_ws_time,priority:1;index:idx_audit_project_time,priority:1;index:idx_audit_target_time,priority:1"`
	ProjectID               *string `gorm:"index:idx_audit_project_time,priority:2"`
	Action                  string  `gorm:"not null;index"`
	TargetType              string  `gorm:"index:idx_audit_target_time,priority:2"`
	TargetID                string  `gorm:"index:idx_audit_target_time,priority:3"`
	PayloadJSON             string
	DelegatorTokenID        *string `gorm:"index"`
	DelegatorUserID         *string `gorm:"index"`
	AdminActingSessionID    *string `gorm:"index"`
	DelegatorAdminTokenID   *string `gorm:"index"`
	DelegatorAdminTokenName string  `gorm:"not null;default:''"`
	CreatedAt               int64   `gorm:"not null;index;index:idx_audit_ws_time,priority:2,sort:desc;index:idx_audit_project_time,priority:3,sort:desc;index:idx_audit_target_time,priority:4,sort:desc"`
}

type Project struct {
	ID           string `gorm:"primaryKey;uniqueIndex:idx_projects_id_ws,priority:1"`
	WorkspaceID  string `gorm:"not null;uniqueIndex:idx_projects_ws_slug,priority:1;uniqueIndex:idx_projects_id_ws,priority:2;index:idx_projects_ws_status,priority:1"`
	Slug         string `gorm:"not null;uniqueIndex:idx_projects_ws_slug,priority:2"`
	Name         string `gorm:"not null"`
	Description  string `gorm:"not null;default:''"`
	Status       string `gorm:"not null;default:'active';index:idx_projects_ws_status,priority:2"`
	SettingsJSON string `gorm:"not null;default:'{}'"`
	NextTaskSeq  int64  `gorm:"not null;default:1"`
	CreatedAt    int64  `gorm:"not null"`
	ModifiedAt   int64  `gorm:"not null"`
	ArchivedAt   *int64
	Annotations  []ProjectAnnotation `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE"`
}

type ProjectAnnotation struct {
	ID                   string  `gorm:"primaryKey"`
	ProjectID            string  `gorm:"not null;index:idx_project_annotations_project;uniqueIndex:idx_project_annotations_entry,priority:1"`
	Entry                int64   `gorm:"not null;uniqueIndex:idx_project_annotations_entry,priority:2"`
	Content              string  `gorm:"not null;type:text"`
	CreatedBy            string  `gorm:"not null"`
	CreatedByActorType   string  `gorm:"not null;default:'user';index"`
	CreatedByUserID      *string `gorm:"index"`
	CreatedByTokenID     *string `gorm:"index"`
	CreatedByTokenName   *string
	CreatedByTokenPrefix *string
	CreatedAt            int64 `gorm:"not null"`
}

type Config struct {
	WorkspaceID string `gorm:"primaryKey;not null;default:''"`
	Scope       string `gorm:"primaryKey;not null"`
	ScopeID     string `gorm:"primaryKey;not null;default:''"`
	Key         string `gorm:"primaryKey;not null"`
	Value       string `gorm:"not null"`
}

type ConfigDefinition struct {
	WorkspaceID       string `gorm:"primaryKey;not null"`
	Key               string `gorm:"primaryKey;not null"`
	ValueType         string `gorm:"not null"`
	AllowedScopesJSON string `gorm:"not null"`
	Label             string `gorm:"not null;default:''"`
	Description       string `gorm:"not null;default:''"`
	EnumValuesJSON    string `gorm:"not null;default:'[]'"`
	DefaultValue      string `gorm:"not null;default:''"`
	HasDefault        bool   `gorm:"not null;default:false"`
	Required          bool   `gorm:"not null;default:false"`
	Secret            bool   `gorm:"not null;default:false"`
	ShowOnConsoleHome bool   `gorm:"not null;default:false"`
	CreatedAt         int64  `gorm:"not null"`
	ModifiedAt        int64  `gorm:"not null"`
}

type ApiToken struct {
	ID                     string  `gorm:"primaryKey"`
	UserID                 *string `gorm:"index:idx_api_tokens_user"`
	Name                   string  `gorm:"not null"`
	Type                   string  `gorm:"not null"`
	TokenPrefix            string  `gorm:"not null;uniqueIndex:idx_api_tokens_prefix"`
	TokenHash              string  `gorm:"not null"`
	TokenSecretCiphertext  string  `gorm:"not null;default:''"`
	ScopesJSON             string  `gorm:"not null;default:'[]'"`
	WorkspaceIDsJSON       string  `gorm:"not null;default:'[]'"`
	ProjectIDsJSON         string  `gorm:"not null;default:'[]'"`
	IssuedVia              string  `gorm:"not null;default:'user';index"`
	IssuedByAdminTokenID   *string
	IssuedByAdminTokenName *string
	Purpose                string `gorm:"not null;default:'api';index"`
	// WebLoginDisabled 标记该 token 不能用于 Web Console 登录页登录。
	// SSO browser session 创建的 PAT/Agent token 自动打标，守住「SSO workspace 人工 token 走 SSO 登 Console」边界。
	// token 在 HTTP API / MCP / CLI 等场景仍照常可用。
	WebLoginDisabled bool  `gorm:"not null;default:false"`
	CreatedAt        int64 `gorm:"not null"`
	ExpiresAt        *int64
	RevokedAt        *int64
	LastUsedAt       *int64
}

type ServerAdminToken struct {
	ID          string `gorm:"primaryKey"`
	Name        string `gorm:"not null"`
	TokenPrefix string `gorm:"not null;uniqueIndex:idx_server_admin_tokens_prefix"`
	TokenHash   string `gorm:"not null"`
	Enabled     bool   `gorm:"not null;index"`
	CreatedAt   int64  `gorm:"not null"`
	RevokedAt   *int64
	LastUsedAt  *int64
	Description string `gorm:"not null;default:''"`
}

// AdminActingSession 是 server admin 委托签发的短期 acting session。
// acting token 只保存在当前浏览器 tab，不进入普通 api_tokens 表。
// Role 是创建时的快照，只用于审计展示，不作为后续授权来源。
type AdminActingSession struct {
	ID             string  `gorm:"primaryKey"`
	TokenPrefix    string  `gorm:"not null;uniqueIndex:idx_admin_acting_sessions_prefix"`
	TokenHash      string  `gorm:"not null"`
	AdminTokenID   *string `gorm:"index"`
	AdminTokenName string  `gorm:"not null"`
	WorkspaceID    string  `gorm:"not null;index"`
	ActorUserID    string  `gorm:"not null;index"`
	Role           string  `gorm:"not null"`
	CreatedAt      int64   `gorm:"not null"`
	ExpiresAt      int64   `gorm:"not null;index"`
	RevokedAt      *int64
	LastUsedAt     *int64
}

type Context struct {
	WorkspaceID  string `gorm:"primaryKey;not null"`
	Name         string `gorm:"primaryKey;not null"`
	FilterSource string `gorm:"not null"`
	CreatedAt    int64  `gorm:"not null"`
	ModifiedAt   int64  `gorm:"not null"`
}

type UDADefinition struct {
	WorkspaceID  string `gorm:"primaryKey;not null"`
	Name         string `gorm:"primaryKey;not null"`
	Type         string `gorm:"not null"`
	Label        string
	ValuesJSON   string
	DefaultValue string
	CreatedAt    int64 `gorm:"not null"`
	ModifiedAt   int64 `gorm:"not null"`
}

type Task struct {
	UUID        string `gorm:"primaryKey"`
	WorkspaceID string `gorm:"not null;uniqueIndex:idx_tasks_ws_project_seq,priority:1"`
	Title       string `gorm:"not null"`
	Description *string
	Status      string `gorm:"not null;index"`
	Entry       int64  `gorm:"not null"`
	Modified    int64  `gorm:"not null"`
	EndTS       *int64
	Due         *int64 `gorm:"index"`
	Project     *string
	ProjectID   *string `gorm:"uniqueIndex:idx_tasks_ws_project_seq,priority:2"`
	ProjectSeq  *int64  `gorm:"uniqueIndex:idx_tasks_ws_project_seq,priority:3"`
	Priority    *string
	Tags        []TaskTag `gorm:"foreignKey:TaskUUID;constraint:OnDelete:CASCADE"`
	Start       *int64
	Wait        *int64           `gorm:"index"`
	Scheduled   *int64           `gorm:"index"`
	Until       *int64           `gorm:"index"`
	Parent      *string          `gorm:"index"`
	Assignees   []TaskAssignee   `gorm:"foreignKey:TaskUUID;constraint:OnDelete:CASCADE"`
	Annotations []TaskAnnotation `gorm:"foreignKey:TaskUUID;constraint:OnDelete:CASCADE"`
	Depends     []TaskDependency `gorm:"foreignKey:TaskUUID;constraint:OnDelete:CASCADE"`
	UDAs        []TaskUDAValue   `gorm:"foreignKey:TaskUUID;constraint:OnDelete:CASCADE"`
	Links       []TaskLink       `gorm:"foreignKey:TaskUUID;constraint:OnDelete:CASCADE"`
	// occurrence 持久字段（spec §7.2）。partial unique index 由迁移脚本建立
	// idx_tasks_ws_series_slot（WHERE series_id IS NOT NULL AND recurrence_at IS NOT NULL）。
	SeriesID                *string `gorm:"column:series_id"`
	RecurrenceAt            *int64  `gorm:"column:recurrence_at;index"`
	RecurrenceRuleSnapshot  *string `gorm:"column:recurrence_rule_snapshot"`
	RecurrenceOverridesJSON *string `gorm:"column:recurrence_overrides_json;not null;default:'[]'"`
}

type TaskTag struct {
	TaskUUID string `gorm:"primaryKey;not null"`
	Tag      string `gorm:"primaryKey;not null"`
}

type TaskAnnotation struct {
	ID          string `gorm:"primaryKey"`
	TaskUUID    string `gorm:"not null;index:idx_task_annotations_task"`
	Entry       int64  `gorm:"not null"`
	Description string `gorm:"not null"`
}

type TaskDependency struct {
	TaskUUID  string `gorm:"primaryKey;not null"`
	DependsOn string `gorm:"primaryKey;not null;index"`
}

type TaskAssignee struct {
	TaskUUID string `gorm:"primaryKey;not null"`
	UserID   string `gorm:"primaryKey;not null;index:idx_task_assignees_user_id"`
}

type TaskUDAValue struct {
	WorkspaceID string `gorm:"not null;index"`
	TaskUUID    string `gorm:"primaryKey;not null;index"`
	Name        string `gorm:"primaryKey;not null"`
	Value       string `gorm:"not null"`
	ValueType   string
	Orphan      bool `gorm:"not null;default:false"`
}

type HookDefinition struct {
	ID               string  `gorm:"primaryKey"`
	Name             string  `gorm:"not null"`
	ScopeType        string  `gorm:"not null;index:idx_hooks_scope,priority:1"`
	WorkspaceID      string  `gorm:"not null;index:idx_hooks_scope,priority:2;index:idx_hooks_enabled"`
	ProjectID        *string `gorm:"index:idx_hooks_scope,priority:3"`
	ActorUserID      string  `gorm:"not null;index"`
	ActorType        string  `gorm:"not null;default:'user';index"`
	ActorTokenID     *string `gorm:"index"`
	ActorTokenName   *string
	ActorTokenPrefix *string
	EventTypesJSON   string `gorm:"not null"`
	SinkID           string `gorm:"not null;index"`
	EndpointURL      string `gorm:"not null;default:''"`
	Secret           string `gorm:"not null;default:''"`
	Enabled          *bool  `gorm:"not null;default:true;index:idx_hooks_enabled"`
	TimeoutSeconds   int    `gorm:"not null;default:10"`
	MaxAttempts      int    `gorm:"not null;default:5"`
	CreatedAt        int64  `gorm:"not null"`
	ModifiedAt       int64  `gorm:"not null"`
}

type HookDelivery struct {
	ID                          string  `gorm:"primaryKey"`
	HookID                      string  `gorm:"not null;index:idx_deliveries_due,priority:2;index:idx_deliveries_hook_status,priority:1"`
	EventID                     string  `gorm:"not null;index"`
	EventType                   string  `gorm:"not null;index"`
	WorkspaceID                 string  `gorm:"not null;index"`
	ProjectID                   *string `gorm:"index"`
	ActorUserID                 string  `gorm:"not null;index"`
	ActorType                   string  `gorm:"not null;default:'user';index"`
	ActorTokenID                *string `gorm:"index"`
	ActorTokenName              *string
	ActorTokenPrefix            *string
	SinkID                      string `gorm:"not null;default:'';index"`
	ResolvedURL                 string `gorm:"not null;default:''"`
	ResolvedEndpointSource      string `gorm:"not null;default:''"`
	ResolvedEndpointFingerprint string `gorm:"not null;default:''"`
	RenderedMethod              string `gorm:"not null;default:'POST'"`
	RenderedHeadersJSON         string `gorm:"not null;default:'{}'"`
	RenderedBody                string `gorm:"not null;default:''"`
	RenderedContentType         string `gorm:"not null;default:''"`
	PayloadJSON                 string `gorm:"not null"`
	HeadersJSON                 string `gorm:"not null;default:'{}'"`
	Status                      string `gorm:"not null;index:idx_deliveries_due,priority:1;index:idx_deliveries_hook_status,priority:2"`
	AttemptCount                int    `gorm:"not null;default:0"`
	NextAttemptAt               *int64 `gorm:"index:idx_deliveries_due,priority:3"`
	ClaimExpiresAt              *int64 `gorm:"index"`
	LastAttemptAt               *int64
	LastStatusCode              *int
	LastError                   string
	CreatedAt                   int64 `gorm:"not null;index"`
	ModifiedAt                  int64 `gorm:"not null"`
}

type NotificationSink struct {
	ID                   string  `gorm:"primaryKey"`
	WorkspaceID          string  `gorm:"not null;index:idx_notification_sinks_workspace;uniqueIndex:idx_notification_sinks_ws_name,priority:1"`
	Name                 string  `gorm:"not null;uniqueIndex:idx_notification_sinks_ws_name,priority:2"`
	Type                 string  `gorm:"not null"`
	EndpointMode         string  `gorm:"not null"`
	URL                  string  `gorm:"not null;default:''"`
	URLTemplate          string  `gorm:"not null;default:''"`
	ConfigKey            string  `gorm:"not null;default:''"`
	AllowedHostsJSON     string  `gorm:"not null;default:'[]'"`
	HTTPMethod           string  `gorm:"not null;default:'POST'"`
	HeaderTemplatesJSON  string  `gorm:"not null;default:'[]'"`
	BodyTemplate         string  `gorm:"not null;default:''"`
	BodyContentType      string  `gorm:"not null;default:''"`
	SecretRefsJSON       string  `gorm:"not null;default:'{}'"`
	Secret               string  `gorm:"not null;default:''"`
	Enabled              *bool   `gorm:"not null;default:true;index"`
	TimeoutSeconds       int     `gorm:"not null;default:10"`
	MaxAttempts          int     `gorm:"not null;default:5"`
	MaxConcurrency       int     `gorm:"not null;default:0"`
	CreatedBy            string  `gorm:"not null;index"`
	CreatedByActorType   string  `gorm:"not null;default:'user';index"`
	CreatedByUserID      *string `gorm:"index"`
	CreatedByTokenID     *string `gorm:"index"`
	CreatedByTokenName   *string
	CreatedByTokenPrefix *string
	CreatedAt            int64 `gorm:"not null"`
	ModifiedAt           int64 `gorm:"not null"`
}

type ReminderRule struct {
	ID                   string  `gorm:"primaryKey"`
	WorkspaceID          string  `gorm:"not null;index:idx_reminder_rules_workspace;uniqueIndex:idx_reminder_rules_ws_name,priority:1"`
	ProjectID            *string `gorm:"index"`
	Name                 string  `gorm:"not null;uniqueIndex:idx_reminder_rules_ws_name,priority:2"`
	Enabled              *bool   `gorm:"not null;default:true;index"`
	TriggerType          string  `gorm:"not null;index"`
	OffsetSeconds        int64   `gorm:"not null;default:0"`
	AfterSeconds         int64   `gorm:"not null;default:0"`
	RepeatPolicy         string  `gorm:"not null;default:'once'"`
	ScheduleType         string  `gorm:"not null;default:'';index"`
	ScheduleValue        string  `gorm:"not null;default:''"`
	FilterSource         string  `gorm:"not null;default:''"`
	AudienceType         string  `gorm:"not null"`
	RecipientUserIDsJSON string  `gorm:"not null;default:'[]'"`
	SinkID               string  `gorm:"not null;index"`
	CreatedBy            string  `gorm:"not null;index"`
	CreatedByActorType   string  `gorm:"not null;default:'user';index"`
	CreatedByUserID      *string `gorm:"index"`
	CreatedByTokenID     *string `gorm:"index"`
	CreatedByTokenName   *string
	CreatedByTokenPrefix *string
	CreatedAt            int64 `gorm:"not null"`
	ModifiedAt           int64 `gorm:"not null"`
}

type EventNotificationRule struct {
	ID                   string  `gorm:"primaryKey"`
	WorkspaceID          string  `gorm:"not null;index:idx_event_notification_rules_workspace;uniqueIndex:idx_event_notification_rules_ws_name,priority:1"`
	ProjectID            *string `gorm:"index"`
	Name                 string  `gorm:"not null;uniqueIndex:idx_event_notification_rules_ws_name,priority:2"`
	Enabled              *bool   `gorm:"not null;default:true;index"`
	EventType            string  `gorm:"not null;index"`
	FilterSource         string  `gorm:"not null;default:''"`
	AudienceType         string  `gorm:"not null"`
	RecipientUserIDsJSON string  `gorm:"not null;default:'[]'"`
	SinkID               string  `gorm:"not null;index"`
	TemplateSubject      string  `gorm:"not null;default:''"`
	TemplateBody         string  `gorm:"not null;default:''"`
	CreatedBy            string  `gorm:"not null;index"`
	CreatedByActorType   string  `gorm:"not null;default:'user';index"`
	CreatedByUserID      *string `gorm:"index"`
	CreatedByTokenID     *string `gorm:"index"`
	CreatedByTokenName   *string
	CreatedByTokenPrefix *string
	CreatedAt            int64 `gorm:"not null"`
	ModifiedAt           int64 `gorm:"not null"`
}

type NotificationDelivery struct {
	ID                          string  `gorm:"primaryKey"`
	WorkspaceID                 string  `gorm:"not null;index"`
	ProjectID                   *string `gorm:"index"`
	RuleID                      string  `gorm:"not null;index"`
	SinkID                      string  `gorm:"not null;index"`
	TaskUUID                    string  `gorm:"not null;default:'';index"`
	ObjectKind                  string  `gorm:"not null;default:'task';index"`
	ObjectID                    string  `gorm:"not null;default:'';index"`
	RecipientUserID             string  `gorm:"not null;index"`
	EventID                     string  `gorm:"not null;index"`
	EventType                   string  `gorm:"not null;index"`
	ActorType                   string  `gorm:"not null;default:'user';index"`
	ActorUserID                 *string `gorm:"index"`
	ActorTokenID                *string `gorm:"index"`
	ActorTokenName              *string
	ActorTokenPrefix            *string
	DedupeKey                   string `gorm:"not null;uniqueIndex"`
	ResolvedURL                 string `gorm:"not null"`
	ResolvedEndpointSource      string `gorm:"not null;default:''"`
	ResolvedEndpointFingerprint string `gorm:"not null;default:''"`
	RenderedMethod              string `gorm:"not null;default:'POST'"`
	RenderedHeadersJSON         string `gorm:"not null;default:'{}'"`
	RenderedBody                string `gorm:"not null;default:''"`
	RenderedContentType         string `gorm:"not null;default:''"`
	PayloadJSON                 string `gorm:"not null"`
	Status                      string `gorm:"not null;index:idx_notification_deliveries_due,priority:1"`
	AttemptCount                int    `gorm:"not null;default:0"`
	NextAttemptAt               *int64 `gorm:"index:idx_notification_deliveries_due,priority:2"`
	ClaimExpiresAt              *int64 `gorm:"index"`
	LastAttemptAt               *int64
	LastStatusCode              *int
	LastError                   string
	CreatedAt                   int64 `gorm:"not null;index"`
	ModifiedAt                  int64 `gorm:"not null"`
}

type ProjectAutomationRule struct {
	ID                   string  `gorm:"primaryKey"`
	WorkspaceID          string  `gorm:"not null;index:idx_project_automation_rules_scope,priority:1;uniqueIndex:idx_project_automation_rules_ws_project_name,priority:1"`
	ProjectID            string  `gorm:"not null;index:idx_project_automation_rules_scope,priority:2;uniqueIndex:idx_project_automation_rules_ws_project_name,priority:2"`
	Name                 string  `gorm:"not null;uniqueIndex:idx_project_automation_rules_ws_project_name,priority:3"`
	Description          string  `gorm:"not null;default:''"`
	Enabled              *bool   `gorm:"not null;default:true;index"`
	TriggerType          string  `gorm:"not null;index"`
	TriggerConfigJSON    string  `gorm:"not null;default:'{}'"`
	ConditionJSON        string  `gorm:"not null;default:'{}'"`
	ActionType           string  `gorm:"not null;default:'openai_compatible';index"`
	ActionConfigJSON     string  `gorm:"not null;default:'{}'"`
	ContextConfigJSON    string  `gorm:"not null;default:'{}'"`
	InstructionTemplate  string  `gorm:"not null;default:''"`
	SystemPrompt         string  `gorm:"not null;default:''"`
	CreatedByActorType   string  `gorm:"not null;default:'user';index"`
	CreatedByUserID      *string `gorm:"index"`
	CreatedByTokenID     *string `gorm:"index"`
	CreatedByTokenName   *string
	CreatedByTokenPrefix *string
	CreatedAt            int64 `gorm:"not null"`
	ModifiedAt           int64 `gorm:"not null"`
}

type ProjectAutomationDelivery struct {
	ID                  string `gorm:"primaryKey"`
	WorkspaceID         string `gorm:"not null;index:idx_project_automation_deliveries_scope,priority:1"`
	ProjectID           string `gorm:"not null;index:idx_project_automation_deliveries_scope,priority:2"`
	RuleID              string `gorm:"not null;index"`
	TriggerType         string `gorm:"not null;index"`
	EventID             string `gorm:"not null;default:'';index"`
	EventType           string `gorm:"not null;default:'';index"`
	DedupeKey           string `gorm:"not null;uniqueIndex"`
	Status              string `gorm:"not null;index:idx_project_automation_deliveries_due,priority:1"`
	ResolvedURL         string `gorm:"not null;default:''"`
	RenderedMethod      string `gorm:"not null;default:'POST'"`
	RenderedHeadersJSON string `gorm:"not null;default:'{}'"`
	RequestBodyJSON     string `gorm:"not null;default:''"`
	RequestBodyPreview  string `gorm:"not null;default:''"`
	RequestBodyHash     string `gorm:"not null;default:''"`
	ResponseStatusCode  *int
	ResponseBodyPreview string `gorm:"not null;default:''"`
	ProviderRequestID   string `gorm:"not null;default:''"`
	UsageJSON           string `gorm:"not null;default:'{}'"`
	AttemptCount        int    `gorm:"not null;default:0"`
	NextAttemptAt       *int64 `gorm:"index:idx_project_automation_deliveries_due,priority:2"`
	ClaimExpiresAt      *int64 `gorm:"index"`
	LastAttemptAt       *int64
	LastError           string `gorm:"not null;default:''"`
	CreatedAt           int64  `gorm:"not null;index"`
	ModifiedAt          int64  `gorm:"not null"`
}

type UserExternalID struct {
	ID         string `gorm:"primaryKey"`
	UserID     string `gorm:"not null;index:idx_user_ext_id_user"`
	Provider   string `gorm:"not null;uniqueIndex:idx_user_ext_id_provider_value,priority:1"`
	UserType   string `gorm:"not null;default:'user_id';uniqueIndex:idx_user_ext_id_provider_value,priority:2"`
	ExternalID string `gorm:"not null;uniqueIndex:idx_user_ext_id_provider_value,priority:3"`
	CreatedAt  int64  `gorm:"not null"`
}

type TaskLink struct {
	ID                   string  `gorm:"primaryKey"`
	TaskUUID             string  `gorm:"not null;uniqueIndex:idx_task_links_task_url,priority:1;index:idx_task_links_task"`
	Type                 string  `gorm:"not null"`
	URL                  string  `gorm:"not null;uniqueIndex:idx_task_links_task_url,priority:2"`
	Title                string  `gorm:"not null;default:''"`
	CreatedAt            int64   `gorm:"not null"`
	CreatedBy            string  `gorm:"not null"`
	CreatedByActorType   string  `gorm:"not null;default:'user';index"`
	CreatedByUserID      *string `gorm:"index"`
	CreatedByTokenID     *string `gorm:"index"`
	CreatedByTokenName   *string
	CreatedByTokenPrefix *string
}

// BrowserSession 是 OIDC 登录后建立的浏览器会话，独立于 ApiToken。
type BrowserSession struct {
	ID          string `gorm:"primaryKey"` // 存哈希后的 session id
	UserID      string `gorm:"not null;index"`
	WorkspaceID string `gorm:"not null;index"`
	CSRFHash    string `gorm:"not null"` // CSRF token hash，明文只存在浏览器 csrf cookie 中
	ExpiresAt   int64  `gorm:"not null;index"`
	CreatedAt   int64  `gorm:"not null"`
	LastSeenAt  int64  `gorm:"not null"`
}

// BrowserAuthFlow 记录一次进行中的 OIDC Auth Code Flow（state + PKCE），短 TTL。
type BrowserAuthFlow struct {
	State        string `gorm:"primaryKey"` // OIDC state
	WorkspaceID  string `gorm:"not null;index"`
	PKCEVerifier string `gorm:"not null"`
	CreatedAt    int64  `gorm:"not null"`
	ExpiresAt    int64  `gorm:"not null;index"`
}

// DirectorySyncJob 记录一次通讯录同步任务，复用 dispatcher claim/lease 模式。
type DirectorySyncJob struct {
	ID             string `gorm:"primaryKey"`
	WorkspaceID    string `gorm:"not null;index"`
	Status         string `gorm:"not null;default:'pending';index"` // pending/running/succeeded/failed
	ClaimedAt      *int64
	ClaimExpiresAt *int64
	ErrorMessage   string `gorm:"not null;default:''"`
	StatsJSON      string `gorm:"not null;default:'{}'"` // {"added":N,"removed":M,"updated":K}
	CreatedAt      int64  `gorm:"not null"`
	FinishedAt     *int64
}

// TaskSeries 是循环任务系列聚合（spec §7.1）。独立于 tasks 表。
type TaskSeries struct {
	ID             string `gorm:"primaryKey"`
	WorkspaceID    string `gorm:"not null;index:idx_task_series_ws_project_status,priority:1"`
	ProjectID      string `gorm:"not null;index:idx_task_series_ws_project_status,priority:2"`
	Title          string `gorm:"not null"`
	Description    *string
	Status         string `gorm:"not null;index:idx_task_series_ws_project_status,priority:3;index"`
	RecurrenceRule string `gorm:"not null"`
	FirstDue       int64  `gorm:"not null"`
	Until          *int64 `gorm:"index"`
	EffectiveEndAt *int64
	StopReason     *string
	Priority       *string
	CreatedBy      string                  `gorm:"not null"`
	CreatedAt      int64                   `gorm:"not null"`
	ModifiedAt     int64                   `gorm:"not null"`
	RuleVersions   []TaskSeriesRuleVersion `gorm:"foreignKey:SeriesID;constraint:OnDelete:RESTRICT"`
	Assignees      []TaskSeriesAssignee    `gorm:"foreignKey:SeriesID;constraint:OnDelete:CASCADE"`
	Tags           []TaskSeriesTag         `gorm:"foreignKey:SeriesID;constraint:OnDelete:CASCADE"`
	UDAValues      []TaskSeriesUDAValue    `gorm:"foreignKey:SeriesID;constraint:OnDelete:CASCADE"`
}

// TaskSeriesRuleVersion 保存规则切换历史（spec §7.1）。
// 同一 series 内 effective_from 唯一。
type TaskSeriesRuleVersion struct {
	ID             string `gorm:"primaryKey"`
	SeriesID       string `gorm:"not null;uniqueIndex:idx_task_series_rule_versions_series_eff,priority:1"`
	EffectiveFrom  int64  `gorm:"not null;uniqueIndex:idx_task_series_rule_versions_series_eff,priority:2"`
	RecurrenceRule string `gorm:"not null"`
	CreatedBy      string `gorm:"not null"`
	CreatedAt      int64  `gorm:"not null"`
}

// TaskSeriesAssignee 是 series 的多值负责人关联（spec §7.1）。
type TaskSeriesAssignee struct {
	SeriesID string `gorm:"primaryKey;not null"`
	UserID   string `gorm:"primaryKey;not null"`
}

// TaskSeriesTag 是 series 的多值标签关联。
type TaskSeriesTag struct {
	SeriesID string `gorm:"primaryKey;not null"`
	Tag      string `gorm:"primaryKey;not null"`
}

// TaskSeriesUDAValue 是 series 的 UDA 关联。
type TaskSeriesUDAValue struct {
	SeriesID  string `gorm:"primaryKey;not null"`
	Name      string `gorm:"primaryKey;not null"`
	Value     string
	ValueType string
	Orphan    bool
}
