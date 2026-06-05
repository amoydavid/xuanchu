package app

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/dajee/taskg/internal/storage"
	"github.com/dajee/taskg/internal/task"
)

// HookScopeType 定义 hook 的作用域类型。
type HookScopeType string

const (
	HookScopeWorkspace HookScopeType = "workspace"
	HookScopeProject   HookScopeType = "project"
)

// 允许的 hook 事件类型。
var allowedHookEventTypes = map[string]bool{
	"task.created":     true,
	"task.modified":    true,
	"task.completed":   true,
	"task.deleted":     true,
	"project.archived": true,
}

// HookAddInput 创建 hook 的输入参数。
type HookAddInput struct {
	Name           string
	ScopeType      HookScopeType
	ProjectRef     string
	EventTypes     []string
	EndpointURL    string
	Secret         string
	TimeoutSeconds int
	MaxAttempts    int
}

// HookModifyInput 修改 hook 的输入参数，nil 字段表示不修改。
type HookModifyInput struct {
	Name           *string
	EventTypes     *[]string
	EndpointURL    *string
	Secret         *string
	TimeoutSeconds *int
	MaxAttempts    *int
}

// HookView 是 hook 的只读视图，不包含 secret。
type HookView struct {
	ID             string
	Name           string
	ScopeType      string
	WorkspaceID    string
	ProjectID      *string
	ActorUserID    string
	EventTypes     []string
	EndpointURL    string
	Enabled        bool
	TimeoutSeconds int
	MaxAttempts    int
	CreatedAt      int64
	ModifiedAt     int64
}

// HookDeliveryView 是 hook 投递记录的只读视图。
type HookDeliveryView struct {
	ID             string
	HookID         string
	EventID        string
	EventType      string
	WorkspaceID    string
	ProjectID      *string
	Actor          task.UserInfo
	Payload        map[string]any
	Headers        map[string]string
	Status         string
	AttemptCount   int
	NextAttemptAt  *int64
	ClaimExpiresAt *int64
	LastAttemptAt  *int64
	LastStatusCode *int
	LastError      string
	CreatedAt      int64
	ModifiedAt     int64
}

// AddHook 创建一个新的 webhook hook。
func (s *Service) AddHook(input HookAddInput) (HookView, error) {
	if err := s.Require(PermissionHookWrite); err != nil {
		return HookView{}, err
	}
	if err := validateHookName(input.Name); err != nil {
		return HookView{}, err
	}
	if err := validateHookEventTypes(input.EventTypes); err != nil {
		return HookView{}, err
	}
	if err := validateHookEndpointLength(input.EndpointURL); err != nil {
		return HookView{}, err
	}
	if err := validateHookSecret(input.Secret); err != nil {
		return HookView{}, err
	}
	if err := ValidateWebhookEndpointURLWithDefault(input.EndpointURL); err != nil {
		return HookView{}, err
	}
	timeout := input.TimeoutSeconds
	if timeout == 0 {
		timeout = 10
	}
	if err := validateTimeout(timeout); err != nil {
		return HookView{}, err
	}
	maxAttempts := input.MaxAttempts
	if maxAttempts == 0 {
		maxAttempts = 5
	}
	if err := validateMaxAttempts(maxAttempts); err != nil {
		return HookView{}, err
	}

	scopeType := string(input.ScopeType)
	if scopeType == "" {
		scopeType = string(HookScopeWorkspace)
	}
	if scopeType != string(HookScopeWorkspace) && scopeType != string(HookScopeProject) {
		return HookView{}, RuntimeError{Code: "hook_scope_invalid", Message: "hook scope must be workspace or project"}
	}
	if scopeType == string(HookScopeWorkspace) && s.hasProjectScope() {
		return HookView{}, RuntimeError{Code: "project_scope_denied", Message: "token cannot create workspace-scoped hook"}
	}

	var projectID *string
	if scopeType == string(HookScopeProject) {
		if input.ProjectRef == "" {
			return HookView{}, RuntimeError{Code: "hook_project_required", Message: "project ref is required for project-scoped hooks"}
		}
		project, err := s.ResolveProject(input.ProjectRef)
		if err != nil {
			return HookView{}, err
		}
		projectID = &project.ID
	}

	enabled := true
	eventTypesJSON, err := json.Marshal(input.EventTypes)
	if err != nil {
		return HookView{}, err
	}
	now := s.clock.Unix()

	secretFingerprint := secretFingerprint(input.Secret)

	row := storage.HookDefinition{
		ID:             uuid.NewString(),
		Name:           input.Name,
		ScopeType:      scopeType,
		WorkspaceID:    s.workspaceID,
		ProjectID:      projectID,
		ActorUserID:    s.runtime.ActorUserID,
		EventTypesJSON: string(eventTypesJSON),
		EndpointURL:    input.EndpointURL,
		Secret:         input.Secret,
		Enabled:        &enabled,
		TimeoutSeconds: timeout,
		MaxAttempts:    maxAttempts,
		CreatedAt:      now,
		ModifiedAt:     now,
	}

	var view HookView
	err = s.withAudit("hook.create", func(tx *Service) (AuditEntry, error) {
		if err := tx.hookRepo.Create(row); err != nil {
			return AuditEntry{}, err
		}
		created, err := tx.hookRepo.GetByID(row.ID)
		if err != nil {
			return AuditEntry{}, err
		}
		view = hookViewFromRow(created)
		return AuditEntry{
			TargetType: "hook",
			TargetID:   created.ID,
			Payload: map[string]any{
				"name":               created.Name,
				"scope_type":         created.ScopeType,
				"event_types":        input.EventTypes,
				"endpoint_url":       created.EndpointURL,
				"secret_fingerprint": secretFingerprint,
			},
		}, nil
	})
	return view, err
}

