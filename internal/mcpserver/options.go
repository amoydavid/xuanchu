package mcpserver

import (
	"io"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/storage/sqlite"
)

// Mode 表示 MCP server 的运行模式。
type Mode string

const (
	ModeStdio Mode = "stdio"
	ModeHTTP  Mode = "http"
)

// Options 是创建 MCP server 的配置参数。
type Options struct {
	Store   *sqlite.Store
	Clock   app.Clock
	Version string
	Mode    Mode
	Stderr  io.Writer
}
