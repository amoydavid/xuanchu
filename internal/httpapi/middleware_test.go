package httpapi

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/logging"
)

func TestAccessLogUsesStructuredLogger(t *testing.T) {
	var buf bytes.Buffer
	logger, closeLogger, err := logging.Setup(logging.LogConfig{Format: "text"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeLogger() })

	fixture := newHTTPServerWithTokenFixture(t, "workspace:read")
	fixture.server.logger = logger
	workspace, err := fixture.server.store.LocalWorkspace()
	if err != nil {
		t.Fatal(err)
	}

	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/me", map[string]string{
		"Authorization": "Bearer " + fixture.token,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}

	logLine := buf.String()
	for _, want := range []string{
		"component=http",
		"operation=http_request",
		"method=GET",
		"path=/api/v1/me",
		"status=200",
		"actor_user_id=",
		"token_id=" + fixture.id,
		"workspace_id=" + workspace.ID,
		"workspace_ref=" + workspace.Slug,
		"duration_ms=",
	} {
		if !strings.Contains(logLine, want) {
			t.Fatalf("log = %q, want substring %q", logLine, want)
		}
	}
}
