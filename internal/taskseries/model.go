// Package taskseries 定义循环任务系列的领域模型、规则版本与槽位展开算法。
//
// 本包不依赖 GORM、Cobra 或 CLI 输出，只依赖 Go 标准库，便于被 storage、app、httpapi、
// remote、cli、mcpserver 等多层复用。spec §6-7 定义了术语与数据模型。
package taskseries

// Series 表示一个独立的循环任务系列聚合。
// 存储形态见 storage.TaskSeries；本结构是持久化领域对象。
type Series struct {
	ID            string
	WorkspaceID   string
	ProjectID     string
	Title         string
	Description   *string
	Status        string // active|ended|stopped
	RecurrenceRule string // 当前生效的 canonical 规则
	FirstDue      int64  // 第一个日历槽位（不可变）
	Until         *int64 // 包含式上界，nil 表示无限
	EffectiveEndAt *int64 // ended/stopped 的有效边界
	StopReason    *string // user_stopped|project_archived|project_cancelled
	Priority      *string
	AssigneeIDs   []string
	Tags          []string
	UDAs          map[string]string
	CreatedBy     string
	CreatedAt     int64
	ModifiedAt    int64
	RuleVersions  []RuleVersion
}

// RuleVersion 保存规则切换历史，用于重建任意时间段的投影。
// 同一 series 内 effective_from 唯一且严格递增。
type RuleVersion struct {
	ID             string
	SeriesID       string
	EffectiveFrom  int64 // 新规则的第一个槽位（anchor）
	RecurrenceRule string
	CreatedBy      string
	CreatedAt      int64
}

// Slot 表示由规则展开得到的一个日历槽位。
type Slot struct {
	RecurrenceAt int64  // 槽位时间戳（date-only 存为本地 23:59:59）
	Rule         string // 该槽位所用规则段
}

// 系列状态枚举。
const (
	StatusActive  = "active"
	StatusEnded   = "ended"
	StatusStopped = "stopped"
)

// StopReason 枚举。
const (
	StopReasonUserStopped     = "user_stopped"
	StopReasonProjectArchived = "project_archived"
	StopReasonProjectCancelled = "project_cancelled"
)

// 单次字段覆盖允许的字段集合（spec §7.5）。
var allowedOverrideFields = map[string]struct{}{
	"title":       {},
	"description": {},
	"priority":    {},
	"due":         {},
	"assignees":   {},
	"tags":        {},
	"udas":        {},
	"wait":        {},
	"scheduled":   {},
	"depends":     {},
}

// AllowedOverrideField 判断给定字段名是否允许作为 occurrence 单次覆盖。
func AllowedOverrideField(field string) bool {
	_, ok := allowedOverrideFields[field]
	return ok
}
