package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/query"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

const defaultAutomationSystemPrompt = "你是项目自动化执行 Agent。你会收到来自璇础的项目上下文，请按用户指令执行。需要调用外部系统时，使用你所在 Agent 平台已配置的工具、skill、MCP 或 CLI。"

// ProjectAutomationPreviewInput 等价于 ProjectAutomationRuleAddInput，用于未保存规则的预览。
type ProjectAutomationPreviewInput = ProjectAutomationRuleAddInput

// ProjectAutomationPreviewView 是预览接口返回的脱敏投递请求。
type ProjectAutomationPreviewView struct {
	Method   string            `json:"method"`
	URL      string            `json:"url"`
	Headers  map[string]string `json:"headers"`
	Body     map[string]any    `json:"body"`
	Warnings []string          `json:"warnings"`
}

// ProjectAutomationRenderedRequest 是渲染后的完整投递请求，供入队冻结使用。
type ProjectAutomationRenderedRequest struct {
	Method        string
	URL           string
	Headers       map[string]string
	MaskedHeaders map[string]string
	BodyJSON      string
	BodyPreview   string
	BodyHash      string
	Model         string
}

// PreviewProjectAutomation 基于未保存的规则输入生成预览，不写投递记录。
func (s *Service) PreviewProjectAutomation(projectRef string, input ProjectAutomationPreviewInput) (ProjectAutomationPreviewView, error) {
	if err := s.requireProjectAutomationWrite(); err != nil {
		return ProjectAutomationPreviewView{}, err
	}
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return ProjectAutomationPreviewView{}, err
	}
	projectView, err := s.projectViewForRow(project)
	if err != nil {
		return ProjectAutomationPreviewView{}, err
	}
	normalized, err := normalizeProjectAutomationAddInput(ProjectAutomationRuleAddInput(input))
	if err != nil {
		return ProjectAutomationPreviewView{}, err
	}
	rendered, err := s.renderProjectAutomationRequest(projectView, "", normalized, "preview", "", nil)
	if err != nil {
		return ProjectAutomationPreviewView{}, err
	}
	body := map[string]any{}
	if err := json.Unmarshal([]byte(rendered.BodyJSON), &body); err != nil {
		return ProjectAutomationPreviewView{}, err
	}
	return ProjectAutomationPreviewView{
		Method:   rendered.Method,
		URL:      rendered.URL,
		Headers:  rendered.MaskedHeaders,
		Body:     body,
		Warnings: nil,
	}, nil
}

// PreviewSavedProjectAutomation 基于已保存规则生成预览，可覆盖部分字段。
func (s *Service) PreviewSavedProjectAutomation(projectRef string, ruleID string, override *ProjectAutomationPreviewInput) (ProjectAutomationPreviewView, error) {
	if err := s.requireProjectAutomationWrite(); err != nil {
		return ProjectAutomationPreviewView{}, err
	}
	row, err := s.projectAutomationRuleRepo.GetByID(ruleID)
	if err != nil {
		return ProjectAutomationPreviewView{}, automationRuleNotFound(err)
	}
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return ProjectAutomationPreviewView{}, err
	}
	if row.WorkspaceID != s.workspaceID || row.ScopeType != storage.AutomationScopeProject || row.ScopeID != project.ID {
		return ProjectAutomationPreviewView{}, RuntimeError{Code: "automation_rule_not_found", Message: "automation rule not found"}
	}
	input := projectAutomationRuleAddInputFromRow(row)
	if override != nil {
		input = ProjectAutomationRuleAddInput(*override)
	}
	return s.PreviewProjectAutomation(projectRef, ProjectAutomationPreviewInput(input))
}