// ListHooks 列出当前 workspace 的所有 hook。
func (s *Service) ListHooks(projectRef string) ([]HookView, error) {
	if err := s.Require(PermissionHookRead); err != nil {
		return nil, err
	}
	var projectID *string
	if projectRef != "" {
		project, err := s.ResolveProject(projectRef)
		if err != nil {
			return nil, err
		}
		projectID = &project.ID
	}
	rows, err := s.hookRepo.List(s.workspaceID, projectID, true)
	if err != nil {
		return nil, err
	}
	views := make([]HookView, 0, len(rows))
	for _, row := range rows {
		if !s.allowsProjectID(row.ProjectID) {
			continue
		}
		views = append(views, hookViewFromRow(row))
	}
	return views, nil
}

// HookInfo 获取单个 hook 的详细信息。
func (s *Service) HookInfo(hookID string) (HookView, error) {
	if err := s.Require(PermissionHookRead); err != nil {
		return HookView{}, err
	}
	row, err := s.hookRepo.GetByID(hookID)
	if err == storage.ErrNotFound {
		return HookView{}, RuntimeError{Code: "hook_not_found", Message: "hook not found"}
	}
	if err != nil {
		return HookView{}, err
	}
	if row.WorkspaceID != s.workspaceID {
		return HookView{}, RuntimeError{Code: "hook_not_found", Message: "hook not found"}
	}
	if err := s.ensureReadableHookScope(row); err != nil {
		return HookView{}, err
	}
	return hookViewFromRow(row), nil
}

// ModifyHook 修改 hook 的属性。
func (s *Service) ModifyHook(hookID string, input HookModifyInput) (HookView, error) {
	if err := s.Require(PermissionHookWrite); err != nil {
		return HookView{}, err
	}
	row, err := s.hookRepo.GetByID(hookID)
	if err == storage.ErrNotFound {
		return HookView{}, RuntimeError{Code: "hook_not_found", Message: "hook not found"}
	}
	if err != nil {
		return HookView{}, err
	}
	if row.WorkspaceID != s.workspaceID {
		return HookView{}, RuntimeError{Code: "hook_not_found", Message: "hook not found"}
	}
	if err := s.ensureWritableHookScope(row); err != nil {
		return HookView{}, err
	}

	now := s.clock.Unix()
	row.ModifiedAt = now

	var auditPayload = map[string]any{
		"hook_id": hookID,
	}

	if input.Name != nil {
		if err := validateHookName(*input.Name); err != nil {
			return HookView{}, err
		}
		row.Name = *input.Name
		auditPayload["name"] = *input.Name
	}
	if input.EventTypes != nil {
		if err := validateHookEventTypes(*input.EventTypes); err != nil {
			return HookView{}, err
		}
		eventTypesJSON, err := json.Marshal(*input.EventTypes)
		if err != nil {
			return HookView{}, err
		}
		row.EventTypesJSON = string(eventTypesJSON)
		auditPayload["event_types"] = *input.EventTypes
	}
	if input.EndpointURL != nil {
		if err := validateHookEndpointLength(*input.EndpointURL); err != nil {
			return HookView{}, err
		}
		if err := ValidateWebhookEndpointURLWithDefault(*input.EndpointURL); err != nil {
			return HookView{}, err
		}
		row.EndpointURL = *input.EndpointURL
		auditPayload["endpoint_url"] = *input.EndpointURL
	}
	if input.Secret != nil {
		if err := validateHookSecret(*input.Secret); err != nil {
			return HookView{}, err
		}
		row.Secret = *input.Secret
		auditPayload["secret_fingerprint"] = secretFingerprint(*input.Secret)
	}
	if input.TimeoutSeconds != nil {
		if err := validateTimeout(*input.TimeoutSeconds); err != nil {
			return HookView{}, err
		}
		row.TimeoutSeconds = *input.TimeoutSeconds
		auditPayload["timeout_seconds"] = *input.TimeoutSeconds
	}
	if input.MaxAttempts != nil {
		if err := validateMaxAttempts(*input.MaxAttempts); err != nil {
			return HookView{}, err
		}
		row.MaxAttempts = *input.MaxAttempts
		auditPayload["max_attempts"] = *input.MaxAttempts
	}

	var view HookView
	err = s.withAudit("hook.modify", func(tx *Service) (AuditEntry, error) {
		if err := tx.hookRepo.Update(row); err != nil {
			return AuditEntry{}, err
		}
		updated, err := tx.hookRepo.GetByID(hookID)
		if err != nil {
			return AuditEntry{}, err
		}
		view = hookViewFromRow(updated)
		return AuditEntry{
			TargetType: "hook",
			TargetID:   hookID,
			Payload:    auditPayload,
		}, nil
	})
	return view, err
}

