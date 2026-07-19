package mcpserver

import (
	"io"
	"net/http"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/logging"
	"git.dajee.net/dajee/xuanchu/internal/runtimeutil"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

// Mode 表示 MCP server 的运行模式。
type Mode string

const (
	ModeStdio Mode = "stdio"
	ModeHTTP  Mode = "http"
)

// Options 是创建 MCP server 的配置参数。
type Options struct {
	Store              *storage.Store
	Clock              app.Clock
	ResourceBaseURL    string
	Version            string
	Mode               Mode
	Stderr             io.Writer
	Request            *http.Request
	LocalRuntimeValues map[string]string
	Logger             *logging.Logger
	Shutdown           *runtimeutil.ShutdownCoordinator
	// Attachments 注入附件运行时；nil 时附件工具返回 attachment_storage_unavailable。
	Attachments *app.AttachmentRuntime
}
