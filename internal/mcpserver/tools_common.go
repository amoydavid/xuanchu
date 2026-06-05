package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/render"
	"github.com/dajee/taskg/internal/task"
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

func addTool[In any](s *mcp.Server, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, ToolEnvelope]) {
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
	s.AddTool(tool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var input In
		if req != nil && req.Params != nil && len(req.Params.Arguments) > 0 {
			if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
				var result mcp.CallToolResult
				result.SetError(err)
				return &result, nil
			}
		}
		result, _, err := handler(ctx, req, input)
		if err != nil {
			return businessErrorResult(err), nil
		}
		return result, nil
	})
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
	if opts.Mode == ModeHTTP {
		return factory.ServiceForHTTP(requestForTool(ctx, req, opts), input, capability, permission)
	}
	return factory.ServiceForStdio(ctx, input, capability, permission)
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
