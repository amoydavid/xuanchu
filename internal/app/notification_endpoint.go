package app

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/task"
)

type NotificationWorkspaceContext struct {
	ID   string
	Slug string
	Name string
}

type NotificationProjectContext struct {
	ID   string
	Slug string
	Name string
}

type NotificationRuleContext struct {
	ID          string
	Name        string
	TriggerType string
}

type NotificationTaskContext struct {
	UUID        string
	TaskSlug    string
	Description string
	Status      string
	Due         *int64
}

type NotificationReminderContext struct {
	Sequence        int64
	OverdueSequence int64
	WindowStart     int64
	WindowEnd       int64
}

type NotificationEventContext struct {
	ID         string
	Type       string
	Version    int
	OccurredAt int64
	ObjectKind string
	ObjectID   string
	JSON       string
}

type NotificationRequestResolveInput struct {
	Sink         NotificationSinkView
	Workspace    NotificationWorkspaceContext
	Project      *NotificationProjectContext
	Rule         NotificationRuleContext
	Task         NotificationTaskContext
	Recipient    task.UserInfo
	Actor        task.UserInfo
	Reminder     NotificationReminderContext
	Event        NotificationEventContext
	EventType    string
	SecretValues map[string]string
	ConfigValues map[string]string
}

type NotificationResolvedRequest struct {
	ResolvedURL                 string
	ResolvedEndpointSource      string
	ResolvedEndpointFingerprint string
	RenderedMethod              string
	RenderedHeadersJSON         string
	RenderedBody                string
	RenderedContentType         string
	PayloadJSON                 string
}

var templateVarPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.-]+)\s*\}\}`)

func ResolveNotificationRequest(input NotificationRequestResolveInput) (NotificationResolvedRequest, error) {
	resolvedURL, source, err := resolveNotificationEndpoint(input)
	if err != nil {
		return NotificationResolvedRequest{}, err
	}
	payloadJSON, err := buildNotificationPayloadJSON(input)
	if err != nil {
		return NotificationResolvedRequest{}, err
	}
	out := NotificationResolvedRequest{
		ResolvedURL:                 resolvedURL,
		ResolvedEndpointSource:      source,
		ResolvedEndpointFingerprint: secretFingerprint(resolvedURL),
		RenderedMethod:              http.MethodPost,
		RenderedContentType:         "application/json",
		PayloadJSON:                 payloadJSON,
	}
	switch input.Sink.Type {
	case NotificationSinkTypeHTTPTemplate:
		out.RenderedMethod = strings.TrimSpace(input.Sink.HTTPMethod)
		if out.RenderedMethod == "" {
			out.RenderedMethod = http.MethodPost
		}
		headers := http.Header{}
		for _, header := range input.Sink.HeaderTemplates {
			value, err := renderNotificationTemplate(header.Value, input, true)
			if err != nil {
				return NotificationResolvedRequest{}, err
			}
			headers.Add(header.Name, value)
		}
		if input.Sink.BodyContentType != "" {
			out.RenderedContentType = input.Sink.BodyContentType
			if headers.Get("Content-Type") == "" {
				headers.Set("Content-Type", input.Sink.BodyContentType)
			}
		}
		body, err := renderNotificationTemplate(input.Sink.BodyTemplate, input, true)
		if err != nil {
			return NotificationResolvedRequest{}, err
		}
		headersJSON, err := json.Marshal(headers)
		if err != nil {
			return NotificationResolvedRequest{}, err
		}
		out.RenderedHeadersJSON = string(headersJSON)
		out.RenderedBody = body
	default:
		out.RenderedBody = payloadJSON
		headers := http.Header{"Content-Type": []string{"application/json"}}
		headersJSON, err := json.Marshal(headers)
		if err != nil {
			return NotificationResolvedRequest{}, err
		}
		out.RenderedHeadersJSON = string(headersJSON)
	}
	return out, nil
}

func resolveNotificationEndpoint(input NotificationRequestResolveInput) (string, string, error) {
	mode := input.Sink.EndpointMode
	if mode == "" {
		mode = NotificationEndpointStaticURL
	}
	var raw string
	switch mode {
	case NotificationEndpointStaticURL:
		raw = input.Sink.URL
	case NotificationEndpointTemplate:
		if err := validateEndpointTemplateVariables(input.Sink.URLTemplate); err != nil {
			return "", "", err
		}
		rendered, err := renderNotificationTemplate(input.Sink.URLTemplate, input, false)
		if err != nil {
			return "", "", err
		}
		raw = rendered
	case NotificationEndpointConfigValue:
		raw = input.ConfigValues[input.Sink.ConfigKey]
		if raw == "" {
			return "", "", RuntimeError{Code: "endpoint_unresolved", Message: "endpoint config value is missing"}
		}
	default:
		return "", "", RuntimeError{Code: "endpoint_mode_invalid", Message: "unsupported endpoint mode"}
	}
	if err := validateResolvedNotificationURL(raw, input.Sink.AllowedHosts); err != nil {
		return "", "", err
	}
	return raw, mode, nil
}

func validateResolvedNotificationURL(raw string, allowedHosts []string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return RuntimeError{Code: "endpoint_unresolved", Message: "invalid endpoint url"}
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return RuntimeError{Code: "endpoint_unresolved", Message: "unsupported endpoint scheme"}
	}
	if len(allowedHosts) > 0 && !slices.Contains(allowedHosts, parsed.Hostname()) {
		return RuntimeError{Code: "endpoint_host_denied", Message: "endpoint host is not allowed"}
	}
	return nil
}

func validateEndpointTemplateVariables(tpl string) error {
	for _, match := range templateVarPattern.FindAllStringSubmatch(tpl, -1) {
		name := match[1]
		if strings.HasPrefix(name, "secret.") || strings.HasPrefix(name, "task.") {
			return RuntimeError{Code: "endpoint_template_invalid", Message: "endpoint template contains forbidden variable"}
		}
		if !allowedEndpointVariable(name) {
			return RuntimeError{Code: "endpoint_template_invalid", Message: "endpoint template contains unsupported variable"}
		}
	}
	return nil
}

func allowedEndpointVariable(name string) bool {
	switch {
	case name == "workspace.id", name == "workspace.slug", name == "project.id", name == "project.slug", name == "rule.id", name == "rule.name", name == "recipient.id":
		return true
	case name == "event.id", name == "event.type", name == "event.object_kind", name == "event.object_id", name == "actor.id":
		return true
	case strings.HasPrefix(name, "recipient.external_ids."):
		return true
	default:
		return false
	}
}

func renderNotificationTemplate(tpl string, input NotificationRequestResolveInput, allowSecrets bool) (string, error) {
	var err error
	out := templateVarPattern.ReplaceAllStringFunc(tpl, func(token string) string {
		if err != nil {
			return ""
		}
		match := templateVarPattern.FindStringSubmatch(token)
		if len(match) < 2 {
			return token
		}
		value, valueErr := notificationTemplateValue(match[1], input, allowSecrets)
		if valueErr != nil {
			err = valueErr
			return ""
		}
		return value
	})
	return out, err
}

func notificationTemplateValue(name string, input NotificationRequestResolveInput, allowSecrets bool) (string, error) {
	switch name {
	case "workspace.id":
		return input.Workspace.ID, nil
	case "workspace.slug":
		return input.Workspace.Slug, nil
	case "project.id":
		if input.Project == nil {
			return "", RuntimeError{Code: "template_unresolved", Message: "project is missing"}
		}
		return input.Project.ID, nil
	case "project.slug":
		if input.Project == nil {
			return "", RuntimeError{Code: "template_unresolved", Message: "project is missing"}
		}
		return input.Project.Slug, nil
	case "rule.id":
		return input.Rule.ID, nil
	case "rule.name":
		return input.Rule.Name, nil
	case "recipient.id":
		return input.Recipient.ID, nil
	case "actor.id":
		return input.Actor.ID, nil
	case "actor.name":
		return input.Actor.Name, nil
	case "event.id":
		return input.Event.ID, nil
	case "event.type":
		return input.Event.Type, nil
	case "event.version":
		return strconv.Itoa(input.Event.Version), nil
	case "event.occurred_at":
		return strconvFormatInt64(input.Event.OccurredAt), nil
	case "event.object_kind":
		return input.Event.ObjectKind, nil
	case "event.object_id":
		return input.Event.ObjectID, nil
	case "event.json":
		return input.Event.JSON, nil
	case "task.uuid":
		return input.Task.UUID, nil
	case "task.task_slug":
		return input.Task.TaskSlug, nil
	case "task.description":
		return input.Task.Description, nil
	case "task.status":
		return input.Task.Status, nil
	case "task.due":
		if input.Task.Due == nil {
			return "", nil
		}
		return strconvFormatInt64(*input.Task.Due), nil
	case "reminder.sequence":
		return strconvFormatInt64(input.Reminder.Sequence), nil
	case "reminder.overdue_sequence":
		return strconvFormatInt64(input.Reminder.OverdueSequence), nil
	case "reminder.window_start":
		return strconvFormatInt64(input.Reminder.WindowStart), nil
	case "reminder.window_end":
		return strconvFormatInt64(input.Reminder.WindowEnd), nil
	}
	if strings.HasPrefix(name, "recipient.external_ids.") {
		provider := strings.TrimPrefix(name, "recipient.external_ids.")
		for _, ext := range input.Recipient.ExternalIDs {
			if ext.Provider == provider {
				return ext.ExternalID, nil
			}
		}
		return "", RuntimeError{Code: "template_unresolved", Message: "recipient external id is missing"}
	}
	if strings.HasPrefix(name, "secret.") {
		if !allowSecrets {
			return "", RuntimeError{Code: "endpoint_template_invalid", Message: "secret is not allowed here"}
		}
		alias := strings.TrimPrefix(name, "secret.")
		if !secretAliasDeclared(input.Sink.SecretRefs, alias) {
			return "", RuntimeError{Code: "template_unresolved", Message: "secret ref is not declared"}
		}
		value := input.SecretValues[alias]
		if value == "" {
			return "", RuntimeError{Code: "template_unresolved", Message: "secret value is missing"}
		}
		return value, nil
	}
	return "", RuntimeError{Code: "template_unresolved", Message: "template variable is unsupported"}
}

func secretAliasDeclared(refs []HTTPTemplateSecretRefInput, alias string) bool {
	for _, ref := range refs {
		if ref.Alias == alias {
			return true
		}
	}
	return false
}

func buildNotificationPayloadJSON(input NotificationRequestResolveInput) (string, error) {
	if input.Event.JSON != "" {
		return input.Event.JSON, nil
	}
	eventType := input.EventType
	if eventType == "" {
		eventType = "task.due_soon"
	}
	payload := map[string]any{
		"event_type":    eventType,
		"event_version": 1,
		"workspace":     map[string]any{"id": input.Workspace.ID, "slug": input.Workspace.Slug, "name": input.Workspace.Name},
		"rule":          map[string]any{"id": input.Rule.ID, "name": input.Rule.Name, "trigger_type": input.Rule.TriggerType},
		"task":          map[string]any{"uuid": input.Task.UUID, "task_slug": input.Task.TaskSlug, "description": input.Task.Description, "status": input.Task.Status, "due": input.Task.Due},
		"recipient":     task.UserInfoToJSON(input.Recipient),
		"reminder": map[string]any{
			"sequence":         input.Reminder.Sequence,
			"overdue_sequence": input.Reminder.OverdueSequence,
			"window_start":     input.Reminder.WindowStart,
			"window_end":       input.Reminder.WindowEnd,
		},
	}
	if input.Project != nil {
		payload["project"] = map[string]any{"id": input.Project.ID, "slug": input.Project.Slug, "name": input.Project.Name}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func validateJSONBodyTemplate(tpl string) error {
	rendered := templateVarPattern.ReplaceAllStringFunc(tpl, func(token string) string {
		match := templateVarPattern.FindStringSubmatch(token)
		if len(match) == 2 && notificationTemplateVariableIsNumber(match[1]) {
			return `0`
		}
		return `x`
	})
	var payload any
	return json.Unmarshal([]byte(rendered), &payload)
}

func notificationTemplateVariableIsNumber(name string) bool {
	switch name {
	case "task.due", "reminder.sequence", "reminder.overdue_sequence", "reminder.window_start", "reminder.window_end":
		return true
	default:
		return false
	}
}

func strconvFormatInt64(v int64) string {
	return strconv.FormatInt(v, 10)
}
