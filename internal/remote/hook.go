package remote

import (
	"context"
	"net/url"
	"strconv"

	"github.com/dajee/taskg/internal/app"
)

// hookDTO 对应 HTTP API 返回的 hook JSON 结构。
type hookDTO struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	ScopeType      string   `json:"scope_type"`
	WorkspaceID    string   `json:"workspace_id"`
	ProjectID      *string  `json:"project_id,omitempty"`
	EventTypes     []string `json:"event_types"`
	EndpointURL    string   `json:"endpoint_url"`
	Enabled        bool     `json:"enabled"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	MaxAttempts    int      `json:"max_attempts"`
	CreatedAt      int64    `json:"created_at"`
	ModifiedAt     int64    `json:"modified_at"`
}

// HookCreateRequest 对应创建 hook 的请求体。
type HookCreateRequest struct {
	Name           string   `json:"name"`
	ScopeType      string   `json:"scope_type"`
	ProjectRef     string   `json:"project_ref,omitempty"`
	EventTypes     []string `json:"event_types"`
	EndpointURL    string   `json:"endpoint_url"`
	Secret         string   `json:"secret,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`
	MaxAttempts    int      `json:"max_attempts,omitempty"`
}

// HookModifyRequest 对应修改 hook 的请求体。
type HookModifyRequest struct {
	Name           *string   `json:"name,omitempty"`
	EventTypes     *[]string `json:"event_types,omitempty"`
	EndpointURL    *string   `json:"endpoint_url,omitempty"`
	Secret         *string   `json:"secret,omitempty"`
	TimeoutSeconds *int      `json:"timeout_seconds,omitempty"`
	MaxAttempts    *int      `json:"max_attempts,omitempty"`
}

// deliveryDTO 对应 HTTP API 返回的 delivery JSON 结构。
type deliveryDTO struct {
	ID             string            `json:"id"`
	HookID         string            `json:"hook_id"`
	EventID        string            `json:"event_id"`
	EventType      string            `json:"event_type"`
	WorkspaceID    string            `json:"workspace_id"`
	ProjectID      *string           `json:"project_id,omitempty"`
	ActorUserID    string            `json:"actor_user_id"`
	Payload        map[string]any    `json:"payload"`
	Headers        map[string]string `json:"headers"`
	Status         string            `json:"status"`
	AttemptCount   int               `json:"attempt_count"`
	NextAttemptAt  *int64            `json:"next_attempt_at,omitempty"`
	ClaimExpiresAt *int64            `json:"claim_expires_at,omitempty"`
	LastAttemptAt  *int64            `json:"last_attempt_at,omitempty"`
	LastStatusCode *int              `json:"last_status_code,omitempty"`
	LastError      string            `json:"last_error,omitempty"`
	CreatedAt      int64             `json:"created_at"`
	ModifiedAt     int64             `json:"modified_at"`
}

// ListHooks 列出 workspace 的 hook。
func (c *Client) ListHooks(ctx context.Context, workspace string, projectRef string) ([]app.HookView, error) {
	values := url.Values{}
	if workspace != "" {
		values.Set("workspace", workspace)
	}
	if projectRef != "" {
		values.Set("project", projectRef)
	}
	var envelope apiEnvelope[[]hookDTO]
	if err := c.get(ctx, "/api/v1/hooks", values, &envelope); err != nil {
		return nil, err
	}
	out := make([]app.HookView, 0, len(envelope.Data))
	for _, row := range envelope.Data {
		out = append(out, hookDTOToView(row))
	}
	return out, nil
}

// AddHook 创建一个新的 webhook。
func (c *Client) AddHook(ctx context.Context, workspace string, input HookCreateRequest) (app.HookView, error) {
	path := "/api/v1/hooks"
	if workspace != "" {
		path += "?workspace=" + url.QueryEscape(workspace)
	}
	var envelope apiEnvelope[hookDTO]
	if err := c.post(ctx, path, input, &envelope); err != nil {
		return app.HookView{}, err
	}
	return hookDTOToView(envelope.Data), nil
}

// HookInfo 获取单个 hook 的详细信息。
func (c *Client) HookInfo(ctx context.Context, hookID string) (app.HookView, error) {
	var envelope apiEnvelope[hookDTO]
	if err := c.get(ctx, "/api/v1/hooks/"+url.PathEscape(hookID), nil, &envelope); err != nil {
		return app.HookView{}, err
	}
	return hookDTOToView(envelope.Data), nil
}

