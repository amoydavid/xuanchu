package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/app"
)

func TestUserListReturnsUsers(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "user:read")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddUser(app.AddUserInput{Name: "alice"}); err != nil {
		t.Fatal(err)
	}
	authHeader := map[string]string{"Authorization": "Bearer " + fixture.token}
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/users", authHeader)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) < 2 {
		t.Fatalf("expected at least 2 users, got %d: %s", len(payload.Data), rr.Body.String())
	}
	assertSnakeCaseResponse(t, rr.Body.String())
}

func TestUserCreateCreatesUser(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "user:write")
	authHeader := map[string]string{
		"Authorization": "Bearer " + fixture.token,
		"Content-Type":  "application/json",
	}
	body := `{"name":"bob","display_name":"Bob Zhang","email":"bob@example.com"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/users", body, authHeader)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Data struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Name != "bob" {
		t.Fatalf("expected name=bob, got %s: %s", payload.Data.Name, rr.Body.String())
	}
	if payload.Data.DisplayName != "Bob Zhang" {
		t.Fatalf("expected display_name=Bob Zhang, got %q: %s", payload.Data.DisplayName, rr.Body.String())
	}
	assertSnakeCaseResponse(t, rr.Body.String())
}

func TestUserModifyUpdatesDisplayNameOnly(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "user:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	user, err := svc.AddUser(app.AddUserInput{Name: "erin", DisplayName: "Erin Old"})
	if err != nil {
		t.Fatal(err)
	}
	authHeader := map[string]string{
		"Authorization": "Bearer " + fixture.token,
		"Content-Type":  "application/json",
	}
	rr := requestHTTPBody(t, fixture.server, http.MethodPatch, "/api/v1/users/"+user.ID, `{"display_name":"Erin New"}`, authHeader)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Data struct {
			Name        string `json:"name"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Name != "erin" {
		t.Fatalf("name changed to %q", payload.Data.Name)
	}
	if payload.Data.DisplayName != "Erin New" {
		t.Fatalf("display_name = %q, want Erin New", payload.Data.DisplayName)
	}
}

func TestUserInfoReturnsUser(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "user:read")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	user, err := svc.AddUser(app.AddUserInput{Name: "carol"})
	if err != nil {
		t.Fatal(err)
	}
	authHeader := map[string]string{"Authorization": "Bearer " + fixture.token}
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/users/"+user.ID, authHeader)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "carol") {
		t.Fatalf("expected carol in body: %s", rr.Body.String())
	}
}

func TestUserInfoByNameReturnsUser(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "user:read")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddUser(app.AddUserInput{Name: "dave"}); err != nil {
		t.Fatal(err)
	}
	authHeader := map[string]string{"Authorization": "Bearer " + fixture.token}
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/users/dave", authHeader)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestExternalIDBindUnbindAndList(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "user:read,user:write,task:read")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	svc.BindExternalID(svc.Runtime().ActorUserID, "feishu", "ou_http_test")
	authHeader := map[string]string{"Authorization": "Bearer " + fixture.token}

	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/users/local/external-ids", `{"provider":"feishu","external_id":"ou_bind_http"}`, authHeader)
	if rr.Code != http.StatusCreated {
		t.Fatalf("bind: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	rr = requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/users/local/external-ids", authHeader)
	if rr.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	rr = requestHTTP(t, fixture.server, http.MethodDelete, "/api/v1/users/local/external-ids/feishu/ou_bind_http", authHeader)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("unbind: expected 204, got %d: %s", rr.Code, rr.Body.String())
	}
}
