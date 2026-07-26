package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/authz"
	"git.dajee.net/dajee/xuanchu/internal/projecttemplate"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

// ComponentCounts 是 Snapshot 内各类可实例化组件的数量摘要。
type ComponentCounts struct {
	Configs     int `json:"configs"`
	Tasks       int `json:"tasks"`
	Series      int `json:"series"`
	Automations int `json:"automations"`
}

type ProjectTemplateSnapshotSummaryView struct {
	ID                 string                           `json:"id"`
	Version            int64                            `json:"version"`
	Hash               string                           `json:"hash"`
	SourceProjectID    string                           `json:"source_project_id"`
	Counts             ComponentCounts                  `json:"counts"`
	RequiredSecretKeys []string                         `json:"required_secret_keys"`
	ConfigInputs       []ProjectTemplateConfigInputView `json:"config_inputs"`
	CreatedBy          task.ActorInfo                   `json:"created_by"`
	CreatedAt          int64                            `json:"created_at"`
}

// ProjectTemplateSummaryView 只包含元数据和当前 Snapshot 摘要，永不暴露原始 JSON。
type ProjectTemplateSummaryView struct {
	ID              string                              `json:"id"`
	Key             string                              `json:"key"`
	Name            string                              `json:"name"`
	Description     string                              `json:"description"`
	Status          string                              `json:"status"`
	CurrentSnapshot *ProjectTemplateSnapshotSummaryView `json:"current_snapshot,omitempty"`
	CreatedBy       task.ActorInfo                      `json:"created_by"`
	CreatedAt       int64                               `json:"created_at"`
	ModifiedAt      int64                               `json:"modified_at"`
	ArchivedAt      *int64                              `json:"archived_at,omitempty"`
}

type ProjectTemplatePage struct {
	Items  []ProjectTemplateSummaryView `json:"items"`
	Total  int64                        `json:"total"`
	Limit  int                          `json:"limit"`
	Offset int                          `json:"offset"`
}

type ProjectTemplateConfigView struct {
	Key    string                          `json:"key"`
	Mode   string                          `json:"mode"`
	Value  *string                         `json:"value,omitempty"`
	Prompt *projecttemplate.ConfigPromptV2 `json:"prompt,omitempty"`
}

type ProjectTemplateTaskView struct {
	Ref         string                                    `json:"ref"`
	Title       string                                    `json:"title"`
	Description *string                                   `json:"description,omitempty"`
	Priority    *string                                   `json:"priority,omitempty"`
	Tags        []string                                  `json:"tags"`
	Assignees   []task.UserInfo                           `json:"assignees"`
	UDAs        map[string]projecttemplate.UDABlueprintV1 `json:"udas"`
	Dates       projecttemplate.TaskDatesV1               `json:"dates"`
	ParentRef   *string                                   `json:"parent_ref,omitempty"`
	DependsRefs []string                                  `json:"depends_refs"`
	Links       []projecttemplate.TaskLinkBlueprintV1     `json:"links"`
}

func (view ProjectTemplateTaskView) MarshalJSON() ([]byte, error) {
	type wireView struct {
		Ref         string                                    `json:"ref"`
		Title       string                                    `json:"title"`
		Description *string                                   `json:"description,omitempty"`
		Priority    *string                                   `json:"priority,omitempty"`
		Tags        []string                                  `json:"tags"`
		Assignees   []task.JSONUserInfo                       `json:"assignees"`
		UDAs        map[string]projecttemplate.UDABlueprintV1 `json:"udas"`
		Dates       projecttemplate.TaskDatesV1               `json:"dates"`
		ParentRef   *string                                   `json:"parent_ref,omitempty"`
		DependsRefs []string                                  `json:"depends_refs"`
		Links       []projecttemplate.TaskLinkBlueprintV1     `json:"links"`
	}
	return json.Marshal(wireView{
		Ref: view.Ref, Title: view.Title, Description: view.Description, Priority: view.Priority,
		Tags: view.Tags, Assignees: task.UserInfoListToJSON(view.Assignees), UDAs: view.UDAs,
		Dates: view.Dates, ParentRef: view.ParentRef, DependsRefs: view.DependsRefs, Links: view.Links,
	})
}

