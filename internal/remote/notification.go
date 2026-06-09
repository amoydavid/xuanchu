package remote

import (
	"context"
	"net/url"
	"strconv"

	"git.dajee.net/dajee/xuanchu/internal/app"
)

type HTTPHeaderTemplateRequest struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type HTTPTemplateSecretRefRequest struct {
	Alias     string `json:"alias"`
	ConfigKey string `json:"config_key"`
}

type NotificationSinkCreateRequest struct {
	Name            string                         `json:"name"`
	Type            string                         `json:"type"`
	EndpointMode    string                         `json:"endpoint_mode"`
	URL             string                         `json:"url,omitempty"`
	URLTemplate     string                         `json:"url_template,omitempty"`
	ConfigKey       string                         `json:"config_key,omitempty"`
	AllowedHosts    []string                       `json:"allowed_hosts,omitempty"`
	HTTPMethod      string                         `json:"http_method,omitempty"`
	HeaderTemplates []HTTPHeaderTemplateRequest    `json:"header_templates,omitempty"`
	BodyTemplate    string                         `json:"body_template,omitempty"`
	BodyContentType string                         `json:"body_content_type,omitempty"`
	SecretRefs      []HTTPTemplateSecretRefRequest `json:"secret_refs,omitempty"`
	Secret          string                         `json:"secret,omitempty"`
	TimeoutSeconds  int                            `json:"timeout_seconds,omitempty"`
	MaxAttempts     int                            `json:"max_attempts,omitempty"`
}

type NotificationSinkModifyRequest struct {
	Name            *string                         `json:"name,omitempty"`
	Type            *string                         `json:"type,omitempty"`
	EndpointMode    *string                         `json:"endpoint_mode,omitempty"`
	URL             *string                         `json:"url,omitempty"`
	URLTemplate     *string                         `json:"url_template,omitempty"`
	ConfigKey       *string                         `json:"config_key,omitempty"`
	AllowedHosts    *[]string                       `json:"allowed_hosts,omitempty"`
	HTTPMethod      *string                         `json:"http_method,omitempty"`
	HeaderTemplates *[]HTTPHeaderTemplateRequest    `json:"header_templates,omitempty"`
	BodyTemplate    *string                         `json:"body_template,omitempty"`
	BodyContentType *string                         `json:"body_content_type,omitempty"`
	SecretRefs      *[]HTTPTemplateSecretRefRequest `json:"secret_refs,omitempty"`
	Secret          *string                         `json:"secret,omitempty"`
	TimeoutSeconds  *int                            `json:"timeout_seconds,omitempty"`
	MaxAttempts     *int                            `json:"max_attempts,omitempty"`
	Enabled         *bool                           `json:"enabled,omitempty"`
}

type ReminderRuleCreateRequest struct {
	Name          string   `json:"name"`
	ProjectRef    string   `json:"project_ref,omitempty"`
	TriggerType   string   `json:"trigger_type"`
	OffsetSeconds int64    `json:"offset_seconds,omitempty"`
	AfterSeconds  int64    `json:"after_seconds,omitempty"`
	RepeatPolicy  string   `json:"repeat_policy,omitempty"`
	TaskFilter    string   `json:"task_filter,omitempty"`
	AudienceType  string   `json:"audience_type"`
	Recipients    []string `json:"recipients,omitempty"`
	SinkRef       string   `json:"sink_ref"`
}

type ReminderRuleModifyRequest struct {
	Name          *string   `json:"name,omitempty"`
	ProjectRef    *string   `json:"project_ref,omitempty"`
	TriggerType   *string   `json:"trigger_type,omitempty"`
	OffsetSeconds *int64    `json:"offset_seconds,omitempty"`
	AfterSeconds  *int64    `json:"after_seconds,omitempty"`
	RepeatPolicy  *string   `json:"repeat_policy,omitempty"`
	TaskFilter    *string   `json:"task_filter,omitempty"`
	AudienceType  *string   `json:"audience_type,omitempty"`
	Recipients    *[]string `json:"recipients,omitempty"`
	SinkRef       *string   `json:"sink_ref,omitempty"`
	Enabled       *bool     `json:"enabled,omitempty"`
}

