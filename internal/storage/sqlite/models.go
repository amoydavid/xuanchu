package sqlite

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
	ID               int64   `gorm:"primaryKey;autoIncrement"`
	ActorUserID      *string `gorm:"index"`
	WorkspaceID      *string `gorm:"index;index:idx_audit_ws_time,priority:1;index:idx_audit_project_time,priority:1"`
	ProjectID        *string `gorm:"index:idx_audit_project_time,priority:2"`
	Action           string  `gorm:"not null;index"`
	TargetType       string
	TargetID         string
	PayloadJSON      string
	DelegatorTokenID *string `gorm:"index"`
	DelegatorUserID  *string `gorm:"index"`
	CreatedAt        int64   `gorm:"not null;index;index:idx_audit_ws_time,priority:2,sort:desc;index:idx_audit_project_time,priority:3,sort:desc"`
}

type Project struct {
	ID           string `gorm:"primaryKey;uniqueIndex:idx_projects_id_ws,priority:1"`
	WorkspaceID  string `gorm:"not null;uniqueIndex:idx_projects_ws_slug,priority:1;uniqueIndex:idx_projects_id_ws,priority:2;index:idx_projects_ws_status,priority:1"`
	Slug         string `gorm:"not null;uniqueIndex:idx_projects_ws_slug,priority:2"`
	Name         string `gorm:"not null"`
	Description  string `gorm:"not null;default:''"`
	Status       string `gorm:"not null;default:'active';index:idx_projects_ws_status,priority:2"`
	SettingsJSON string `gorm:"not null;default:'{}'"`
	CreatedAt    int64  `gorm:"not null"`
	ModifiedAt   int64  `gorm:"not null"`
	ArchivedAt   *int64
	Annotations  []ProjectAnnotation `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE"`
}

type ProjectAnnotation struct {
	ID        string `gorm:"primaryKey"`
	ProjectID string `gorm:"not null;index:idx_project_annotations_project;uniqueIndex:idx_project_annotations_entry,priority:1"`
	Entry     int64  `gorm:"not null;uniqueIndex:idx_project_annotations_entry,priority:2"`
	Content   string `gorm:"not null;type:text"`
	CreatedBy string `gorm:"not null"`
	CreatedAt int64  `gorm:"not null"`
}

type Config struct {
	WorkspaceID string `gorm:"primaryKey;not null;default:''"`
	Scope       string `gorm:"primaryKey;not null"`
	ScopeID     string `gorm:"primaryKey;not null;default:''"`
	Key         string `gorm:"primaryKey;not null"`
	Value       string `gorm:"not null"`
}

type ApiToken struct {
	ID               string `gorm:"primaryKey"`
	UserID           string `gorm:"not null;index:idx_api_tokens_user"`
	Name             string `gorm:"not null"`
	Type             string `gorm:"not null"`
	TokenPrefix      string `gorm:"not null;uniqueIndex:idx_api_tokens_prefix"`
	TokenHash        string `gorm:"not null"`
	ScopesJSON       string `gorm:"not null;default:'[]'"`
	WorkspaceIDsJSON string `gorm:"not null;default:'[]'"`
	ProjectIDsJSON   string `gorm:"not null;default:'[]'"`
	CreatedAt        int64  `gorm:"not null"`
	ExpiresAt        *int64
	RevokedAt        *int64
	LastUsedAt       *int64
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
	WorkspaceID string `gorm:"not null"`
	Description string `gorm:"not null"`
	Status      string `gorm:"not null;index"`
	Entry       int64  `gorm:"not null"`
	Modified    int64  `gorm:"not null"`
	EndTS       *int64
	Due         *int64
	Project     *string
	ProjectID   *string
	Priority    *string
	Tags        []TaskTag `gorm:"foreignKey:TaskUUID;constraint:OnDelete:CASCADE"`
	Start       *int64
	Wait        *int64  `gorm:"index"`
	Scheduled   *int64  `gorm:"index"`
	Until       *int64  `gorm:"index"`
	Recur       *string `gorm:"index"`
	Parent      *string `gorm:"index"`
	Mask        *string
	IMask       *int
	Assignees   []TaskAssignee   `gorm:"foreignKey:TaskUUID;constraint:OnDelete:CASCADE"`
	Annotations []TaskAnnotation `gorm:"foreignKey:TaskUUID;constraint:OnDelete:CASCADE"`
	Depends     []TaskDependency `gorm:"foreignKey:TaskUUID;constraint:OnDelete:CASCADE"`
	UDAs        []TaskUDAValue   `gorm:"foreignKey:TaskUUID;constraint:OnDelete:CASCADE"`
	Links       []TaskLink       `gorm:"foreignKey:TaskUUID;constraint:OnDelete:CASCADE"`
}

