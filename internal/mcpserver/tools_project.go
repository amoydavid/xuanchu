package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/dajee/taskg/internal/app"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ProjectListInput struct {
	Workspace       string `json:"workspace,omitempty"`
	IncludeArchived bool   `json:"include_archived,omitempty"`
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
	Content   string `json:"content"`
}

type ProjectDenotateInput struct {
	Workspace    string `json:"workspace,omitempty"`
	Project      string `json:"project,omitempty"`
	ProjectID    string `json:"project_id,omitempty"`
	AnnotationID string `json:"annotation_id"`
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
	Limit     int    `json:"limit,omitempty"`
}

func registerProjectTools(s *mcp.Server, opts Options) {
	addTool(s, &mcp.Tool{Name: "project.list", Description: "List projects in the effective workspace; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectListInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace}, "project:read", app.PermissionProjectRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		rows, err := svc.ListProjects(in.IncludeArchived)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		data := map[string]any{"projects": projectViewsFromApp(rows), "count": len(rows)}
		return successWithEnvelope(data, fmt.Sprintf("%d project(s)", len(rows)))
	})

	addTool(s, &mcp.Tool{Name: "project.get", Description: "Get one project and its agent-readable config; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectGetInput) (*mcp.CallToolResult, ToolEnvelope, error) {
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

	addTool(s, &mcp.Tool{Name: "project.current", Description: "Read explicit effective project scope; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectCurrentInput) (*mcp.CallToolResult, ToolEnvelope, error) {
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

	addTool(s, &mcp.Tool{Name: "project.annotate", Description: "Add an annotation to a project; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectAnnotateInput) (*mcp.CallToolResult, ToolEnvelope, error) {
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
		data := map[string]any{"annotation": annotation}
		return successWithEnvelope(data, "annotated project")
	})

	addTool(s, &mcp.Tool{Name: "project.denotate", Description: "Remove an annotation from a project; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectDenotateInput) (*mcp.CallToolResult, ToolEnvelope, error) {
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

	addTool(s, &mcp.Tool{Name: "project.annotations", Description: "List annotations on a project; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectAnnotationsInput) (*mcp.CallToolResult, ToolEnvelope, error) {
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
		data := map[string]any{"annotations": annotations, "count": len(annotations)}
		return successWithEnvelope(data, fmt.Sprintf("%d annotation(s)", len(annotations)))
	})

	addTool(s, &mcp.Tool{Name: "project.timeline", Description: "List timeline entries for a project; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectTimelineInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		ref := projectRefForScope(in.Project, in.ProjectID)
		if strings.TrimSpace(ref) == "" {
			return businessErrorWithEnvelope(app.RuntimeError{Code: "project_not_found", Message: "project reference is required"})
		}
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}, "project:read", app.PermissionProjectRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		timelineOpts := app.TimelineOptions{Limit: in.Limit}
		if timelineOpts.Limit <= 0 {
			timelineOpts.Limit = 50
		}
		entries, err := svc.ProjectTimeline(ref, timelineOpts)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		data := map[string]any{"entries": entries, "count": len(entries)}
		return successWithEnvelope(data, fmt.Sprintf("%d timeline entry/entries", len(entries)))
	})
}