// EnableHook 启用 hook。
func (s *Service) EnableHook(hookID string) (HookView, error) {
	if err := s.Require(PermissionHookWrite); err != nil {
		return HookView{}, err
	}
	return s.toggleHook(hookID, true, "hook.enable")
}

// DisableHook 禁用 hook。
func (s *Service) DisableHook(hookID string) (HookView, error) {
	if err := s.Require(PermissionHookWrite); err != nil {
		return HookView{}, err
	}
	return s.toggleHook(hookID, false, "hook.disable")
}

func (s *Service) toggleHook(hookID string, enabled bool, action string) (HookView, error) {
	row, err := s.hookRepo.GetByID(hookID)
	if err == storage.ErrNotFound {
		return HookView{}, RuntimeError{Code: "hook_not_found", Message: "hook not found"}
	}
	if err != nil {
		return HookView{}, err
	}
	if row.WorkspaceID != s.workspaceID {
		return HookView{}, RuntimeError{Code: "hook_not_found", Message: "hook not found"}
	}
	if err := s.ensureWritableHookScope(row); err != nil {
		return HookView{}, err
	}
	row.Enabled = &enabled
	row.ModifiedAt = s.clock.Unix()

	var view HookView
	err = s.withAudit(action, func(tx *Service) (AuditEntry, error) {
		if err := tx.hookRepo.Update(row); err != nil {
			return AuditEntry{}, err
		}
		updated, err := tx.hookRepo.GetByID(hookID)
		if err != nil {
			return AuditEntry{}, err
		}
		view = hookViewFromRow(updated)
		return AuditEntry{
			TargetType: "hook",
			TargetID:   hookID,
			Payload:    map[string]any{"enabled": enabled},
		}, nil
	})
	return view, err
}

// DeleteHook 删除 hook。
func (s *Service) DeleteHook(hookID string) error {
	if err := s.Require(PermissionHookWrite); err != nil {
		return err
	}
	row, err := s.hookRepo.GetByID(hookID)
	if err == storage.ErrNotFound {
		return RuntimeError{Code: "hook_not_found", Message: "hook not found"}
	}
	if err != nil {
		return err
	}
	if row.WorkspaceID != s.workspaceID {
		return RuntimeError{Code: "hook_not_found", Message: "hook not found"}
	}
	if err := s.ensureWritableHookScope(row); err != nil {
		return err
	}
	return s.withAudit("hook.delete", func(tx *Service) (AuditEntry, error) {
		if err := tx.hookRepo.Delete(hookID); err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{
			TargetType: "hook",
			TargetID:   hookID,
			Payload: map[string]any{
				"name": row.Name,
			},
		}, nil
	})
}

