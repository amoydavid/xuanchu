package storage

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ConfigScope string

const (
	ConfigScopeServer    ConfigScope = "server"
	ConfigScopeWorkspace ConfigScope = "workspace"
	ConfigScopeProject   ConfigScope = "project"
	ConfigScopeUser      ConfigScope = "user"
)

var (
	ErrInvalidConfigKey   = errors.New("invalid config key")
	ErrInvalidConfigScope = errors.New("invalid config scope")
)

type ConfigKey struct {
	WorkspaceID string
	Scope       ConfigScope
	ScopeID     string
	Key         string
}

type ConfigRepository struct {
	db *gorm.DB
}

func NewConfigRepository(db *gorm.DB) *ConfigRepository {
	return &ConfigRepository{db: db}
}

// DB 暴露底层 *gorm.DB，供 app 层在同一事务内写多个 config key。
func (r *ConfigRepository) DB() *gorm.DB {
	return r.db
}

func (r *ConfigRepository) Get(key ConfigKey) (string, bool, error) {
	normalized, err := normalizeConfigKey(key)
	if err != nil {
		return "", false, err
	}
	var row Config
	err = r.db.Where("workspace_id = ? AND scope = ? AND scope_id = ? AND key = ?",
		normalized.WorkspaceID, string(normalized.Scope), normalized.ScopeID, normalized.Key,
	).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return row.Value, true, nil
}

func (r *ConfigRepository) Set(key ConfigKey, value string) error {
	normalized, err := normalizeConfigKey(key)
	if err != nil {
		return err
	}
	row := Config{
		WorkspaceID: normalized.WorkspaceID,
		Scope:       string(normalized.Scope),
		ScopeID:     normalized.ScopeID,
		Key:         normalized.Key,
		Value:       value,
	}
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "workspace_id"},
			{Name: "scope"},
			{Name: "scope_id"},
			{Name: "key"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&row).Error
}

func (r *ConfigRepository) Unset(key ConfigKey) error {
	normalized, err := normalizeConfigKey(key)
	if err != nil {
		return err
	}
	return r.db.Where("workspace_id = ? AND scope = ? AND scope_id = ? AND key = ?",
		normalized.WorkspaceID, string(normalized.Scope), normalized.ScopeID, normalized.Key,
	).Delete(&Config{}).Error
}

func (r *ConfigRepository) ListScope(workspaceID string, scope ConfigScope, scopeID string) (map[string]string, error) {
	normalized, err := normalizeConfigKey(ConfigKey{
		WorkspaceID: workspaceID,
		Scope:       scope,
		ScopeID:     scopeID,
		Key:         "list-scope-placeholder",
	})
	if err != nil {
		return nil, err
	}
	var rows []Config
	if err := r.db.Where("workspace_id = ? AND scope = ? AND scope_id = ?",
		normalized.WorkspaceID, string(normalized.Scope), normalized.ScopeID,
	).Order("key ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, row := range rows {
		out[row.Key] = row.Value
	}
	return out, nil
}

func (r *ConfigRepository) CountByKey(workspaceID, key string) (workspaceCount int64, projectCount int64, err error) {
	if err = r.db.Model(&Config{}).
		Where("workspace_id = ? AND scope = ? AND key = ?", workspaceID, string(ConfigScopeWorkspace), key).
		Count(&workspaceCount).Error; err != nil {
		return 0, 0, err
	}
	if err = r.db.Model(&Config{}).
		Where("workspace_id = ? AND scope = ? AND key = ?", workspaceID, string(ConfigScopeProject), key).
		Count(&projectCount).Error; err != nil {
		return 0, 0, err
	}
	return workspaceCount, projectCount, nil
}

func (r *ConfigRepository) DeleteByKey(workspaceID, key string) error {
	return r.db.Where("workspace_id = ? AND key = ?", workspaceID, key).Delete(&Config{}).Error
}

// ValuesByKey 返回 workspace 下同 key 的所有 config value，按 scope/scope_id/value 排序。
// 用于 config definition 更新锁定规则和 effective value 解析。
func (r *ConfigRepository) ValuesByKey(workspaceID, key string) ([]string, error) {
	var rows []Config
	if err := r.db.Where("workspace_id = ? AND key = ?", workspaceID, key).
		Order("scope ASC, scope_id ASC, value ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	values := make([]string, 0, len(rows))
	for _, row := range rows {
		values = append(values, row.Value)
	}
	return values, nil
}

// ValuesByKeyAndScope 返回 workspace 下同 key、指定 scope 的 config value。
// scope 锁定规则需要区分 workspace/project value 是否存在。
func (r *ConfigRepository) ValuesByKeyAndScope(workspaceID, key string, scope ConfigScope) ([]string, error) {
	var rows []Config
	if err := r.db.Where("workspace_id = ? AND key = ? AND scope = ?", workspaceID, key, string(scope)).
		Order("scope_id ASC, value ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	values := make([]string, 0, len(rows))
	for _, row := range rows {
		values = append(values, row.Value)
	}
	return values, nil
}

func normalizeConfigKey(key ConfigKey) (ConfigKey, error) {
	if key.Key == "" {
		return ConfigKey{}, fmt.Errorf("%w: key is required", ErrInvalidConfigKey)
	}
	switch key.Scope {
	case ConfigScopeServer:
		key.WorkspaceID = ""
		key.ScopeID = ""
	case ConfigScopeWorkspace:
		if key.WorkspaceID == "" {
			return ConfigKey{}, fmt.Errorf("%w: workspace_id is required for %s config", ErrInvalidConfigScope, key.Scope)
		}
		key.ScopeID = key.WorkspaceID
	case ConfigScopeProject:
		if key.WorkspaceID == "" {
			return ConfigKey{}, fmt.Errorf("%w: workspace_id is required for %s config", ErrInvalidConfigScope, key.Scope)
		}
		if key.ScopeID == "" {
			return ConfigKey{}, fmt.Errorf("%w: scope_id is required for %s config", ErrInvalidConfigScope, key.Scope)
		}
	case ConfigScopeUser:
		if key.WorkspaceID == "" {
			return ConfigKey{}, fmt.Errorf("%w: workspace_id is required for %s config", ErrInvalidConfigScope, key.Scope)
		}
		if key.ScopeID == "" {
			return ConfigKey{}, fmt.Errorf("%w: scope_id is required for %s config", ErrInvalidConfigScope, key.Scope)
		}
	default:
		return ConfigKey{}, fmt.Errorf("%w: unknown config scope %q", ErrInvalidConfigScope, key.Scope)
	}
	return key, nil
}
