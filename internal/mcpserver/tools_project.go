package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ProjectListInput struct {
	Workspace       string `json:"workspace,omitempty"`
	IncludeArchived bool   `json:"include_archived,omitempty" jsonschema:"include archived projects in the result"`
	Status          string `json:"status,omitempty" jsonschema:"filter by status: open, planning, active, archived, cancelled, or all (default open)"`
}

type ProjectGetInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

type ProjectCurrentInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

type ProjectAnnotateInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Content   string `json:"content" jsonschema:"annotation body text"`
}

type ProjectDenotateInput struct {
	Workspace    string `json:"workspace,omitempty"`
	Project      string `json:"project,omitempty"`
	ProjectID    string `json:"project_id,omitempty"`
	AnnotationID string `json:"annotation_id" jsonschema:"annotation ID to remove"`
}

type ProjectAnnotationsInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

type ProjectTimelineInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Limit     int    `json:"limit,omitempty" jsonschema:"max timeline entries to return (default 20)"`
	Offset    int    `json:"offset,omitempty" jsonschema:"number of timeline entries to skip"`
}

type ProjectAddInput struct {
	Workspace   string `json:"workspace,omitempty"`
	Slug        string `json:"slug" jsonschema:"unique project slug within the workspace"`
	Name        string `json:"name,omitempty" jsonschema:"human-readable project name"`
	Description string `json:"description,omitempty" jsonschema:"project description"`
}

type ProjectModifyInput struct {
	Workspace   string  `json:"workspace,omitempty"`
	Project     string  `json:"project,omitempty"`
	ProjectID   string  `json:"project_id,omitempty"`
	Name        *string `json:"name,omitempty" jsonschema:"new human-readable project name"`
	Description *string `json:"description,omitempty" jsonschema:"new project description; pass empty string to clear"`
}

type ProjectArchiveInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

type ProjectTransitionInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Status    string `json:"status" jsonschema:"target status: planning, active, archived, or cancelled"`
}

type ProjectConfigSetInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Key       string `json:"key" jsonschema:"config key, e.g. agent.background"`
	Value     string `json:"value" jsonschema:"config value"`
}

type ProjectConfigUnsetInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Key       string `json:"key" jsonschema:"config key to remove"`
}

type ProjectConfigListInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