type ProjectTemplateSeriesView struct {
	Ref            string                                    `json:"ref"`
	Title          string                                    `json:"title"`
	RecurrenceRule string                                    `json:"recurrence_rule"`
	Description    *string                                   `json:"description,omitempty"`
	Priority       *string                                   `json:"priority,omitempty"`
	Tags           []string                                  `json:"tags"`
	Assignees      []task.UserInfo                           `json:"assignees"`
	UDAs           map[string]projecttemplate.UDABlueprintV1 `json:"udas"`
	FirstDue       projecttemplate.RelativeLocalTimeV1       `json:"first_due"`
	Until          *projecttemplate.RelativeLocalTimeV1      `json:"until,omitempty"`
}

func (view ProjectTemplateSeriesView) MarshalJSON() ([]byte, error) {
	type wireView struct {
		Ref            string                                    `json:"ref"`
		Title          string                                    `json:"title"`
		RecurrenceRule string                                    `json:"recurrence_rule"`
		Description    *string                                   `json:"description,omitempty"`
		Priority       *string                                   `json:"priority,omitempty"`
		Tags           []string                                  `json:"tags"`
		Assignees      []task.JSONUserInfo                       `json:"assignees"`
		UDAs           map[string]projecttemplate.UDABlueprintV1 `json:"udas"`
		FirstDue       projecttemplate.RelativeLocalTimeV1       `json:"first_due"`
		Until          *projecttemplate.RelativeLocalTimeV1      `json:"until,omitempty"`
	}
	return json.Marshal(wireView{
		Ref: view.Ref, Title: view.Title, RecurrenceRule: view.RecurrenceRule,
		Description: view.Description, Priority: view.Priority, Tags: view.Tags,
		Assignees: task.UserInfoListToJSON(view.Assignees), UDAs: view.UDAs,
		FirstDue: view.FirstDue, Until: view.Until,
	})
}

type ProjectTemplateAutomationView = projecttemplate.AutomationBlueprintV1

type ProjectTemplateSnapshotView struct {
	Project     projecttemplate.ProjectBlueprintV1 `json:"project"`
	Configs     []ProjectTemplateConfigView        `json:"configs"`
	Tasks       []ProjectTemplateTaskView          `json:"tasks"`
	Series      []ProjectTemplateSeriesView        `json:"series"`
	Automations []ProjectTemplateAutomationView    `json:"automations"`
}

type ProjectTemplateView struct {
	Template ProjectTemplateSummaryView           `json:"template"`
	Snapshot *ProjectTemplateSnapshotView         `json:"snapshot,omitempty"`
	Versions []ProjectTemplateSnapshotSummaryView `json:"versions,omitempty"`
}

type TemplateInstantiationListInput struct {
	Q      string
	Limit  int
	Offset int
}

type ModifyTemplateInput struct {
	Name        *string
	Description *string
}

func (s *Service) rejectProjectTemplateProjectScope() error {
	if s.hasProjectScope() {
		return RuntimeError{Code: authz.CodeProjectScopeDenied, Message: "project-scoped token cannot access project templates"}
	}
	return nil
}

func (s *Service) ListProjectTemplates(status, q string, limit, offset int) (ProjectTemplatePage, error) {
	if err := s.rejectProjectTemplateProjectScope(); err != nil {
		return ProjectTemplatePage{}, err
	}
	if err := s.Require(PermissionProjectRead); err != nil {
		return ProjectTemplatePage{}, err
	}
	status, err := normalizeProjectTemplateStatusFilter(status)
	if err != nil {
		return ProjectTemplatePage{}, err
	}
	page, err := s.projectTemplateRepo.List(storage.ProjectTemplateListOptions{WorkspaceID: s.workspaceID, Status: status, Q: q, Limit: templatePageLimit(limit), Offset: offset})
	if err != nil {
		return ProjectTemplatePage{}, err
	}
	items, err := s.projectTemplateSummaryViews(page.Items)
	if err != nil {
		return ProjectTemplatePage{}, err
	}
	return ProjectTemplatePage{Items: items, Total: page.Total, Limit: page.Limit, Offset: page.Offset}, nil
}

// ListProjectTemplatesForInstantiation 是 CLI、Remote 和 MCP 的收窄读取用例：
// 它仅返回 active template 的 current snapshot 摘要，不返回详情或历史版本。
func (s *Service) ListProjectTemplatesForInstantiation(input TemplateInstantiationListInput) (ProjectTemplatePage, error) {
	return s.ListProjectTemplates("active", input.Q, input.Limit, input.Offset)
}

