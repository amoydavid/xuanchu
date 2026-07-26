package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

// buildProjectCreatedContextSnapshot 在事务内构造 project.created 事件所需的
// Project + Workspace + effective project_config 快照。effective config 走
// project > workspace > default 解析，secret config 永远不进入快照。
func (s *Service) buildProjectCreatedContextSnapshot(project storage.Project) (AutomationContextSnapshot, error) {
	view, err := s.projectViewForRow(project)
	if err != nil {
		return AutomationContextSnapshot{}, err
	}
	ws, err := s.workspaceRepo.GetByID(s.workspaceID)
	if err != nil {
		return AutomationContextSnapshot{}, err
	}
	effective, err := s.buildProjectEffectiveConfigSnapshot(project.ID)
	if err != nil {
		return AutomationContextSnapshot{}, err
	}
	return AutomationContextSnapshot{
		Project:         project,
		ProjectView:     view,
		Workspace:       ws,
		EffectiveConfig: effective,
	}, nil
}

// buildProjectEffectiveConfigSnapshot 计算当前事务下 project > workspace > default
// 的 effective 值，并过滤掉所有 secret definition 的 key。返回 map 只包含非 secret 的有效值。
func (s *Service) buildProjectEffectiveConfigSnapshot(projectID string) (map[string]string, error) {
	out := map[string]string{}
	// workspace scope 值先入表，再由 project scope 覆盖。
	wsValues, err := s.configRepo.ListScope(s.workspaceID, storage.ConfigScopeWorkspace, s.workspaceID)
	if err != nil {
		return nil, err
	}
	for k, v := range wsValues {
		if s.configKeyIsSecret(k) {
			continue
		}
		out[k] = v
	}
	projValues, err := s.configRepo.ListScope(s.workspaceID, storage.ConfigScopeProject, projectID)
	if err != nil {
		return nil, err
	}
	for k, v := range projValues {
		if s.configKeyIsSecret(k) {
			continue
		}
		out[k] = v
	}
	// 不注入 default 值：spec §9.1 要求 missing 项不出现在便捷 map 中。
	return out, nil
}

// 该结构体只在事务内使用，序列化为 event_id/event_type/event_version 等
// 投递给 Agent 的 _xuanchu/event 节点，不携带 secret 或 snapshot 原文。
type AutomationEvent struct {
	EventID      string
	EventType    string
	EventVersion int
	OccurredAt   int64
	WorkspaceID  string
	ProjectID    string
	Actor        HookActorSnapshot
	Metadata     map[string]any
}

// HookActorSnapshot 是事件触发者的脱敏快照；secret 永不出现。
type HookActorSnapshot struct {
	ActorType        string
	ActorUserID      string
	ActorTokenID     string
	ActorTokenName   string
	ActorTokenPrefix string
}

// AutomationContextSnapshot 是事件发生时的 Project + 初始 project_config 快照。
// 用作 RouteAutomationEventTx 的输入，避免路由时再去查询可能已被修改的 Project/config。
type AutomationContextSnapshot struct {
	Project         storage.Project
	ProjectView     ProjectView
	Workspace       storage.Workspace
	EffectiveConfig map[string]string // 已过滤 secret，key→value
}

// automationRequestBodyLimit 控制 frozen request body 的最大字节数。
// 超限会被路由器视为可预期错误，落 dead_lettered Delivery 而不是回滚事务。
const automationRequestBodyLimit = 256 * 1024

// buildProjectCreatedAutomationEvent 构造 project.created 事件。
// eventID 在调用点生成，保证同一次 Project 创建只产生一个事件 UUID。
func buildProjectCreatedAutomationEvent(
	project storage.Project,
	actor HookActorSnapshot,
	metadata map[string]any,
	now int64,
) AutomationEvent {
	return AutomationEvent{
		EventID:      uuid.NewString(),
		EventType:    AutomationEventTypeProjectCreated,
		EventVersion: 1,
		OccurredAt:   now,
		WorkspaceID:  project.WorkspaceID,
		ProjectID:    project.ID,
		Actor:        actor,
		Metadata:     metadata,
	}
}