type TaskTag struct {
	TaskUUID string `gorm:"primaryKey;not null"`
	Tag      string `gorm:"primaryKey;not null"`
}

type TaskAnnotation struct {
	TaskUUID    string `gorm:"primaryKey;not null"`
	Entry       int64  `gorm:"primaryKey;not null"`
	Description string `gorm:"primaryKey;not null"`
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
	ID             string  `gorm:"primaryKey"`
	Name           string  `gorm:"not null"`
	ScopeType      string  `gorm:"not null;index:idx_hooks_scope,priority:1"`
	WorkspaceID    string  `gorm:"not null;index:idx_hooks_scope,priority:2;index:idx_hooks_enabled"`
	ProjectID      *string `gorm:"index:idx_hooks_scope,priority:3"`
	ActorUserID    string  `gorm:"not null;index"`
	EventTypesJSON string  `gorm:"not null"`
	EndpointURL    string  `gorm:"not null"`
	Secret         string  `gorm:"not null;default:''"`
	Enabled        *bool   `gorm:"not null;default:true;index:idx_hooks_enabled"`
	TimeoutSeconds int     `gorm:"not null;default:10"`
	MaxAttempts    int     `gorm:"not null;default:5"`
	CreatedAt      int64   `gorm:"not null"`
	ModifiedAt     int64   `gorm:"not null"`
}

type HookDelivery struct {
	ID             string  `gorm:"primaryKey"`
	HookID         string  `gorm:"not null;index:idx_deliveries_due,priority:2;index:idx_deliveries_hook_status,priority:1"`
	EventID        string  `gorm:"not null;index"`
	EventType      string  `gorm:"not null;index"`
	WorkspaceID    string  `gorm:"not null;index"`
	ProjectID      *string `gorm:"index"`
	ActorUserID    string  `gorm:"not null;index"`
	PayloadJSON    string  `gorm:"not null"`
	HeadersJSON    string  `gorm:"not null;default:'{}'"`
	Status         string  `gorm:"not null;index:idx_deliveries_due,priority:1;index:idx_deliveries_hook_status,priority:2"`
	AttemptCount   int     `gorm:"not null;default:0"`
	NextAttemptAt  *int64  `gorm:"index:idx_deliveries_due,priority:3"`
	ClaimExpiresAt *int64  `gorm:"index"`
	LastAttemptAt  *int64
	LastStatusCode *int
	LastError      string
	CreatedAt      int64 `gorm:"not null;index"`
	ModifiedAt     int64 `gorm:"not null"`
}

type UserExternalID struct {
	ID          string `gorm:"primaryKey"`
	UserID      string `gorm:"not null;index:idx_user_ext_id_user"`
	Provider    string `gorm:"not null;uniqueIndex:idx_user_ext_id_provider_value,priority:1"`
	ExternalID  string `gorm:"not null;uniqueIndex:idx_user_ext_id_provider_value,priority:2"`
	CreatedAt   int64  `gorm:"not null"`
}

type TaskLink struct {
	ID        string `gorm:"primaryKey"`
	TaskUUID  string `gorm:"not null;uniqueIndex:idx_task_links_task_url,priority:1;index:idx_task_links_task"`
	Type      string `gorm:"not null"`
	URL       string `gorm:"not null;uniqueIndex:idx_task_links_task_url,priority:2"`
	Title     string `gorm:"not null;default:''"`
	CreatedAt int64  `gorm:"not null"`
	CreatedBy string `gorm:"not null"`
}
