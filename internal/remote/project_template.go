package remote

import (
	"context"
	"net/url"
	"strconv"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

type projectTemplateSnapshotSummaryDTO struct {
	ID                 string                               `json:"id"`
	Version            int64                                `json:"version"`
	Hash               string                               `json:"hash"`
	SourceProjectID    string                               `json:"source_project_id"`
	Counts             app.ComponentCounts                  `json:"counts"`
	RequiredSecretKeys []string                             `json:"required_secret_keys"`
	ConfigInputs       []app.ProjectTemplateConfigInputView `json:"config_inputs"`
	CreatedBy          task.JSONActorInfo                   `json:"created_by"`
	CreatedAt          int64                                `json:"created_at"`
}

type projectTemplateSummaryDTO struct {
	ID              string                             `json:"id"`
	Key             string                             `json:"key"`
	Name            string                             `json:"name"`
	Description     string                             `json:"description"`
	Status          string                             `json:"status"`
	CurrentSnapshot *projectTemplateSnapshotSummaryDTO `json:"current_snapshot,omitempty"`
	CreatedBy       task.JSONActorInfo                 `json:"created_by"`
	CreatedAt       int64                              `json:"created_at"`
	ModifiedAt      int64                              `json:"modified_at"`
	ArchivedAt      *int64                             `json:"archived_at,omitempty"`
}

type projectTemplatePageDTO struct {
	Items  []projectTemplateSummaryDTO `json:"items"`
	Total  int64                       `json:"total"`
	Limit  int                         `json:"limit"`
	Offset int                         `json:"offset"`
}

type currentProjectTemplateInstantiateRequest struct {
	app.CurrentSnapshotInstantiateInput
	CurrentOnly bool `json:"current_only"`
}

type projectTemplateInstantiateDTO struct {
	Project projectDTO          `json:"project"`
	Counts  app.ComponentCounts `json:"counts"`
}

// ListProjectTemplatesForInstantiation 只读取 active Template 的 current Snapshot 摘要。
func (c *Client) ListProjectTemplatesForInstantiation(ctx context.Context, workspace, q string, limit, offset int) (app.ProjectTemplatePage, error) {
	values := url.Values{"status": []string{"active"}}
	if workspace != "" {
		values.Set("workspace", workspace)
	}
	if q != "" {
		values.Set("q", q)
	}
	if limit > 0 {
		values.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		values.Set("offset", strconv.Itoa(offset))
	}
	var envelope apiEnvelope[projectTemplatePageDTO]
	if err := c.get(ctx, "/api/v1/project-templates", values, &envelope); err != nil {
		return app.ProjectTemplatePage{}, err
	}
	return projectTemplatePageDTOToView(envelope.Data), nil
}

// InstantiateCurrentProjectTemplate 固定 current_only 协议位，服务端在同一写事务内拒绝 Snapshot 漂移。
func (c *Client) InstantiateCurrentProjectTemplate(ctx context.Context, workspace, templateRef string, input app.CurrentSnapshotInstantiateInput) (app.InstantiateResult, error) {
	path := "/api/v1/project-templates/" + url.PathEscape(templateRef) + "/instantiate"
	if workspace != "" {
		path += "?workspace=" + url.QueryEscape(workspace)
	}
	var envelope apiEnvelope[projectTemplateInstantiateDTO]
	if err := c.post(ctx, path, currentProjectTemplateInstantiateRequest{CurrentSnapshotInstantiateInput: input, CurrentOnly: true}, &envelope); err != nil {
		return app.InstantiateResult{}, err
	}
	return app.InstantiateResult{Project: projectDTOToView(envelope.Data.Project), Counts: envelope.Data.Counts}, nil
}

func projectTemplatePageDTOToView(page projectTemplatePageDTO) app.ProjectTemplatePage {
	items := make([]app.ProjectTemplateSummaryView, 0, len(page.Items))
	for _, item := range page.Items {
		view := app.ProjectTemplateSummaryView{
			ID: item.ID, Key: item.Key, Name: item.Name, Description: item.Description, Status: item.Status,
			CreatedBy: task.ActorInfoFromJSON(item.CreatedBy), CreatedAt: item.CreatedAt, ModifiedAt: item.ModifiedAt, ArchivedAt: item.ArchivedAt,
		}
		if item.CurrentSnapshot != nil {
			current := app.ProjectTemplateSnapshotSummaryView{
				ID: item.CurrentSnapshot.ID, Version: item.CurrentSnapshot.Version, Hash: item.CurrentSnapshot.Hash,
				SourceProjectID: item.CurrentSnapshot.SourceProjectID, Counts: item.CurrentSnapshot.Counts,
				RequiredSecretKeys: append([]string{}, item.CurrentSnapshot.RequiredSecretKeys...),
				ConfigInputs:       append([]app.ProjectTemplateConfigInputView{}, item.CurrentSnapshot.ConfigInputs...),
				CreatedBy:          task.ActorInfoFromJSON(item.CurrentSnapshot.CreatedBy), CreatedAt: item.CurrentSnapshot.CreatedAt,
			}
			view.CurrentSnapshot = &current
		}
		items = append(items, view)
	}
	return app.ProjectTemplatePage{Items: items, Total: page.Total, Limit: page.Limit, Offset: page.Offset}
}
