package mcpserver

import "github.com/modelcontextprotocol/go-sdk/mcp"

// NewServer 创建注册了 taskg 默认工具的 MCP server 实例。
func NewServer(opts Options) *mcp.Server {
	version := opts.Version
	if version == "" {
		version = "dev"
	}
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "taskg",
		Version: version,
	}, nil)
	RegisterTools(srv, opts)
	RegisterResources(srv, opts)
	return srv
}

// RegisterTools 在 server 上注册所有核心工具。
// 必须在 NewServer 之后、Run/Connect 之前调用。
func RegisterTools(s *mcp.Server, opts Options) {
	registerTaskTools(s, opts)
	registerReportTools(s, opts)
	registerWorkspaceTools(s, opts)
	registerProjectTools(s, opts)
	registerMemberTools(s, opts)
	registerUserTools(s, opts)
	registerContextTools(s, opts)
	registerConfigTools(s, opts)
	registerHookTools(s, opts)
	registerTokenTools(s, opts)
	registerMiscTools(s, opts)
}