func (s *Service) ProjectTemplateInfo(templateRef string, snapshotID *string) (ProjectTemplateView, error) {
	if err := s.rejectProjectTemplateProjectScope(); err != nil {
		return ProjectTemplateView{}, err
	}
	if err := s.Require(PermissionProjectRead); err != nil {
		return ProjectTemplateView{}, err
	}
	template, err := s.resolveProjectTemplate(templateRef)
	if err != nil {
		return ProjectTemplateView{}, err
	}
	snapshot, err := s.projectTemplateSnapshot(template, snapshotID)
	if err != nil {
		return ProjectTemplateView{}, err
	}
	decoded, err := projecttemplate.Decode([]byte(snapshot.SnapshotJSON), projecttemplate.DefaultLimits)
	if err != nil {
		return ProjectTemplateView{}, err
	}
	if err := s.requireProjectTemplateDetailPermissions(decoded); err != nil {
		return ProjectTemplateView{}, err
	}
	return s.projectTemplateDetailedView(template, snapshot, decoded)
}

func (s *Service) ModifyProjectTemplate(templateRef string, input ModifyTemplateInput) (ProjectTemplateView, error) {
	if err := s.rejectProjectTemplateProjectScope(); err != nil {
		return ProjectTemplateView{}, err
	}
	if err := s.Require(PermissionProjectManage); err != nil {
		return ProjectTemplateView{}, err
	}
	template, err := s.resolveProjectTemplate(templateRef)
	if err != nil {
		return ProjectTemplateView{}, err
	}
	if input.Name == nil && input.Description == nil {
		return ProjectTemplateView{}, RuntimeError{Code: "project_template_no_changes", Message: "template name or description is required"}
	}
	name, description := template.Name, template.Description
	if input.Name != nil {
		name = strings.TrimSpace(*input.Name)
		if name == "" {
			return ProjectTemplateView{}, RuntimeError{Code: "project_template_invalid_name", Message: "template name is required"}
		}
	}
	if input.Description != nil {
		description = strings.TrimSpace(*input.Description)
	}
	var updated storage.ProjectTemplate
	err = s.withAudit("project_template.modify", func(tx *Service) (AuditEntry, error) {
		if err := tx.projectTemplateRepo.UpdateMetadata(template.WorkspaceID, template.ID, name, description, tx.clock.Unix()); err != nil {
			return AuditEntry{}, tx.projectTemplateStorageError(templateRef, err)
		}
		row, err := tx.projectTemplateRepo.GetByRef(template.WorkspaceID, template.ID)
		if err != nil {
			return AuditEntry{}, err
		}
		updated = row
		return AuditEntry{WorkspaceID: &template.WorkspaceID, TargetType: "project_template", TargetID: template.ID, Payload: map[string]any{"name_before": template.Name, "name_after": name, "description_before": template.Description, "description_after": description}}, nil
	})
	if err != nil {
		return ProjectTemplateView{}, err
	}
	return s.projectTemplateMetadataView(updated)
}

func (s *Service) ArchiveProjectTemplate(templateRef string) (ProjectTemplateView, error) {
	return s.transitionProjectTemplateStatus(templateRef, "archived", "project_template.archive")
}

func (s *Service) ReactivateProjectTemplate(templateRef string) (ProjectTemplateView, error) {
	return s.transitionProjectTemplateStatus(templateRef, "active", "project_template.modify")
}

func (s *Service) transitionProjectTemplateStatus(templateRef, nextStatus, action string) (ProjectTemplateView, error) {
	if err := s.rejectProjectTemplateProjectScope(); err != nil {
		return ProjectTemplateView{}, err
	}
	if err := s.Require(PermissionProjectManage); err != nil {
		return ProjectTemplateView{}, err
	}
	template, err := s.resolveProjectTemplate(templateRef)
	if err != nil {
		return ProjectTemplateView{}, err
	}
	if template.Status == nextStatus {
		return ProjectTemplateView{}, RuntimeError{Code: "project_template_status_unchanged", Message: fmt.Sprintf("template %q is already %s", template.Key, nextStatus)}
	}
	var archivedAt *int64
	if nextStatus == "archived" {
		now := s.clock.Unix()
		archivedAt = &now
	}
	var updated storage.ProjectTemplate
	err = s.withAudit(action, func(tx *Service) (AuditEntry, error) {
		if err := tx.projectTemplateRepo.SetStatus(template.WorkspaceID, template.ID, nextStatus, archivedAt, tx.clock.Unix()); err != nil {
			return AuditEntry{}, tx.projectTemplateStorageError(templateRef, err)
		}
		row, err := tx.projectTemplateRepo.GetByRef(template.WorkspaceID, template.ID)
		if err != nil {
			return AuditEntry{}, err
		}
		updated = row
		return AuditEntry{WorkspaceID: &template.WorkspaceID, TargetType: "project_template", TargetID: template.ID, Payload: map[string]any{"status_before": template.Status, "status_after": nextStatus}}, nil
	})
	if err != nil {
		return ProjectTemplateView{}, err
	}
	return s.projectTemplateMetadataView(updated)
}