// RouteAutomationEventTx 在调用方事务内匹配当前 Workspace 的 event 规则，
// 为每条命中规则插入冻结的 AutomationDelivery。返回 nil 表示所有 Delivery 落库成功，
// 返回错误表示出现内部序列化/DB 不变量失败，调用方必须回滚整个事务。
//
// Provider 缺失、allowed host 拒绝、body 超限等可预期错误映射为 dead_lettered Delivery，
// 不阻断 Project 创建；只有数据库写入或 JSON 不变量失败才回滚事务。
//
// event 必须通过 AutomationEvent 类型传入；HookEvent 不能直接复用，因为
// project.created 故意不进入 Hook/Notification 白名单。
func (s *Service) RouteAutomationEventTx(event AutomationEvent, snapshot AutomationContextSnapshot) error {
	if event.EventType != AutomationEventTypeProjectCreated {
		// 首版只接受 project.created；其它事件由现有 EnqueueProjectAutomationForEvents 处理。
		return nil
	}
	if event.WorkspaceID != s.workspaceID {
		return RuntimeError{Code: "automation_scope_invalid", Message: "event workspace does not match current workspace"}
	}
	if strings.TrimSpace(event.ProjectID) == "" {
		return RuntimeError{Code: "automation_scope_invalid", Message: "project.created event requires project id"}
	}
	rules, err := s.projectAutomationRuleRepo.ListScope(s.workspaceID, storage.AutomationScopeWorkspace, s.workspaceID, false)
	if err != nil {
		return fmt.Errorf("automation event route: list workspace rules: %w", err)
	}
	if len(rules) == 0 {
		return nil
	}
	now := s.clock.Unix()
	rows := make([]storage.AutomationDelivery, 0, len(rules))
	for _, rule := range rules {
		if rule.TriggerType != ProjectAutomationTriggerEvent {
			continue
		}
		cfg := decodeProjectAutomationTriggerConfig(rule.TriggerConfigJSON)
		if cfg.EventType != event.EventType {
			continue
		}
		delivery, routeErr := s.buildProjectCreatedAutomationDelivery(rule, event, snapshot, now)
		if routeErr != nil {
			// 可预期错误：生成 dead_lettered Delivery 记录稳定错误码，不阻断 Project 创建。
			dead, deadErr := s.buildDeadLetterAutomationDelivery(rule, event, snapshot, now, routeErr)
			if deadErr != nil {
				// 内部不变量失败：回滚事务。
				return deadErr
			}
			rows = append(rows, dead)
			continue
		}
		rows = append(rows, delivery)
	}
	if len(rows) == 0 {
		return nil
	}
	if err := s.projectAutomationDeliveryRepo.Enqueue(rows); err != nil {
		return fmt.Errorf("automation event route: enqueue: %w", err)
	}
	return nil
}

// buildProjectCreatedAutomationDelivery 渲染并冻结一条 project.created Delivery。
// 返回的错误视为可预期错误，由 RouteAutomationEventTx 翻译为 dead_lettered Delivery。
func (s *Service) buildProjectCreatedAutomationDelivery(
	rule storage.AutomationRule,
	event AutomationEvent,
	snapshot AutomationContextSnapshot,
	now int64,
) (storage.AutomationDelivery, error) {
	input := automationRuleInputFromRow(rule)
	systemPrompt := input.SystemPrompt
	if strings.TrimSpace(systemPrompt) == "" {
		systemPrompt = defaultAutomationSystemPromptForScope(AutomationScope{Type: AutomationScopeWorkspace, ID: s.workspaceID})
	}
	baseURL, _, model, err := s.resolveWorkspaceAutomationProviderConfig(input.Action)
	if err != nil {
		return storage.AutomationDelivery{}, err
	}
	body, bodyErr := buildProjectCreatedAutomationBody(input, rule, event, snapshot, baseURL, model, systemPrompt)
	if bodyErr != nil {
		return storage.AutomationDelivery{}, bodyErr
	}
	if len(body.JSON) > automationRequestBodyLimit {
		return storage.AutomationDelivery{}, RuntimeError{Code: "automation_context_too_large", Message: "frozen request body exceeds limit"}
	}
	maskedHeaders := map[string][]string{
		"Authorization": {"Bearer ****"},
		"Content-Type":  {"application/json"},
	}
	headersJSON, _ := json.Marshal(maskedHeaders)
	projectID := event.ProjectID
	maxAttempts := input.Action.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	return storage.AutomationDelivery{
		ID:                    uuid.NewString(),
		WorkspaceID:           s.workspaceID,
		RuleScopeType:         storage.AutomationScopeWorkspace,
		RuleScopeID:           s.workspaceID,
		ProjectID:             &projectID,
		RuleID:                rule.ID,
		TriggerType:           ProjectAutomationTriggerEvent,
		EventID:               event.EventID,
		EventType:             event.EventType,
		DedupeKey:             fmt.Sprintf("event:%s:%s:%s", storage.AutomationScopeWorkspace, rule.ID, event.EventID),
		APIKeyConfigKey:       input.Action.APIKeyConfigKey,
		AllowedHostsConfigKey: input.Action.AllowedHostsConfigKey,
		MaxAttempts:           maxAttempts,
		Status:                storage.DeliveryStatusQueued,
		ResolvedURL:           strings.TrimRight(baseURL, "/") + "/v1/chat/completions",
		RenderedMethod:        "POST",
		RenderedHeadersJSON:   string(headersJSON),
		RequestBodyJSON:       body.JSON,
		RequestBodyPreview:    truncatePreview(body.JSON, 12000),
		RequestBodyHash:       body.Hash,
		UsageJSON:             "{}",
		CreatedAt:             now,
		ModifiedAt:            now,
	}, nil
}

