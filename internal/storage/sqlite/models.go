package sqlite

type Meta struct {
	Key   string `gorm:"primaryKey"`
	Value string `gorm:"not null"`
}

type Workspace struct {
	ID        string `gorm:"primaryKey"`
	Slug      string `gorm:"not null;uniqueIndex"`
	Name      string `gorm:"not null"`
	CreatedAt int64  `gorm:"not null"`
}

type Task struct {
	UUID        string    `gorm:"primaryKey"`
	WorkspaceID string    `gorm:"not null;index"`
	Description string    `gorm:"not null"`
	Status      string    `gorm:"not null;index"`
	Entry       int64     `gorm:"not null"`
	Modified    int64     `gorm:"not null"`
	EndTS       *int64
	Due         *int64
	Project     *string   `gorm:"index"`
	Priority    *string
	Tags        []TaskTag `gorm:"foreignKey:TaskUUID;constraint:OnDelete:CASCADE"`
}

type TaskTag struct {
	TaskUUID string `gorm:"primaryKey;not null"`
	Tag      string `gorm:"primaryKey;not null"`
}
