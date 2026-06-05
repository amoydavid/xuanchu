package mcpserver

import (
	"io"
	"net/http"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/logging"
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
	Version            string
	Mode               Mode
	Stderr             io.Writer
	Request            *http.Request
	LocalRuntimeValues map[string]string
	Logger             *logging.Logger
}