func (c *Client) ListNotificationSinks(ctx context.Context, workspace string, includeDisabled bool) ([]app.NotificationSinkView, error) {
	values := url.Values{}
	if workspace != "" {
		values.Set("workspace", workspace)
	}
	if includeDisabled {
		values.Set("all", "true")
	}
	var envelope apiEnvelope[[]app.NotificationSinkView]
	if err := c.get(ctx, "/api/v1/notification-sinks", values, &envelope); err != nil {
		return nil, err
	}
	return envelope.Data, nil
}

func (c *Client) AddNotificationSink(ctx context.Context, workspace string, input NotificationSinkCreateRequest) (app.NotificationSinkView, error) {
	path := "/api/v1/notification-sinks"
	if workspace != "" {
		path += "?workspace=" + url.QueryEscape(workspace)
	}
	var envelope apiEnvelope[app.NotificationSinkView]
	if err := c.post(ctx, path, input, &envelope); err != nil {
		return app.NotificationSinkView{}, err
	}
	return envelope.Data, nil
}

func (c *Client) NotificationSinkInfo(ctx context.Context, sinkID string) (app.NotificationSinkView, error) {
	var envelope apiEnvelope[app.NotificationSinkView]
	if err := c.get(ctx, "/api/v1/notification-sinks/"+url.PathEscape(sinkID), nil, &envelope); err != nil {
		return app.NotificationSinkView{}, err
	}
	return envelope.Data, nil
}

func (c *Client) ModifyNotificationSink(ctx context.Context, sinkID string, input NotificationSinkModifyRequest) (app.NotificationSinkView, error) {
	var envelope apiEnvelope[app.NotificationSinkView]
	if err := c.patch(ctx, "/api/v1/notification-sinks/"+url.PathEscape(sinkID), input, &envelope); err != nil {
		return app.NotificationSinkView{}, err
	}
	return envelope.Data, nil
}

func (c *Client) EnableNotificationSink(ctx context.Context, sinkID string) (app.NotificationSinkView, error) {
	var envelope apiEnvelope[app.NotificationSinkView]
	if err := c.post(ctx, "/api/v1/notification-sinks/"+url.PathEscape(sinkID)+"/enable", nil, &envelope); err != nil {
		return app.NotificationSinkView{}, err
	}
	return envelope.Data, nil
}

func (c *Client) DisableNotificationSink(ctx context.Context, sinkID string) (app.NotificationSinkView, error) {
	var envelope apiEnvelope[app.NotificationSinkView]
	if err := c.post(ctx, "/api/v1/notification-sinks/"+url.PathEscape(sinkID)+"/disable", nil, &envelope); err != nil {
		return app.NotificationSinkView{}, err
	}
	return envelope.Data, nil
}

func (c *Client) DeleteNotificationSink(ctx context.Context, sinkID string) error {
	return c.delete(ctx, "/api/v1/notification-sinks/"+url.PathEscape(sinkID), nil)
}

func (c *Client) ListReminderRules(ctx context.Context, workspace, projectRef string, includeDisabled bool) ([]app.ReminderRuleView, error) {
	values := url.Values{}
	if workspace != "" {
		values.Set("workspace", workspace)
	}
	if projectRef != "" {
		values.Set("project", projectRef)
	}
	if includeDisabled {
		values.Set("all", "true")
	}
	var envelope apiEnvelope[[]app.ReminderRuleView]
	if err := c.get(ctx, "/api/v1/reminder-rules", values, &envelope); err != nil {
		return nil, err
	}
	return envelope.Data, nil
}

