package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/authz"
	"git.dajee.net/dajee/xuanchu/internal/projecttemplate"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

const projectTemplateBodyLimitBytes int64 = 9 << 20

type projectTemplateCaptureSelectionRequest struct {
	ConfigKeys        *[]string `json:"config_keys"`
	TaskRefs          *[]string `json:"task_refs"`
	SeriesRefs        *[]string `json:"series_refs"`
	AutomationRuleIDs *[]string `json:"automation_rule_ids"`
}

type projectTemplateCaptureRequest struct {
	SourceProject      string                                 `json:"source_project"`
	AnchorDate         string                                 `json:"anchor_date"`
	Selection          projectTemplateCaptureSelectionRequest `json:"selection"`
	Resolution         app.CaptureResolution                  `json:"resolution,omitempty"`
	ExpectedSourceHash string                                 `json:"expected_source_hash,omitempty"`
}

type projectTemplateCreateRequest struct {
	Key         string                        `json:"key"`
	Name        string                        `json:"name"`
	Description string                        `json:"description,omitempty"`
	Capture     projectTemplateCaptureRequest `json:"capture"`
}

type projectTemplateModifyRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

type projectTemplateResolveSelectionRequest struct {
	Kind         string                                    `json:"kind"`
	Task         *projectTemplateTaskCandidateFilter       `json:"task,omitempty"`
	Series       *projectTemplateSeriesCandidateFilter     `json:"series,omitempty"`
	Config       *projectTemplateConfigCandidateFilter     `json:"config,omitempty"`
	Automation   *projectTemplateAutomationCandidateFilter `json:"automation,omitempty"`
	invalidField string
}

