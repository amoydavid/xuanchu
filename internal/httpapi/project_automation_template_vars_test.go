package httpapi

import (
	"net/http"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/app"
)

func TestHTTPAutomationTemplateVars(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "project:read", "hook:read")
	// 通过 service 层创建项目。
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store, ActorRef: "local", WorkspaceRef: "local"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddProject(app.AddProjectInput{Slug: "adsops", Name: "广告投放优化"}); err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/projects/adsops/automation-template-vars", restfulFilterHeader(fixture.token))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	data := httpResponseDataMap(t, rr)
	triggers, ok := data["triggers"].([]any)
	if !ok || len(triggers) != 2 {
		t.Fatalf("triggers = %#v", data["triggers"])
	}
	// 验证 schedule 组包含 project.slug
	schedule := triggers[0].(map[string]any)
	if schedule["trigger"] != "schedule" {
		t.Fatalf("first trigger = %v", schedule["trigger"])
	}
	vars := schedule["vars"].([]any)
	found := false
	for _, v := range vars {
		if v.(map[string]any)["name"] == "project.slug" {
			found = true
		}
	}
	if !found {
		t.Fatalf("schedule vars missing project.slug: %#v", vars)
	}
}
