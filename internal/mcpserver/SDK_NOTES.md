# MCP SDK API Notes

记录 `github.com/modelcontextprotocol/go-sdk` 的关键 API 签名，供后续实现参考。

## Module Version

`github.com/modelcontextprotocol/go-sdk v1.6.1`

## Server Constructor

```go
func NewServer(impl *Implementation, options *ServerOptions) *Server
```

- `Implementation` 包含 `Name`, `Version`, `Title`, `WebsiteURL`, `Icons` 字段
- `ServerOptions` 可为 nil（使用默认值）
- 返回的 Server 无任何 feature，需通过 AddXXX 方法添加

## Tool Registration

```go
// 泛型版（自动生成 input/output schema）
func AddTool[In, Out any](s *Server, t *Tool, h ToolHandlerFor[In, Out])

// 原始版（手动处理 raw input）
func (s *Server) AddTool(t *Tool, h ToolHandler)
```

- `Tool` 结构体包含: `Name`, `Description`, `Title`, `InputSchema`, `OutputSchema`, `Annotations`, `Icons`, `Meta`
- `ToolHandlerFor[In, Out]` 签名: `func(_ context.Context, request *CallToolRequest, input In) (result *CallToolResult, output Out, _ error)`
- `ToolHandler` 签名: `func(context.Context, *CallToolRequest) (*CallToolResult, error)`

## In-Memory Transport

```go
func NewInMemoryTransports() (*InMemoryTransport, *InMemoryTransport)
```

- 返回两个互相连接的 transport，对称的
- 服务端必须先连接，客户端再连接（客户端在连接时初始化 session）

## Stdio Transport

```go
type StdioTransport struct{}
```

- 零值即可用，通过 stdin/stdout 通信
- 服务端通过 `server.Run(ctx, &mcp.StdioTransport{})` 启动

## Streamable HTTP Handler

```go
func NewStreamableHTTPHandler(getServer func(*http.Request) *Server, opts *StreamableHTTPOptions) *StreamableHTTPHandler
```

- `getServer` 回调用于为每个新 session 创建或查找 Server
- 可以多次返回同一个 Server

## Server.Run / Server.Connect

```go
func (s *Server) Run(ctx context.Context, t Transport) error
func (s *Server) Connect(ctx context.Context, t Transport, opts *ServerSessionOptions) (*ServerSession, error)
```

- `Run` 阻塞运行直到 ctx 取消或 transport 关闭
- `Connect` 返回一个 `*ServerSession`，适合需要手动管理 session 的场景

## Client / ClientSession

```go
func NewClient(impl *Implementation, options *ClientOptions) *Client
func (c *Client) Connect(ctx context.Context, t Transport, opts *ClientSessionOptions) (cs *ClientSession, err error)
func (cs *ClientSession) ListTools(ctx context.Context, params *ListToolsParams) (*ListToolsResult, error)
func (cs *ClientSession) Close() error
func (cs *ClientSession) Wait() error
```

## ListTools 返回

```go
type ListToolsResult struct {
    Meta       Meta    `json:"_meta,omitempty"`
    NextCursor string  `json:"nextCursor,omitempty"`
    Tools      []*Tool `json:"tools"`
}
```

空服务器的 `tools/list` 返回 `Tools` 为空切片。
