package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/task"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ProjectTemplateListInput struct {
	Workspace string `json:"workspace" jsonschema:"workspace slug or UUID; required for every template call"`
	Q         string `json:"q,omitempty" jsonschema:"optional template key or name search text"`
	Limit     int    `json:"limit,omitempty" jsonschema:"max active templates to return"`
	Offset    int    `json:"offset,omitempty" jsonschema:"number of active templates to skip"`
}

type ProjectTemplateInstantiateInput struct {
	Workspace            string             `json:"workspace" jsonschema:"workspace slug or UUID; required for every template call"`
	Template             string             `json:"template" jsonschema:"template stable key or UUID"`
	SnapshotID           string             `json:"snapshot_id" jsonschema:"current snapshot UUID returned by project_template_list"`
	ExpectedSnapshotHash string             `json:"expected_snapshot_hash" jsonschema:"current snapshot hash returned by project_template_list"`
	ProjectSlug          string             `json:"project_slug" jsonschema:"unique slug for the new project"`
	ProjectName          string             `json:"project_name" jsonschema:"human-readable name for the new project"`
	StartDate            string             `json:"start_date" jsonschema:"project start date in YYYY-MM-DD format"`
	Description          *string            `json:"description,omitempty" jsonschema:"optional description override for the new project"`
	ConfigInputs         map[string]string  `json:"config_inputs,omitempty" jsonschema:"values for prompt config fields declared by the current snapshot; values are never returned"`
	SecretInputs         map[string]string  `json:"secret_inputs,omitempty" jsonschema:"secret values required by the current snapshot; values are never returned"`
	AssigneeReplacements map[string]*string `json:"assignee_replacements,omitempty" jsonschema:"source user ID to target user ID; null removes the assignee"`
}

type ProjectTemplateToolError struct {
	Code    string                     `json:"code"`
	Message string                     `json:"message"`
	Issues  []app.ProjectTemplateIssue `json:"issues,omitempty"`
}

type projectTemplateSnapshotSummaryMCPView struct {
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

type projectTemplateSummaryMCPView struct {
	ID              string                                 `json:"id"`
	Key             string                                 `json:"key"`
	Name            string                                 `json:"name"`
	Description     string                                 `json:"description"`
	Status          string                                 `json:"status"`
	CurrentSnapshot *projectTemplateSnapshotSummaryMCPView `json:"current_snapshot,omitempty"`
	CreatedBy       task.JSONActorInfo                     `json:"created_by"`
	CreatedAt       int64                                  `json:"created_at"`
	ModifiedAt      int64                                  `json:"modified_at"`
	ArchivedAt      *int64                                 `json:"archived_at,omitempty"`
}

type projectTemplatePageMCPView struct {
	Items  []projectTemplateSummaryMCPView `json:"items"`
	Total  int64                           `json:"total"`
	Limit  int                             `json:"limit"`
	Offset int                             `json:"offset"`
}

func registerProjectTemplateTools(s *mcp.Server, opts Options) {
	addTool(s, opts, &mcp.Tool{Name: "project_template_list", Description: "List active project templates and their current snapshot summaries; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectTemplateListInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		if err := requireProjectTemplateWorkspace(in.Workspace); err != nil {
			return businessErrorWithEnvelope(err)
		}
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace}, "project:read", app.PermissionProjectRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		page, err := svc.ListProjectTemplatesForInstantiation(app.TemplateInstantiationListInput{Q: in.Q, Limit: in.Limit, Offset: in.Offset})
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(projectTemplatePageForMCP(page), fmt.Sprintf("%d active project template(s)", page.Total))
	})

	addTool(s, opts, &mcp.Tool{Name: "project_template_instantiate", Description: "Instantiate a project from an active template's current snapshot; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectTemplateInstantiateInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		if err := requireProjectTemplateWorkspace(in.Workspace); err != nil {
			return businessErrorWithEnvelope(err)
		}
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace}, "project:write", app.PermissionProjectManage)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		result, err := svc.InstantiateCurrentProjectTemplate(in.Template, app.CurrentSnapshotInstantiateInput{
			SnapshotID: in.SnapshotID, ExpectedHash: in.ExpectedSnapshotHash,
			ProjectSlug: in.ProjectSlug, ProjectName: in.ProjectName, Description: in.Description,
			StartDate: in.StartDate, ConfigInputs: in.ConfigInputs, SecretInputs: in.SecretInputs, AssigneeReplacements: in.AssigneeReplacements,
		})
		if err != nil {
			return projectTemplateErrorWithEnvelope(err)
		}
		data := map[string]any{"project": projectViewFromApp(result.Project), "counts": result.Counts}
		return successWithEnvelope(data, "created project "+result.Project.Slug+" from current project template snapshot")
	})
}

func projectTemplatePageForMCP(page app.ProjectTemplatePage) projectTemplatePageMCPView {
	items := make([]projectTemplateSummaryMCPView, 0, len(page.Items))
	for _, item := range page.Items {
		view := projectTemplateSummaryMCPView{
			ID: item.ID, Key: item.Key, Name: item.Name, Description: item.Description, Status: item.Status,
			CreatedBy: task.ActorInfoToJSON(item.CreatedBy), CreatedAt: item.CreatedAt, ModifiedAt: item.ModifiedAt, ArchivedAt: item.ArchivedAt,
		}
		if item.CurrentSnapshot != nil {
			current := item.CurrentSnapshot
			view.CurrentSnapshot = &projectTemplateSnapshotSummaryMCPView{
				ID: current.ID, Version: current.Version, Hash: current.Hash, SourceProjectID: current.SourceProjectID,
				Counts: current.Counts, RequiredSecretKeys: append([]string{}, current.RequiredSecretKeys...),
				ConfigInputs: append([]app.ProjectTemplateConfigInputView{}, current.ConfigInputs...),
				CreatedBy:    task.ActorInfoToJSON(current.CreatedBy), CreatedAt: current.CreatedAt,
			}
		}
		items = append(items, view)
	}
	return projectTemplatePageMCPView{Items: items, Total: page.Total, Limit: page.Limit, Offset: page.Offset}
}

func requireProjectTemplateWorkspace(workspace string) error {
	if strings.TrimSpace(workspace) == "" {
		return app.RuntimeError{Code: "workspace_required", Message: "workspace is required for every project template call"}
	}
	return nil
}

func projectTemplateErrorWithEnvelope(err error) (*mcp.CallToolResult, ToolEnvelope, error) {
	var validationErr app.ProjectTemplateValidationError
	if !errors.As(err, &validationErr) {
		return businessErrorWithEnvelope(err)
	}
	code := validationErr.PrimaryCode()
	message := "project template validation failed"
	for _, issue := range validationErr.Issues {
		if issue.Code == code {
			message = issue.Message
			break
		}
	}
	payload := ProjectTemplateToolError{
		Code: code, Message: message,
		Issues: append([]app.ProjectTemplateIssue{}, validationErr.Issues...),
	}
	result := &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("%s: %s", payload.Code, payload.Message)},
		},
		StructuredContent: payload,
	}
	result.SetError(validationErr)
	return result, ToolEnvelope{}, nil
}
