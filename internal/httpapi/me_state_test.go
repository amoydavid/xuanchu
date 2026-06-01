package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/dajee/taskg/internal/app"
)

func TestMeActiveWorkspaceSwitchesWorkspace(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:read", "workspace:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	ws, err := svc.AddWorkspace(app.AddWorkspaceInput{Slug: "second", Name: "Second"})
	if err != nil {
		t.Fatal(err)
	}
	// 重新创建 token 使其包含 second workspace
	tokenOut, err := svc.CreateToken(app.CreateTokenInput{
		Name:          "ws-test",
		Scopes:        []string{"workspace:read", "workspace:write"},
		WorkspaceRefs: []string{"local", ws.Slug},
	})
	if err != nil {
		t.Fatal(err)
	}
	authHeader := map[string]string{
		"Authorization": "Bearer " + tokenOut.RawToken,
		"Content-Type":  "application/json",
	}
	body := `{"workspace":"` + ws.Slug + `"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPut, "/api/v1/me/active_workspace", body, authHeader)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !json.Valid(rr.Body.Bytes()) {
		t.Fatalf("invalid json: %s", rr.Body.String())
	}
}
