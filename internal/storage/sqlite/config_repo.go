package sqlite

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