// ListHookDeliveries 列出指定 hook 的投递记录。
func (s *Service) ListHookDeliveries(hookID string, status string, limit int, offset int) ([]HookDeliveryView, error) {
	if err := s.Require(PermissionHookRead); err != nil {
		return nil, err
	}
	hook, err := s.hookRepo.GetByID(hookID)
	if err == storage.ErrNotFound {
		return nil, RuntimeError{Code: "hook_not_found", Message: "hook not found"}
	}
	if err != nil {
		return nil, err
	}
	if hook.WorkspaceID != s.workspaceID {
		return nil, RuntimeError{Code: "hook_not_found", Message: "hook not found"}
	}
	if err := s.ensureReadableHookScope(hook); err != nil {
		return nil, err
	}
	rows, err := s.hookDeliveryRepo.ListByHook(hookID, status, limit, offset)
	if err != nil {
		return nil, err
	}
	views := make([]HookDeliveryView, 0, len(rows))
	for _, row := range rows {
		view, err := hookDeliveryViewFromRowChecked(row)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

// HookDeliveryInfo 获取单条投递记录的详细信息。
func (s *Service) HookDeliveryInfo(deliveryID string) (HookDeliveryView, error) {
	if err := s.Require(PermissionHookRead); err != nil {
		return HookDeliveryView{}, err
	}
	row, err := s.hookDeliveryRepo.GetByID(deliveryID)
	if err == storage.ErrNotFound {
		return HookDeliveryView{}, RuntimeError{Code: "hook_delivery_not_found", Message: "delivery not found"}
	}
	if err != nil {
		return HookDeliveryView{}, err
	}
	if row.WorkspaceID != s.workspaceID {
		return HookDeliveryView{}, RuntimeError{Code: "hook_delivery_not_found", Message: "delivery not found"}
	}
	if err := s.ensureReadableDeliveryScope(row); err != nil {
		return HookDeliveryView{}, err
	}
	return hookDeliveryViewFromRowChecked(row)
}

// ReplayHookDelivery 重试 dead-lettered 或 disabled_skipped 的投递。
func (s *Service) ReplayHookDelivery(deliveryID string) (HookDeliveryView, error) {
	if err := s.Require(PermissionHookWrite); err != nil {
		return HookDeliveryView{}, err
	}
	row, err := s.hookDeliveryRepo.GetByID(deliveryID)
	if err == storage.ErrNotFound {
		return HookDeliveryView{}, RuntimeError{Code: "hook_delivery_not_found", Message: "delivery not found"}
	}
	if err != nil {
		return HookDeliveryView{}, err
	}
	if row.WorkspaceID != s.workspaceID {
		return HookDeliveryView{}, RuntimeError{Code: "hook_delivery_not_found", Message: "delivery not found"}
	}
	if err := s.ensureReadableDeliveryScope(row); err != nil {
		return HookDeliveryView{}, err
	}
	if row.Status != storage.DeliveryStatusDeadLettered && row.Status != storage.DeliveryStatusDisabledSkipped {
		return HookDeliveryView{}, RuntimeError{Code: "hook_delivery_not_replayable", Message: "delivery is not in a replayable state"}
	}
	var view HookDeliveryView
	err = s.withAudit("hook.replay", func(tx *Service) (AuditEntry, error) {
		if err := tx.hookDeliveryRepo.Requeue(deliveryID, tx.clock.Unix()); err != nil {
			return AuditEntry{}, err
		}
		updated, err := tx.hookDeliveryRepo.GetByID(deliveryID)
		if err != nil {
			return AuditEntry{}, err
		}
		view, err = hookDeliveryViewFromRowChecked(updated)
		if err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{
			TargetType: "hook_delivery",
			TargetID:   deliveryID,
			Payload:    map[string]any{"hook_id": row.HookID, "event_type": row.EventType},
		}, nil
	})
	return view, err
}

func (s *Service) ensureReadableHookScope(row storage.HookDefinition) error {
	if s.allowsProjectID(row.ProjectID) {
		return nil
	}
	return RuntimeError{Code: "hook_not_found", Message: "hook not found"}
}

func (s *Service) ensureWritableHookScope(row storage.HookDefinition) error {
	if s.allowsProjectID(row.ProjectID) {
		return nil
	}
	return RuntimeError{Code: "project_scope_denied", Message: "token cannot access project"}
}

func (s *Service) ensureReadableDeliveryScope(row storage.HookDelivery) error {
	if s.allowsProjectID(row.ProjectID) {
		return nil
	}
	return RuntimeError{Code: "hook_delivery_not_found", Message: "delivery not found"}
}

// hookDeliveryViewFromRow 将 GORM 模型转换为只读视图。
func hookDeliveryViewFromRow(row storage.HookDelivery) HookDeliveryView {
	view, _ := hookDeliveryViewFromRowChecked(row)
	return view
}

func hookDeliveryViewFromRowChecked(row storage.HookDelivery) (HookDeliveryView, error) {
	var payload map[string]any
	if row.PayloadJSON != "" {
		if err := json.Unmarshal([]byte(row.PayloadJSON), &payload); err != nil {
			return HookDeliveryView{}, fmt.Errorf("parse hook delivery payload: %w", err)
		}
	}
	var headers map[string]string
	if row.HeadersJSON != "" {
		if err := json.Unmarshal([]byte(row.HeadersJSON), &headers); err != nil {
			return HookDeliveryView{}, fmt.Errorf("parse hook delivery headers: %w", err)
		}
	}
	return HookDeliveryView{
		ID:             row.ID,
		HookID:         row.HookID,
		EventID:        row.EventID,
		EventType:      row.EventType,
		WorkspaceID:    row.WorkspaceID,
		ProjectID:      row.ProjectID,
		Actor:          task.UserInfo{ID: row.ActorUserID},
		Payload:        payload,
		Headers:        headers,
		Status:         row.Status,
		AttemptCount:   row.AttemptCount,
		NextAttemptAt:  row.NextAttemptAt,
		ClaimExpiresAt: row.ClaimExpiresAt,
		LastAttemptAt:  row.LastAttemptAt,
		LastStatusCode: row.LastStatusCode,
		LastError:      row.LastError,
		CreatedAt:      row.CreatedAt,
		ModifiedAt:     row.ModifiedAt,
	}, nil
}

// hookViewFromRow 将 GORM 模型转换为只读视图（不含 secret）。
func hookViewFromRow(row storage.HookDefinition) HookView {
	var eventTypes []string
	if row.EventTypesJSON != "" {
		_ = json.Unmarshal([]byte(row.EventTypesJSON), &eventTypes)
	}
	enabled := row.Enabled != nil && *row.Enabled
	return HookView{
		ID:             row.ID,
		Name:           row.Name,
		ScopeType:      row.ScopeType,
		WorkspaceID:    row.WorkspaceID,
		ProjectID:      row.ProjectID,
		ActorUserID:    row.ActorUserID,
		EventTypes:     eventTypes,
		EndpointURL:    row.EndpointURL,
		Enabled:        enabled,
		TimeoutSeconds: row.TimeoutSeconds,
		MaxAttempts:    row.MaxAttempts,
		CreatedAt:      row.CreatedAt,
		ModifiedAt:     row.ModifiedAt,
	}
}

// secretFingerprint 计算 secret 的指纹：SHA256 的前 8 个十六进制字符。
func secretFingerprint(secret string) string {
	if secret == "" {
		return ""
	}
	h := sha256.Sum256([]byte(secret))
	return fmt.Sprintf("%x", h[:])[:8]
}

func validateHookName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return RuntimeError{Code: "hook_name_invalid", Message: "hook name is required"}
	}
	if len(name) > 200 {
		return RuntimeError{Code: "hook_name_invalid", Message: "hook name must be at most 200 characters"}
	}
	return nil
}

