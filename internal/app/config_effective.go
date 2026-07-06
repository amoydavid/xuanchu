package app

import (
	"sort"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

// ConfigSchemaUsageView 描述单个 config key 当前被多少 workspace/project value 引用。
type ConfigSchemaUsageView struct {
	Key             string
	WorkspaceValues int64
	ProjectValues   int64
	TotalValues     int64
}

// ConfigEffectiveFilter 控制有效值查询范围。
type ConfigEffectiveFilter struct {
	// ConsoleHomeOnly 仅返回 definition.show_on_console_home=true 的 key，
	// 且过滤掉 project-only key（首页无法无歧义选择 project）。
	ConsoleHomeOnly bool
}

// ConfigEffectiveValueView 描述一个 config key 的有效值来源。
// source 顺序：project > workspace > default > missing。
type ConfigEffectiveValueView struct {
	Key               string
	Value             *string
	Source            string // project | workspace | default | missing
	ProjectValue      *string
	WorkspaceValue    *string
	DefaultValue      *string
	Definition        ConfigDefinitionView
	ShowOnConsoleHome bool
	MissingRequired   bool
}

// ConfigSchemaUsage 返回单个 config key 的使用量统计。
func (s *Service) ConfigSchemaUsage(key string) (ConfigSchemaUsageView, error) {
	if err := s.Require(PermissionConfigSchemaRead); err != nil {
		return ConfigSchemaUsageView{}, err
	}
	key, err := normalizeScopedConfigKey(key)
	if err != nil {
		return ConfigSchemaUsageView{}, err
	}
	if _, err := s.scopedConfigDefinition(key); err != nil {
		return ConfigSchemaUsageView{}, err
	}
	workspaceCount, projectCount, err := s.configRepo.CountByKey(s.workspaceID, key)
	if err != nil {
		return ConfigSchemaUsageView{}, err
	}
	return ConfigSchemaUsageView{
		Key:             key,
		WorkspaceValues: workspaceCount,
		ProjectValues:   projectCount,
		TotalValues:     workspaceCount + projectCount,
	}, nil
}

// ProjectConfigEffectiveValues 返回 project 视角的有效值列表。
// 覆盖全部 workspace 定义；source 顺序 project > workspace > default > missing。
func (s *Service) ProjectConfigEffectiveValues(projectRef string) ([]ConfigEffectiveValueView, error) {
	if err := s.Require(PermissionProjectConfigRead); err != nil {
		return nil, err
	}
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return nil, err
	}
	defs, err := s.configDefRepo.List(s.workspaceID)
	if err != nil {
		return nil, err
	}

	workspaceValues, err := s.configRepo.ListScope(s.workspaceID, storage.ConfigScopeWorkspace, s.workspaceID)
	if err != nil {
		return nil, err
	}
	projectValues, err := s.configRepo.ListScope(s.workspaceID, storage.ConfigScopeProject, project.ID)
	if err != nil {
		return nil, err
	}

	out := make([]ConfigEffectiveValueView, 0, len(defs))
	for _, row := range defs {
		view, err := configDefinitionViewFromRow(row)
		if err != nil {
			return nil, err
		}
		row := buildEffectiveValueView(view, projectValues, workspaceValues)
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// WorkspaceConfigEffectiveValues 返回 workspace 视角的有效值列表。
// 仅解析 allowed_scopes 包含 workspace 的 key。
func (s *Service) WorkspaceConfigEffectiveValues(filter ConfigEffectiveFilter) ([]ConfigEffectiveValueView, error) {
	if err := s.Require(PermissionConfigSchemaRead); err != nil {
		return nil, err
	}
	defs, err := s.configDefRepo.List(s.workspaceID)
	if err != nil {
		return nil, err
	}
	workspaceValues, err := s.configRepo.ListScope(s.workspaceID, storage.ConfigScopeWorkspace, s.workspaceID)
	if err != nil {
		return nil, err
	}

	out := make([]ConfigEffectiveValueView, 0, len(defs))
	for _, row := range defs {
		view, err := configDefinitionViewFromRow(row)
		if err != nil {
			return nil, err
		}
		allowsWorkspace := false
		for _, scope := range view.AllowedScopes {
			if scope == string(ConfigAllowedScopeWorkspace) {
				allowsWorkspace = true
				break
			}
		}
		if !allowsWorkspace {
			continue
		}
		if filter.ConsoleHomeOnly && !view.ShowOnConsoleHome {
			continue
		}
		// workspace 视角没有 project 显式值，projectValues 传空。
		out = append(out, buildEffectiveValueView(view, nil, workspaceValues))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// buildEffectiveValueView 根据定义和已加载的 project/workspace 值解析有效值来源。
// source 优先级：project > workspace > default > missing。
func buildEffectiveValueView(def ConfigDefinitionView, projectValues, workspaceValues map[string]string) ConfigEffectiveValueView {
	row := ConfigEffectiveValueView{
		Key:               def.Key,
		Definition:        def,
		ShowOnConsoleHome: def.ShowOnConsoleHome,
	}

	if v, ok := projectValues[def.Key]; ok {
		value := v
		row.ProjectValue = &value
		row.Value = &value
		row.Source = "project"
		return row
	}
	if v, ok := workspaceValues[def.Key]; ok {
		value := v
		row.WorkspaceValue = &value
		row.Value = &value
		row.Source = "workspace"
	}
	if def.DefaultValue != nil {
		row.DefaultValue = def.DefaultValue
	}
	if row.Source == "" {
		if def.DefaultValue != nil {
			defValue := *def.DefaultValue
			row.Value = &defValue
			row.Source = "default"
		} else {
			row.Source = "missing"
			row.MissingRequired = def.Required
		}
	}
	// 补充 project/workspace 显式值（即使最终来源是 default，也保留显式值供 UI 展示）
	if row.ProjectValue == nil {
		if v, ok := projectValues[def.Key]; ok {
			value := v
			row.ProjectValue = &value
		}
	}
	if row.WorkspaceValue == nil {
		if v, ok := workspaceValues[def.Key]; ok {
			value := v
			row.WorkspaceValue = &value
		}
	}
	return row
}
