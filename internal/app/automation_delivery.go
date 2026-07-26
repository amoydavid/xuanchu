package app

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

// AutomationScopeWorkspaceValue 是 Workspace scope 的 AutomationScope 值，供 HTTP/MCP 引用。
var AutomationScopeWorkspaceValue = AutomationScope{Type: AutomationScopeWorkspace}

// AutomationDeliveryListInput 是 scope-aware Delivery 列表查询条件。
// ProjectRef 用 slug/id 过滤 project_id；空表示不过滤；"-" 表示 schedule 无 Project。
type AutomationDeliveryListInput struct {
	RuleID      string
	ProjectRef  string // project slug/id；"-" 表示只看 schedule 无 project
	Status      string
	TriggerType string
	Q           string
	Limit       int
	Offset      int
}

// AutomationDeliveryView 是 scope-aware Delivery 视图。ProjectID 在 project_id 为 nil 时为空。
type AutomationDeliveryView struct {
	ID                  string                     `json:"id"`
	WorkspaceID         string                     `json:"workspace_id"`
	RuleScopeType       string                     `json:"rule_scope_type"`
	RuleScopeID         string                     `json:"rule_scope_id"`
	ProjectID           string                     `json:"project_id,omitempty"`
	RuleID              string                     `json:"rule_id"`
	TriggerType         string                     `json:"trigger_type"`
	EventID             string                     `json:"event_id"`
	EventType           string                     `json:"event_type"`
	DedupeKey           string                     `json:"dedupe_key"`
	ReplayOfDeliveryID  string                     `json:"replay_of_delivery_id,omitempty"`
	Status              string                     `json:"status"`
	ResolvedURL         string                     `json:"resolved_url"`
	RenderedMethod      string                     `json:"rendered_method"`
	RenderedHeaders     map[string][]string        `json:"rendered_headers"`
	RequestBodyPreview  string                     `json:"request_body_preview"`
	RequestBodyHash     string                     `json:"request_body_hash"`
	ResponseStatusCode  *int                       `json:"response_status_code,omitempty"`
	ResponseBodyPreview string                     `json:"response_body_preview"`
	ProviderRequestID   string                     `json:"provider_request_id"`
	Usage               map[string]any             `json:"usage"`
	AttemptCount        int                        `json:"attempt_count"`
	MaxAttempts         int                        `json:"max_attempts"`
	NextAttemptAt       *int64                     `json:"next_attempt_at,omitempty"`
	LastError           string                     `json:"last_error"`
	CreatedAt           int64                      `json:"created_at"`
	ModifiedAt          int64                      `json:"modified_at"`
	Project             *AutomationDeliveryProject `json:"project,omitempty"`
}