func (s *Service) resolveProjectTemplate(ref string) (storage.ProjectTemplate, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return storage.ProjectTemplate{}, RuntimeError{Code: "project_template_not_found", Message: "project template reference is required"}
	}
	row, err := s.projectTemplateRepo.GetByRef(s.workspaceID, ref)
	if err != nil {
		return storage.ProjectTemplate{}, s.projectTemplateStorageError(ref, err)
	}
	return row, nil
}

func (s *Service) projectTemplateStorageError(ref string, err error) error {
	if errors.Is(err, storage.ErrNotFound) {
		return RuntimeError{Code: "project_template_not_found", Message: fmt.Sprintf("project template %q not found", ref)}
	}
	return err
}

func (s *Service) projectTemplateSnapshot(template storage.ProjectTemplate, requested *string) (storage.ProjectTemplateSnapshot, error) {
	id := ""
	if requested != nil {
		id = strings.TrimSpace(*requested)
	}
	if id == "" && template.CurrentSnapshotID != nil {
		id = *template.CurrentSnapshotID
	}
	if id == "" {
		return storage.ProjectTemplateSnapshot{}, RuntimeError{Code: "project_template_snapshot_not_found", Message: "project template has no current snapshot"}
	}
	row, err := s.projectTemplateRepo.GetSnapshot(template.WorkspaceID, template.ID, id)
	if errors.Is(err, storage.ErrNotFound) {
		return storage.ProjectTemplateSnapshot{}, RuntimeError{Code: "project_template_snapshot_not_found", Message: fmt.Sprintf("project template snapshot %q not found", id)}
	}
	return row, err
}

