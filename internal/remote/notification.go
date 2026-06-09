package remote

import (
	"context"
	"net/url"
	"strconv"

	"git.dajee.net/dajee/xuanchu/internal/app"
)

type NotificationSinkRequest struct {
	Name            string                           `json:"name"`
	Type            string                           `json:"type"`
	EndpointMode    string                           `json:"endpoint_mode"`
	URL             string                           `json:"url"`
	URLTemplate     string                           `json:"url_template"`
	ConfigKey       string                           `json:"config_key"`
	AllowedHosts    []string                         `json:"allowed_hosts"`
	HeaderTemplates []app.HTTPHeaderTemplateInput    `json:"header_templates"`
	BodyTemplate    string                           `json:"body_template"`
	BodyContentType string                           `json:"body_content_type"`
	SecretRefs      []app.HTTPTemplateSecretRefInput `json:"secret_refs"`
	Secret          string                           `json:"secret,omitempty"`
	TimeoutSeconds  int                              `json:"timeout_seconds,omitempty"`
	MaxAttempts     int                              `json:"max_attempts,omitempty"`
}

type NotificationSinkModifyRequest struct {
	Name            *string                           `json:"name,omitempty"`
	Type            *string                           `json:"type,omitempty"`
	EndpointMode    *string                           `json:"endpoint_mode,omitempty"`
	URL             *string                           `json:"url,omitempty"`
	URLTemplate     *string                           `json:"url_template,omitempty"`
	ConfigKey       *string                           `json:"config_key,omitempty"`
	AllowedHosts    *[]string                         `json:"allowed_hosts,omitempty"`
	HeaderTemplates *[]app.HTTPHeaderTemplateInput    `json:"header_templates,omitempty"`
	BodyTemplate    *string                           `json:"body_template,omitempty"`
	BodyContentType *string                           `json:"body_content_type,omitempty"`
	SecretRefs      *[]app.HTTPTemplateSecretRefInput `json:"secret_refs,omitempty"`
	Secret          *string                           `json:"secret,omitempty"`
	TimeoutSeconds  *int                              `json:"timeout_seconds,omitempty"`
	MaxAttempts     *int                              `json:"max_attempts,omitempty"`
}

type ReminderRuleRequest struct {
	Name          string   `json:"name"`
	ProjectRef    string   `json:"project_ref"`
	TriggerType   string   `json:"trigger_type"`
	OffsetSeconds int64    `json:"offset_seconds"`
	AfterSeconds  int64    `json:"after_seconds"`
	RepeatPolicy  string   `json:"repeat_policy"`
	ScheduleType  string   `json:"schedule_type"`
	ScheduleValue string   `json:"schedule_value"`
	FilterSource  string   `json:"filter_source"`
	AudienceType  string   `json:"audience_type"`
	Recipients    []string `json:"recipients"`
	SinkRef       string   `json:"sink_ref"`
}

type ReminderRuleModifyRequest struct {
	Name          *string   `json:"name,omitempty"`
	ProjectRef    *string   `json:"project_ref,omitempty"`
	TriggerType   *string   `json:"trigger_type,omitempty"`
	OffsetSeconds *int64    `json:"offset_seconds,omitempty"`
	AfterSeconds  *int64    `json:"after_seconds,omitempty"`
	RepeatPolicy  *string   `json:"repeat_policy,omitempty"`
	ScheduleType  *string   `json:"schedule_type,omitempty"`
	ScheduleValue *string   `json:"schedule_value,omitempty"`
	FilterSource  *string   `json:"filter_source,omitempty"`
	AudienceType  *string   `json:"audience_type,omitempty"`
	Recipients    *[]string `json:"recipients,omitempty"`
	SinkRef       *string   `json:"sink_ref,omitempty"`
}

type EventNotificationRuleRequest struct {
	Name            string   `json:"name"`
	ProjectRef      string   `json:"project_ref"`
	EventType       string   `json:"event_type"`
	FilterSource    string   `json:"filter_source"`
	AudienceType    string   `json:"audience_type"`
	Recipients      []string `json:"recipients"`
	Sink            string   `json:"sink"`
	TemplateSubject string   `json:"template_subject"`
	TemplateBody    string   `json:"template_body"`
}