// AutomationDeliveryProject 是 Delivery 关联 Project 的安全引用，
// schedule Delivery 为 nil。
type AutomationDeliveryProject struct {
	ID     string `json:"id"`
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// ListAutomationDeliveries 按 scope 列出 Delivery，返回 (rows, total)。
// scope=workspace 列出当前 workspace 全部 Delivery（不限 rule scope_type），
// scope=project 列出指定 project 的 Delivery（HTTP 层调用 ProjectAutomationDelivery 入口）。
func (s *Service) ListAutomationDeliveries(scope AutomationScope, input AutomationDeliveryListInput) ([]AutomationDeliveryView, int64, error) {
	if err := s.requireAutomationRead(scope); err != nil {
		return nil, 0, err
	}
	normalized, err := s.normalizeAutomationScope(scope)
	if err != nil {
		return nil, 0, err
	}
	opts := storage.AutomationDeliveryListOptions{
		WorkspaceID: s.workspaceID,
		RuleID:      input.RuleID,
		Status:      input.Status,
		TriggerType: input.TriggerType,
		Q:           input.Q,
		Limit:       input.Limit,
		Offset:      input.Offset,
	}
	if normalized.Type == AutomationScopeProject {
		// Project scope 必须锁定 project_id。
		projectID := normalized.ID
		opts.ProjectID = &projectID
	} else if input.ProjectRef == "-" {
		// Workspace scope 下显式查 schedule（project_id 为 nil）。
		empty := ""
		opts.ProjectID = &empty
	} else if input.ProjectRef != "" {
		// Workspace scope 下按 project_ref 过滤；解析为 project_id。
		project, err := s.ResolveProject(input.ProjectRef)
		if err != nil {
			return nil, 0, err
		}
		projectID := project.ID
		opts.ProjectID = &projectID
	}
	rows, err := s.projectAutomationDeliveryRepo.List(opts)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.projectAutomationDeliveryRepo.Count(opts)
	if err != nil {
		return nil, 0, err
	}
	views := make([]AutomationDeliveryView, 0, len(rows))
	for _, row := range rows {
		views = append(views, s.automationDeliveryViewFromRow(row))
	}
	if err := s.fillAutomationDeliveryProjects(views); err != nil {
		return nil, 0, err
	}
	return views, total, nil
}

// AutomationDeliveryInfo 读取单条 Delivery 详情。
// scope 用于权限校验；实际 Delivery 归属由 row.RuleScopeType 决定。
func (s *Service) AutomationDeliveryInfo(scope AutomationScope, deliveryID string) (AutomationDeliveryView, error) {
	if err := s.requireAutomationRead(scope); err != nil {
		return AutomationDeliveryView{}, err
	}
	if _, err := s.normalizeAutomationScope(scope); err != nil {
		return AutomationDeliveryView{}, err
	}
	row, err := s.projectAutomationDeliveryRepo.GetByID(deliveryID)
	if err != nil {
		return AutomationDeliveryView{}, err
	}
	if row.WorkspaceID != s.workspaceID {
		return AutomationDeliveryView{}, RuntimeError{Code: "automation_delivery_not_found", Message: "automation delivery not found"}
	}
	view := s.automationDeliveryViewFromRow(row)
	views := []AutomationDeliveryView{view}
	if err := s.fillAutomationDeliveryProjects(views); err != nil {
		return AutomationDeliveryView{}, err
	}
	return views[0], nil
}

// ReplayAutomationDelivery 重新投递一条 Delivery。
// Workspace scope 无关联 Project，总是允许 replay；Project scope 关联 Project 关闭时拒绝。
func (s *Service) ReplayAutomationDelivery(scope AutomationScope, deliveryID string) (AutomationDeliveryView, error) {
	if err := s.requireAutomationWrite(scope); err != nil {
		return AutomationDeliveryView{}, err
	}
	normalized, err := s.normalizeAutomationScope(scope)
	if err != nil {
		return AutomationDeliveryView{}, err
	}
	row, err := s.projectAutomationDeliveryRepo.GetByID(deliveryID)
	if err != nil {
		return AutomationDeliveryView{}, err
	}
	if row.WorkspaceID != s.workspaceID {
		return AutomationDeliveryView{}, RuntimeError{Code: "automation_delivery_not_found", Message: "automation delivery not found"}
	}
	if normalized.Type == AutomationScopeProject {
		if row.ProjectID == nil {
			return AutomationDeliveryView{}, RuntimeError{Code: "automation_delivery_not_found", Message: "automation delivery not found"}
		}
		project, err := s.ResolveProject(*row.ProjectID)
		if err != nil {
			return AutomationDeliveryView{}, err
		}
		if isProjectClosed(project) {
			return AutomationDeliveryView{}, RuntimeError{Code: "project_closed", Message: "project is closed"}
		}
	}
	replayed, err := s.replayAutomationDelivery(row, s.clock.Unix())
	if err != nil {
		return AutomationDeliveryView{}, err
	}
	view := s.automationDeliveryViewFromRow(replayed)
	views := []AutomationDeliveryView{view}
	if err := s.fillAutomationDeliveryProjects(views); err != nil {
		return AutomationDeliveryView{}, err
	}
	if err := s.appendAuditEntry(AuditEntry{
		Action:      "automation.delivery.replayed",
		TargetType:  "automation_delivery",
		TargetID:    replayed.ID,
		WorkspaceID: &replayed.WorkspaceID,
		ProjectID:   replayed.ProjectID,
		Payload: map[string]any{
			"scope_type":            replayed.RuleScopeType,
			"scope_id":              replayed.RuleScopeID,
			"replay_of_delivery_id": row.ID,
		},
	}); err != nil {
		return AutomationDeliveryView{}, err
	}
	return views[0], nil
}

// automationDeliveryViewFromRow 把 storage 行转 view；project_id 为 nil 时返回空字符串。
func (s *Service) automationDeliveryViewFromRow(row storage.AutomationDelivery) AutomationDeliveryView {
	headers := map[string][]string{}
	if row.RenderedHeadersJSON != "" {
		_ = json.Unmarshal([]byte(row.RenderedHeadersJSON), &headers)
	}
	usage := map[string]any{}
	if row.UsageJSON != "" {
		_ = json.Unmarshal([]byte(row.UsageJSON), &usage)
	}
	projectID := ""
	if row.ProjectID != nil {
		projectID = *row.ProjectID
	}
	view := AutomationDeliveryView{
		ID:                  row.ID,
		WorkspaceID:         row.WorkspaceID,
		RuleScopeType:       row.RuleScopeType,
		RuleScopeID:         row.RuleScopeID,
		ProjectID:           projectID,
		RuleID:              row.RuleID,
		TriggerType:         row.TriggerType,
		EventID:             row.EventID,
		EventType:           row.EventType,
		DedupeKey:           row.DedupeKey,
		Status:              row.Status,
		ResolvedURL:         row.ResolvedURL,
		RenderedMethod:      row.RenderedMethod,
		RenderedHeaders:     headers,
		RequestBodyPreview:  row.RequestBodyPreview,
		RequestBodyHash:     row.RequestBodyHash,
		ResponseStatusCode:  row.ResponseStatusCode,
		ResponseBodyPreview: row.ResponseBodyPreview,
		ProviderRequestID:   row.ProviderRequestID,
		Usage:               usage,
		AttemptCount:        row.AttemptCount,
		MaxAttempts:         row.MaxAttempts,
		NextAttemptAt:       row.NextAttemptAt,
		LastError:           row.LastError,
		CreatedAt:           row.CreatedAt,
		ModifiedAt:          row.ModifiedAt,
	}
	if row.ReplayOfDeliveryID != nil {
		view.ReplayOfDeliveryID = *row.ReplayOfDeliveryID
	}
	return view
}

// fillAutomationDeliveryProjects 批量填充 Project 引用，避免 N+1 查询。
// schedule Delivery 的 ProjectID 为 nil，Project 字段保持 nil。
func (s *Service) fillAutomationDeliveryProjects(views []AutomationDeliveryView) error {
	projectIDSet := make(map[string]struct{}, len(views))
	for _, v := range views {
		if v.ProjectID != "" {
			projectIDSet[v.ProjectID] = struct{}{}
		}
	}
	if len(projectIDSet) == 0 {
		return nil
	}
	byID := make(map[string]storage.Project, len(projectIDSet))
	for id := range projectIDSet {
		project, err := s.projectRepo.GetByID(id)
		if err != nil {
			if isStorageNotFound(err) {
				continue
			}
			return err
		}
		if project.WorkspaceID != s.workspaceID {
			continue
		}
		byID[project.ID] = project
	}
	for i := range views {
		if views[i].ProjectID == "" {
			continue
		}
		if p, ok := byID[views[i].ProjectID]; ok {
			views[i].Project = &AutomationDeliveryProject{
				ID:     p.ID,
				Slug:   p.Slug,
				Name:   p.Name,
				Status: p.Status,
			}
		}
	}
	return nil
}

// isStorageNotFound 判断错误是否为 storage.ErrNotFound。
func isStorageNotFound(err error) bool {
	return err == storage.ErrNotFound
}

// replayAutomationDeliveryInternal 是测试可注入的 replay helper；
// 生产路径走 storage.CreateReplay，复制 immutable 字段并清空 attempt/claim。
func (s *Service) replayAutomationDeliveryInternal(original storage.AutomationDelivery, now int64) (storage.AutomationDelivery, error) {
	newID := uuid.NewString()
	newDedupe := fmt.Sprintf("replay:%s:%s:%d", original.ID, newID, now)
	return s.projectAutomationDeliveryRepo.CreateReplay(original, newID, newDedupe, now)
}

// ensure strings/task imports are not pruned when future helpers need them。
var _ = strings.TrimSpace
var _ task.UserInfo
