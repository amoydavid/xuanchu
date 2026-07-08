package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestHTTPNotificationTemplateVars(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "notification:read")
	auth := map[string]string{"Authorization": "Bearer " + fixture.token}

	rr := requestHTTPBody(t, fixture.server, http.MethodGet, "/api/v1/notification-template-vars", "", auth)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data struct {
			Triggers []struct {
				Trigger string `json:"trigger"`
				Fields  []struct {
					Field string `json:"field"`
					Vars  []struct {
						Name        string `json:"name"`
						Description string `json:"description"`
						Dynamic     bool   `json:"dynamic"`
						PrefixGroup string `json:"prefix_group,omitempty"`
					} `json:"vars"`
				} `json:"fields"`
			} `json:"triggers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	// 必须有 reminder 和 event 两个 trigger。
	triggers := map[string]bool{}
	for _, tr := range resp.Data.Triggers {
		triggers[tr.Trigger] = true
	}
	if !triggers["reminder"] || !triggers["event"] {
		t.Fatalf("missing triggers, got %+v", triggers)
	}

	// reminder 的 body 字段里必须包含 task.title，不能包含 event.type。
	for _, tr := range resp.Data.Triggers {
		if tr.Trigger != "reminder" {
			continue
		}
		var bodyVars []string
		for _, f := range tr.Fields {
			if f.Field == "body" {
				for _, v := range f.Vars {
					bodyVars = append(bodyVars, v.Name)
				}
			}
		}
		if !containsStr(bodyVars, "task.title") {
			t.Errorf("reminder body vars missing task.title: %v", bodyVars)
		}
		if containsStr(bodyVars, "event.type") {
			t.Errorf("reminder body vars must not contain event.type: %v", bodyVars)
		}
	}
}

func containsStr(slice []string, s string) bool {
	for _, x := range slice {
		if x == s {
			return true
		}
	}
	return false
}