func validateHookEndpointLength(endpoint string) error {
	if len(endpoint) > 2048 {
		return RuntimeError{Code: "hook_endpoint_invalid", Message: "endpoint URL must be at most 2048 characters"}
	}
	return nil
}

func validateHookSecret(secret string) error {
	if len(secret) > 4096 {
		return RuntimeError{Code: "hook_secret_invalid", Message: "hook secret must be at most 4096 characters"}
	}
	return nil
}

func validateHookEventTypes(types []string) error {
	if len(types) == 0 {
		return RuntimeError{Code: "hook_event_types_invalid", Message: "at least one event type is required"}
	}
	for _, t := range types {
		if !allowedHookEventTypes[t] {
			return RuntimeError{Code: "hook_event_types_invalid", Message: fmt.Sprintf("event type %q is not allowed", t)}
		}
	}
	return nil
}

func validateTimeout(timeout int) error {
	if timeout < 1 || timeout > 120 {
		return RuntimeError{Code: "hook_timeout_invalid", Message: "timeout_seconds must be between 1 and 120"}
	}
	return nil
}

func validateMaxAttempts(max int) error {
	if max < 1 || max > 20 {
		return RuntimeError{Code: "hook_max_attempts_invalid", Message: "max_attempts must be between 1 and 20"}
	}
	return nil
}
