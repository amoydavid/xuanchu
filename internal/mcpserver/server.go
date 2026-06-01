package mcpserver

import "github.com/modelcontextprotocol/go-sdk/mcp"

// NewServer 创建一个空的 MCP server 实例。
// 调用方后续通过 AddTool 等方法注册能力。
func NewServer(opts Options) *mcp.Server {
	version := opts.Version
	if version == "" {
		version = "dev"
	}
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "taskg",
		Version: version,
	}, nil)
	return srv
}