// renderProjectAutomationRequest 渲染最终投递请求：解析 provider config、用模板变量渲染 system/user message、组装 OpenAI 兼容 body。
func (s *Service) renderProjectAutomationRequest(project ProjectView, ruleID string, input ProjectAutomationRuleAddInput, triggerType string, deliveryID string, event *HookEvent) (ProjectAutomationRenderedRequest, error) {
	baseURL, apiKey, model, err := s.resolveProjectAutomationProviderConfig(project.ID, input.Action)
	if err != nil {
		return ProjectAutomationRenderedRequest{}, err
	}
	vars, err := s.buildAutomationTemplateVars(project, ruleID, input, triggerType, deliveryID, event)
	if err != nil {
		return ProjectAutomationRenderedRequest{}, err
	}
	systemPrompt := input.SystemPrompt
	if strings.TrimSpace(systemPrompt) == "" {
		systemPrompt = defaultAutomationSystemPrompt
	}
	renderedSystem := renderAutomationTemplate(systemPrompt, vars)
	renderedUser := renderAutomationTemplate(input.InstructionTemplate, vars)
	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": renderedSystem,
			},
			{
				"role":    "user",
				"content": renderedUser,
			},
		},
		"temperature": input.Action.Temperature,
	}
	if input.Action.AttachMetadata {
		body["metadata"] = map[string]any{
			"source":             "xuanchu",
			"workspace_id":       project.WorkspaceID,
			"project_id":         project.ID,
			"automation_rule_id": ruleID,
			"delivery_id":        deliveryID,
		}
	}
	bodyJSONBytes, err := json.Marshal(body)
	if err != nil {
		return ProjectAutomationRenderedRequest{}, err
	}
	sum := sha256.Sum256(bodyJSONBytes)
	return ProjectAutomationRenderedRequest{
		Method: http.MethodPost,
		URL:    strings.TrimRight(baseURL, "/") + "/v1/chat/completions",
		Headers: map[string]string{
			"Authorization": "Bearer " + apiKey,
			"Content-Type":  "application/json",
		},
		MaskedHeaders: map[string]string{
			"Authorization": "Bearer ****",
			"Content-Type":  "application/json",
		},
		BodyJSON:    string(bodyJSONBytes),
		BodyPreview: truncatePreview(string(bodyJSONBytes), 12000),
		BodyHash:    "sha256:" + hex.EncodeToString(sum[:]),
		Model:       model,
	}, nil
}

// resolveProjectAutomationProviderConfig 解析 provider config 并校验 allowed_hosts。
func (s *Service) resolveProjectAutomationProviderConfig(projectID string, action ProjectAutomationActionConfig) (baseURL string, apiKey string, model string, err error) {
	return validateProjectAutomationProviderConfig(action, func(key string) (string, bool, error) {
		value, err := s.projectAutomationEffectiveConfigValue(projectID, key)
		return value, strings.TrimSpace(value) != "", err
	})
}

// automationConfigResolver 让已保存项目和“尚未落库的新项目”复用同一套
// provider/config/allowed_hosts 校验，不需要为了预检伪造 project row。
type automationConfigResolver func(key string) (value string, ok bool, err error)

func validateProjectAutomationProviderConfig(action ProjectAutomationActionConfig, resolve automationConfigResolver) (baseURL string, apiKey string, model string, err error) {
	required := func(key, label string) (string, error) {
		value, ok, resolveErr := resolve(strings.TrimSpace(key))
		// 延续既有对外语义：provider 必填值的 definition/读取失败与空值
		// 都统一映射为 missing，不向调用方泄漏底层 config 细节。
		if resolveErr != nil || !ok || strings.TrimSpace(value) == "" {
			return "", RuntimeError{Code: "automation_provider_config_missing", Message: "missing " + label}
		}
		return strings.TrimSpace(value), nil
	}
	baseURL, err = required(action.BaseURLConfigKey, "agent.provider.base_url")
	if err != nil {
		return "", "", "", err
	}
	apiKey, err = required(action.APIKeyConfigKey, "agent.provider.api_key")
	if err != nil {
		return "", "", "", err
	}
	model = strings.TrimSpace(action.ModelOverride)
	if model == "" {
		model, err = required(action.ModelConfigKey, "agent.provider.model")
		if err != nil {
			return "", "", "", err
		}
	}
	allowedKey := strings.TrimSpace(action.AllowedHostsConfigKey)
	if allowedKey == "" {
		allowedKey = "agent.provider.allowed_hosts"
	}
	allowedRaw, ok, err := resolve(allowedKey)
	if err != nil {
		return "", "", "", err
	}
	var allowedHosts []string
	if ok && strings.TrimSpace(allowedRaw) != "" && strings.TrimSpace(allowedRaw) != "[]" {
		if err := json.Unmarshal([]byte(allowedRaw), &allowedHosts); err != nil || allowedHosts == nil {
			return "", "", "", RuntimeError{Code: "automation_provider_allowed_hosts_invalid", Message: "agent.provider.allowed_hosts must be a JSON string array"}
		}
	}
	if err := validateResolvedNotificationURL(strings.TrimRight(baseURL, "/")+"/v1/chat/completions", allowedHosts); err != nil {
		return "", "", "", err
	}
	return baseURL, apiKey, model, nil
}

func truncatePreview(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "[truncated]"
}

