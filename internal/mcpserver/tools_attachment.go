package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"git.dajee.net/dajee/xuanchu/internal/app"
)

// TaskAttachmentListInput 列出任务附件。
type TaskAttachmentListInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Task      string `json:"task" jsonschema:"task reference: UUID, materialized task_slug, or occurrence_ref"`
}

func (in TaskAttachmentListInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

// TaskAttachmentGetInput 获取附件 metadata。
type TaskAttachmentGetInput struct {
	Workspace     string `json:"workspace,omitempty"`
	Project       string `json:"project,omitempty"`
	ProjectID     string `json:"project_id,omitempty"`
	AttachmentID  string `json:"attachment_id" jsonschema:"attachment id (UUID)"`
}

func (in TaskAttachmentGetInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

// TaskAttachmentRenameInput 修改附件展示名。
type TaskAttachmentRenameInput struct {
	Workspace    string `json:"workspace,omitempty"`
	Project      string `json:"project,omitempty"`
	ProjectID    string `json:"project_id,omitempty"`
	AttachmentID string `json:"attachment_id" jsonschema:"attachment id (UUID)"`
	DisplayName  string `json:"display_name" jsonschema:"new display name"`
}

func (in TaskAttachmentRenameInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

// TaskAttachmentRemoveInput 删除附件。
type TaskAttachmentRemoveInput struct {
	Workspace    string `json:"workspace,omitempty"`
	Project      string `json:"project,omitempty"`
	ProjectID    string `json:"project_id,omitempty"`
	AttachmentID string `json:"attachment_id" jsonschema:"attachment id (UUID)"`
}

func (in TaskAttachmentRemoveInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

func registerAttachmentTools(s *mcp.Server, opts Options) {
	addTool(s, opts, &mcp.Tool{
		Name:        "task_attachment_list",
		Description: "List metadata-only attachments on a task. To download content, use the returned content_url with the same Bearer token over HTTP.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskAttachmentListInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:read", app.PermissionTaskRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		if err := validateToolTaskRef(in.Task, "task"); err != nil {
			return businessErrorWithEnvelope(err)
		}
		list, err := svc.ListAttachments("task", strings.TrimSpace(in.Task), false)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"attachments": list, "count": len(list)}, fmt.Sprintf("%d attachment(s)", len(list)))
	})

	addTool(s, opts, &mcp.Tool{
		Name:        "task_attachment_get",
		Description: "Get attachment metadata. To download content, use the returned content_url with the same Bearer token over HTTP.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskAttachmentGetInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:read", app.PermissionTaskRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.GetAttachment(strings.TrimSpace(in.AttachmentID))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"attachment": view}, "Attachment metadata")
	})

	addTool(s, opts, &mcp.Tool{
		Name:        "task_attachment_rename",
		Description: "Rename a task attachment.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskAttachmentRenameInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:write", app.PermissionTaskWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.RenameAttachment(strings.TrimSpace(in.AttachmentID), in.DisplayName)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"attachment": view}, "Renamed attachment")
	})

	addTool(s, opts, &mcp.Tool{
		Name:        "task_attachment_remove",
		Description: "Remove a task attachment. Active attachments that are still referenced by the description cannot be removed.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskAttachmentRemoveInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:write", app.PermissionTaskWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		if err := svc.RemoveAttachment(ctx, strings.TrimSpace(in.AttachmentID)); err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"removed": true}, "Removed attachment")
	})
}