type EventNotificationRuleModifyRequest struct {
	Name            *string   `json:"name,omitempty"`
	ProjectRef      *string   `json:"project_ref,omitempty"`
	EventType       *string   `json:"event_type,omitempty"`
	FilterSource    *string   `json:"filter_source,omitempty"`
	AudienceType    *string   `json:"audience_type,omitempty"`
	Recipients      *[]string `json:"recipients,omitempty"`
	Sink            *string   `json:"sink,omitempty"`
	TemplateSubject *string   `json:"template_subject,omitempty"`
	TemplateBody    *string   `json:"template_body,omitempty"`
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

func (c *Client) AddNotificationSink(ctx context.Context, workspace string, input NotificationSinkRequest) (app.NotificationSinkView, error) {
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

func (c *Client) ListReminderRules(ctx context.Context, workspace string, projectRef string, includeDisabled bool) ([]app.ReminderRuleView, error) {
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

func (c *Client) AddReminderRule(ctx context.Context, workspace string, input ReminderRuleRequest) (app.ReminderRuleView, error) {
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

func (c *Client) ListEventNotificationRules(ctx context.Context, workspace string, projectRef string, includeDisabled bool) ([]app.EventNotificationRuleView, error) {
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
	var envelope apiEnvelope[[]app.EventNotificationRuleView]
	if err := c.get(ctx, "/api/v1/notification-rules", values, &envelope); err != nil {
		return nil, err
	}
	return envelope.Data, nil
}

func (c *Client) AddEventNotificationRule(ctx context.Context, workspace string, input EventNotificationRuleRequest) (app.EventNotificationRuleView, error) {
	path := "/api/v1/notification-rules"
	if workspace != "" {
		path += "?workspace=" + url.QueryEscape(workspace)
	}
	var envelope apiEnvelope[app.EventNotificationRuleView]
	if err := c.post(ctx, path, input, &envelope); err != nil {
		return app.EventNotificationRuleView{}, err
	}
	return envelope.Data, nil
}

func (c *Client) EventNotificationRuleInfo(ctx context.Context, ruleID string) (app.EventNotificationRuleView, error) {
	var envelope apiEnvelope[app.EventNotificationRuleView]
	if err := c.get(ctx, "/api/v1/notification-rules/"+url.PathEscape(ruleID), nil, &envelope); err != nil {
		return app.EventNotificationRuleView{}, err
	}
	return envelope.Data, nil
}

func (c *Client) ModifyEventNotificationRule(ctx context.Context, ruleID string, input EventNotificationRuleModifyRequest) (app.EventNotificationRuleView, error) {
	var envelope apiEnvelope[app.EventNotificationRuleView]
	if err := c.patch(ctx, "/api/v1/notification-rules/"+url.PathEscape(ruleID), input, &envelope); err != nil {
		return app.EventNotificationRuleView{}, err
	}
	return envelope.Data, nil
}

func (c *Client) EnableEventNotificationRule(ctx context.Context, ruleID string) (app.EventNotificationRuleView, error) {
	var envelope apiEnvelope[app.EventNotificationRuleView]
	if err := c.post(ctx, "/api/v1/notification-rules/"+url.PathEscape(ruleID)+"/enable", nil, &envelope); err != nil {
		return app.EventNotificationRuleView{}, err
	}
	return envelope.Data, nil
}

func (c *Client) DisableEventNotificationRule(ctx context.Context, ruleID string) (app.EventNotificationRuleView, error) {
	var envelope apiEnvelope[app.EventNotificationRuleView]
	if err := c.post(ctx, "/api/v1/notification-rules/"+url.PathEscape(ruleID)+"/disable", nil, &envelope); err != nil {
		return app.EventNotificationRuleView{}, err
	}
	return envelope.Data, nil
}

func (c *Client) DeleteEventNotificationRule(ctx context.Context, ruleID string) error {
	return c.delete(ctx, "/api/v1/notification-rules/"+url.PathEscape(ruleID), nil)
}

func (c *Client) ListNotificationDeliveries(ctx context.Context, workspace string, sinkID string, status string, limit int, offset int) ([]app.NotificationDeliveryView, error) {
	values := url.Values{}
	if workspace != "" {
		values.Set("workspace", workspace)
	}
	if sinkID != "" {
		values.Set("sink", sinkID)
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