func (s *Service) projectTemplateSummaryViews(rows []storage.ProjectTemplate) ([]ProjectTemplateSummaryView, error) {
	if len(rows) == 0 {
		return []ProjectTemplateSummaryView{}, nil
	}
	snapshots := make(map[string]storage.ProjectTemplateSnapshot, len(rows))
	userIDs := make([]string, 0, len(rows)*2)
	for _, row := range rows {
		userIDs = append(userIDs, valueOrEmpty(row.CreatedByUserID))
		if row.CurrentSnapshotID == nil || *row.CurrentSnapshotID == "" {
			continue
		}
		snapshot, err := s.projectTemplateRepo.GetSnapshot(row.WorkspaceID, row.ID, *row.CurrentSnapshotID)
		if err != nil {
			return nil, err
		}
		snapshots[row.ID] = snapshot
		userIDs = append(userIDs, valueOrEmpty(snapshot.CreatedByUserID))
	}
	users, err := s.resolveUserInfos(userIDs)
	if err != nil {
		return nil, err
	}
	views := make([]ProjectTemplateSummaryView, 0, len(rows))
	for _, row := range rows {
		view, err := s.projectTemplateSummaryViewFromRows(row, snapshots[row.ID], users)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

func (s *Service) projectTemplateMetadataView(row storage.ProjectTemplate) (ProjectTemplateView, error) {
	views, err := s.projectTemplateSummaryViews([]storage.ProjectTemplate{row})
	if err != nil {
		return ProjectTemplateView{}, err
	}
	return ProjectTemplateView{Template: views[0]}, nil
}

func (s *Service) projectTemplateSummaryViewFromRows(row storage.ProjectTemplate, snapshot storage.ProjectTemplateSnapshot, users map[string]task.UserInfo) (ProjectTemplateSummaryView, error) {
	view := ProjectTemplateSummaryView{ID: row.ID, Key: row.Key, Name: row.Name, Description: row.Description, Status: row.Status, CreatedBy: actorInfoFromColumns(projectTemplateActorColumns(row), valueOrEmpty(row.CreatedByUserID), users), CreatedAt: row.CreatedAt, ModifiedAt: row.ModifiedAt, ArchivedAt: row.ArchivedAt}
	if snapshot.ID == "" {
		return view, nil
	}
	decoded, err := projecttemplate.Decode([]byte(snapshot.SnapshotJSON), projecttemplate.DefaultLimits)
	if err != nil {
		return ProjectTemplateSummaryView{}, err
	}
	view.CurrentSnapshot, err = s.projectTemplateSnapshotSummaryView(snapshot, decoded, users)
	if err != nil {
		return ProjectTemplateSummaryView{}, err
	}
	return view, nil
}

func (s *Service) projectTemplateSnapshotSummaryView(row storage.ProjectTemplateSnapshot, decoded projecttemplate.Snapshot, users map[string]task.UserInfo) (*ProjectTemplateSnapshotSummaryView, error) {
	configInputs, err := s.projectTemplateConfigInputViews(decoded)
	if err != nil {
		return nil, err
	}
	return &ProjectTemplateSnapshotSummaryView{ID: row.ID, Version: row.Version, Hash: row.SnapshotHash, SourceProjectID: row.SourceProjectID, Counts: componentCounts(decoded), RequiredSecretKeys: requiredTemplateSecretKeys(decoded), ConfigInputs: configInputs, CreatedBy: actorInfoFromColumns(projectTemplateSnapshotActorColumns(row), valueOrEmpty(row.CreatedByUserID), users), CreatedAt: row.CreatedAt}, nil
}

func (s *Service) projectTemplateConfigInputViews(snapshot projecttemplate.Snapshot) ([]ProjectTemplateConfigInputView, error) {
	views := make([]ProjectTemplateConfigInputView, 0)
	for _, blueprint := range snapshot.Configs {
		if blueprint.Mode != "prompt" || blueprint.Prompt == nil {
			continue
		}
		def, err := s.scopedConfigDefinition(blueprint.Key)
		if err != nil {
			if code, ok := IsRuntimeErrorCode(err); ok && code == "config_definition_not_found" {
				views = append(views, ProjectTemplateConfigInputView{Key: blueprint.Key, Label: blueprint.Key, EnumValues: []string{}, Required: blueprint.Prompt.Required, Status: "definition_missing"})
				continue
			}
			return nil, err
		}
		status := "ready"
		if !configDefinitionAllowsScope(def, storage.ConfigScopeProject) {
			status = "scope_invalid"
		}
		views = append(views, projectTemplateConfigInputView(blueprint, def, status))
	}
	return views, nil
}

func componentCounts(snapshot projecttemplate.Snapshot) ComponentCounts {
	return ComponentCounts{Configs: len(snapshot.Configs), Tasks: len(snapshot.Tasks), Series: len(snapshot.Series), Automations: len(snapshot.Automations)}
}

func requiredTemplateSecretKeys(snapshot projecttemplate.Snapshot) []string {
	keys := make([]string, 0)
	for _, config := range snapshot.Configs {
		if config.Mode == "secret_input" {
			keys = append(keys, config.Key)
		}
	}
	sort.Strings(keys)
	return keys
}

func (s *Service) requireProjectTemplateDetailPermissions(snapshot projecttemplate.Snapshot) error {
	if len(snapshot.Tasks) > 0 || len(snapshot.Series) > 0 {
		if err := s.Require(PermissionTaskRead); err != nil {
			return err
		}
	}
	if len(snapshot.Configs) > 0 {
		if err := s.Require(PermissionProjectConfigRead); err != nil {
			return err
		}
	}
	if len(snapshot.Automations) > 0 {
		if err := s.Require(PermissionHookRead); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) projectTemplateDetailedView(template storage.ProjectTemplate, snapshot storage.ProjectTemplateSnapshot, decoded projecttemplate.Snapshot) (ProjectTemplateView, error) {
	versions, err := s.projectTemplateRepo.ListSnapshots(template.WorkspaceID, template.ID)
	if err != nil {
		return ProjectTemplateView{}, err
	}
	userIDs := []string{valueOrEmpty(template.CreatedByUserID)}
	for _, version := range versions {
		userIDs = append(userIDs, valueOrEmpty(version.CreatedByUserID))
	}
	for _, item := range decoded.Tasks {
		userIDs = append(userIDs, item.AssigneeIDs...)
	}
	for _, item := range decoded.Series {
		userIDs = append(userIDs, item.AssigneeIDs...)
	}
	users, err := s.resolveUserInfos(userIDs)
	if err != nil {
		return ProjectTemplateView{}, err
	}
	templateSummary, err := s.projectTemplateSummaryViewFromRows(template, snapshot, users)
	if err != nil {
		return ProjectTemplateView{}, err
	}
	versionViews := make([]ProjectTemplateSnapshotSummaryView, 0, len(versions))
	for _, version := range versions {
		versionDecoded, err := projecttemplate.Decode([]byte(version.SnapshotJSON), projecttemplate.DefaultLimits)
		if err != nil {
			return ProjectTemplateView{}, err
		}
		versionView, err := s.projectTemplateSnapshotSummaryView(version, versionDecoded, users)
		if err != nil {
			return ProjectTemplateView{}, err
		}
		versionViews = append(versionViews, *versionView)
	}
	return ProjectTemplateView{Template: templateSummary, Snapshot: projectTemplateSnapshotView(decoded, users), Versions: versionViews}, nil
}

func projectTemplateSnapshotView(snapshot projecttemplate.Snapshot, users map[string]task.UserInfo) *ProjectTemplateSnapshotView {
	view := &ProjectTemplateSnapshotView{Project: snapshot.Project, Configs: make([]ProjectTemplateConfigView, 0, len(snapshot.Configs)), Tasks: make([]ProjectTemplateTaskView, 0, len(snapshot.Tasks)), Series: make([]ProjectTemplateSeriesView, 0, len(snapshot.Series)), Automations: append([]ProjectTemplateAutomationView(nil), snapshot.Automations...)}
	for _, config := range snapshot.Configs {
		value := config.Value
		if config.Mode == "secret_input" || config.Mode == "secret_copy" {
			value = nil
		}
		view.Configs = append(view.Configs, ProjectTemplateConfigView{Key: config.Key, Mode: config.Mode, Value: value, Prompt: config.Prompt})
	}
	for _, item := range snapshot.Tasks {
		view.Tasks = append(view.Tasks, ProjectTemplateTaskView{Ref: item.Ref, Title: item.Title, Description: item.Description, Priority: item.Priority, Tags: append([]string(nil), item.Tags...), Assignees: userInfosForIDs(item.AssigneeIDs, users), UDAs: item.UDAs, Dates: item.Dates, ParentRef: item.ParentRef, DependsRefs: append([]string(nil), item.DependsRefs...), Links: append([]projecttemplate.TaskLinkBlueprintV1(nil), item.Links...)})
	}
	for _, item := range snapshot.Series {
		view.Series = append(view.Series, ProjectTemplateSeriesView{Ref: item.Ref, Title: item.Title, RecurrenceRule: item.RecurrenceRule, Description: item.Description, Priority: item.Priority, Tags: append([]string(nil), item.Tags...), Assignees: userInfosForIDs(item.AssigneeIDs, users), UDAs: item.UDAs, FirstDue: item.FirstDue, Until: item.Until})
	}
	return view
}

func userInfosForIDs(ids []string, users map[string]task.UserInfo) []task.UserInfo {
	items := make([]task.UserInfo, 0, len(ids))
	for _, id := range ids {
		user := users[id]
		if user.ID == "" {
			user = task.UserInfo{ID: id, Name: id}
		}
		items = append(items, user)
	}
	return items
}

func projectTemplateActorColumns(row storage.ProjectTemplate) actorColumns {
	return actorColumns{Type: row.CreatedByActorType, UserID: row.CreatedByUserID, TokenID: row.CreatedByTokenID, TokenName: row.CreatedByTokenName, TokenPrefix: row.CreatedByTokenPrefix}
}

func projectTemplateSnapshotActorColumns(row storage.ProjectTemplateSnapshot) actorColumns {
	return actorColumns{Type: row.CreatedByActorType, UserID: row.CreatedByUserID, TokenID: row.CreatedByTokenID, TokenName: row.CreatedByTokenName, TokenPrefix: row.CreatedByTokenPrefix}
}

func normalizeProjectTemplateStatusFilter(status string) (string, error) {
	status = strings.TrimSpace(strings.ToLower(status))
	switch status {
	case "", "all":
		return "", nil
	case "active", "archived":
		return status, nil
	default:
		return "", RuntimeError{Code: "project_template_invalid_status_filter", Message: fmt.Sprintf("invalid project template status filter %q", status)}
	}
}

func templatePageLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 100 {
		return 100
	}
	return limit
}