func (s *Service) projectAutomationEffectiveConfigValue(projectID string, key string) (string, error) {
	key, err := normalizeScopedConfigKey(key)
	if err != nil {
		return "", err
	}
	def, err := s.scopedConfigDefinition(key)
	if err != nil {
		return "", err
	}
	if v, ok, err := s.configRepo.Get(storage.ConfigKey{WorkspaceID: s.workspaceID, Scope: storage.ConfigScopeProject, ScopeID: projectID, Key: key}); err != nil || ok {
		return v, err
	}
	if v, ok, err := s.configRepo.Get(storage.ConfigKey{WorkspaceID: s.workspaceID, Scope: storage.ConfigScopeWorkspace, ScopeID: s.workspaceID, Key: key}); err != nil || ok {
		return v, err
	}
	if def.DefaultValue != nil {
		return *def.DefaultValue, nil
	}
	return "", nil
}

// workspaceAutomationConfigValue 只读取 Workspace scope/default，不走 Project scope。
// Workspace 规则的 Provider 配置必须由 Workspace 决定，Project 自身 config 不能反向覆盖。
func (s *Service) workspaceAutomationConfigValue(key string) (string, error) {
	key, err := normalizeScopedConfigKey(key)
	if err != nil {
		return "", err
	}
	def, err := s.scopedConfigDefinition(key)
	if err != nil {
		return "", err
	}
	if v, ok, err := s.configRepo.Get(storage.ConfigKey{WorkspaceID: s.workspaceID, Scope: storage.ConfigScopeWorkspace, ScopeID: s.workspaceID, Key: key}); err != nil || ok {
		return v, err
	}
	if def.DefaultValue != nil {
		return *def.DefaultValue, nil
	}
	return "", nil
}

// resolveAutomationDeliveryAPIKey 按 Delivery 冻结的 scope 读取当前 secret 明文。
// Project scope：保留 project > workspace > default 解析；Workspace scope：只读 Workspace/default。
// dispatcher 不读取当前 Rule，因此 Rule 被修改或删除后仍能发送。
func (s *Service) resolveAutomationDeliveryAPIKey(row storage.AutomationDelivery) (string, error) {
	key := strings.TrimSpace(row.APIKeyConfigKey)
	if key == "" {
		return "", RuntimeError{Code: "automation_provider_config_missing", Message: "delivery missing api_key_config_key"}
	}
	normalized, err := normalizeScopedConfigKey(key)
	if err != nil {
		return "", err
	}
	switch row.RuleScopeType {
	case storage.AutomationScopeWorkspace:
		value, err := s.workspaceAutomationConfigValue(normalized)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(value) == "" {
			return "", RuntimeError{Code: "automation_provider_config_missing", Message: "missing agent.provider.api_key"}
		}
		return value, nil
	default:
		if row.ProjectID == nil || *row.ProjectID == "" {
			return "", RuntimeError{Code: "automation_provider_config_missing", Message: "missing agent.provider.api_key"}
		}
		value, err := s.projectAutomationEffectiveConfigValue(*row.ProjectID, normalized)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(value) == "" {
			return "", RuntimeError{Code: "automation_provider_config_missing", Message: "missing agent.provider.api_key"}
		}
		return value, nil
	}
}

