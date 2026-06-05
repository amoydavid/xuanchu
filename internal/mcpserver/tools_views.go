package mcpserver

import (
	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/task"
)

type workspaceView struct {
	ID              string  `json:"id"`
	Slug            string  `json:"slug"`
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	Visibility  string             `json:"visibility"`
	CreatedBy   *task.JSONUserInfo `json:"created_by,omitempty"`
	ArchivedAt  *int64             `json:"archived_at,omitempty"`
	Role        string             `json:"role"`
	Active      bool               `json:"active"`
	CreatedAt   int64              `json:"created_at"`
	ModifiedAt  int64              `json:"modified_at"`
}

type projectView struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"`
	TaskCount   int    `json:"task_count"`
	CreatedAt   int64  `json:"created_at"`
	ModifiedAt  int64  `json:"modified_at"`
	ArchivedAt  *int64 `json:"archived_at,omitempty"`
}

type contextView struct {
	Name       string `json:"name"`
	Filter     string `json:"filter"`
	Active     bool   `json:"active"`
	CreatedAt  int64  `json:"created_at,omitempty"`
	ModifiedAt int64  `json:"modified_at,omitempty"`
}

type memberView struct {
	UserID     string  `json:"user_id"`
	Name       string  `json:"name"`
	Email      *string `json:"email,omitempty"`
	Role       string  `json:"role"`
	JoinedAt   int64   `json:"joined_at"`
	ModifiedAt int64   `json:"modified_at"`
}

type userView struct {
	ID                 string           `json:"id"`
	Name               string           `json:"name"`
	Email              *string          `json:"email,omitempty"`
	DefaultWorkspaceID *string          `json:"default_workspace_id,omitempty"`
	ExternalIDs        []externalIDView `json:"external_ids,omitempty"`
	Active             bool             `json:"active"`
	CreatedAt          int64            `json:"created_at"`
	ModifiedAt         int64            `json:"modified_at"`
}

type externalIDView struct {
	Provider   string `json:"provider"`
	ExternalID string `json:"external_id"`
}

type annotationView struct {
	ID        string             `json:"id"`
	ProjectID string             `json:"project_id"`
	Entry     int64              `json:"entry"`
	Content   string             `json:"content"`
	CreatedBy task.JSONUserInfo  `json:"created_by"`
	CreatedAt int64              `json:"created_at"`
}

type taskLinkView struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"`
	URL       string            `json:"url"`
	Title     string            `json:"title,omitempty"`
	CreatedAt int64             `json:"created_at"`
	CreatedBy task.JSONUserInfo `json:"created_by"`
}

func workspaceViewFromApp(row app.WorkspaceView) workspaceView {
	role := string(row.Role)
	if role == "" {
		role = string(app.RoleViewer)
	}
	var createdBy *task.JSONUserInfo
	if row.CreatedBy != nil {
		jui := task.UserInfoToJSON(*row.CreatedBy)
		createdBy = &jui
	}
	return workspaceView{
		ID:          row.ID,
		Slug:        row.Slug,
		Name:        row.Name,
		Description: row.Description,
		Visibility:  row.Visibility,
		CreatedBy:   createdBy,
		ArchivedAt:  row.ArchivedAt,
		Role:        role,
		Active:      row.Active,
		CreatedAt:   row.CreatedAt,
		ModifiedAt:  row.ModifiedAt,
	}
}

func workspaceViewsFromApp(rows []app.WorkspaceView) []workspaceView {
	out := make([]workspaceView, 0, len(rows))
	for _, row := range rows {
		out = append(out, workspaceViewFromApp(row))
	}
	return out
}

func projectViewFromApp(row app.ProjectView) projectView {
	return projectView{
		ID:          row.ID,
		WorkspaceID: row.WorkspaceID,
		Slug:        row.Slug,
		Name:        row.Name,
		Description: row.Description,
		Status:      row.Status,
		TaskCount:   row.TaskCount,
		CreatedAt:   row.CreatedAt,
		ModifiedAt:  row.ModifiedAt,
		ArchivedAt:  row.ArchivedAt,
	}
}

func projectViewsFromApp(rows []app.ProjectView) []projectView {
	out := make([]projectView, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectViewFromApp(row))
	}
	return out
}

func memberViewFromApp(row app.MemberView) memberView {
	return memberView{
		UserID:     row.UserID,
		Name:       row.Name,
		Email:      row.Email,
		Role:       string(row.Role),
		JoinedAt:   row.JoinedAt,
		ModifiedAt: row.ModifiedAt,
	}
}

func memberViewsFromApp(rows []app.MemberView) []memberView {
	out := make([]memberView, 0, len(rows))
	for _, row := range rows {
		out = append(out, memberViewFromApp(row))
	}
	return out
}

func userViewFromApp(row app.UserView) userView {
	extIDs := make([]externalIDView, 0, len(row.ExternalIDs))
	for _, eid := range row.ExternalIDs {
		extIDs = append(extIDs, externalIDView{Provider: eid.Provider, ExternalID: eid.ExternalID})
	}
	return userView{
		ID:                 row.ID,
		Name:               row.Name,
		Email:              row.Email,
		DefaultWorkspaceID: row.DefaultWorkspaceID,
		ExternalIDs:        extIDs,
		Active:             row.Active,
		CreatedAt:          row.CreatedAt,
		ModifiedAt:         row.ModifiedAt,
	}
}

func userViewsFromApp(rows []app.UserView) []userView {
	out := make([]userView, 0, len(rows))
	for _, row := range rows {
		out = append(out, userViewFromApp(row))
	}
	return out
}

func annotationViewFromApp(row app.ProjectAnnotationInfo) annotationView {
	return annotationView{
		ID:        row.ID,
		ProjectID: row.ProjectID,
		Entry:     row.Entry,
		Content:   row.Content,
		CreatedBy: task.UserInfoToJSON(row.CreatedBy),
		CreatedAt: row.CreatedAt,
	}
}

func annotationViewsFromApp(rows []app.ProjectAnnotationInfo) []annotationView {
	out := make([]annotationView, 0, len(rows))
	for _, row := range rows {
		out = append(out, annotationViewFromApp(row))
	}
	return out
}

func taskLinkViewFromApp(row task.TaskLinkInfo) taskLinkView {
	return taskLinkView{
		ID:        row.ID,
		Type:      row.Type,
		URL:       row.URL,
		Title:     row.Title,
		CreatedAt: row.CreatedAt,
		CreatedBy: task.UserInfoToJSON(row.CreatedBy),
	}
}
