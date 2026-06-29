package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/authz"
	"git.dajee.net/dajee/xuanchu/internal/render"
	"git.dajee.net/dajee/xuanchu/internal/runtimeutil"
	"git.dajee.net/dajee/xuanchu/internal/task"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	mcpDefaultLimit = 200
	mcpMaxLimit     = 1000
)

func requireUUID(id, fieldName string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return app.RuntimeError{Code: fieldName + "_required", Message: fieldName + " is required"}
	}
	if _, err := uuid.Parse(id); err != nil {
		return app.RuntimeError{Code: fieldName + "_invalid", Message: fieldName + " must be a valid UUID, numeric IDs are not accepted in MCP"}
	}
	return nil
}

func resolveToolTaskRef(svc *app.Service, ref, fieldName string, write bool) (task.Task, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return task.Task{}, app.RuntimeError{Code: fieldName + "_required", Message: fieldName + " is required"}
	}
	if write {
		return svc.ResolveProtocolTargetForWrite(ref)
	}
	return svc.ResolveProtocolTarget(ref)
}

func addTool[In any](s *mcp.Server, opts Options, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, ToolEnvelope]) {
	inputSchema, err := jsonschema.For[In](nil)
	if err != nil {
		panic(err)
	}
	patchInputSchema[In](inputSchema)
	outputSchema, err := jsonschema.For[ToolEnvelope](nil)
	if err != nil {
		panic(err)
	}
	tool.InputSchema = inputSchema
	tool.OutputSchema = outputSchema
	s.AddTool(tool, func(ctx context.Context, req *mcp.CallToolRequest) (out *mcp.CallToolResult, err error) {
		start := time.Now()
		defer func() {
			logMCPToolCall(opts, tool.Name, start, out, err)
		}()
		if opts.Shutdown != nil {
			done, ok := opts.Shutdown.Begin()
			if !ok {
				return businessErrorResult(app.RuntimeError{Code: "server_draining", Message: "server is shutting down"}), nil
			}
			defer done()
			var cancel context.CancelFunc
			ctx, cancel = runtimeutil.ContextWithCancelOnEither(ctx, opts.Shutdown.Context())
			defer cancel()
		}
		var result *mcp.CallToolResult
		var handlerErr error
		func() {
			defer func() {
				if r := recover(); r != nil {
					if opts.Logger != nil {
						opts.Logger.Error("panic in tool", "tool", tool.Name, "panic", r, "stack", string(debug.Stack()))
					}
					var res mcp.CallToolResult
					res.SetError(fmt.Errorf("internal error"))
					result = &res
				}
			}()
			var input In
			if req != nil && req.Params != nil && len(req.Params.Arguments) > 0 {
				if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
					var res mcp.CallToolResult
					res.SetError(err)
					result = &res
					return
				}
			}
			r, _, err := handler(ctx, req, input)
			result = r
			handlerErr = err
		}()
		if handlerErr != nil {
			return businessErrorResult(handlerErr), nil
		}
		return result, nil
	})
}

func logMCPToolCall(opts Options, toolName string, start time.Time, result *mcp.CallToolResult, err error) {
	if opts.Logger == nil {
		return
	}
	args := []any{
		"component", "mcp",
		"operation", "mcp_tool_call",
		"tool", toolName,
		"mode", string(opts.Mode),
		"duration_ms", time.Since(start).Milliseconds(),
	}
	args = appendMCPRequestLogFields(args, opts)
	if err != nil {
		args = append(args, "result", "error", "error_code", "mcp_error", "error", err.Error())
		opts.Logger.Warn("mcp tool call", args...)
		return
	}
	if result != nil && result.IsError {
		args = append(args, "result", "error")
		if code := toolErrorCode(result); code != "" {
			args = append(args, "error_code", code)
		}
		opts.Logger.Warn("mcp tool call", args...)
		return
	}
	args = append(args, "result", "success")
	opts.Logger.Info("mcp tool call", args...)
}

func appendMCPRequestLogFields(args []any, opts Options) []any {
	if opts.Request == nil {
		return args
	}
	requestID := strings.TrimSpace(opts.Request.Header.Get("X-Request-Id"))
	if requestID != "" {
		args = append(args, "request_id", requestID)
	}
	if authn, ok := authFromHTTPRequest(opts.Request); ok {
		args = append(args,
			"actor_type", runtimeActorType(authn),
			"actor_user_id", actorUserIDForLog(authn),
			"token_id", authn.Token.ID,
		)
		if authn.TenantActor {
			args = append(args,
				"token_name", authn.Token.Name,
				"token_prefix", authn.Token.Prefix,
			)
		}
	}
	return args
}

