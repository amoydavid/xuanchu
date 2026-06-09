package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

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

type notificationTemplateContext struct {
	values  map[string]string
	secrets map[string]string
}

func (s *Service) ResolveNotificationRequest(
	sink storage.NotificationSink,
	rule storage.ReminderRule,
	tsk task.Task,
	recipient task.UserInfo,
	eventType string,
	deliveryID string,
) (NotificationResolvedRequest, error) {
	workspace, err := s.workspaceRepo.GetByID(s.workspaceID)
	if err != nil {
		return NotificationResolvedRequest{}, err
	}
	var project *storage.Project
	if rule.ProjectID != nil {
		p, err := s.projectRepo.GetByID(*rule.ProjectID)
		if err != nil {
			return NotificationResolvedRequest{}, err
		}
		project = &p
	}
	ctx := buildNotificationTemplateContext(workspace, project, rule, tsk, recipient, eventType)
	resolvedURL, source, err := s.resolveNotificationEndpointURL(sink, ctx, project)
	if err != nil {
		return NotificationResolvedRequest{}, err
	}
	payloadJSON, err := buildNotificationPayloadJSON(workspace, project, sink, rule, tsk, recipient, eventType, deliveryID)
	if err != nil {
		return NotificationResolvedRequest{}, err
	}
	request := NotificationResolvedRequest{
		ResolvedURL:                 resolvedURL,
		ResolvedEndpointSource:      source,
		ResolvedEndpointFingerprint: endpointFingerprint(resolvedURL, source),
		PayloadJSON:                 payloadJSON,
	}
	switch sink.Type {
	case string(NotificationSinkTypeWebhook):
		request.RenderedMethod = "POST"
		request.RenderedContentType = "application/json"
		request.RenderedBody = payloadJSON
		request.RenderedHeadersJSON = mustJSON(map[string]string{
			"Content-Type": "application/json",
			"User-Agent":   "xuanchu/dev",
		})
	case string(NotificationSinkTypeHTTPTemplate):
		request.RenderedMethod = strings.ToUpper(strings.TrimSpace(sink.HTTPMethod))
		if request.RenderedMethod == "" {
			request.RenderedMethod = "POST"
		}
		headerTemplates := decodeHeaderTemplateRows(sink.HeaderTemplatesJSON)
		secretRefs := decodeSecretRefRows(sink.SecretRefsJSON)
		secretValues, err := s.resolveTemplateSecretValues(project, secretRefs)
		if err != nil {
			return NotificationResolvedRequest{}, err
		}
		request.RenderedHeadersJSON, err = renderNotificationHeadersJSON(headerTemplates, ctx, secretValues)
		if err != nil {
			return NotificationResolvedRequest{}, err
		}
		request.RenderedBody, err = renderNotificationTemplate(sink.BodyTemplate, ctx, secretValues)
		if err != nil {
			return NotificationResolvedRequest{}, err
		}
		request.RenderedContentType = sink.BodyContentType
		if request.RenderedContentType == "" {
			request.RenderedContentType = "application/json"
		}
	default:
		return NotificationResolvedRequest{}, RuntimeError{Code: "notification_sink_invalid", Message: fmt.Sprintf("unsupported notification sink type %q", sink.Type)}
	}
	return request, nil
}

func buildNotificationTemplateContext(workspace storage.Workspace, project *storage.Project, rule storage.ReminderRule, tsk task.Task, recipient task.UserInfo, eventType string) notificationTemplateContext {
	values := map[string]string{
		"workspace.id":     workspace.ID,
		"workspace.slug":   workspace.Slug,
		"rule.id":          rule.ID,
		"rule.name":        rule.Name,
		"recipient.id":     recipient.ID,
		"event_type":       eventType,
		"task.uuid":        tsk.UUID,
		"task.description": tsk.Description,
		"task.status":      tsk.Status,
	}
	jsonTask := task.ToJSON(tsk)
	if jsonTask.TaskSlug != nil {
		values["task.task_slug"] = *jsonTask.TaskSlug
	}
	if jsonTask.Due != nil {
		values["task.due"] = *jsonTask.Due
	}
	if project != nil {
		values["project.id"] = project.ID
		values["project.slug"] = project.Slug
	}
	for _, ext := range recipient.ExternalIDs {
		values["recipient.external_ids."+ext.Provider] = ext.ExternalID
	}
	return notificationTemplateContext{values: values, secrets: map[string]string{}}
}

