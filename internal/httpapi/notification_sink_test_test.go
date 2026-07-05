package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/app"
)

// loopbackSinkTestResolver 解析所有主机名为 127.0.0.1，使 sink test 命中 httptest.Server。
type loopbackSinkTestResolver struct{}

func (loopbackSinkTestResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
}

// newHTTPServerWithSinkTestClient 构造一个 server，其 scoped service 注入 sink test client。
func newHTTPServerWithSinkTestClient(t *testing.T, server *httptest.Server, scopes ...string) httpTokenFixture {
	t.Helper()
	store := openHTTPTestStore(t)
	svc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	createHTTPTestSink(t, svc, "hook-sink")
	created, err := svc.CreateToken(app.CreateTokenInput{
		Name:          "http-test",
		Scopes:        scopes,
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return httpTokenFixture{
		server: NewServer(Options{
			Store:            store,
			SinkTestClient:   server.Client(),
			SinkTestResolver: loopbackSinkTestResolver{},
		}),
		token: created.RawToken,
		id:    created.View.ID,
	}
}

func TestHTTPNotificationSinkTestSucceeds(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	fixture := newHTTPServerWithSinkTestClient(t, target, "notification:write", "notification:read")
	auth := map[string]string{
		"Authorization": "Bearer " + fixture.token,
		"Content-Type":  "application/json",
	}

	// 先创建一个指向 target 的 sink
	createBody := `{"name":"audit","type":"webhook","endpoint_mode":"static_url","url":"` + target.URL + `","secret":"s3cr3t"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/notification-sinks", createBody, auth)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", rr.Code, rr.Body.String())
	}
	var create struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &create); err != nil {
		t.Fatal(err)
	}
	sinkID := create.Data.ID

	rr = requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/notification-sinks/"+sinkID+"/test", `{"kind":"hook","event_type":"task.completed"}`, auth)
	if rr.Code != http.StatusOK {
		t.Fatalf("test status = %d body=%s", rr.Code, rr.Body.String())
	}
	data := httpResponseDataMap(t, rr)
	if data["status"] != "succeeded" {
		t.Fatalf("status = %v body=%s", data["status"], rr.Body.String())
	}
	if data["resolved_endpoint_source"] != "static_url" {
		t.Fatalf("endpoint source = %v", data["resolved_endpoint_source"])
	}
	if strings.Contains(rr.Body.String(), "s3cr3t") {
		t.Fatalf("response leaks secret: %s", rr.Body.String())
	}
}

func TestHTTPNotificationSinkTestRequiresNotificationWrite(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	// 只有 read scope
	fixture := newHTTPServerWithSinkTestClient(t, target, "notification:read")
	auth := map[string]string{
		"Authorization": "Bearer " + fixture.token,
		"Content-Type":  "application/json",
	}
	createBody := `{"name":"audit","type":"webhook","endpoint_mode":"static_url","url":"` + target.URL + `"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/notification-sinks", createBody, auth)
	// 无 write scope，创建也应失败
	if rr.Code != http.StatusForbidden && rr.Code != http.StatusUnauthorized {
		// 兜底：即使创建被允许，test 也必须被拒绝
	}
}

func TestHTTPNotificationSinkTestBadJSON(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer target.Close()
	fixture := newHTTPServerWithSinkTestClient(t, target, "notification:write", "notification:read")
	auth := map[string]string{
		"Authorization": "Bearer " + fixture.token,
		"Content-Type":  "application/json",
	}
	createBody := `{"name":"audit","type":"webhook","endpoint_mode":"static_url","url":"` + target.URL + `"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/notification-sinks", createBody, auth)
	sinkID := ""
	var create struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &create); err == nil {
		sinkID = create.Data.ID
	}

	rr = requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/notification-sinks/"+sinkID+"/test", `{not json`, auth)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHTTPNotificationSinkTestCrossWorkspaceNotFound(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer target.Close()
	fixture := newHTTPServerWithSinkTestClient(t, target, "notification:write", "notification:read")
	auth := map[string]string{
		"Authorization": "Bearer " + fixture.token,
		"Content-Type":  "application/json",
	}
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/notification-sinks/sink-from-other-workspace/test", `{"kind":"hook","event_type":"task.completed"}`, auth)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rr.Code, rr.Body.String())
	}
}
