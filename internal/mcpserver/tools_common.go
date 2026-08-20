package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

func validateToolTaskRef(ref, fieldName string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return app.RuntimeError{Code: fieldName + "_required", Message: fieldName + " is required"}
	}
	return app.ValidateProtocolTaskRef(ref)
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
				if err := decodeToolInput(req.Params.Arguments, &input); err != nil {
					var runtimeErr app.RuntimeError
					if errors.As(err, &runtimeErr) {
						result = businessErrorResult(runtimeErr)
						return
					}
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

// decodeToolInput 按工具声明的 closed JSON Schema 严格解码。
// MCP SDK 会把未知参数静默丢弃，因此服务端必须在 handler 前自行拒绝，
// 避免拼错字段或已移除字段被当作一次成功调用。
func decodeToolInput[In any](raw json.RawMessage, input *In) error {
	if err := rejectRetiredTaskRecurrenceFields[In](raw); err != nil {
		return err
	}
	if err := rejectInvalidTaskSeriesPagination[In](raw); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(input); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("MCP tool arguments must contain exactly one JSON object")
		}
		return err
	}
	return nil
}

func rejectInvalidTaskSeriesPagination[In any](raw json.RawMessage) error {
	switch any(*new(In)).(type) {
	case TaskSeriesListInput, TaskSeriesOccurrenceListInput:
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return err
		}
		if value, exists := fields["limit"]; exists {
			var limit int
			if bytes.Equal(bytes.TrimSpace(value), []byte("null")) || json.Unmarshal(value, &limit) != nil || limit < 1 || limit > mcpMaxLimit {
				return app.RuntimeError{Code: "api_bad_limit", Message: "limit must be between 1 and 1000"}
			}
		}
		if value, exists := fields["offset"]; exists {
			var offset int
			if bytes.Equal(bytes.TrimSpace(value), []byte("null")) || json.Unmarshal(value, &offset) != nil || offset < 0 {
				return app.RuntimeError{Code: "api_bad_offset", Message: "offset must be >= 0"}
			}
		}
	}
	return nil
}

func rejectRetiredTaskRecurrenceFields[In any](raw json.RawMessage) error {
	switch any(*new(In)).(type) {
	case TaskAddInput, TaskModifyInput:
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return err
		}
		for _, field := range []string{"recur", "clear_recur", "mask", "imask"} {
			if _, exists := fields[field]; exists {
				return app.RuntimeError{
					Code:    "task_series_endpoint_required",
					Message: "recur/clear_recur/mask/imask fields were removed; use task_series_* tools",
				}
			}
		}
	}
	return nil
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
	annotateScopeFields(schema)

	if projectRefRequired(any(*new(In))) {
		// handler 通过 projectRefForScope 校验：project 或 project_id 至少传一个，
		// 二者皆空会直接返回 project_not_found。anyOf 把这一约束暴露给调用方。
		schema.AnyOf = []*jsonschema.Schema{
			{Required: []string{"project"}},
			{Required: []string{"project_id"}},
		}
	}

	switch any(*new(In)).(type) {
	case TaskSeriesListInput, TaskSeriesOccurrenceListInput:
		minimum, maximum := float64(1), float64(mcpMaxLimit)
		if limit := schema.Properties["limit"]; limit != nil {
			limit.Default = json.RawMessage("200")
			limit.Minimum = &minimum
			limit.Maximum = &maximum
		}
		zero := float64(0)
		if offset := schema.Properties["offset"]; offset != nil {
			offset.Default = json.RawMessage("0")
			offset.Minimum = &zero
		}
	}
}

// projectRefRequired 报告该 input 的 handler 是否依赖 projectRefForScope，
// 即 project/project_id 至少传一个才能定位资源。用类型断言集中维护，
// 避免每个 struct 都写 anyOf。
func projectRefRequired(v any) bool {
	switch v.(type) {
	case ProjectGetInput,
		ProjectAnnotateInput,
		ProjectDenotateInput,
		ProjectAnnotationsInput,
		ProjectTimelineInput,
		ProjectModifyInput,
		ProjectArchiveInput,
		ProjectTransitionInput,
		ProjectConfigSetInput,
		ProjectConfigUnsetInput,
		ProjectConfigListInput,
		ProjectAutomationListInput,
		ProjectAutomationRefInput,
		ProjectAutomationAddInput,
		ProjectAutomationModifyInput,
		ProjectAutomationPreviewSavedInput,
		ProjectAutomationDeliveryListInput,
		ProjectAutomationDeliveryRefInput,
		ProjectAutomationTemplateVarsInput:
		return true
	}
	return false
}

// scopeFieldDescriptions 统一作用域三件套的描述。字段已有 jsonschema tag
// （如 task 系列里更精确的文案）时不覆盖，仅给裸字段补说明，让 LLM 在
// 所有工具上对 workspace/project/project_id 看到一致的语义。
var scopeFieldDescriptions = map[string]string{
	"workspace":  "workspace slug or UUID; omit to use the actor's default workspace",
	"project":    "project slug in the effective workspace",
	"project_id": "stable project UUID",
}

func annotateScopeFields(schema *jsonschema.Schema) {
	if schema == nil || len(schema.Properties) == 0 {
		return
	}
	for name, desc := range scopeFieldDescriptions {
		prop := schema.Properties[name]
		if prop == nil || prop.Description != "" {
			continue
		}
		prop.Description = desc
	}
}

func serviceForTool(ctx context.Context, req *mcp.CallToolRequest, opts Options, input RequestScopeInput, capability string, permission app.Permission) (*app.Service, error) {
	svc, _, err := serviceForToolWithAuth(ctx, req, opts, input, capability, permission)
	return svc, err
}

func serviceForToolWithAuth(ctx context.Context, req *mcp.CallToolRequest, opts Options, input RequestScopeInput, capability string, permission app.Permission) (*app.Service, *app.AuthenticatedToken, error) {
	factory := RuntimeFactory{Store: opts.Store, Clock: opts.Clock, ResourceBaseURL: opts.ResourceBaseURL, Attachments: opts.Attachments}
	var svc *app.Service
	var authn *app.AuthenticatedToken
	var err error
	if opts.Mode == ModeHTTP {
		r := requestForTool(ctx, req, opts)
		r, err = factory.AuthenticateHTTPRequest(r)
		if err != nil {
			return nil, nil, err
		}
		if value, ok := authFromHTTPRequest(r); ok {
			authn = &value
		}
		svc, err = factory.ServiceForHTTP(r, input, capability, permission)
	} else {
		svc, err = factory.ServiceForStdio(ctx, input, capability, permission)
	}
	if err != nil {
		return nil, nil, err
	}
	if err := rejectTenantTool(svc, toolNameFromRequest(req)); err != nil {
		return nil, nil, err
	}
	return svc, authn, nil
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
	case "user_use",
		"workspace_list", "workspace_add", "workspace_archive", "workspace_use":
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