func (s *Service) resolveNotificationEndpointURL(sink storage.NotificationSink, ctx notificationTemplateContext, project *storage.Project) (string, string, error) {
	switch sink.EndpointMode {
	case string(NotificationEndpointStaticURL):
		raw := strings.TrimSpace(sink.URL)
		if raw == "" {
			return "", "", RuntimeError{Code: "endpoint_unresolved", Message: "endpoint URL is required"}
		}
		if err := s.validateNotificationEndpoint(raw, sink.AllowedHostsJSON); err != nil {
			return "", "", err
		}
		return raw, string(NotificationEndpointStaticURL), nil
	case string(NotificationEndpointTemplate):
		if strings.TrimSpace(sink.URLTemplate) == "" {
			return "", "", RuntimeError{Code: "endpoint_unresolved", Message: "endpoint template is required"}
		}
		raw, err := renderNotificationTemplate(sink.URLTemplate, ctx, nil)
		if err != nil {
			return "", "", err
		}
		if err := s.validateNotificationEndpoint(raw, sink.AllowedHostsJSON); err != nil {
			return "", "", err
		}
		return raw, string(NotificationEndpointTemplate), nil
	case string(NotificationEndpointConfigValue):
		if strings.TrimSpace(sink.ConfigKey) == "" {
			return "", "", RuntimeError{Code: "endpoint_unresolved", Message: "config key is required"}
		}
		raw, source, err := s.resolveNotificationConfigValue(sink.ConfigKey, project)
		if err != nil {
			return "", "", err
		}
		if err := s.validateNotificationEndpoint(raw, sink.AllowedHostsJSON); err != nil {
			return "", "", err
		}
		return raw, source, nil
	default:
		return "", "", RuntimeError{Code: "endpoint_mode_invalid", Message: fmt.Sprintf("unsupported endpoint mode %q", sink.EndpointMode)}
	}
}

func (s *Service) resolveNotificationConfigValue(key string, project *storage.Project) (string, string, error) {
	def, ok, err := s.configDefRepo.Get(s.workspaceID, key)
	if err != nil {
		return "", "", err
	}
	if !ok {
		return "", "", RuntimeError{Code: "config_definition_not_found", Message: fmt.Sprintf("config definition %q not found", key)}
	}
	defView, err := configDefinitionViewFromRow(def)
	if err != nil {
		return "", "", err
	}
	if project != nil && configDefinitionAllowsScope(defView, storage.ConfigScopeProject) {
		if value, ok, err := s.configRepo.Get(storage.ConfigKey{WorkspaceID: s.workspaceID, Scope: storage.ConfigScopeProject, ScopeID: project.ID, Key: key}); err != nil {
			return "", "", err
		} else if ok {
			return value, "config_value:project", nil
		}
	}
	if configDefinitionAllowsScope(defView, storage.ConfigScopeWorkspace) {
		if value, ok, err := s.configRepo.Get(storage.ConfigKey{WorkspaceID: s.workspaceID, Scope: storage.ConfigScopeWorkspace, ScopeID: s.workspaceID, Key: key}); err != nil {
			return "", "", err
		} else if ok {
			return value, "config_value:workspace", nil
		}
	}
	if def.HasDefault {
		return def.DefaultValue, "config_value:default", nil
	}
	return "", "", RuntimeError{Code: "endpoint_unresolved", Message: fmt.Sprintf("config key %q has no value", key)}
}

func (s *Service) resolveTemplateSecretValues(project *storage.Project, refs []HTTPTemplateSecretRefView) (map[string]string, error) {
	secrets := map[string]string{}
	for _, ref := range refs {
		value, source, err := s.resolveSecretConfigValue(ref.ConfigKey, project)
		if err != nil {
			return nil, err
		}
		_ = source
		secrets[ref.Alias] = value
	}
	return secrets, nil
}