// buildAutomationTemplateVars 构建模板变量 map，键为变量名（不含 {{}}），值为渲染后的字符串。
// 不依赖 Context.Include（那是旧 <context> JSON 的遗留），模板变量始终全量填充，
// 用户写了 {{task}} 就能拿到值，不需要在 include 列表里加 "task"。
func (s *Service) buildAutomationTemplateVars(project ProjectView, ruleID string, input ProjectAutomationRuleAddInput, triggerType string, deliveryID string, event *HookEvent) (map[string]string, error) {
	vars := map[string]string{
		"delivery_id":  deliveryID,
		"trigger_type": triggerType,
	}
	// workspace
	if ws, err := s.workspaceRepo.GetByID(s.workspaceID); err == nil {
		vars["workspace.id"] = ws.ID
		vars["workspace.slug"] = ws.Slug
		vars["workspace.name"] = ws.Name
	}
	// project
	vars["project.id"] = project.ID
	vars["project.slug"] = project.Slug
	vars["project.name"] = project.Name
	vars["project.status"] = project.Status
	// project_config：整体 JSON + 按 key 平铺为 project_config:<key>
	values, err := s.configRepo.ListScope(s.workspaceID, storage.ConfigScopeProject, project.ID)
	if err != nil {
		return nil, err
	}
	cfgMap := make(map[string]string, len(values))
	for k, v := range values {
		if s.configKeyIsSecret(k) {
			continue
		}
		cfgMap[k] = v
	}
	vars["project_config"] = automationJSONIndented(cfgMap)
	for k, v := range cfgMap {
		vars["project_config:"+k] = v
	}
	// tasks / task_summary（schedule）
	if triggerType == ProjectAutomationTriggerSchedule || triggerType == "preview" {
		tasks, err := s.listProjectAutomationTasks(project.ID, input.Condition)
		if err == nil {
			vars["tasks"] = automationJSONIndented(buildProjectAutomationTaskList(tasks))
			vars["task_summary"] = automationJSONIndented(buildProjectAutomationTaskSummary(tasks))
		}
	}
	// event 相关：始终填充，不依赖 include
	if event != nil {
		vars["event.type"] = event.EventType
		vars["event.id"] = event.EventID
		// task：从事件 ObjectID 加载，平铺标量字段 + 整体 JSON
		if event.ObjectID != "" {
			if tsk, err := s.repo.GetByUUID(s.workspaceID, event.ObjectID); err == nil {
				taskInfo := buildProjectAutomationTaskInfo(tsk)
				vars["task"] = automationJSONIndented(taskInfo)
				// 平铺常用标量字段
				if id, ok := taskInfo["uuid"].(string); ok {
					vars["task.id"] = id
				}
				if slug, ok := taskInfo["task_slug"].(string); ok {
					vars["task.slug"] = slug
				}
				if title, ok := taskInfo["title"].(string); ok {
					vars["task.title"] = title
				}
				if status, ok := taskInfo["status"].(string); ok {
					vars["task.status"] = status
				}
			}
		}
		// added_assignees
		if added, ok := event.Data["added_assignees"]; ok {
			vars["added_assignees"] = automationJSONIndented(added)
		}
		// mentioned_users / current_mentioned_users：spec §16.3 自动化上下文 include。
		if mentioned, ok := event.Data["mentioned_users"]; ok {
			vars["mentioned_users"] = automationJSONIndented(mentioned)
		}
		if current, ok := event.Data["current_mentioned_users"]; ok {
			vars["current_mentioned_users"] = automationJSONIndented(current)
		}
	}
	return vars, nil
}

func automationAnyToString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func automationJSONIndented(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}

// buildProjectAutomationContext 按规则 include 列表构造投递上下文，secret config 值不进入上下文。
func (s *Service) buildProjectAutomationContext(project ProjectView, ruleID string, input ProjectAutomationRuleAddInput, triggerType string, deliveryID string, event *HookEvent) (map[string]any, error) {
	include := map[string]bool{}
	for _, key := range input.Context.Include {
		include[key] = true
	}
	ctx := map[string]any{}
	xuanchuNode := map[string]any{
		"source":             "xuanchu",
		"workspace_id":       project.WorkspaceID,
		"project_id":         project.ID,
		"automation_rule_id": ruleID,
		"trigger_type":       triggerType,
		"delivery_id":        deliveryID,
	}
	if event != nil {
		xuanchuNode["event_type"] = event.EventType
	}
	ctx["_xuanchu"] = xuanchuNode

	if include["workspace"] {
		ws, err := s.workspaceRepo.GetByID(s.workspaceID)
		if err == nil {
			ctx["workspace"] = map[string]any{
				"id":   ws.ID,
				"slug": ws.Slug,
				"name": ws.Name,
			}
		}
	}
	if include["project"] {
		ctx["project"] = map[string]any{
			"id":     project.ID,
			"slug":   project.Slug,
			"name":   project.Name,
			"status": project.Status,
		}
	}
	if include["project_config"] {
		// 只包含非 secret config，secret 不进入上下文。
		values, err := s.configRepo.ListScope(s.workspaceID, storage.ConfigScopeProject, project.ID)
		if err != nil {
			return nil, err
		}
		summary := make(map[string]string, len(values))
		for k, v := range values {
			if s.configKeyIsSecret(k) {
				continue
			}
			summary[k] = v
		}
		ctx["project_config"] = summary
	}
	if include["task_summary"] || include["matched_tasks"] {
		tasks, err := s.listProjectAutomationTasks(project.ID, input.Condition)
		if err != nil {
			return nil, err
		}
		if include["task_summary"] {
			ctx["task_summary"] = buildProjectAutomationTaskSummary(tasks)
		}
		if include["matched_tasks"] {
			ctx["matched_tasks"] = buildProjectAutomationTaskList(tasks)
		}
	}
	if event != nil {
		if include["event"] {
			ctx["event"] = map[string]any{
				"id":          event.EventID,
				"type":        event.EventType,
				"occurred_at": event.OccurredAt,
				"actor":       s.projectAutomationEventActor(event),
			}
		}
		if include["task"] {
			if tsk, err := s.repo.GetByUUID(s.workspaceID, event.ObjectID); err == nil {
				ctx["task"] = buildProjectAutomationTaskInfo(tsk)
			}
		}
		if include["added_assignees"] {
			if added, ok := event.Data["added_assignees"]; ok {
				ctx["added_assignees"] = added
			}
		}
		if include["mentioned_users"] {
			if mentioned, ok := event.Data["mentioned_users"]; ok {
				ctx["mentioned_users"] = mentioned
			}
		}
		if include["current_mentioned_users"] {
			if current, ok := event.Data["current_mentioned_users"]; ok {
				ctx["current_mentioned_users"] = current
			}
		}
	}
	return ctx, nil
}