func runtimeActorType(authn app.AuthenticatedToken) string {
	if authn.TenantActor {
		return "tenant_access_token"
	}
	return "user"
}

func actorUserIDForLog(authn app.AuthenticatedToken) string {
	if authn.TenantActor {
		return "-"
	}
	return authn.User.ID
}

func toolErrorCode(result *mcp.CallToolResult) string {
	if result == nil {
		return ""
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return ""
	}
	var toolErr ToolError
	if err := json.Unmarshal(raw, &toolErr); err == nil {
		return toolErr.Code
	}
	return ""
}

func patchInputSchema[In any](schema *jsonschema.Schema) {
	switch any(*new(In)).(type) {
	case ProjectGetInput:
		schema.AnyOf = []*jsonschema.Schema{
			{Required: []string{"project"}},
			{Required: []string{"project_id"}},
		}
	}
}

func serviceForTool(ctx context.Context, req *mcp.CallToolRequest, opts Options, input RequestScopeInput, capability string, permission app.Permission) (*app.Service, error) {
	factory := RuntimeFactory{Store: opts.Store, Clock: opts.Clock}
	var svc *app.Service
	var err error
	if opts.Mode == ModeHTTP {
		svc, err = factory.ServiceForHTTP(requestForTool(ctx, req, opts), input, capability, permission)
	} else {
		svc, err = factory.ServiceForStdio(ctx, input, capability, permission)
	}
	if err != nil {
		return nil, err
	}
	if err := rejectTenantTool(svc, toolNameFromRequest(req)); err != nil {
		return nil, err
	}
	return svc, nil
}

func toolNameFromRequest(req *mcp.CallToolRequest) string {
	if req == nil || req.Params == nil {
		return ""
	}
	return strings.TrimSpace(req.Params.Name)
}

func rejectTenantTool(svc *app.Service, toolName string) error {
	if svc.Runtime().ActorType != "tenant_access_token" {
		return nil
	}
	switch toolName {
	case "me_get", "context_set", "context_none":
		return app.RuntimeError{Code: "tenant_actor_not_user", Message: "tenant token has no user actor"}
	case "user_list", "user_get", "user_bind", "user_unbind", "user_add", "user_use", "user_list_external_ids",
		"member_list", "member_add", "member_role",
		"token_list", "token_create", "token_modify", "token_revoke",
		"workspace_list", "workspace_add", "workspace_modify", "workspace_archive", "workspace_use":
		return app.RuntimeError{Code: authz.CodeTokenScopeDenied, Message: "tenant token cannot call this tool"}
	default:
		return nil
	}
}

func requestForTool(ctx context.Context, req *mcp.CallToolRequest, opts Options) *http.Request {
	if opts.Request != nil {
		return opts.Request.WithContext(ctx)
	}
	r, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/mcp", nil)
	if req != nil && req.Extra != nil && req.Extra.Header != nil {
		r.Header = req.Extra.Header.Clone()
	}
	return r
}

func limitOrDefault(limit int) (int, error) {
	if limit == 0 {
		return mcpDefaultLimit, nil
	}
	if limit <= 0 {
		return 0, app.RuntimeError{Code: "api_bad_limit", Message: "invalid limit"}
	}
	if limit > mcpMaxLimit {
		return 0, app.RuntimeError{Code: "api_bad_limit", Message: "limit must be <= 1000"}
	}
	return limit, nil
}

func taskData(tsk task.Task) (map[string]any, error) {
	dto := task.ToJSON(tsk)
	var flat map[string]any
	raw, err := json.Marshal(dto)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &flat); err != nil {
		return nil, err
	}
	flat["task"] = dto
	flat["completed"] = tsk.Status == task.StatusCompleted
	flat["deleted"] = tsk.Status == task.StatusDeleted
	return flat, nil
}

func tasksData(rows []task.Task) map[string]any {
	out := make([]task.JSONTask, len(rows))
	for i, row := range rows {
		out[i] = task.ToJSON(row)
	}
	return map[string]any{"tasks": out, "count": len(out)}
}

func renderTaskInfo(tsk task.Task) string {
	var buf bytes.Buffer
	render.TaskInfo(&buf, tsk)
	return buf.String()
}

func renderTaskList(rows []task.Task) string {
	var buf bytes.Buffer
	render.TaskList(&buf, rows)
	return buf.String()
}

func stringPtrFromValue(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

// ptrToStringSlice 把切片转为指针：nil 切片返回 nil（表示「不修改」），
// 非 nil 切片返回指针（含空切片，表示「清空」）。
func ptrToStringSlice(values []string) *[]string {
	if values == nil {
		return nil
	}
	return &values
}