// buildDeadLetterAutomationDelivery 在 Provider 缺失/URL 拒绝/body 超限时构造 dead_lettered Delivery。
// 不渲染完整 request body，但保留 scope/project/event 引用，便于运行记录诊断。
func (s *Service) buildDeadLetterAutomationDelivery(
	rule storage.AutomationRule,
	event AutomationEvent,
	snapshot AutomationContextSnapshot,
	now int64,
	cause error,
) (storage.AutomationDelivery, error) {
	causeMsg := safeError(cause)
	causeCode := "automation_provider_target_denied"
	var rt RuntimeError
	if errors.As(cause, &rt) && rt.Code != "" {
		causeCode = rt.Code
	}
	input := automationRuleInputFromRow(rule)
	projectID := event.ProjectID
	maxAttempts := input.Action.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	// dead-letter 仍尝试冻结 URL（若有），便于 UI 显示目标；body 留空避免泄漏部分渲染内容。
	deadURL := ""
	if baseURL, _, _, err := s.resolveWorkspaceAutomationProviderConfig(input.Action); err == nil {
		deadURL = strings.TrimRight(baseURL, "/") + "/v1/chat/completions"
	}
	failure := fmt.Sprintf("%s: %s", causeCode, causeMsg)
	return storage.AutomationDelivery{
		ID:                    uuid.NewString(),
		WorkspaceID:           s.workspaceID,
		RuleScopeType:         storage.AutomationScopeWorkspace,
		RuleScopeID:           s.workspaceID,
		ProjectID:             &projectID,
		RuleID:                rule.ID,
		TriggerType:           ProjectAutomationTriggerEvent,
		EventID:               event.EventID,
		EventType:             event.EventType,
		DedupeKey:             fmt.Sprintf("event:%s:%s:%s", storage.AutomationScopeWorkspace, rule.ID, event.EventID),
		APIKeyConfigKey:       input.Action.APIKeyConfigKey,
		AllowedHostsConfigKey: input.Action.AllowedHostsConfigKey,
		MaxAttempts:           maxAttempts,
		Status:                storage.DeliveryStatusDeadLettered,
		ResolvedURL:           deadURL,
		RenderedMethod:        "POST",
		RenderedHeadersJSON:   "{}",
		RequestBodyJSON:       "",
		RequestBodyPreview:    "",
		RequestBodyHash:       "",
		LastError:             failure,
		AttemptCount:          1,
		UsageJSON:             "{}",
		CreatedAt:             now,
		ModifiedAt:            now,
	}, nil
}

// resolveWorkspaceAutomationProviderConfig 只读 Workspace scope/default 解析 Provider。
// Workspace 规则的 Provider 必须由 Workspace 决定，Project config 不能反向覆盖。
func (s *Service) resolveWorkspaceAutomationProviderConfig(action ProjectAutomationActionConfig) (baseURL string, apiKey string, model string, err error) {
	return validateProjectAutomationProviderConfig(action, func(key string) (string, bool, error) {
		value, err := s.workspaceAutomationConfigValue(key)
		return value, strings.TrimSpace(value) != "", err
	})
}

