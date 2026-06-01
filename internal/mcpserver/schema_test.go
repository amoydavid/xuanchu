package mcpserver

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var update = flag.Bool("update", false, "update golden files")

// goldenGet 读取 golden 文件内容；当 -update 时写入 actual 并返回 actual。
func goldenGet(t *testing.T, name string, actual []byte) []byte {
	t.Helper()
	path := "testdata/" + name
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("create testdata: %v", err)
		}
		if err := os.WriteFile(path, actual, 0o644); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
		return actual
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	return want
}

func TestListToolsDefaultServerHasTools(t *testing.T) {
	srv := NewServer(Options{Version: "test"})

	// 创建 in-memory transport 对
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	// 启动服务端
	go func() {
		if err := srv.Run(context.Background(), serverTransport); err != nil {
			t.Logf("server run: %v", err)
		}
	}()

	// 连接客户端
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "0.0.1",
	}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer session.Close()

	// 调用 tools/list
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	if len(result.Tools) == 0 {
		t.Fatal("expected default MCP server to expose tools")
	}

	// golden 比较：序列化完整结果
	got, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}

	want := goldenGet(t, "list-tools-default.json", got)
	gotS := string(got)
	wantS := string(want)
	if gotS != wantS {
		t.Fatalf("golden mismatch:\n--- want\n%s\n--- got\n%s", wantS, gotS)
	}
}