// ModifyHook 修改 hook 的属性。
func (c *Client) ModifyHook(ctx context.Context, hookID string, input HookModifyRequest) (app.HookView, error) {
	var envelope apiEnvelope[hookDTO]
	if err := c.patch(ctx, "/api/v1/hooks/"+url.PathEscape(hookID), input, &envelope); err != nil {
		return app.HookView{}, err
	}
	return hookDTOToView(envelope.Data), nil
}

// EnableHook 启用 hook。
func (c *Client) EnableHook(ctx context.Context, hookID string) (app.HookView, error) {
	var envelope apiEnvelope[hookDTO]
	if err := c.post(ctx, "/api/v1/hooks/"+url.PathEscape(hookID)+"/enable", nil, &envelope); err != nil {
		return app.HookView{}, err
	}
	return hookDTOToView(envelope.Data), nil
}

// DisableHook 禁用 hook。
func (c *Client) DisableHook(ctx context.Context, hookID string) (app.HookView, error) {
	var envelope apiEnvelope[hookDTO]
	if err := c.post(ctx, "/api/v1/hooks/"+url.PathEscape(hookID)+"/disable", nil, &envelope); err != nil {
		return app.HookView{}, err
	}
	return hookDTOToView(envelope.Data), nil
}

// DeleteHook 删除 hook。
func (c *Client) DeleteHook(ctx context.Context, hookID string) error {
	return c.delete(ctx, "/api/v1/hooks/"+url.PathEscape(hookID), nil)
}

// ListHookDeliveries 列出指定 hook 的投递记录。
func (c *Client) ListHookDeliveries(ctx context.Context, hookID, status string, limit int) ([]app.HookDeliveryView, error) {
	values := url.Values{}
	if status != "" {
		values.Set("status", status)
	}
	if limit > 0 {
		values.Set("limit", strconv.Itoa(limit))
	}
	var envelope apiEnvelope[[]deliveryDTO]
	if err := c.get(ctx, "/api/v1/hooks/"+url.PathEscape(hookID)+"/deliveries", values, &envelope); err != nil {
		return nil, err
	}
	out := make([]app.HookDeliveryView, 0, len(envelope.Data))
	for _, row := range envelope.Data {
		out = append(out, deliveryDTOToView(row))
	}
	return out, nil
}

// HookDeliveryInfo 获取单条投递记录的详细信息。
func (c *Client) HookDeliveryInfo(ctx context.Context, deliveryID string) (app.HookDeliveryView, error) {
	var envelope apiEnvelope[deliveryDTO]
	if err := c.get(ctx, "/api/v1/hook-deliveries/"+url.PathEscape(deliveryID), nil, &envelope); err != nil {
		return app.HookDeliveryView{}, err
	}
	return deliveryDTOToView(envelope.Data), nil
}

// ReplayHookDelivery 重试投递。
func (c *Client) ReplayHookDelivery(ctx context.Context, deliveryID string) (app.HookDeliveryView, error) {
	var envelope apiEnvelope[deliveryDTO]
	if err := c.post(ctx, "/api/v1/hook-deliveries/"+url.PathEscape(deliveryID)+"/replay", nil, &envelope); err != nil {
		return app.HookDeliveryView{}, err
	}
	return deliveryDTOToView(envelope.Data), nil
}

func hookDTOToView(row hookDTO) app.HookView {
	return app.HookView{
		ID:             row.ID,
		Name:           row.Name,
		ScopeType:      row.ScopeType,
		WorkspaceID:    row.WorkspaceID,
		ProjectID:      row.ProjectID,
		EventTypes:     row.EventTypes,
		EndpointURL:    row.EndpointURL,
		Enabled:        row.Enabled,
		TimeoutSeconds: row.TimeoutSeconds,
		MaxAttempts:    row.MaxAttempts,
		CreatedAt:      row.CreatedAt,
		ModifiedAt:     row.ModifiedAt,
	}
}

func deliveryDTOToView(row deliveryDTO) app.HookDeliveryView {
	return app.HookDeliveryView{
		ID:             row.ID,
		HookID:         row.HookID,
		EventID:        row.EventID,
		EventType:      row.EventType,
		WorkspaceID:    row.WorkspaceID,
		ProjectID:      row.ProjectID,
		ActorUserID:    row.ActorUserID,
		Payload:        row.Payload,
		Headers:        row.Headers,
		Status:         row.Status,
		AttemptCount:   row.AttemptCount,
		NextAttemptAt:  row.NextAttemptAt,
		ClaimExpiresAt: row.ClaimExpiresAt,
		LastAttemptAt:  row.LastAttemptAt,
		LastStatusCode: row.LastStatusCode,
		LastError:      row.LastError,
		CreatedAt:      row.CreatedAt,
		ModifiedAt:     row.ModifiedAt,
	}
}