// automationBody 是渲染后的请求体及其摘要。
type automationBody struct {
	JSON string
	Hash string
}

// buildProjectCreatedAutomationBody 构造 Workspace 规则的 frozen request body。
// context_node 直接平铺 workspace/project/project_config/event/_xuanchu，不依赖 include 列表。
// secret config 的 key/value 不会进入 context；project_config 只包含非 secret effective 值。
func buildProjectCreatedAutomationBody(
	input AutomationRuleInput,
	rule storage.AutomationRule,
	event AutomationEvent,
	snapshot AutomationContextSnapshot,
	baseURL, model, systemPrompt string,
) (automationBody, error) {
	workspaceNode := map[string]any{
		"id":   snapshot.Workspace.ID,
		"slug": snapshot.Workspace.Slug,
		"name": snapshot.Workspace.Name,
	}
	projectNode := map[string]any{
		"id":     snapshot.Project.ID,
		"slug":   snapshot.Project.Slug,
		"name":   snapshot.Project.Name,
		"status": snapshot.Project.Status,
	}
	if snapshot.Project.Description != "" {
		projectNode["description"] = snapshot.Project.Description
	}
	configNode := map[string]string{}
	for k, v := range snapshot.EffectiveConfig {
		configNode[k] = v
	}
	xuanchuNode := map[string]any{
		"automation_rule_id": rule.ID,
		"automation_scope":   string(storage.AutomationScopeWorkspace),
		"delivery_id":        "", // 由 buildProjectCreatedAutomationDelivery 填充（body 中可空）
		"event_id":           event.EventID,
		"trigger_type":       ProjectAutomationTriggerEvent,
		"triggered_at":       event.OccurredAt,
	}
	eventNode := map[string]any{
		"event_id":      event.EventID,
		"event_type":    event.EventType,
		"event_version": event.EventVersion,
		"occurred_at":   event.OccurredAt,
		"metadata":      event.Metadata,
	}
	contextNode := map[string]any{
		"_xuanchu":       xuanchuNode,
		"workspace":      workspaceNode,
		"project":        projectNode,
		"project_config": configNode,
		"event":          eventNode,
	}
	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": renderAutomationTemplate(systemPrompt, map[string]string{})},
			{"role": "user", "content": renderAutomationTemplate(input.InstructionTemplate, flattenAutomationContextVars(contextNode))},
		},
		"temperature": input.Action.Temperature,
		"metadata":    xuanchuNode,
		"input":       []map[string]any{{"type": "input_json", "input_json": contextNode}},
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return automationBody{}, err
	}
	sum := sha256.Sum256(bodyBytes)
	return automationBody{
		JSON: string(bodyBytes),
		Hash: "sha256:" + hex.EncodeToString(sum[:]),
	}, nil
}

// flattenAutomationContextVars 把 context 节点中常用标量平铺成模板变量，
// 便于 instruction 使用 {{project.slug}} 等占位符。
func flattenAutomationContextVars(ctx map[string]any) map[string]string {
	out := map[string]string{}
	add := func(key, value string) {
		if value != "" {
			out[key] = value
		}
	}
	if ws, ok := ctx["workspace"].(map[string]any); ok {
		add("workspace.id", toStringOrEmpty(ws["id"]))
		add("workspace.slug", toStringOrEmpty(ws["slug"]))
		add("workspace.name", toStringOrEmpty(ws["name"]))
	}
	if proj, ok := ctx["project"].(map[string]any); ok {
		add("project.id", toStringOrEmpty(proj["id"]))
		add("project.slug", toStringOrEmpty(proj["slug"]))
		add("project.name", toStringOrEmpty(proj["name"]))
		add("project.status", toStringOrEmpty(proj["status"]))
	}
	if xuanchu, ok := ctx["_xuanchu"].(map[string]any); ok {
		add("trigger_type", toStringOrEmpty(xuanchu["trigger_type"]))
	}
	if event, ok := ctx["event"].(map[string]any); ok {
		add("event.id", toStringOrEmpty(event["event_id"]))
		add("event.type", toStringOrEmpty(event["event_type"]))
	}
	if cfg, ok := ctx["project_config"].(map[string]string); ok {
		// project_config:<key> 平铺。
		for k, v := range cfg {
			out["project_config:"+k] = v
		}
		out["project_config"] = automationJSONIndented(cfg)
	}
	return out
}

func toStringOrEmpty(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}