func (s *Service) listProjectAutomationTasks(projectID string, condition ProjectAutomationCondition) ([]task.Task, error) {
	expr := query.Predicate{Attribute: query.AttrProjectID, Operator: query.OpEqual, Value: query.StringValue(projectID)}
	limit := condition.MaxTasks
	if limit <= 0 {
		limit = 50
	}
	tasks, err := s.repo.List(s.workspaceID, storage.ListOptions{
		Query:   expr,
		NowUnix: s.clock.Unix(),
		Limit:   limit,
		Dialect: s.store.Dialect(),
	})
	if err != nil {
		return nil, mapProjectQueryCompileError(err)
	}
	return tasks, nil
}

func buildProjectAutomationTaskSummary(tasks []task.Task) map[string]any {
	summary := map[string]any{
		"pending":       0,
		"waiting":       0,
		"completed":     0,
		"overdue":       0,
		"high_priority": 0,
		"unassigned":    0,
	}
	for _, tsk := range tasks {
		switch tsk.Status {
		case task.StatusPending:
			summary["pending"] = summary["pending"].(int) + 1
		case task.StatusWaiting:
			summary["waiting"] = summary["waiting"].(int) + 1
		case task.StatusCompleted:
			summary["completed"] = summary["completed"].(int) + 1
		}
		if tsk.Priority != nil && *tsk.Priority == "H" {
			summary["high_priority"] = summary["high_priority"].(int) + 1
		}
		if len(tsk.Assignees) == 0 {
			summary["unassigned"] = summary["unassigned"].(int) + 1
		}
	}
	return summary
}

func buildProjectAutomationTaskList(tasks []task.Task) []map[string]any {
	out := make([]map[string]any, 0, len(tasks))
	for _, tsk := range tasks {
		out = append(out, buildProjectAutomationTaskInfo(tsk))
	}
	return out
}

func buildProjectAutomationTaskInfo(tsk task.Task) map[string]any {
	info := map[string]any{
		"uuid":      tsk.UUID,
		"task_slug": taskRefForNotification(tsk),
		"title":     tsk.Title,
		"status":    tsk.Status,
	}
	if tsk.Priority != nil {
		info["priority"] = *tsk.Priority
	}
	if tsk.Due != nil {
		info["due"] = *tsk.Due
	}
	if len(tsk.Assignees) > 0 {
		assignees := make([]map[string]any, 0, len(tsk.Assignees))
		for _, a := range tsk.Assignees {
			assignees = append(assignees, map[string]any{
				"id":           a.UserID,
				"name":         a.Name,
				"email":        a.Email,
				"external_ids": buildAssigneeExternalIDs(a.ExternalIDs),
			})
		}
		info["assignees"] = assignees
	}
	return info
}

func buildAssigneeExternalIDs(ids []task.ExternalIDInfo) []map[string]any {
	out := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		out = append(out, map[string]any{
			"provider":    id.Provider,
			"external_id": id.ExternalID,
		})
	}
	return out
}

func (s *Service) projectAutomationEventActor(event *HookEvent) map[string]any {
	if event.ActorUserID == "" && event.ActorTokenID == "" {
		return map[string]any{}
	}
	id := event.ActorUserID
	if id == "" {
		id = event.ActorTokenID
	}
	users, err := s.resolveUserInfos([]string{id})
	if err != nil || len(users) == 0 {
		return map[string]any{"id": id, "name": event.ActorTokenName}
	}
	u := users[id]
	if u.ID == "" {
		return map[string]any{"id": id, "name": event.ActorTokenName}
	}
	return map[string]any{
		"id":           u.ID,
		"name":         u.Name,
		"email":        u.Email,
		"external_ids": buildAssigneeExternalIDs(u.ExternalIDs),
	}
}
