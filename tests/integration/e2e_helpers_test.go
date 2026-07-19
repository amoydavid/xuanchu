package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func createTokenJSON(t *testing.T, bin, dbFlag, dbValue, name string, scopes ...string) string {
	t.Helper()
	args := []string{dbFlag, dbValue, "--json", "--workspace", "local", "token", "create", name, "--expires-in", "720h"}
	for _, scope := range scopes {
		if strings.TrimSpace(scope) == "" {
			continue
		}
		args = append(args, "--scope", scope)
	}
	return run(t, bin, args...)
}

func parseRawToken(t *testing.T, raw string) string {
	t.Helper()
	var created map[string]any
	if err := json.Unmarshal([]byte(raw), &created); err != nil {
		t.Fatalf("token create output is not JSON: %v\n%s", err, raw)
	}
	token, _ := created["token"].(string)
	if token == "" {
		t.Fatalf("token create output missing token: %s", raw)
	}
	return token
}

func parseJSONMap(t *testing.T, raw string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("output is not JSON object: %v\n%s", err, raw)
	}
	return out
}

func httpStatus(t *testing.T, method, url string, body io.Reader, headers map[string]string) int {
	t.Helper()
	resp, _ := httpDo(t, method, url, body, headers)
	defer resp.Body.Close()
	return resp.StatusCode
}

func httpJSON(t *testing.T, method, url string, body any, headers map[string]string) map[string]any {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
		if headers == nil {
			headers = map[string]string{}
		}
		headers["Content-Type"] = "application/json"
	}
	resp, payload := httpDo(t, method, url, reader, headers)
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("%s %s status = %d body=%s", method, url, resp.StatusCode, payload)
	}
	var out map[string]any
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("%s %s response is not JSON: %v\n%s", method, url, err, payload)
	}
	return out
}

func httpDo(t *testing.T, method, url string, body io.Reader, headers map[string]string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range headers {
		if strings.EqualFold(key, "Host") {
			req.Host = value
			continue
		}
		req.Header.Set(key, value)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		resp.Body.Close()
		t.Fatal(err)
	}
	resp.Body = io.NopCloser(bytes.NewReader(payload))
	return resp, payload
}

func authHeaders(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

func assertLogContains(t *testing.T, path string, wants ...string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var text string
	for {
		raw, err := os.ReadFile(path)
		if err == nil {
			text = string(raw)
			missing := ""
			for _, want := range wants {
				if !strings.Contains(text, want) {
					missing = want
					break
				}
			}
			if missing == "" {
				return
			}
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Fatalf("log file %s = %q, want substring %q", path, text, want)
		}
	}
}

func writeE2ELogConfig(t *testing.T, dir string, trustedHosts ...string) (string, string) {
	t.Helper()
	logPath := filepath.Join(dir, "xuanchu.log")
	configPath := filepath.Join(dir, "xuanchu.toml")
	var b strings.Builder
	b.WriteString("[log]\n")
	b.WriteString(`level = "info"` + "\n")
	b.WriteString(`format = "text"` + "\n")
	b.WriteString(`file = "` + logPath + `"` + "\n")
	b.WriteString(`rotate = "none"` + "\n\n")
	if len(trustedHosts) > 0 {
		b.WriteString("[server.mcp]\n")
		b.WriteString("trusted_proxy_hosts = [")
		for i, host := range trustedHosts {
			if i > 0 {
				b.WriteString(", ")
			}
			encoded, err := json.Marshal(host)
			if err != nil {
				t.Fatal(err)
			}
			b.Write(encoded)
		}
		b.WriteString("]\n")
	}
	if err := os.WriteFile(configPath, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return configPath, logPath
}

type bearerRoundTripper struct {
	token string
	base  http.RoundTripper
}

func (rt bearerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+rt.token)
	return rt.base.RoundTrip(req)
}

func mcpHTTPClient(token string) *http.Client {
	return &http.Client{Transport: bearerRoundTripper{token: token, base: http.DefaultTransport}}
}

func mcpStructuredMap(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	if result == nil {
		t.Fatal("nil MCP call tool result")
	}
	if result.IsError {
		t.Fatalf("MCP tool returned error content: %#v", result.Content)
	}
	if result.StructuredContent == nil {
		t.Fatalf("MCP result missing structured content: %#v", result.Content)
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("structured content is not a JSON object: %v\n%s", err, raw)
	}
	return out
}

func connectHTTPMCP(t *testing.T, baseURL, token string) (*mcp.ClientSession, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	client := mcp.NewClient(&mcp.Implementation{Name: "xuanchu-e2e", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:             baseURL + "/mcp",
		HTTPClient:           mcpHTTPClient(token),
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		cancel()
		t.Fatalf("connect HTTP MCP: %v", err)
	}
	t.Cleanup(func() {
		_ = session.Close()
		cancel()
	})
	return session, cancel
}

func connectStdioMCP(t *testing.T, cmd *exec.Cmd) (*mcp.ClientSession, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	client := mcp.NewClient(&mcp.Implementation{Name: "xuanchu-e2e", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		cancel()
		t.Fatalf("connect stdio MCP: %v", err)
	}
	t.Cleanup(func() {
		_ = session.Close()
		cancel()
	})
	return session, cancel
}

// nestedMap 从 map 中按路径逐层取嵌套 map[string]any。
// 例如 nestedMap(t, payload, "data", "task") 返回 payload["data"]["task"]。
// 修复 postgres_e2e_test.go 中引用但未定义的 helper（pre-existing 编译错误）。
func nestedMap(t *testing.T, m map[string]any, keys ...string) map[string]any {
	t.Helper()
	current := m
	for _, key := range keys {
		next, ok := current[key].(map[string]any)
		if !ok {
			t.Fatalf("nestedMap: key %q not found or not a map in %#v", key, current)
		}
		current = next
	}
	return current
}

// callMCPToolData 通过 MCP ClientSession 调用 tool 并返回 structured content 的 data 部分。
// 修复 postgres_e2e_test.go 中引用但未定义的 helper。
func callMCPToolData(t *testing.T, session *mcp.ClientSession, toolName string, args map[string]any) map[string]any {
	t.Helper()
	ctx := context.Background()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("callMCPToolData %s: %v", toolName, err)
	}
	// structuredContent 是 *mcp.CallToolResult 中的字段。
	if result.StructuredContent != nil {
		if data, ok := result.StructuredContent.(map[string]any); ok {
			if dataObj, ok := data["data"].(map[string]any); ok {
				return dataObj
			}
			return data
		}
	}
	// fallback: parse text content.
	if len(result.Content) > 0 {
		if text, ok := result.Content[0].(*mcp.TextContent); ok {
			var embedded map[string]any
			if err := json.Unmarshal([]byte(text.Text), &embedded); err == nil {
				if data, ok := embedded["data"].(map[string]any); ok {
					return data
				}
				return embedded
			}
		}
	}
	t.Fatalf("callMCPToolData: could not extract data from result for %s", toolName)
	return nil
}
