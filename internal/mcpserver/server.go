package mcpserver

import (
	"git.dajee.net/dajee/xuanchu/internal/logging"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var mcpLogger *logging.Logger

func NewServer(opts Options) *mcp.Server {
	version := opts.Version
	if version == "" {
		version = "dev"
	}
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "xuanchu",
		Version: version,
	}, nil)
	RegisterTools(srv, opts)
	RegisterResources(srv, opts)
	return srv
}

func RegisterTools(s *mcp.Server, opts Options) {
	if opts.Logger != nil {
		mcpLogger = opts.Logger
	}
	registerTaskTools(s, opts)
	registerReportTools(s, opts)
	registerWorkspaceTools(s, opts)
	registerProjectTools(s, opts)
	registerMemberTools(s, opts)
	registerUserTools(s, opts)
	registerContextTools(s, opts)
	registerConfigTools(s, opts)
	registerHookTools(s, opts)
	registerNotificationTools(s, opts)
	registerTokenTools(s, opts)
	registerMiscTools(s, opts)
}
