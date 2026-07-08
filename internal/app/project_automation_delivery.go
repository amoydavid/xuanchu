package app

import (
	"encoding/json"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

// ProjectAutomationDeliveryListInput 描述投递记录列表查询条件。
type ProjectAutomationDeliveryListInput struct {
	RuleID string
	Status string
	Limit  int
	Offset int
}

// ProjectAutomationDeliveryView 是对外暴露的投递记录视图。
type ProjectAutomationDeliveryView struct {
	ID                  string                 `json:"id"`
	WorkspaceID         string                 `json:"workspace_id"`
	ProjectID           string                 `json:"project_id"`
	RuleID              string                 `json:"rule_id"`
	TriggerType         string                 `json:"trigger_type"`
	EventID             string                 `json:"event_id"`
	EventType           string                 `json:"event_type"`
	Status              string                 `json:"status"`
	ResolvedURL         string                 `json:"resolved_url"`
	RenderedMethod      string                 `json:"rendered_method"`
	RenderedHeaders     map[string][]string    `json:"rendered_headers"`
	RequestBodyPreview  string                 `json:"request_body_preview"`
	RequestBodyHash     string                 `json:"request_body_hash"`
	ResponseStatusCode  *int                   `json:"response_status_code"`
	ResponseBodyPreview string                 `json:"response_body_preview"`
	ProviderRequestID   string                 `json:"provider_request_id"`
	Usage               map[string]any         `json:"usage"`
	AttemptCount        int                    `json:"attempt_count"`
	NextAttemptAt       *int64                 `json:"next_attempt_at"`
	LastError           string                 `json:"last_error"`
	CreatedAt           int64                  `json:"created_at"`
	ModifiedAt          int64                  `json:"modified_at"`
}

// ListProjectAutomationDeliveries 列出项目投递记录。
func (s *Service) ListProjectAutomationDeliveries(projectRef string, input ProjectAutomationDeliveryListInput) ([]ProjectAutomationDeliveryView, error) {
	if err := s.requireProjectAutomationRead(); err != nil {
		return nil, err
	}
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return nil, err
	}
	rows, err := s.projectAutomationDeliveryRepo.List(storage.ProjectAutomationDeliveryListOptions{WorkspaceID: s.workspaceID, ProjectID: &project.ID, RuleID: input.RuleID, Status: input.Status, Limit: input.Limit, Offset: input.Offset})
	if err != nil {
		return nil, err
	}
	out := make([]ProjectAutomationDeliveryView, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectAutomationDeliveryViewFromRow(row))
	}
	return out, nil
}

// ProjectAutomationDeliveryInfo 读取单条投递记录详情。
func (s *Service) ProjectAutomationDeliveryInfo(projectRef string, deliveryID string) (ProjectAutomationDeliveryView, error) {
	if err := s.requireProjectAutomationRead(); err != nil {
		return ProjectAutomationDeliveryView{}, err
	}
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return ProjectAutomationDeliveryView{}, err
	}
	row, err := s.projectAutomationDeliveryRepo.GetByID(deliveryID)
	if err != nil {
		return ProjectAutomationDeliveryView{}, err
	}
	if row.WorkspaceID != s.workspaceID || row.ProjectID != project.ID {
		return ProjectAutomationDeliveryView{}, RuntimeError{Code: "automation_delivery_not_found", Message: "automation delivery not found"}
	}
	return projectAutomationDeliveryViewFromRow(row), nil
}

// ReplayProjectAutomationDelivery 重新投递一条记录，closed project 拒绝。
func (s *Service) ReplayProjectAutomationDelivery(projectRef string, deliveryID string) (ProjectAutomationDeliveryView, error) {
	if err := s.requireProjectAutomationWrite(); err != nil {
		return ProjectAutomationDeliveryView{}, err
	}
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return ProjectAutomationDeliveryView{}, err
	}
	if isProjectClosed(project) {
		return ProjectAutomationDeliveryView{}, RuntimeError{Code: "project_closed", Message: "project is closed"}
	}
	row, err := s.projectAutomationDeliveryRepo.GetByID(deliveryID)
	if err != nil {
		return ProjectAutomationDeliveryView{}, err
	}
	if row.WorkspaceID != s.workspaceID || row.ProjectID != project.ID {
		return ProjectAutomationDeliveryView{}, RuntimeError{Code: "automation_delivery_not_found", Message: "automation delivery not found"}
	}
	if err := s.projectAutomationDeliveryRepo.Requeue(deliveryID, s.clock.Unix()); err != nil {
		return ProjectAutomationDeliveryView{}, err
	}
	requeued, err := s.projectAutomationDeliveryRepo.GetByID(deliveryID)
	if err != nil {
		return ProjectAutomationDeliveryView{}, err
	}
	return projectAutomationDeliveryViewFromRow(requeued), nil
}

func projectAutomationDeliveryViewFromRow(row storage.ProjectAutomationDelivery) ProjectAutomationDeliveryView {
	headers := map[string][]string{}
	if row.RenderedHeadersJSON != "" {
		_ = json.Unmarshal([]byte(row.RenderedHeadersJSON), &headers)
	}
	usage := map[string]any{}
	if row.UsageJSON != "" {
		_ = json.Unmarshal([]byte(row.UsageJSON), &usage)
	}
	return ProjectAutomationDeliveryView{
		ID:                  row.ID,
		WorkspaceID:         row.WorkspaceID,
		ProjectID:           row.ProjectID,
		RuleID:              row.RuleID,
		TriggerType:         row.TriggerType,
		EventID:             row.EventID,
		EventType:           row.EventType,
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
		NextAttemptAt:       row.NextAttemptAt,
		LastError:           row.LastError,
		CreatedAt:           row.CreatedAt,
		ModifiedAt:          row.ModifiedAt,
	}
}
