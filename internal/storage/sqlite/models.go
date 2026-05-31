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
	ID          int64   `gorm:"primaryKey;autoIncrement"`
	ActorUserID *string `gorm:"index"`
	WorkspaceID *string `gorm:"index;index:idx_audit_ws_time,priority:1;index:idx_audit_project_time,priority:1"`
	ProjectID   *string `gorm:"index:idx_audit_project_time,priority:2"`
	Action      string  `gorm:"not null;index"`
	TargetType  string
	TargetID    string
	PayloadJSON string
	CreatedAt   int64 `gorm:"not null;index;index:idx_audit_ws_time,priority:2,sort:desc;index:idx_audit_project_time,priority:3,sort:desc"`
}

type Project struct {
	ID           string `gorm:"primaryKey;uniqueIndex:idx_projects_id_ws,priority:1"`
	WorkspaceID  string `gorm:"not null;uniqueIndex:idx_projects_ws_slug,priority:1;uniqueIndex:idx_projects_id_ws,priority:2;index:idx_projects_ws_status,priority:1"`
	Slug         string `gorm:"not null;uniqueIndex:idx_projects_ws_slug,priority:2"`
	Name         string `gorm:"not null"`
	Description  string `gorm:"type:TEXT;not null;default:''"`
	Status       string `gorm:"not null;default:'active';index:idx_projects_ws_status,priority:2"`
	SettingsJSON string `gorm:"not null;default:'{}'"`
	CreatedAt    int64  `gorm:"not null"`
	ModifiedAt   int64  `gorm:"not null"`
	ArchivedAt   *int64
}

type Config struct {
	WorkspaceID string `gorm:"primaryKey;not null;default:''"`
	Scope       string `gorm:"primaryKey;not null"`
	ScopeID     string `gorm:"primaryKey;not null;default:''"`
	Key         string `gorm:"primaryKey;not null"`
	Value       string `gorm:"not null"`
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
	WorkspaceID string `gorm:"not null;index"`
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
	Annotations []TaskAnnotation `gorm:"foreignKey:TaskUUID;constraint:OnDelete:CASCADE"`
	Depends     []TaskDependency `gorm:"foreignKey:TaskUUID;constraint:OnDelete:CASCADE"`
	UDAs        []TaskUDAValue   `gorm:"foreignKey:TaskUUID;constraint:OnDelete:CASCADE"`
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

type TaskUDAValue struct {
	WorkspaceID string `gorm:"not null;index"`
	TaskUUID    string `gorm:"primaryKey;not null;index"`
	Name        string `gorm:"primaryKey;not null"`
	Value       string `gorm:"not null"`
	ValueType   string
	Orphan      bool `gorm:"not null;default:false"`
}