type projectTemplateTaskCandidateFilter struct {
	Q         string   `json:"q,omitempty"`
	Status    string   `json:"status,omitempty"`
	Priority  string   `json:"priority,omitempty"`
	Assignees []string `json:"assignees,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	DueAfter  string   `json:"due_after,omitempty"`
	DueBefore string   `json:"due_before,omitempty"`
	Query     string   `json:"query,omitempty"`
	Sort      string   `json:"sort,omitempty"`
}

type projectTemplateSeriesCandidateFilter struct {
	Q        string `json:"q,omitempty"`
	Status   string `json:"status,omitempty"`
	Assignee string `json:"assignee,omitempty"`
	Sort     string `json:"sort,omitempty"`
}

type projectTemplateConfigCandidateFilter struct {
	Q    string `json:"q,omitempty"`
	Mode string `json:"mode,omitempty"`
}

type projectTemplateAutomationCandidateFilter struct {
	Q           string `json:"q,omitempty"`
	Status      string `json:"status,omitempty"`
	TriggerType string `json:"trigger_type,omitempty"`
}

func (req *projectTemplateResolveSelectionRequest) UnmarshalJSON(data []byte) error {
	type wire projectTemplateResolveSelectionRequest
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*req = projectTemplateResolveSelectionRequest(decoded)
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return err
	}
	allowedRoot := map[string]bool{"kind": true, "task": true, "series": true, "config": true, "automation": true}
	for key := range root {
		if !allowedRoot[key] {
			req.invalidField = key
			return nil
		}
	}
	allowedFilters := map[string]map[string]bool{
		"task":       {"q": true, "status": true, "priority": true, "assignees": true, "tags": true, "due_after": true, "due_before": true, "query": true, "sort": true},
		"series":     {"q": true, "status": true, "assignee": true, "sort": true},
		"config":     {"q": true, "mode": true},
		"automation": {"q": true, "status": true, "trigger_type": true},
	}
	for name, allowed := range allowedFilters {
		raw, ok := root[name]
		if !ok || string(raw) == "null" {
			continue
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return err
		}
		for key := range fields {
			if !allowed[key] {
				req.invalidField = name + "." + key
				return nil
			}
		}
	}
	return nil
}

type projectTemplateSnapshotSummaryResponse struct {
	ID                 string              `json:"id"`
	Version            int64               `json:"version"`
	Hash               string              `json:"hash"`
	SourceProjectID    string              `json:"source_project_id"`
	Counts             app.ComponentCounts `json:"counts"`
	RequiredSecretKeys []string            `json:"required_secret_keys"`
	CreatedBy          task.JSONActorInfo  `json:"created_by"`
	CreatedAt          int64               `json:"created_at"`
}

type projectTemplateSummaryResponse struct {
	ID              string                                  `json:"id"`
	Key             string                                  `json:"key"`
	Name            string                                  `json:"name"`
	Description     string                                  `json:"description"`
	Status          string                                  `json:"status"`
	CurrentSnapshot *projectTemplateSnapshotSummaryResponse `json:"current_snapshot,omitempty"`
	CreatedBy       task.JSONActorInfo                      `json:"created_by"`
	CreatedAt       int64                                   `json:"created_at"`
	ModifiedAt      int64                                   `json:"modified_at"`
	ArchivedAt      *int64                                  `json:"archived_at,omitempty"`
}

type projectTemplatePageResponse struct {
	Items  []projectTemplateSummaryResponse `json:"items"`
	Total  int64                            `json:"total"`
	Limit  int                              `json:"limit"`
	Offset int                              `json:"offset"`
}

type projectTemplateDetailResponse struct {
	Template projectTemplateSummaryResponse           `json:"template"`
	Snapshot *app.ProjectTemplateSnapshotView         `json:"snapshot,omitempty"`
	Versions []projectTemplateSnapshotSummaryResponse `json:"versions,omitempty"`
}

type projectTemplateTaskCandidateResponse struct {
	Ref          string              `json:"ref"`
	ProjectID    string              `json:"project_id"`
	ProjectSeq   *int64              `json:"project_seq,omitempty"`
	SeriesID     *string             `json:"series_id,omitempty"`
	Title        string              `json:"title"`
	Status       string              `json:"status"`
	Priority     *string             `json:"priority,omitempty"`
	Due          *int64              `json:"due,omitempty"`
	Assignees    []task.JSONUserInfo `json:"assignees"`
	WarningCount int                 `json:"warning_count"`
}

type projectTemplateSeriesCandidateResponse struct {
	Ref            string              `json:"ref"`
	ProjectID      string              `json:"project_id"`
	ProjectSeq     *int64              `json:"project_seq,omitempty"`
	Title          string              `json:"title"`
	Status         string              `json:"status"`
	RecurrenceRule string              `json:"recurrence_rule"`
	FirstDue       int64               `json:"first_due"`
	Assignees      []task.JSONUserInfo `json:"assignees"`
	CreatedBy      task.JSONUserInfo   `json:"created_by"`
	WarningCount   int                 `json:"warning_count"`
}

type projectTemplateAutomationCandidateResponse struct {
	Ref          string            `json:"ref"`
	ID           string            `json:"id"`
	ProjectID    string            `json:"project_id"`
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Enabled      bool              `json:"enabled"`
	TriggerType  string            `json:"trigger_type"`
	CreatedBy    task.JSONUserInfo `json:"created_by"`
	CreatedAt    int64             `json:"created_at"`
	WarningCount int               `json:"warning_count"`
}

type projectTemplateInstantiateResponse struct {
	Project projectResponse     `json:"project"`
	Counts  app.ComponentCounts `json:"counts"`
}

type projectTemplateInstantiateRequest struct {
	app.InstantiateInput
	CurrentOnly bool `json:"current_only,omitempty"`
}

func (s *Server) handleProjectTemplateTaskCandidates(w http.ResponseWriter, r *http.Request) {
	projectRef := chi.URLParam(r, "projectRef")
	scoped, ok := s.projectTemplateScopedService(w, r, auth.ScopeProjectRead, app.PermissionProjectRead, "")
	if !ok || !s.requireProjectTemplateCapability(w, r, auth.ScopeProjectRead, app.PermissionProjectRead, projectRef) || !s.requireProjectTemplateCapability(w, r, auth.ScopeTaskRead, app.PermissionTaskRead, projectRef) {
		return
	}
	limit, offset, ok := projectTemplatePageParams(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	page, err := scoped.ListProjectTemplateTaskCandidates(app.TaskCandidateListInput{
		SourceProjectRef: projectRef, Refs: queryStringList(q["ref"]), Q: q.Get("q"), Status: q.Get("status"), Priority: q.Get("priority"),
		Assignees: queryStringList(q["assignee"]), Tags: queryStringList(q["tags"]), DueAfter: q.Get("due_after"),
		DueBefore: q.Get("due_before"), Query: q.Get("query"), Sort: q.Get("sort"), Limit: limit, Offset: offset,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	items := make([]projectTemplateTaskCandidateResponse, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, projectTemplateTaskCandidateResponse{
			Ref: item.Ref, ProjectID: item.ProjectID, ProjectSeq: item.ProjectSeq, SeriesID: item.SeriesID,
			Title: item.Title, Status: item.Status, Priority: item.Priority, Due: item.Due,
			Assignees: task.UserInfoListToJSON(item.Assignees), WarningCount: item.WarningCount,
		})
	}
	writeSuccess(w, http.StatusOK, map[string]any{"items": items, "total": page.Total, "limit": page.Limit, "offset": page.Offset}, nil)
}

func (s *Server) handleProjectTemplateSeriesCandidates(w http.ResponseWriter, r *http.Request) {
	projectRef := chi.URLParam(r, "projectRef")
	scoped, ok := s.projectTemplateScopedService(w, r, auth.ScopeProjectRead, app.PermissionProjectRead, "")
	if !ok || !s.requireProjectTemplateCapability(w, r, auth.ScopeProjectRead, app.PermissionProjectRead, projectRef) || !s.requireProjectTemplateCapability(w, r, auth.ScopeTaskRead, app.PermissionTaskRead, projectRef) {
		return
	}
	limit, offset, ok := projectTemplatePageParams(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	page, err := scoped.ListProjectTemplateSeriesCandidates(app.SeriesCandidateListInput{
		SourceProjectRef: projectRef, Refs: queryStringList(q["ref"]), Q: q.Get("q"), Status: q.Get("status"), Assignee: q.Get("assignee"),
		Sort: q.Get("sort"), Limit: limit, Offset: offset,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	items := make([]projectTemplateSeriesCandidateResponse, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, projectTemplateSeriesCandidateResponse{
			Ref: item.Ref, ProjectID: item.ProjectID, ProjectSeq: item.ProjectSeq, Title: item.Title, Status: item.Status,
			RecurrenceRule: item.RecurrenceRule, FirstDue: item.FirstDue, Assignees: task.UserInfoListToJSON(item.Assignees),
			CreatedBy: task.UserInfoToJSON(item.CreatedBy), WarningCount: item.WarningCount,
		})
	}
	writeSuccess(w, http.StatusOK, map[string]any{"items": items, "total": page.Total, "limit": page.Limit, "offset": page.Offset}, nil)
}

func (s *Server) handleProjectTemplateConfigCandidates(w http.ResponseWriter, r *http.Request) {
	projectRef := chi.URLParam(r, "projectRef")
	scoped, ok := s.projectTemplateScopedService(w, r, auth.ScopeProjectRead, app.PermissionProjectRead, "")
	if !ok || !s.requireProjectTemplateCapability(w, r, auth.ScopeProjectRead, app.PermissionProjectRead, projectRef) || !s.requireProjectTemplateCapability(w, r, auth.ScopeConfigRead, app.PermissionProjectConfigRead, projectRef) {
		return
	}
	limit, offset, ok := projectTemplatePageParams(w, r)
	if !ok {
		return
	}
	page, err := scoped.ListProjectTemplateConfigCandidates(app.ConfigCandidateListInput{
		SourceProjectRef: projectRef, Refs: queryStringList(r.URL.Query()["ref"]), Q: r.URL.Query().Get("q"), Mode: r.URL.Query().Get("mode"), Limit: limit, Offset: offset,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, page, nil)
}

func (s *Server) handleProjectTemplateAutomationCandidates(w http.ResponseWriter, r *http.Request) {
	projectRef := chi.URLParam(r, "projectRef")
	scoped, ok := s.projectTemplateScopedService(w, r, auth.ScopeProjectRead, app.PermissionProjectRead, "")
	if !ok || !s.requireProjectTemplateCapability(w, r, auth.ScopeProjectRead, app.PermissionProjectRead, projectRef) || !s.requireProjectTemplateCapability(w, r, auth.ScopeHookRead, app.PermissionHookRead, projectRef) {
		return
	}
	limit, offset, ok := projectTemplatePageParams(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	page, err := scoped.ListProjectTemplateAutomationCandidates(app.AutomationCandidateListInput{
		SourceProjectRef: projectRef, Refs: queryStringList(q["ref"]), Q: q.Get("q"), Status: q.Get("status"), TriggerType: q.Get("trigger_type"), Limit: limit, Offset: offset,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	items := make([]projectTemplateAutomationCandidateResponse, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, projectTemplateAutomationCandidateResponse{
			Ref: item.Ref, ID: item.ID, ProjectID: item.ProjectID, Name: item.Name, Description: item.Description,
			Enabled: item.Enabled, TriggerType: item.TriggerType, CreatedBy: task.UserInfoToJSON(item.CreatedBy),
			CreatedAt: item.CreatedAt, WarningCount: item.WarningCount,
		})
	}
	writeSuccess(w, http.StatusOK, map[string]any{"items": items, "total": page.Total, "limit": page.Limit, "offset": page.Offset}, nil)
}

func (s *Server) handleProjectTemplateResolveSelection(w http.ResponseWriter, r *http.Request) {
	var req projectTemplateResolveSelectionRequest
	if !decodeProjectTemplateJSON(w, r, &req) {
		return
	}
	if err := req.validate(); err != nil {
		writeProjectTemplateAppError(w, err)
		return
	}
	projectRef := chi.URLParam(r, "projectRef")
	scoped, ok := s.projectTemplateScopedService(w, r, auth.ScopeProjectRead, app.PermissionProjectRead, "")
	if !ok || !s.requireProjectTemplateCapability(w, r, auth.ScopeProjectRead, app.PermissionProjectRead, projectRef) {
		return
	}
	capability, permission := projectTemplateCandidateCapability(req.Kind)
	if capability == "" || !s.requireProjectTemplateCapability(w, r, capability, permission, projectRef) {
		if capability == "" {
			writeError(w, http.StatusUnprocessableEntity, "project_template_candidate_invalid", "candidate kind must be task|series|config|automation", nil)
		}
		return
	}
	view, err := scoped.ResolveProjectTemplateCandidateSelection(req.appInput(projectRef))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

func (s *Server) handleProjectTemplateCapturePreview(w http.ResponseWriter, r *http.Request) {
	var req projectTemplateCaptureRequest
	if !decodeProjectTemplateJSON(w, r, &req) {
		return
	}
	if !requireProjectTemplateCaptureSelection(w, req.Selection) {
		return
	}
	input := req.appInput()
	scoped, ok := s.projectTemplateScopedService(w, r, auth.ScopeProjectWrite, app.PermissionProjectManage, "")
	if !ok || !s.requireCaptureCapabilities(w, r, input) {
		return
	}
	view, err := scoped.PreviewProjectTemplateCapture(input)
	if err != nil {
		writeProjectTemplateAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

func (s *Server) handleProjectTemplateCreate(w http.ResponseWriter, r *http.Request) {
	var req projectTemplateCreateRequest
	if !decodeProjectTemplateJSON(w, r, &req) {
		return
	}
	if !requireProjectTemplateCaptureSelection(w, req.Capture.Selection) {
		return
	}
	input := req.Capture.appInput()
	scoped, ok := s.projectTemplateScopedService(w, r, auth.ScopeProjectWrite, app.PermissionProjectManage, "")
	if !ok || !s.requireCaptureCapabilities(w, r, input) {
		return
	}
	view, err := scoped.CreateProjectTemplate(app.CreateTemplateInput{Key: req.Key, Name: req.Name, Description: req.Description, Capture: input})
	if err != nil {
		writeProjectTemplateAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, projectTemplateDetailToResponse(view), nil)
}

func (s *Server) handleProjectTemplateList(w http.ResponseWriter, r *http.Request) {
	scoped, ok := s.projectTemplateScopedService(w, r, auth.ScopeProjectRead, app.PermissionProjectRead, "")
	if !ok {
		return
	}
	limit, offset, ok := projectTemplatePageParams(w, r)
	if !ok {
		return
	}
	page, err := scoped.ListProjectTemplates(r.URL.Query().Get("status"), r.URL.Query().Get("q"), limit, offset)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, projectTemplatePageToResponse(page), nil)
}

func (s *Server) handleProjectTemplateInfo(w http.ResponseWriter, r *http.Request) {
	scoped, ok := s.projectTemplateScopedService(w, r, auth.ScopeProjectRead, app.PermissionProjectRead, "")
	if !ok {
		return
	}
	var snapshotID *string
	if raw := strings.TrimSpace(r.URL.Query().Get("snapshot_id")); raw != "" {
		snapshotID = &raw
	}
	view, err := scoped.ProjectTemplateInfo(chi.URLParam(r, "templateRef"), snapshotID)
	if err != nil {
		writeProjectTemplateAppError(w, err)
		return
	}
	if !s.requireTemplateDetailCapabilities(w, r, view) {
		return
	}
	writeSuccess(w, http.StatusOK, projectTemplateDetailToResponse(view), nil)
}

func (s *Server) handleProjectTemplateModify(w http.ResponseWriter, r *http.Request) {
	var req projectTemplateModifyRequest
	if !decodeProjectTemplateJSON(w, r, &req) {
		return
	}
	scoped, ok := s.projectTemplateScopedService(w, r, auth.ScopeProjectWrite, app.PermissionProjectManage, "")
	if !ok {
		return
	}
	view, err := scoped.ModifyProjectTemplate(chi.URLParam(r, "templateRef"), app.ModifyTemplateInput{Name: req.Name, Description: req.Description})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, projectTemplateDetailToResponse(view), nil)
}

func (s *Server) handleProjectTemplateArchive(w http.ResponseWriter, r *http.Request) {
	if !limitProjectTemplateBody(w, r) {
		return
	}
	scoped, ok := s.projectTemplateScopedService(w, r, auth.ScopeProjectWrite, app.PermissionProjectManage, "")
	if !ok {
		return
	}
	view, err := scoped.ArchiveProjectTemplate(chi.URLParam(r, "templateRef"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, projectTemplateDetailToResponse(view), nil)
}

func (s *Server) handleProjectTemplateReactivate(w http.ResponseWriter, r *http.Request) {
	if !limitProjectTemplateBody(w, r) {
		return
	}
	scoped, ok := s.projectTemplateScopedService(w, r, auth.ScopeProjectWrite, app.PermissionProjectManage, "")
	if !ok {
		return
	}
	view, err := scoped.ReactivateProjectTemplate(chi.URLParam(r, "templateRef"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, projectTemplateDetailToResponse(view), nil)
}

func (s *Server) handleProjectTemplateSnapshotCapturePreview(w http.ResponseWriter, r *http.Request) {
	var req projectTemplateCaptureRequest
	if !decodeProjectTemplateJSON(w, r, &req) || !requireProjectTemplateCaptureSelection(w, req.Selection) {
		return
	}
	input := req.appInput()
	scoped, ok := s.projectTemplateScopedService(w, r, auth.ScopeProjectWrite, app.PermissionProjectManage, "")
	if !ok {
		return
	}
	if err := requireActiveProjectTemplate(scoped, chi.URLParam(r, "templateRef")); err != nil {
		writeProjectTemplateAppError(w, err)
		return
	}
	if !s.requireCaptureCapabilities(w, r, input) {
		return
	}
	view, err := scoped.PreviewProjectTemplateCapture(input)
	if err != nil {
		writeProjectTemplateAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

func (s *Server) handleProjectTemplateSnapshotCreate(w http.ResponseWriter, r *http.Request) {
	var req projectTemplateCaptureRequest
	if !decodeProjectTemplateJSON(w, r, &req) {
		return
	}
	if !requireProjectTemplateCaptureSelection(w, req.Selection) {
		return
	}
	input := req.appInput()
	scoped, ok := s.projectTemplateScopedService(w, r, auth.ScopeProjectWrite, app.PermissionProjectManage, "")
	if !ok || !s.requireCaptureCapabilities(w, r, input) {
		return
	}
	view, err := scoped.CreateProjectTemplateSnapshot(chi.URLParam(r, "templateRef"), input)
	if err != nil {
		writeProjectTemplateAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, projectTemplateDetailToResponse(view), nil)
}

func (s *Server) handleProjectTemplateInstantiatePreview(w http.ResponseWriter, r *http.Request) {
	var req app.InstantiateInput
	if !decodeProjectTemplateJSON(w, r, &req) {
		return
	}
	scoped, ok := s.projectTemplateScopedService(w, r, auth.ScopeProjectWrite, app.PermissionProjectManage, "")
	if !ok {
		return
	}
	view, err := scoped.PreviewProjectTemplateInstantiation(chi.URLParam(r, "templateRef"), req)
	if err != nil {
		writeProjectTemplateAppError(w, err)
		return
	}
	if !s.requireInstantiateCapabilities(w, r, view.Counts) {
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

func (s *Server) handleProjectTemplateInstantiate(w http.ResponseWriter, r *http.Request) {
	var req projectTemplateInstantiateRequest
	if !decodeProjectTemplateJSON(w, r, &req) {
		return
	}
	scoped, ok := s.projectTemplateScopedService(w, r, auth.ScopeProjectWrite, app.PermissionProjectManage, "")
	if !ok {
		return
	}
	preview, err := scoped.PreviewProjectTemplateInstantiation(chi.URLParam(r, "templateRef"), req.InstantiateInput)
	if err != nil {
		writeProjectTemplateAppError(w, err)
		return
	}
	if !s.requireInstantiateCapabilities(w, r, preview.Counts) {
		return
	}
	var view app.InstantiateResult
	if req.CurrentOnly {
		view, err = scoped.InstantiateCurrentProjectTemplate(chi.URLParam(r, "templateRef"), req.InstantiateInput)
	} else {
		view, err = scoped.InstantiateProjectTemplate(chi.URLParam(r, "templateRef"), req.InstantiateInput)
	}
	if err != nil {
		writeProjectTemplateAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, projectTemplateInstantiateResponse{Project: projectResponseFromView(view.Project), Counts: view.Counts}, nil)
}

func (req projectTemplateCaptureRequest) appInput() app.CaptureInput {
	selection := app.CaptureSelection{}
	presence := app.SelectionPresence{}
	if req.Selection.ConfigKeys != nil {
		selection.ConfigKeys, presence.ConfigKeys = append([]string{}, (*req.Selection.ConfigKeys)...), true
	}
	if req.Selection.TaskRefs != nil {
		selection.TaskRefs, presence.TaskRefs = append([]string{}, (*req.Selection.TaskRefs)...), true
	}
	if req.Selection.SeriesRefs != nil {
		selection.SeriesRefs, presence.SeriesRefs = append([]string{}, (*req.Selection.SeriesRefs)...), true
	}
	if req.Selection.AutomationRuleIDs != nil {
		selection.AutomationRuleIDs, presence.AutomationRuleIDs = append([]string{}, (*req.Selection.AutomationRuleIDs)...), true
	}
	return app.CaptureInput{
		SourceProjectRef: req.SourceProject, AnchorDate: req.AnchorDate, Selection: selection,
		SelectionPresence: presence, Resolution: req.Resolution, ExpectedSourceHash: req.ExpectedSourceHash,
	}
}

func (req projectTemplateResolveSelectionRequest) appInput(projectRef string) app.CandidateSelectionQuery {
	input := app.CandidateSelectionQuery{SourceProjectRef: projectRef, Kind: req.Kind}
	if req.Task != nil {
		input.Task = &app.TaskCandidateListInput{
			SourceProjectRef: projectRef, Q: req.Task.Q, Status: req.Task.Status, Priority: req.Task.Priority,
			Assignees: append([]string{}, req.Task.Assignees...), Tags: append([]string{}, req.Task.Tags...),
			DueAfter: req.Task.DueAfter, DueBefore: req.Task.DueBefore, Query: req.Task.Query, Sort: req.Task.Sort,
		}
	}
	if req.Series != nil {
		input.Series = &app.SeriesCandidateListInput{SourceProjectRef: projectRef, Q: req.Series.Q, Status: req.Series.Status, Assignee: req.Series.Assignee, Sort: req.Series.Sort}
	}
	if req.Config != nil {
		input.Config = &app.ConfigCandidateListInput{SourceProjectRef: projectRef, Q: req.Config.Q, Mode: req.Config.Mode}
	}
	if req.Automation != nil {
		input.Automation = &app.AutomationCandidateListInput{SourceProjectRef: projectRef, Q: req.Automation.Q, Status: req.Automation.Status, TriggerType: req.Automation.TriggerType}
	}
	return input
}

func (req projectTemplateResolveSelectionRequest) validate() error {
	if req.invalidField != "" {
		return app.RuntimeError{Code: "project_template_candidate_invalid", Message: "unsupported candidate filter field " + req.invalidField}
	}
	kind := strings.TrimSpace(req.Kind)
	present := 0
	if req.Task != nil {
		present++
	}
	if req.Series != nil {
		present++
	}
	if req.Config != nil {
		present++
	}
	if req.Automation != nil {
		present++
	}
	matched := kind == "task" && req.Task != nil || kind == "series" && req.Series != nil || kind == "config" && req.Config != nil || kind == "automation" && req.Automation != nil
	if present != 1 || !matched {
		return app.RuntimeError{Code: "project_template_candidate_invalid", Message: "kind must match exactly one candidate filter object"}
	}
	return nil
}

func requireActiveProjectTemplate(svc *app.Service, templateRef string) error {
	ref := strings.TrimSpace(templateRef)
	for offset := 0; ; offset += 100 {
		page, err := svc.ListProjectTemplates("all", "", 100, offset)
		if err != nil {
			return err
		}
		for _, item := range page.Items {
			if item.ID != ref && item.Key != ref {
				continue
			}
			if item.Status == "archived" {
				return app.RuntimeError{Code: "project_template_archived", Message: "archived project template cannot accept snapshots"}
			}
			return nil
		}
		if int64(offset+page.Limit) >= page.Total || page.Limit <= 0 {
			break
		}
	}
	return app.RuntimeError{Code: "project_template_not_found", Message: "project template not found"}
}

func limitProjectTemplateBody(w http.ResponseWriter, r *http.Request) bool {
	if r.ContentLength > projectTemplateBodyLimitBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "api_payload_too_large", "request payload too large", nil)
		return false
	}
	if r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, projectTemplateBodyLimitBytes)
	}
	return true
}

func decodeProjectTemplateJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if !limitProjectTemplateBody(w, r) {
		return false
	}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(dst); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "api_payload_too_large", "request payload too large", nil)
			return false
		}
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return false
	}
	return true
}

func requireProjectTemplateCaptureSelection(w http.ResponseWriter, selection projectTemplateCaptureSelectionRequest) bool {
	if selection.ConfigKeys == nil || selection.TaskRefs == nil || selection.SeriesRefs == nil || selection.AutomationRuleIDs == nil {
		writeError(w, http.StatusUnprocessableEntity, "project_template_selection_invalid", "all four selection arrays are required", nil)
		return false
	}
	return true
}

func writeProjectTemplateAppError(w http.ResponseWriter, err error) {
	var validationErr app.ProjectTemplateValidationError
	if errors.As(err, &validationErr) {
		code := validationErr.PrimaryCode()
		writeError(w, statusForAppErrorCode(code), code, "project template validation failed", map[string]any{"issues": validationErr.Issues})
		return
	}
	var runtimeErr app.RuntimeError
	if errors.As(err, &runtimeErr) && runtimeErr.Code == "project_template_snapshot_invalid" && strings.Contains(runtimeErr.Message, "maximum JSON size") {
		writeError(w, http.StatusRequestEntityTooLarge, runtimeErr.Code, runtimeErr.Message, nil)
		return
	}
	if code := projecttemplate.ErrorCode(err); code != "" {
		status := statusForAppErrorCode(code)
		if code == "project_template_snapshot_invalid" && strings.Contains(err.Error(), "maximum JSON size") {
			status = http.StatusRequestEntityTooLarge
		}
		writeError(w, status, code, err.Error(), nil)
		return
	}
	writeAppError(w, err)
}

func (s *Server) projectTemplateScopedService(w http.ResponseWriter, r *http.Request, capability string, permission app.Permission, projectRef string) (*app.Service, bool) {
	workspace := strings.TrimSpace(r.URL.Query().Get("workspace"))
	if workspace == "" {
		writeError(w, http.StatusBadRequest, authz.CodeWorkspaceRequired, "workspace query parameter is required", nil)
		return nil, false
	}
	scoped, _, err := s.scopedServiceWithWorkspace(r, capability, permission, workspace, projectRef)
	if err != nil {
		writeAppError(w, err)
		return nil, false
	}
	return scoped, true
}

func (s *Server) requireProjectTemplateCapability(w http.ResponseWriter, r *http.Request, capability string, permission app.Permission, projectRef string) bool {
	_, ok := s.projectTemplateScopedService(w, r, capability, permission, projectRef)
	return ok
}

func (s *Server) requireCaptureCapabilities(w http.ResponseWriter, r *http.Request, input app.CaptureInput) bool {
	if len(input.Selection.TaskRefs) > 0 || len(input.Selection.SeriesRefs) > 0 {
		if !s.requireProjectTemplateCapability(w, r, auth.ScopeTaskRead, app.PermissionTaskRead, input.SourceProjectRef) {
			return false
		}
	}
	if len(input.Selection.ConfigKeys) > 0 {
		if !s.requireProjectTemplateCapability(w, r, auth.ScopeConfigRead, app.PermissionProjectConfigRead, input.SourceProjectRef) {
			return false
		}
	}
	if len(input.Selection.AutomationRuleIDs) > 0 {
		if !s.requireProjectTemplateCapability(w, r, auth.ScopeHookRead, app.PermissionHookRead, input.SourceProjectRef) {
			return false
		}
	}
	return true
}

func (s *Server) requireTemplateDetailCapabilities(w http.ResponseWriter, r *http.Request, view app.ProjectTemplateView) bool {
	if view.Snapshot == nil {
		return true
	}
	counts := app.ComponentCounts{Configs: len(view.Snapshot.Configs), Tasks: len(view.Snapshot.Tasks), Series: len(view.Snapshot.Series), Automations: len(view.Snapshot.Automations)}
	if counts.Tasks > 0 || counts.Series > 0 {
		if !s.requireProjectTemplateCapability(w, r, auth.ScopeTaskRead, app.PermissionTaskRead, "") {
			return false
		}
	}
	if counts.Configs > 0 && !s.requireProjectTemplateCapability(w, r, auth.ScopeConfigRead, app.PermissionProjectConfigRead, "") {
		return false
	}
	return counts.Automations == 0 || s.requireProjectTemplateCapability(w, r, auth.ScopeHookRead, app.PermissionHookRead, "")
}

func (s *Server) requireInstantiateCapabilities(w http.ResponseWriter, r *http.Request, counts app.ComponentCounts) bool {
	if (counts.Tasks > 0 || counts.Series > 0) && !s.requireProjectTemplateCapability(w, r, auth.ScopeTaskWrite, app.PermissionTaskWrite, "") {
		return false
	}
	if counts.Configs > 0 && !s.requireProjectTemplateCapability(w, r, auth.ScopeConfigWrite, app.PermissionProjectConfigWrite, "") {
		return false
	}
	return counts.Automations == 0 || s.requireProjectTemplateCapability(w, r, auth.ScopeHookWrite, app.PermissionHookWrite, "")
}

func projectTemplateCandidateCapability(kind string) (string, app.Permission) {
	switch strings.TrimSpace(kind) {
	case "task", "series":
		return auth.ScopeTaskRead, app.PermissionTaskRead
	case "config":
		return auth.ScopeConfigRead, app.PermissionProjectConfigRead
	case "automation":
		return auth.ScopeHookRead, app.PermissionHookRead
	default:
		return "", app.PermissionProjectRead
	}
}

func projectTemplatePageParams(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	limit, offset := 0, 0
	if len(queryStringList(r.URL.Query()["ref"])) > 100 {
		writeError(w, http.StatusUnprocessableEntity, "project_template_candidate_invalid", "ref must contain at most 100 values", nil)
		return 0, 0, false
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			writeError(w, http.StatusUnprocessableEntity, "project_template_candidate_invalid", "limit must be an integer from 1 to 100", nil)
			return 0, 0, false
		}
		limit = value
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			writeError(w, http.StatusUnprocessableEntity, "project_template_candidate_invalid", "offset must be a non-negative integer", nil)
			return 0, 0, false
		}
		offset = value
	}
	return limit, offset, true
}

func queryStringList(values []string) []string {
	out := []string{}
	for _, value := range values {
		for _, item := range strings.Split(value, ",") {
			if item = strings.TrimSpace(item); item != "" {
				out = append(out, item)
			}
		}
	}
	return out
}

func projectTemplatePageToResponse(page app.ProjectTemplatePage) projectTemplatePageResponse {
	items := make([]projectTemplateSummaryResponse, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, projectTemplateSummaryToResponse(item))
	}
	return projectTemplatePageResponse{Items: items, Total: page.Total, Limit: page.Limit, Offset: page.Offset}
}

func projectTemplateDetailToResponse(view app.ProjectTemplateView) projectTemplateDetailResponse {
	versions := make([]projectTemplateSnapshotSummaryResponse, 0, len(view.Versions))
	for _, version := range view.Versions {
		versions = append(versions, projectTemplateSnapshotSummaryToResponse(version))
	}
	return projectTemplateDetailResponse{Template: projectTemplateSummaryToResponse(view.Template), Snapshot: view.Snapshot, Versions: versions}
}

func projectTemplateSummaryToResponse(view app.ProjectTemplateSummaryView) projectTemplateSummaryResponse {
	out := projectTemplateSummaryResponse{
		ID: view.ID, Key: view.Key, Name: view.Name, Description: view.Description, Status: view.Status,
		CreatedBy: task.ActorInfoToJSON(view.CreatedBy), CreatedAt: view.CreatedAt, ModifiedAt: view.ModifiedAt, ArchivedAt: view.ArchivedAt,
	}
	if view.CurrentSnapshot != nil {
		current := projectTemplateSnapshotSummaryToResponse(*view.CurrentSnapshot)
		out.CurrentSnapshot = &current
	}
	return out
}

func projectTemplateSnapshotSummaryToResponse(view app.ProjectTemplateSnapshotSummaryView) projectTemplateSnapshotSummaryResponse {
	return projectTemplateSnapshotSummaryResponse{
		ID: view.ID, Version: view.Version, Hash: view.Hash, SourceProjectID: view.SourceProjectID,
		Counts: view.Counts, RequiredSecretKeys: append([]string{}, view.RequiredSecretKeys...),
		CreatedBy: task.ActorInfoToJSON(view.CreatedBy), CreatedAt: view.CreatedAt,
	}
}
