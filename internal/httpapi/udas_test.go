package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/app"
)

func TestHTTPWorkspaceUDACRUDAndConfigCompatibility(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "config:read", "config:write")
	headers := authHeaderJSON(fixture.token)

	put := requestHTTPBody(t, fixture.server, http.MethodPut, "/api/v1/udas/estimate?workspace=local", `{
		"type":"numeric","label":" 工作量 ","values":["1.0","2","2.00"],"default":"2.0"
	}`, headers)
	if put.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", put.Code, put.Body.String())
	}
	row := httpResponseDataMap(t, put)
	if row["name"] != "estimate" || row["type"] != "numeric" || row["label"] != "工作量" || row["default"] != "2" {
		t.Fatalf("PUT data=%#v", row)
	}
	values, _ := row["values"].([]any)
	if len(values) != 2 || values[0] != "1" || values[1] != "2" {
		t.Fatalf("values=%#v, want [1 2]", row["values"])
	}

	getConfig := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/config/uda.estimate.default?workspace=local", headers)
	if getConfig.Code != http.StatusOK || !strings.Contains(getConfig.Body.String(), `"value":"2"`) {
		t.Fatalf("config GET status=%d body=%s", getConfig.Code, getConfig.Body.String())
	}

	list := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/udas?workspace=local", headers)
	if list.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", list.Code, list.Body.String())
	}
	var payload struct {
		Data []app.WorkspaceUDAFieldView `json:"data"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != 1 || payload.Data[0].Source != "database" || payload.Data[0].Values == nil {
		t.Fatalf("list=%#v body=%s", payload.Data, list.Body.String())
	}

	del := requestHTTP(t, fixture.server, http.MethodDelete, "/api/v1/udas/estimate?workspace=local", headers)
	if del.Code != http.StatusOK {
		t.Fatalf("DELETE status=%d body=%s", del.Code, del.Body.String())
	}
}

func TestHTTPWorkspaceUDARejectsInvalidDefinition(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "config:write")
	rr := requestHTTPBody(t, fixture.server, http.MethodPut, "/api/v1/udas/estimate?workspace=local", `{
		"type":"numeric","values":["1"],"default":"2"
	}`, authHeaderJSON(fixture.token))
	assertHTTPErrorCode(t, rr, http.StatusUnprocessableEntity, "uda_definition_invalid")
}

func TestHTTPWorkspaceUDARuntimeDefinitionCanBeOverriddenAndRestored(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "config:read", "config:write")
	fixture.server = NewServer(Options{
		Store: fixture.server.store,
		RuntimeConfig: map[string]string{
			"uda.source.type":   "string",
			"uda.source.label":  "运行时来源",
			"uda.source.values": "organic,paid",
		},
	})
	headers := authHeaderJSON(fixture.token)

	list := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/udas?workspace=local", headers)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"source":"runtime"`) {
		t.Fatalf("runtime list status=%d body=%s", list.Code, list.Body.String())
	}
	assertHTTPErrorCode(t,
		requestHTTP(t, fixture.server, http.MethodDelete, "/api/v1/udas/source?workspace=local", headers),
		http.StatusConflict, "uda_runtime_readonly",
	)

	put := requestHTTPBody(t, fixture.server, http.MethodPut, "/api/v1/udas/source?workspace=local", `{
		"type":"string","label":"Workspace 来源","values":["direct"]
	}`, headers)
	if put.Code != http.StatusOK || !strings.Contains(put.Body.String(), `"source":"database_override"`) {
		t.Fatalf("override PUT status=%d body=%s", put.Code, put.Body.String())
	}
	if rr := requestHTTP(t, fixture.server, http.MethodDelete, "/api/v1/udas/source?workspace=local", headers); rr.Code != http.StatusOK {
		t.Fatalf("override DELETE status=%d body=%s", rr.Code, rr.Body.String())
	}
	list = requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/udas?workspace=local", headers)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"label":"运行时来源"`) || !strings.Contains(list.Body.String(), `"source":"runtime"`) {
		t.Fatalf("restored list status=%d body=%s", list.Code, list.Body.String())
	}
}

func TestHTTPWorkspaceUDARequiresMatchingCapabilities(t *testing.T) {
	readOnly := newHTTPServerWithTokenFixture(t, "config:read")
	readHeaders := authHeaderJSON(readOnly.token)
	if rr := requestHTTP(t, readOnly.server, http.MethodGet, "/api/v1/udas?workspace=local", readHeaders); rr.Code != http.StatusOK {
		t.Fatalf("read status=%d body=%s", rr.Code, rr.Body.String())
	}
	assertHTTPErrorCode(t,
		requestHTTPBody(t, readOnly.server, http.MethodPut, "/api/v1/udas/x?workspace=local", `{"type":"string"}`, readHeaders),
		http.StatusForbidden, "token_scope_denied",
	)

	writeOnly := newHTTPServerWithTokenFixture(t, "config:write")
	writeHeaders := authHeaderJSON(writeOnly.token)
	if rr := requestHTTPBody(t, writeOnly.server, http.MethodPut, "/api/v1/udas/x?workspace=local", `{"type":"string"}`, writeHeaders); rr.Code != http.StatusOK {
		t.Fatalf("write status=%d body=%s", rr.Code, rr.Body.String())
	}
	assertHTTPErrorCode(t,
		requestHTTP(t, writeOnly.server, http.MethodGet, "/api/v1/udas?workspace=local", writeHeaders),
		http.StatusForbidden, "token_scope_denied",
	)
}

func TestHumaRoutesDoNotExposeProjectUDAResource(t *testing.T) {
	server := NewServer(Options{Store: openHTTPTestStore(t)})
	for _, route := range server.humaRoutes() {
		if strings.Contains(route.Path, "/projects/") && strings.Contains(route.Path, "/udas") {
			t.Fatalf("unexpected project UDA route %s %s", route.Method, route.Path)
		}
	}
}