func (c *Client) AddReminderRule(ctx context.Context, workspace string, input ReminderRuleCreateRequest) (app.ReminderRuleView, error) {
	path := "/api/v1/reminder-rules"
	if workspace != "" {
		path += "?workspace=" + url.QueryEscape(workspace)
	}
	var envelope apiEnvelope[app.ReminderRuleView]
	if err := c.post(ctx, path, input, &envelope); err != nil {
		return app.ReminderRuleView{}, err
	}
	return envelope.Data, nil
}

func (c *Client) ReminderRuleInfo(ctx context.Context, ruleID string) (app.ReminderRuleView, error) {
	var envelope apiEnvelope[app.ReminderRuleView]
	if err := c.get(ctx, "/api/v1/reminder-rules/"+url.PathEscape(ruleID), nil, &envelope); err != nil {
		return app.ReminderRuleView{}, err
	}
	return envelope.Data, nil
}

func (c *Client) ModifyReminderRule(ctx context.Context, ruleID string, input ReminderRuleModifyRequest) (app.ReminderRuleView, error) {
	var envelope apiEnvelope[app.ReminderRuleView]
	if err := c.patch(ctx, "/api/v1/reminder-rules/"+url.PathEscape(ruleID), input, &envelope); err != nil {
		return app.ReminderRuleView{}, err
	}
	return envelope.Data, nil
}

func (c *Client) EnableReminderRule(ctx context.Context, ruleID string) (app.ReminderRuleView, error) {
	var envelope apiEnvelope[app.ReminderRuleView]
	if err := c.post(ctx, "/api/v1/reminder-rules/"+url.PathEscape(ruleID)+"/enable", nil, &envelope); err != nil {
		return app.ReminderRuleView{}, err
	}
	return envelope.Data, nil
}

func (c *Client) DisableReminderRule(ctx context.Context, ruleID string) (app.ReminderRuleView, error) {
	var envelope apiEnvelope[app.ReminderRuleView]
	if err := c.post(ctx, "/api/v1/reminder-rules/"+url.PathEscape(ruleID)+"/disable", nil, &envelope); err != nil {
		return app.ReminderRuleView{}, err
	}
	return envelope.Data, nil
}

func (c *Client) DeleteReminderRule(ctx context.Context, ruleID string) error {
	return c.delete(ctx, "/api/v1/reminder-rules/"+url.PathEscape(ruleID), nil)
}

func (c *Client) ListNotificationDeliveries(ctx context.Context, workspace, status string, limit int, offset int) ([]app.NotificationDeliveryView, error) {
	values := url.Values{}
	if workspace != "" {
		values.Set("workspace", workspace)
	}
	if status != "" {
		values.Set("status", status)
	}
	if limit > 0 {
		values.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		values.Set("offset", strconv.Itoa(offset))
	}
	var envelope apiEnvelope[[]app.NotificationDeliveryView]
	if err := c.get(ctx, "/api/v1/notification-deliveries", values, &envelope); err != nil {
		return nil, err
	}
	return envelope.Data, nil
}

func (c *Client) NotificationDeliveryInfo(ctx context.Context, deliveryID string) (app.NotificationDeliveryView, error) {
	var envelope apiEnvelope[app.NotificationDeliveryView]
	if err := c.get(ctx, "/api/v1/notification-deliveries/"+url.PathEscape(deliveryID), nil, &envelope); err != nil {
		return app.NotificationDeliveryView{}, err
	}
	return envelope.Data, nil
}

func (c *Client) ReplayNotificationDelivery(ctx context.Context, deliveryID string) (app.NotificationDeliveryView, error) {
	var envelope apiEnvelope[app.NotificationDeliveryView]
	if err := c.post(ctx, "/api/v1/notification-deliveries/"+url.PathEscape(deliveryID)+"/replay", nil, &envelope); err != nil {
		return app.NotificationDeliveryView{}, err
	}
	return envelope.Data, nil
}
