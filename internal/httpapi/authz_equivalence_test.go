package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/mcpserver"
)

// TestHTTPAPIAndHTTPMCPShareAuthorizationDecision 是 spec §11.2 推荐的跨 transport 一致性测试：
// 同一个 token、同一个 workspace 下，HTTP API 和 HTTP MCP 走同一授权决策入口，
// 对相同的越权请求应返回相同的错误码。
//
// 这条断言证明本次重构的核心目标——HTTP API 与 HTTP MCP 复用同一个 Authorization Decision——
// 真实成立：两侧拒绝原因一致，不会出现一侧放行、另一侧拒绝的漂移。
func TestHTTPAPIAndHTTPMCPShareAuthorizationDecision(t *testing.T) {
	store := openHTTPTestStore(t)
	ownerSvc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	// 只授予 project:read 的 token，却去访问 task:read，两条 transport 都应拒绝为 token_scope_denied。
	created, err := ownerSvc.CreateToken(app.CreateTokenInput{
		Name:          "equiv",
		Scopes:        []string{"project:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// --- HTTP API 侧：走 /api/v1/tasks 路由 ---
	srv := NewServer(Options{Store: store})
	apiRR := requestHTTP(t, srv, http.MethodGet, "/api/v1/tasks?workspace=local", map[string]string{
		"Authorization": "Bearer " + created.RawToken,
	})
	if apiRR.Code != http.StatusForbidden {
		t.Fatalf("HTTP API status = %d, want 403 body=%s", apiRR.Code, apiRR.Body.String())
	}
	apiCode := extractHTTPErrorCode(t, apiRR)
	if apiCode != "token_scope_denied" {
		t.Fatalf("HTTP API code = %q, want token_scope_denied body=%s", apiCode, apiRR.Body.String())
	}

	// --- HTTP MCP 侧：直接走 RuntimeFactory.ServiceForHTTP 的授权入口 ---
	mcpReq, _ := http.NewRequest(http.MethodPost, "/mcp", nil)
	mcpReq.Header.Set("Authorization", "Bearer "+created.RawToken)
	factory := mcpserver.RuntimeFactory{Store: store}
	_, mcpErr := factory.ServiceForHTTP(mcpReq, mcpserver.RequestScopeInput{Workspace: "local"}, "task:read", app.PermissionTaskRead)
	if mcpErr == nil {
		t.Fatal("MCP ServiceForHTTP error = nil, want token_scope_denied")
	}
	mcpRuntimeErr, ok := mcpErr.(app.RuntimeError)
	if !ok {
		t.Fatalf("MCP error = %#v, want app.RuntimeError(token_scope_denied)", mcpErr)
	}
	if mcpRuntimeErr.Code != "token_scope_denied" {
		t.Fatalf("MCP code = %q, want token_scope_denied", mcpRuntimeErr.Code)
	}

	// 两侧错误码必须一致：这是 HTTP API 与 HTTP MCP 共用同一授权决策的直接证据。
	if apiCode != mcpRuntimeErr.Code {
		t.Fatalf("authorization diverged: HTTP API=%q, HTTP MCP=%q", apiCode, mcpRuntimeErr.Code)
	}
}

// extractHTTPErrorCode 从 HTTP 响应体的 error envelope 中解析 code 字段。
func extractHTTPErrorCode(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal error body: %v (body=%s)", err, rr.Body.String())
	}
	return body.Error.Code
}