func registerProjectTools(s *mcp.Server, opts Options) {
	addTool(s, opts, &mcp.Tool{Name: "project_list", Description: "List projects in the effective workspace; filter by status (open/planning/active/archived/cancelled/all); read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectListInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace}, "project:read", app.PermissionProjectRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		var rows []app.ProjectView
		if strings.TrimSpace(in.Status) != "" {
			rows, err = svc.ListProjectsByStatus(in.Status)
		} else if in.IncludeArchived {
			rows, err = svc.ListProjectsByStatus("all")
		} else {
			rows, err = svc.ListProjectsByStatus("open")
		}
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		data := map[string]any{"projects": projectViewsFromApp(rows), "count": len(rows)}
		return successWithEnvelope(data, fmt.Sprintf("%d project(s)", len(rows)))
	})

	addTool(s, opts, &mcp.Tool{Name: "project_get", Description: "Get one project and its agent-readable config; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectGetInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		ref := projectRefForScope(in.Project, in.ProjectID)
		if strings.TrimSpace(ref) == "" {
			return businessErrorWithEnvelope(app.RuntimeError{Code: "project_not_found", Message: "project reference is required"})
		}
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}, "project:read", app.PermissionProjectRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		project, err := svc.ProjectInfo(ref)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		config, err := filterAgentConfig(svc, ref)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		data := map[string]any{"project": projectViewFromApp(project), "config_summary": config}
		return successWithEnvelope(data, "project "+project.Slug)
	})

	addTool(s, opts, &mcp.Tool{Name: "project_get_current", Description: "Read explicit effective project scope; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectCurrentInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		ref := projectRefForScope(in.Project, in.ProjectID)
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}, "project:read", app.PermissionProjectRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		if strings.TrimSpace(ref) == "" {
			return successWithEnvelope(map[string]any{"project": nil}, "no effective project")
		}
		project, err := svc.ProjectInfo(ref)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"project": projectViewFromApp(project)}, "project "+project.Slug)
	})

	addTool(s, opts, &mcp.Tool{Name: "project_annotate", Description: "Add an annotation to a project; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectAnnotateInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		ref := projectRefForScope(in.Project, in.ProjectID)
		if strings.TrimSpace(ref) == "" {
			return businessErrorWithEnvelope(app.RuntimeError{Code: "project_not_found", Message: "project reference is required"})
		}
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}, "project:write", app.PermissionProjectManage)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		annotation, err := svc.ProjectAnnotate(ref, in.Content)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		data := map[string]any{"annotation": annotationViewFromApp(annotation)}
		return successWithEnvelope(data, "annotated project")
	})

	addTool(s, opts, &mcp.Tool{Name: "project_denotate", Description: "Remove an annotation from a project; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectDenotateInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		ref := projectRefForScope(in.Project, in.ProjectID)
		if strings.TrimSpace(ref) == "" {
			return businessErrorWithEnvelope(app.RuntimeError{Code: "project_not_found", Message: "project reference is required"})
		}
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}, "project:write", app.PermissionProjectManage)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		if err := svc.ProjectDenotate(ref, in.AnnotationID); err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"removed": in.AnnotationID}, "removed annotation")
	})

	addTool(s, opts, &mcp.Tool{Name: "project_list_annotations", Description: "List annotations on a project; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectAnnotationsInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		ref := projectRefForScope(in.Project, in.ProjectID)
		if strings.TrimSpace(ref) == "" {
			return businessErrorWithEnvelope(app.RuntimeError{Code: "project_not_found", Message: "project reference is required"})
		}
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}, "project:read", app.PermissionProjectRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		annotations, err := svc.ProjectAnnotations(ref)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		data := map[string]any{"annotations": annotationViewsFromApp(annotations), "count": len(annotations)}
		return successWithEnvelope(data, fmt.Sprintf("%d annotation(s)", len(annotations)))
	})

	addTool(s, opts, &mcp.Tool{Name: "project_list_timeline", Description: "List timeline entries for a project; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectTimelineInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		ref := projectRefForScope(in.Project, in.ProjectID)
		if strings.TrimSpace(ref) == "" {
			return businessErrorWithEnvelope(app.RuntimeError{Code: "project_not_found", Message: "project reference is required"})
		}
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}, "project:read", app.PermissionProjectRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		timelineOpts := app.TimelineOptions{Limit: in.Limit, Offset: in.Offset}
		if timelineOpts.Limit <= 0 {
			timelineOpts.Limit = 50
		}
		if timelineOpts.Limit > mcpMaxLimit {
			timelineOpts.Limit = mcpMaxLimit
		}
		entries, err := svc.ProjectTimeline(ref, timelineOpts)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		data := map[string]any{"entries": entries, "count": len(entries)}
		return successWithEnvelope(data, fmt.Sprintf("%d timeline entry/entries", len(entries)))
	})

	addTool(s, opts, &mcp.Tool{Name: "project_add", Description: "Create a project in the effective workspace; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectAddInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace}, "project:write", app.PermissionProjectManage)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.AddProject(app.AddProjectInput{Slug: in.Slug, Name: in.Name, Description: in.Description})
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"project": projectViewFromApp(view)}, "created project "+view.Slug)
	})

	addTool(s, opts, &mcp.Tool{Name: "project_modify", Description: "Modify a project; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectModifyInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		ref := projectRefForScope(in.Project, in.ProjectID)
		if strings.TrimSpace(ref) == "" {
			return businessErrorWithEnvelope(app.RuntimeError{Code: "project_not_found", Message: "project reference is required"})
		}
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}, "project:write", app.PermissionProjectManage)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		if err := svc.ModifyProject(ref, app.ModifyProjectInput{Name: in.Name, Description: in.Description}); err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.ProjectInfo(ref)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"project": projectViewFromApp(view)}, "modified project "+view.Slug)
	})

	addTool(s, opts, &mcp.Tool{Name: "project_archive", Description: "Archive a project; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectArchiveInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		ref := projectRefForScope(in.Project, in.ProjectID)
		if strings.TrimSpace(ref) == "" {
			return businessErrorWithEnvelope(app.RuntimeError{Code: "project_not_found", Message: "project reference is required"})
		}
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}, "project:write", app.PermissionProjectManage)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.ArchiveProject(ref)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"project": projectViewFromApp(view)}, "archived project "+view.Slug)
	})

	addTool(s, opts, &mcp.Tool{Name: "project_transition", Description: "Transition a project to planning/active/archived/cancelled; any direction allowed; records an annotation and writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectTransitionInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		ref := projectRefForScope(in.Project, in.ProjectID)
		if strings.TrimSpace(ref) == "" {
			return businessErrorWithEnvelope(app.RuntimeError{Code: "project_not_found", Message: "project reference is required"})
		}
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}, "project:write", app.PermissionProjectManage)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.TransitionProject(ref, strings.TrimSpace(in.Status))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"project": projectViewFromApp(view)}, "transitioned project "+view.Slug+" to "+view.Status)
	})

	addTool(s, opts, &mcp.Tool{Name: "project_config_set", Description: "Set a project config value; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectConfigSetInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		ref := projectRefForScope(in.Project, in.ProjectID)
		if strings.TrimSpace(ref) == "" {
			return businessErrorWithEnvelope(app.RuntimeError{Code: "project_not_found", Message: "project reference is required"})
		}
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}, "config:write", app.PermissionProjectConfigWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		if err := svc.ProjectConfigSet(ref, in.Key, in.Value); err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"key": in.Key, "value": in.Value}, "set config "+in.Key)
	})

	addTool(s, opts, &mcp.Tool{Name: "project_config_unset", Description: "Remove a project config value; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectConfigUnsetInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		ref := projectRefForScope(in.Project, in.ProjectID)
		if strings.TrimSpace(ref) == "" {
			return businessErrorWithEnvelope(app.RuntimeError{Code: "project_not_found", Message: "project reference is required"})
		}
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}, "config:write", app.PermissionProjectConfigWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		if err := svc.ProjectConfigUnset(ref, in.Key); err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"key": in.Key}, "unset config "+in.Key)
	})

	addTool(s, opts, &mcp.Tool{Name: "project_config_list", Description: "List project config values; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectConfigListInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		ref := projectRefForScope(in.Project, in.ProjectID)
		if strings.TrimSpace(ref) == "" {
			return businessErrorWithEnvelope(app.RuntimeError{Code: "project_not_found", Message: "project reference is required"})
		}
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}, "config:read", app.PermissionProjectConfigRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		config, err := svc.ProjectConfigList(ref)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"config": config, "count": len(config)}, fmt.Sprintf("%d config value(s)", len(config)))
	})
}
