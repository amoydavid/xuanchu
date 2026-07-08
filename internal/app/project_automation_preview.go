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
		return ProjectAutomationPreviewView{}, err
	}
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return ProjectAutomationPreviewView{}, err
	}
	if row.WorkspaceID != s.workspaceID || row.ProjectID != project.ID {
		return ProjectAutomationPreviewView{}, RuntimeError{Code: "automation_rule_not_found", Message: "automation rule not found"}
	}
	input := projectAutomationRuleAddInputFromRow(row)
	if override != nil {
		input = ProjectAutomationRuleAddInput(*override)
	}
	return s.PreviewProjectAutomation(projectRef, ProjectAutomationPreviewInput(input))
}

// renderProjectAutomationRequest 渲染最终投递请求：解析 provider config、构造上下文、组装最小 OpenAI 兼容 body。
func (s *Service) renderProjectAutomationRequest(project ProjectView, ruleID string, input ProjectAutomationRuleAddInput, triggerType string, deliveryID string, event *HookEvent) (ProjectAutomationRenderedRequest, error) {
	baseURL, apiKey, model, err := s.resolveProjectAutomationProviderConfig(project.ID, input.Action)
	if err != nil {
		return ProjectAutomationRenderedRequest{}, err
	}
	ctx, err := s.buildProjectAutomationContext(project, ruleID, input, triggerType, deliveryID, event)
	if err != nil {
		return ProjectAutomationRenderedRequest{}, err
	}
	ctxJSON, err := json.Marshal(ctx)
	if err != nil {
		return ProjectAutomationRenderedRequest{}, err
	}
	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": "你是项目自动化执行 Agent。你会收到来自璇础的项目上下文，请按用户指令执行。需要调用外部系统时，使用你所在 Agent 平台已配置的工具、skill、MCP 或 CLI。",
			},
			{
				"role":    "user",
				"content": input.InstructionTemplate + "\n\n<context>" + string(ctxJSON) + "</context>",
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
	baseURL, err = s.projectAutomationEffectiveConfigValue(projectID, action.BaseURLConfigKey)
	if err != nil || strings.TrimSpace(baseURL) == "" {
		return "", "", "", RuntimeError{Code: "automation_provider_config_missing", Message: "missing agent.provider.base_url"}
	}
	apiKey, err = s.projectAutomationEffectiveConfigValue(projectID, action.APIKeyConfigKey)
	if err != nil || strings.TrimSpace(apiKey) == "" {
		return "", "", "", RuntimeError{Code: "automation_provider_config_missing", Message: "missing agent.provider.api_key"}
	}
	model = strings.TrimSpace(action.ModelOverride)
	if model == "" {
		model, err = s.projectAutomationEffectiveConfigValue(projectID, action.ModelConfigKey)
		if err != nil || strings.TrimSpace(model) == "" {
			return "", "", "", RuntimeError{Code: "automation_provider_config_missing", Message: "missing agent.provider.model"}
		}
	}
	allowedHosts, err := s.projectAutomationAllowedHosts(projectID, action.AllowedHostsConfigKey)
	if err != nil {
		return "", "", "", err
	}
	if err := validateResolvedNotificationURL(strings.TrimRight(baseURL, "/")+"/v1/chat/completions", allowedHosts); err != nil {
		return "", "", "", err
	}
	return strings.TrimSpace(baseURL), strings.TrimSpace(apiKey), strings.TrimSpace(model), nil
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

func (s *Service) projectAutomationAllowedHosts(projectID string, key string) ([]string, error) {
	if strings.TrimSpace(key) == "" {
		key = "agent.provider.allowed_hosts"
	}
	raw, err := s.projectAutomationEffectiveConfigValue(projectID, key)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(raw) == "[]" {
		return nil, RuntimeError{Code: "automation_provider_allowed_hosts_missing", Message: "agent.provider.allowed_hosts is required"}
	}
	var hosts []string
	if err := json.Unmarshal([]byte(raw), &hosts); err != nil {
		return nil, RuntimeError{Code: "automation_provider_allowed_hosts_invalid", Message: "agent.provider.allowed_hosts must be a JSON string array"}
	}
	if len(hosts) == 0 {
		return nil, RuntimeError{Code: "automation_provider_allowed_hosts_missing", Message: "agent.provider.allowed_hosts is required"}
	}
	return hosts, nil
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
		Query:    expr,
		NowUnix:  s.clock.Unix(),
		Limit:    limit,
		Dialect:  s.store.Dialect(),
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
