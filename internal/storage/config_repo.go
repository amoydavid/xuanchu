package storage

import (
	"errors"
	"fmt"
	"strings"

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

type ConfigCandidateListOptions struct {
	WorkspaceID string
	ProjectID   string
	Refs        []string
	Q           string
	Mode        string // all|fixed_available|prompt_available|secret|non_secret；literal 兼容 non_secret
}

type ConfigCandidate struct {
	Config          Config
	Definition      ConfigDefinition
	HasProjectValue bool
	CanFixed        bool
	EffectiveSource string
}

type ConfigCandidatePage struct {
	Items  []ConfigCandidate
	Total  int64
	Limit  int
	Offset int
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

// ListProjectExplicitByKeys 批量读取源项目显式保存的配置行及其当前定义。
// workspace 继承值和 schema default 不会进入结果；缺失定义的行也不会被伪装成合法候选。
func (r *ConfigRepository) ListProjectExplicitByKeys(workspaceID, projectID string, keys []string) ([]ConfigCandidate, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	var configs []Config
	if err := r.db.Where("workspace_id = ? AND scope = ? AND scope_id = ? AND key IN ?", workspaceID, string(ConfigScopeProject), projectID, keys).
		Order("key ASC").Find(&configs).Error; err != nil {
		return nil, err
	}
	if len(configs) == 0 {
		return nil, nil
	}
	foundKeys := make([]string, 0, len(configs))
	for _, row := range configs {
		foundKeys = append(foundKeys, row.Key)
	}
	var definitionRows []ConfigDefinition
	if err := r.db.Where("workspace_id = ? AND key IN ?", workspaceID, foundKeys).Find(&definitionRows).Error; err != nil {
		return nil, err
	}
	definitions := make(map[string]ConfigDefinition, len(definitionRows))
	for _, row := range definitionRows {
		definitions[row.Key] = row
	}
	items := make([]ConfigCandidate, 0, len(configs))
	for _, row := range configs {
		items = append(items, ConfigCandidate{Config: row, Definition: definitions[row.Key]})
	}
	return items, nil
}

// ListCandidatePage 从允许 project scope 的定义出发列候选。
// 没有源项目显式值的定义仍可用于模板 prompt，但绝不把 workspace/default 值放进 Config。
func (r *ConfigRepository) ListCandidatePage(opts ConfigCandidateListOptions, limit, offset int) (ConfigCandidatePage, error) {
	base := r.db.Model(&ConfigDefinition{}).
		Where("config_definitions.workspace_id = ?", opts.WorkspaceID).
		Where("config_definitions.allowed_scopes_json LIKE ?", `%"project"%`)
	if len(opts.Refs) > 0 {
		base = base.Where("config_definitions.key IN ?", opts.Refs)
	}
	if q := strings.TrimSpace(opts.Q); q != "" {
		like := "%" + q + "%"
		base = base.Where("(LOWER(config_definitions.key) LIKE LOWER(?) OR LOWER(config_definitions.label) LIKE LOWER(?))", like, like)
	}
	switch opts.Mode {
	case "secret":
		base = base.Where("config_definitions.secret = ?", true)
	case "literal", "non_secret":
		base = base.Where("config_definitions.secret = ?", false)
	case "fixed_available":
		base = base.Where(`EXISTS (
			SELECT 1 FROM configs
			WHERE configs.workspace_id = config_definitions.workspace_id
			  AND configs.scope = ? AND configs.scope_id = ?
			  AND configs.key = config_definitions.key
		)`, string(ConfigScopeProject), opts.ProjectID)
	}
	var total int64
	if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return ConfigCandidatePage{}, err
	}
	var definitions []ConfigDefinition
	if err := base.Session(&gorm.Session{}).Order("config_definitions.key ASC").Limit(limit).Offset(offset).Find(&definitions).Error; err != nil {
		return ConfigCandidatePage{}, err
	}
	keys := make([]string, 0, len(definitions))
	for _, row := range definitions {
		keys = append(keys, row.Key)
	}
	projectConfigs := make(map[string]Config, len(keys))
	workspaceConfigured := make(map[string]bool, len(keys))
	if len(keys) > 0 {
		var rows []Config
		if err := r.db.Where("workspace_id = ? AND key IN ? AND ((scope = ? AND scope_id = ?) OR (scope = ? AND scope_id = ?))",
			opts.WorkspaceID, keys, string(ConfigScopeProject), opts.ProjectID, string(ConfigScopeWorkspace), opts.WorkspaceID,
		).Find(&rows).Error; err != nil {
			return ConfigCandidatePage{}, err
		}
		for _, row := range rows {
			if row.Scope == string(ConfigScopeProject) {
				projectConfigs[row.Key] = row
			} else if row.Scope == string(ConfigScopeWorkspace) {
				workspaceConfigured[row.Key] = true
			}
		}
	}
	items := make([]ConfigCandidate, 0, len(definitions))
	for _, definition := range definitions {
		config, hasProjectValue := projectConfigs[definition.Key]
		if !hasProjectValue {
			config.Key = definition.Key
		}
		source := "missing"
		switch {
		case hasProjectValue:
			source = "project"
		case workspaceConfigured[definition.Key]:
			source = "workspace"
		case definition.HasDefault:
			source = "default"
		}
		items = append(items, ConfigCandidate{
			Config: config, Definition: definition, HasProjectValue: hasProjectValue, CanFixed: hasProjectValue, EffectiveSource: source,
		})
	}
	return ConfigCandidatePage{Items: items, Total: total, Limit: limit, Offset: offset}, nil
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