func (s *Service) resolveSecretConfigValue(key string, project *storage.Project) (string, string, error) {
	def, ok, err := s.configDefRepo.Get(s.workspaceID, key)
	if err != nil {
		return "", "", err
	}
	if !ok {
		return "", "", RuntimeError{Code: "config_definition_not_found", Message: fmt.Sprintf("config definition %q not found", key)}
	}
	defView, err := configDefinitionViewFromRow(def)
	if err != nil {
		return "", "", err
	}
	if !defView.Secret {
		return "", "", RuntimeError{Code: "template_unresolved", Message: fmt.Sprintf("config key %q is not secret", key)}
	}
	if project != nil && configDefinitionAllowsScope(defView, storage.ConfigScopeProject) {
		if value, ok, err := s.configRepo.Get(storage.ConfigKey{WorkspaceID: s.workspaceID, Scope: storage.ConfigScopeProject, ScopeID: project.ID, Key: key}); err != nil {
			return "", "", err
		} else if ok {
			return value, "secret:project", nil
		}
	}
	if configDefinitionAllowsScope(defView, storage.ConfigScopeWorkspace) {
		if value, ok, err := s.configRepo.Get(storage.ConfigKey{WorkspaceID: s.workspaceID, Scope: storage.ConfigScopeWorkspace, ScopeID: s.workspaceID, Key: key}); err != nil {
			return "", "", err
		} else if ok {
			return value, "secret:workspace", nil
		}
	}
	if def.HasDefault {
		return def.DefaultValue, "secret:default", nil
	}
	return "", "", RuntimeError{Code: "template_unresolved", Message: fmt.Sprintf("secret config key %q has no value", key)}
}

func (s *Service) validateNotificationEndpoint(rawURL string, allowedHostsJSON string) error {
	if err := ValidateWebhookEndpointURLWithDefault(rawURL); err != nil {
		return err
	}
	allowedHosts := decodeJSONStringSlice(allowedHostsJSON)
	if len(allowedHosts) == 0 {
		return nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return RuntimeError{Code: "endpoint_unresolved", Message: "invalid endpoint url"}
	}
	host := strings.ToLower(parsed.Hostname())
	for _, allowed := range allowedHosts {
		if strings.EqualFold(host, strings.TrimSpace(allowed)) {
			return nil
		}
	}
	return RuntimeError{Code: "endpoint_host_denied", Message: fmt.Sprintf("endpoint host %q is not allowed", host)}
}

func renderNotificationHeadersJSON(rows []HTTPHeaderTemplateView, ctx notificationTemplateContext, secrets map[string]string) (string, error) {
	headers := map[string]string{}
	for _, row := range rows {
		value, err := renderNotificationTemplate(row.Value, ctx, secrets)
		if err != nil {
			return "", err
		}
		headers[strings.TrimSpace(row.Name)] = value
	}
	return mustJSON(headers), nil
}

func renderNotificationTemplate(raw string, ctx notificationTemplateContext, secrets map[string]string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return raw, nil
	}
	re := regexp.MustCompile(`\{\{\s*([a-zA-Z0-9._-]+)\s*\}\}`)
	missing := false
	rendered := re.ReplaceAllStringFunc(raw, func(match string) string {
		sub := re.FindStringSubmatch(match)
		if len(sub) != 2 {
			missing = true
			return ""
		}
		key := sub[1]
		if strings.HasPrefix(key, "secret.") {
			if secrets == nil {
				missing = true
				return ""
			}
			alias := strings.TrimPrefix(key, "secret.")
			value, ok := secrets[alias]
			if !ok {
				missing = true
				return ""
			}
			return value
		}
		value, ok := ctx.values[key]
		if !ok {
			missing = true
			return ""
		}
		return value
	})
	if missing {
		return "", RuntimeError{Code: "template_unresolved", Message: "template variable could not be resolved"}
	}
	return rendered, nil
}

func buildNotificationPayloadJSON(workspace storage.Workspace, project *storage.Project, sink storage.NotificationSink, rule storage.ReminderRule, tsk task.Task, recipient task.UserInfo, eventType, deliveryID string) (string, error) {
	payload := map[string]any{
		"delivery_id": deliveryID,
		"event_id":    deliveryID,
		"event_type":  eventType,
		"workspace": map[string]any{
			"id":   workspace.ID,
			"slug": workspace.Slug,
		},
		"rule": map[string]any{
			"id":            rule.ID,
			"name":          rule.Name,
			"trigger_type":  rule.TriggerType,
			"audience_type": rule.AudienceType,
		},
		"sink": map[string]any{
			"id":   sink.ID,
			"name": sink.Name,
			"type": sink.Type,
		},
		"task":      task.ToJSON(tsk),
		"recipient": recipient,
	}
	if project != nil {
		payload["project"] = map[string]any{
			"id":   project.ID,
			"slug": project.Slug,
		}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func endpointFingerprint(rawURL, source string) string {
	sum := sha256.Sum256([]byte(source + "\n" + rawURL))
	return hex.EncodeToString(sum[:8])
}

func (s *Service) resolveNotificationDeliveryTask(workspaceID, taskUUID string) (task.Task, error) {
	row, err := s.repo.GetByUUID(workspaceID, taskUUID)
	if err != nil {
		return task.Task{}, err
	}
	return row, nil
}
