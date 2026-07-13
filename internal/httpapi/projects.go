package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

type addProjectRequest struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type modifyProjectRequest struct {
	Slug        *string `json:"slug,omitempty"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

type configValueRequest struct {
	Value string `json:"value"`
}

type projectAnnotationInfoResponse struct {
	ID        string             `json:"id"`
	ProjectID string             `json:"project_id"`
	Entry     int64              `json:"entry"`
	Content   string             `json:"content"`
	CreatedBy task.JSONActorInfo `json:"created_by"`
	CreatedAt int64              `json:"created_at"`
}

type projectSummaryTaskRefResponse struct {
	UUID     string `json:"uuid"`
	TaskSlug string `json:"task_slug,omitempty"`
	Title    string `json:"title"`
	Label    string `json:"label"`
}

type projectSummaryWorkloadResponse struct {
	User              *task.JSONUserInfo `json:"user,omitempty"`
	Label             string             `json:"label"`
	OpenCount         int                `json:"open_count"`
	OverdueCount      int                `json:"overdue_count"`
	HighPriorityCount int                `json:"high_priority_count"`
}

type projectTaskSummaryResponse struct {
	OverdueCount          int                             `json:"overdue_count"`
	OverdueRefs           []projectSummaryTaskRefResponse `json:"overdue_refs"`
	HighPriorityOpenCount int                             `json:"high_priority_open_count"`
	HighPriorityOpenRefs  []projectSummaryTaskRefResponse `json:"high_priority_open_refs"`
	WaitReadyCount        int                             `json:"wait_ready_count"`
	WaitReadyRefs         []projectSummaryTaskRefResponse `json:"wait_ready_refs"`
	UnassignedOpenCount   int                             `json:"unassigned_open_count"`
	UnassignedOpenRefs    []projectSummaryTaskRefResponse `json:"unassigned_open_refs"`
	Workload              []projectSummaryWorkloadResponse `json:"workload"`
	SeriesMetrics         projectSeriesMetricsResponse    `json:"series_metrics"`
}

type projectSeriesMetricsResponse struct {
	RecurringSeriesCount            int `json:"recurring_series_count"`
	ActiveRecurringSeriesCount      int `json:"active_recurring_series_count"`
	OpenRecurringOccurrenceCount    int `json:"open_recurring_occurrence_count"`
	OverdueRecurringOccurrenceCount int `json:"overdue_recurring_occurrence_count"`
}

type projectResponse struct {
	ID                string                          `json:"id"`
	WorkspaceID       string                          `json:"workspace_id"`
	Slug              string                          `json:"slug"`
	Name              string                          `json:"name"`
	Description       string                          `json:"description,omitempty"`
	Status            string                          `json:"status"`
	TaskCount         int                             `json:"task_count"`
	PendingCount      int                             `json:"pending_count"`
	CompletedCount    int                             `json:"completed_count"`
	CreatedAt         int64                           `json:"created_at"`
	ModifiedAt        int64                           `json:"modified_at"`
	ArchivedAt        *int64                          `json:"archived_at,omitempty"`
	RecentAnnotations []projectAnnotationInfoResponse `json:"recent_annotations,omitempty"`
}

func (s *Server) handleProjectList(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, auth.ScopeProjectRead, app.PermissionProjectRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	statusFilter := r.URL.Query().Get("status")
	if statusFilter == "" && r.URL.Query().Get("all") == "true" {
		statusFilter = "all"
	}
	rows, err := scoped.ListProjectsByStatus(statusFilter)
	if err != nil {
		writeAppError(w, err)
		return
	}
	out := make([]projectResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectResponseFromView(row))
	}
	writeSuccess(w, http.StatusOK, out, nil)
}

func (s *Server) handleProjectAdd(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, auth.ScopeProjectWrite, app.PermissionProjectManage, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	var req addProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	created, err := scoped.AddProject(app.AddProjectInput{
		Slug:        strings.TrimSpace(req.Slug),
		Name:        strings.TrimSpace(req.Name),
		Description: strings.TrimSpace(req.Description),
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, projectResponseFromView(created), nil)
}

func (s *Server) handleProjectInfo(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, auth.ScopeProjectRead, app.PermissionProjectRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	project, err := scoped.ProjectInfo(chi.URLParam(r, "projectRef"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, projectResponseFromView(project), nil)
}

func (s *Server) handleProjectModify(w http.ResponseWriter, r *http.Request) {
	var req modifyProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	ref := chi.URLParam(r, "projectRef")
	scoped, _, err := s.scopedService(r, auth.ScopeProjectWrite, app.PermissionProjectManage, ref)
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := scoped.ModifyProject(ref, app.ModifyProjectInput{Slug: req.Slug, Name: req.Name, Description: req.Description}); err != nil {
		writeAppError(w, err)
		return
	}
	project, err := scoped.ProjectInfo(ref)
	if err != nil && req.Slug != nil {
		project, err = scoped.ProjectInfo(*req.Slug)
	}
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, projectResponseFromView(project), nil)
}

func (s *Server) handleProjectArchive(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "projectRef")
	scoped, _, err := s.scopedService(r, auth.ScopeProjectWrite, app.PermissionProjectManage, ref)
	if err != nil {
		writeAppError(w, err)
		return
	}
	project, err := scoped.ArchiveProject(ref)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, projectResponseFromView(project), nil)
}

func (s *Server) handleProjectTransition(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	ref := chi.URLParam(r, "projectRef")
	scoped, _, err := s.scopedService(r, auth.ScopeProjectWrite, app.PermissionProjectManage, ref)
	if err != nil {
		writeAppError(w, err)
		return
	}
	project, err := scoped.TransitionProject(ref, strings.TrimSpace(req.Status))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, projectResponseFromView(project), nil)
}

func (s *Server) handleProjectConfigList(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "projectRef")
	scoped, _, err := s.scopedService(r, auth.ScopeProjectRead, app.PermissionProjectConfigRead, ref)
	if err != nil {
		writeAppError(w, err)
		return
	}
	values, err := scoped.ProjectConfigList(ref)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, values, nil)
}

func (s *Server) handleProjectConfigGet(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "projectRef")
	scoped, _, err := s.scopedService(r, auth.ScopeProjectRead, app.PermissionProjectConfigRead, ref)
	if err != nil {
		writeAppError(w, err)
		return
	}
	value, ok, err := scoped.ProjectConfigGet(ref, chi.URLParam(r, "key"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "config_not_found", "config not found", nil)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]string{"value": value}, nil)
}

func (s *Server) handleProjectConfigSet(w http.ResponseWriter, r *http.Request) {
	var req configValueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	ref := chi.URLParam(r, "projectRef")
	scoped, _, err := s.scopedService(r, auth.ScopeProjectWrite, app.PermissionProjectConfigWrite, ref)
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := scoped.ProjectConfigSet(ref, chi.URLParam(r, "key"), req.Value); err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]string{"value": req.Value}, nil)
}

func (s *Server) handleProjectConfigUnset(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "projectRef")
	scoped, _, err := s.scopedService(r, auth.ScopeProjectWrite, app.PermissionProjectConfigWrite, ref)
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := scoped.ProjectConfigUnset(ref, chi.URLParam(r, "key")); err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]bool{"ok": true}, nil)
}

func projectResponseFromView(view app.ProjectView) projectResponse {
	resp := projectResponse{
		ID:             view.ID,
		WorkspaceID:    view.WorkspaceID,
		Slug:           view.Slug,
		Name:           view.Name,
		Description:    view.Description,
		Status:         view.Status,
		TaskCount:      view.TaskCount,
		PendingCount:   view.PendingCount,
		CompletedCount: view.CompletedCount,
		CreatedAt:      view.CreatedAt,
		ModifiedAt:     view.ModifiedAt,
		ArchivedAt:     view.ArchivedAt,
	}
	if len(view.RecentAnnotations) > 0 {
		resp.RecentAnnotations = make([]projectAnnotationInfoResponse, len(view.RecentAnnotations))
		for i, a := range view.RecentAnnotations {
			resp.RecentAnnotations[i] = projectAnnotationInfoResponse{
				ID:        a.ID,
				ProjectID: a.ProjectID,
				Entry:     a.Entry,
				Content:   a.Content,
				CreatedBy: task.ActorInfoToJSON(a.CreatedBy),
				CreatedAt: a.CreatedAt,
			}
		}
	}
	return resp
}

type addProjectAnnotationRequest struct {
	Content string `json:"content"`
}

type projectAnnotationResponse struct {
	ID        string             `json:"id"`
	ProjectID string             `json:"project_id"`
	Entry     int64              `json:"entry"`
	Content   string             `json:"content"`
	CreatedBy task.JSONActorInfo `json:"created_by"`
	CreatedAt int64              `json:"created_at"`
}

type timelineEntryResponse struct {
	SourceType  string             `json:"source_type"`
	SourceID    string             `json:"source_id"`
	SourceLabel string             `json:"source_label"`
	Entry       int64              `json:"entry"`
	Content     string             `json:"content"`
	CreatedBy   task.JSONActorInfo `json:"created_by"`
}

func (s *Server) handleProjectAnnotationAdd(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "projectRef")
	scoped, _, err := s.scopedService(r, auth.ScopeProjectWrite, app.PermissionProjectManage, ref)
	if err != nil {
		writeAppError(w, err)
		return
	}
	var req addProjectAnnotationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	annotation, err := scoped.ProjectAnnotate(ref, req.Content)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, projectAnnotationToJSON(annotation), nil)
}

func (s *Server) handleProjectAnnotationList(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "projectRef")
	scoped, _, err := s.scopedService(r, auth.ScopeProjectRead, app.PermissionProjectRead, ref)
	if err != nil {
		writeAppError(w, err)
		return
	}
	annotations, err := scoped.ProjectAnnotations(ref)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, projectAnnotationsToJSON(annotations), nil)
}

func (s *Server) handleProjectAnnotationDelete(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "projectRef")
	annotationID := chi.URLParam(r, "annotationID")
	scoped, _, err := s.scopedService(r, auth.ScopeProjectWrite, app.PermissionProjectManage, ref)
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := scoped.ProjectDenotate(ref, annotationID); err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]bool{"ok": true}, nil)
}

func (s *Server) handleProjectTimeline(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "projectRef")
	scoped, _, err := s.scopedService(r, auth.ScopeProjectRead, app.PermissionProjectRead, ref)
	if err != nil {
		writeAppError(w, err)
		return
	}
	opts := app.TimelineOptions{Limit: 50}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "api_bad_limit", "invalid limit", nil)
			return
		}
		opts.Limit = parsed
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			writeError(w, http.StatusBadRequest, "api_bad_offset", "invalid offset", nil)
			return
		}
		opts.Offset = parsed
	}
	entries, err := scoped.ProjectTimeline(ref, opts)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, timelineEntriesToJSON(entries), nil)
}

func (s *Server) handleProjectTaskSummary(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "projectRef")
	scoped, _, err := s.scopedService(r, auth.ScopeTaskRead, app.PermissionTaskRead, ref)
	if err != nil {
		writeAppError(w, err)
		return
	}
	summary, err := scoped.ProjectTaskSummary(ref)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, projectTaskSummaryToJSON(summary), nil)
}

func projectTaskSummaryToJSON(summary app.ProjectTaskSummaryView) projectTaskSummaryResponse {
	resp := projectTaskSummaryResponse{
		OverdueCount:          summary.OverdueCount,
		OverdueRefs:           projectTaskRefResponses(summary.OverdueRefs),
		HighPriorityOpenCount: summary.HighPriorityOpenCount,
		HighPriorityOpenRefs:  projectTaskRefResponses(summary.HighPriorityOpenRefs),
		WaitReadyCount:        summary.WaitReadyCount,
		WaitReadyRefs:         projectTaskRefResponses(summary.WaitReadyRefs),
		UnassignedOpenCount:   summary.UnassignedOpenCount,
		UnassignedOpenRefs:    projectTaskRefResponses(summary.UnassignedOpenRefs),
		Workload:              make([]projectSummaryWorkloadResponse, 0, len(summary.Workload)),
		SeriesMetrics: projectSeriesMetricsResponse{
			RecurringSeriesCount:            summary.SeriesMetrics.RecurringSeriesCount,
			ActiveRecurringSeriesCount:      summary.SeriesMetrics.ActiveRecurringSeriesCount,
			OpenRecurringOccurrenceCount:    summary.SeriesMetrics.OpenRecurringOccurrenceCount,
			OverdueRecurringOccurrenceCount: summary.SeriesMetrics.OverdueRecurringOccurrenceCount,
		},
	}
	for _, row := range summary.Workload {
		var user *task.JSONUserInfo
		if row.User != nil {
			jui := task.UserInfoToJSON(*row.User)
			user = &jui
		}
		resp.Workload = append(resp.Workload, projectSummaryWorkloadResponse{
			User:              user,
			Label:             row.Label,
			OpenCount:         row.OpenCount,
			OverdueCount:      row.OverdueCount,
			HighPriorityCount: row.HighPriorityCount,
		})
	}
	return resp
}

func projectTaskRefResponses(refs []app.ProjectSummaryTaskRefView) []projectSummaryTaskRefResponse {
	out := make([]projectSummaryTaskRefResponse, 0, len(refs))
	for _, ref := range refs {
		out = append(out, projectSummaryTaskRefResponse{
			UUID:     ref.UUID,
			TaskSlug: ref.TaskSlug,
			Title:    ref.Title,
			Label:    ref.Label,
		})
	}
	return out
}

func projectAnnotationToJSON(a app.ProjectAnnotationInfo) projectAnnotationResponse {
	return projectAnnotationResponse{
		ID:        a.ID,
		ProjectID: a.ProjectID,
		Entry:     a.Entry,
		Content:   a.Content,
		CreatedBy: task.ActorInfoToJSON(a.CreatedBy),
		CreatedAt: a.CreatedAt,
	}
}

func projectAnnotationsToJSON(annotations []app.ProjectAnnotationInfo) []projectAnnotationResponse {
	out := make([]projectAnnotationResponse, len(annotations))
	for i, a := range annotations {
		out[i] = projectAnnotationToJSON(a)
	}
	return out
}

func timelineEntriesToJSON(entries []app.TimelineEntry) []timelineEntryResponse {
	out := make([]timelineEntryResponse, len(entries))
	for i, e := range entries {
		out[i] = timelineEntryResponse{
			SourceType:  e.SourceType,
			SourceID:    e.SourceID,
			SourceLabel: e.SourceLabel,
			Entry:       e.Entry,
			Content:     e.Content,
			CreatedBy:   task.ActorInfoToJSON(e.CreatedBy),
		}
	}
	return out
}
