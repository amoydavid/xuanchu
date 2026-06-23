package integration

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestE2EServerRuntimeDispatchersAttemptHookDelivery(t *testing.T) {
	bin := buildXuanchu(t)
	dir := t.TempDir()
	db := filepath.Join(dir, "xuanchu.db")
	configPath, logPath := writeE2ELogConfig(t, dir)
	secret := "runtime-secret-should-not-leak"

	run(t, bin, "--db", db, "notification", "sink", "add", "runtime-hook-sink",
		"--url", "http://127.0.0.1:1/webhook",
		"--secret", secret,
		"--max-attempts", "1",
	)
	hookOut := run(t, bin, "--db", db, "--json", "hook", "add", "runtime-hook",
		"--event", "task.created",
		"--sink", "runtime-hook-sink",
		"--max-attempts", "1",
	)
	hookID, _ := parseJSONMap(t, hookOut)["id"].(string)
	if hookID == "" {
		t.Fatalf("hook add output missing id: %s", hookOut)
	}
	ruleOut := run(t, bin, "--db", db, "--json", "notification", "rule", "add", "runtime-notification",
		"--event", "task.created",
		"--audience", "actor",
		"--sink", "runtime-hook-sink",
		"--template-subject", "Runtime notification",
		"--template-body", "{{event.type}} {{task.title}}",
	)
	ruleID, _ := parseJSONMap(t, ruleOut)["id"].(string)
	if ruleID == "" {
		t.Fatalf("notification rule add output missing id: %s", ruleOut)
	}
	token := parseRawToken(t, createTokenJSON(t, bin, "--db", db, "runtime-e2e", "*"))

	cmd, baseURL := startXuanchuServer(t, bin,
		"--config", configPath,
		"--db", db,
		"--hook-dispatcher-interval", "100ms",
		"--hook-dispatcher-batch-size", "1",
		"--notification-dispatcher-interval", "100ms",
		"--notification-dispatcher-batch-size", "1",
	)
	defer stopXuanchuServer(t, cmd)

	run(t, bin, "--server", baseURL, "--token", token, "add", "runtime dispatcher e2e task")
	hookDelivery := waitForHookDeliveryStatus(t, baseURL, token, hookID, "dead_lettered", 6*time.Second)
	if got, _ := hookDelivery["event_type"].(string); got != "task.created" {
		t.Fatalf("hook delivery = %#v, want task.created", hookDelivery)
	}
	if lastError, _ := hookDelivery["last_error"].(string); !strings.Contains(lastError, "SSRF validation failed") {
		t.Fatalf("hook delivery last_error = %q, want SSRF validation failure", lastError)
	}
	notificationDelivery := waitForNotificationDeliveryStatus(t, baseURL, token, "dead_lettered", 6*time.Second)
	if got, _ := notificationDelivery["event_type"].(string); got != "task.created" {
		t.Fatalf("notification delivery = %#v, want task.created", notificationDelivery)
	}
	if got, _ := notificationDelivery["rule_id"].(string); got != ruleID {
		t.Fatalf("notification delivery rule_id = %q, want %q", got, ruleID)
	}
	if lastError, _ := notificationDelivery["last_error"].(string); !strings.Contains(lastError, "endpoint validation failed") {
		t.Fatalf("notification delivery last_error = %q, want endpoint validation failure", lastError)
	}
	deliveryText := toJSONString(t, map[string]any{"hook": hookDelivery, "notification": notificationDelivery})
	if strings.Contains(deliveryText, secret) {
		t.Fatalf("hook delivery view leaked secret: %s", deliveryText)
	}

	stopXuanchuServer(t, cmd)
	assertLogContains(t, logPath,
		"component=hook_dispatcher",
		"operation=hook_delivery_attempt",
		"result=dead_lettered",
		"component=notification_dispatcher",
		"operation=notification_delivery_attempt",
	)
	assertLogDoesNotContain(t, logPath, secret)
}

func waitForHookDeliveryStatus(t *testing.T, baseURL, token, hookID, status string, timeout time.Duration) map[string]any {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last []any
	for time.Now().Before(deadline) {
		payload := httpJSON(t, http.MethodGet, baseURL+"/api/v1/hooks/"+hookID+"/deliveries?limit=1", nil, authHeaders(token))
		rows, _ := payload["data"].([]any)
		last = rows
		if len(rows) > 0 {
			row, _ := rows[0].(map[string]any)
			if row["status"] == status {
				return row
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("delivery for hook %s did not reach status %s; last rows = %#v", hookID, status, last)
	return nil
}

func waitForNotificationDeliveryStatus(t *testing.T, baseURL, token, status string, timeout time.Duration) map[string]any {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last []any
	for time.Now().Before(deadline) {
		payload := httpJSON(t, http.MethodGet, baseURL+"/api/v1/notification-deliveries?limit=1", nil, authHeaders(token))
		rows, _ := payload["data"].([]any)
		last = rows
		if len(rows) > 0 {
			row, _ := rows[0].(map[string]any)
			if row["status"] == status {
				return row
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("notification delivery did not reach status %s; last rows = %#v", status, last)
	return nil
}

func assertLogDoesNotContain(t *testing.T, path string, forbidden string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), forbidden) {
		t.Fatalf("log file %s leaked %q: %s", path, forbidden, raw)
	}
}
