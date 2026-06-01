package mcpserver

import (
	"errors"
	"fmt"

	"github.com/dajee/taskg/internal/app"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolEnvelope 是 MCP tool 成功返回的标准信封。
type ToolEnvelope struct {
	Data     any    `json:"data"`
	Rendered string `json:"rendered"`
}

// ToolError 是 MCP tool 错误返回的结构化内容。
type ToolError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// successResult 将业务数据封装为 MCP CallToolResult。
// 返回 result 和 envelope（供测试验证）。
func successResult(data any, rendered string) (*mcp.CallToolResult, ToolEnvelope, error) {
	envelope := ToolEnvelope{
		Data:     data,
		Rendered: rendered,
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: rendered},
		},
		StructuredContent: envelope,
	}, envelope, nil
}

// businessErrorResult 将 app 层错误映射为 MCP IsError=true 结果。
// 非 app error 映射到 mcp_internal，不泄露内部信息。
func businessErrorResult(err error) *mcp.CallToolResult {
	var runtimeErr app.RuntimeError
	if errors.As(err, &runtimeErr) {
		text := fmt.Sprintf("%s: %s", runtimeErr.Code, runtimeErr.Message)
		result := &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: text},
			},
			StructuredContent: ToolError{Code: runtimeErr.Code, Message: runtimeErr.Message},
		}
		result.SetError(runtimeErr)
		return result
	}
	var permissionErr app.PermissionError
	if errors.As(err, &permissionErr) {
		text := fmt.Sprintf("%s: %s", permissionErr.Code, permissionErr.Message)
		result := &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: text},
			},
			StructuredContent: ToolError{Code: permissionErr.Code, Message: permissionErr.Message},
		}
		result.SetError(permissionErr)
		return result
	}
	// 非 app error：不泄露 stack trace，返回通用内部错误
	result := &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{Text: "mcp_internal: internal error"},
		},
		StructuredContent: ToolError{Code: "mcp_internal", Message: "internal error"},
	}
	result.SetError(errors.New("mcp_internal: internal error"))
	return result
}

// renderedText 从 CallToolResult 提取第一个 TextContent 的文本。
func renderedText(result *mcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	if tc, ok := result.Content[0].(*mcp.TextContent); ok {
		return tc.Text
	}
	return ""
}
