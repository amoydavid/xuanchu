package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestNotificationCommandRegistered(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := NewRootCommand(setupHookTestOpts(&stdout, &stderr))
	notificationCmd, _, err := cmd.Find([]string{"notification"})
	if err != nil {
		t.Fatalf("Find(notification) error = %v", err)
	}
	if notificationCmd == nil || notificationCmd.Name() != "notification" {
		t.Fatalf("Find(notification) = %#v", notificationCmd)
	}
	reminderCmd, _, err := cmd.Find([]string{"reminder"})
	if err != nil {
		t.Fatalf("Find(reminder) error = %v", err)
	}
	if reminderCmd == nil || reminderCmd.Name() != "reminder" {
		t.Fatalf("Find(reminder) = %#v", reminderCmd)
	}
}

func TestCLINotificationSinkHTTPTemplateLifecycle(t *testing.T) {
	var stdout, stderr bytes.Buffer
	db := setupHookTestDB(t)
	opts := setupHookTestOpts(&stdout, &stderr)

	cmd := NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "user", "add", "alice"}); err != nil {
		t.Fatalf("user add error = %v", err)
	}
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "config", "schema", "set", "integrations.feishu.webhook_url", "type:string", "scopes:workspace"}); err != nil {
		t.Fatalf("config schema webhook_url error = %v", err)
	}
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "config", "schema", "set", "integrations.feishu.bot_token", "type:string", "scopes:workspace", "secret:true"}); err != nil {
		t.Fatalf("config schema bot_token error = %v", err)
	}

	stdout.Reset()
	cmd = NewRootCommand(opts)
	err := Execute(cmd, opts, []string{"--db", db, "--json", "notification", "sink", "add", "feishu",
		"--type", "http_template",
		"--endpoint-mode", "config_value",
		"--config-key", "integrations.feishu.webhook_url",
		"--allowed-host", "open.feishu.cn",
		"--header", "Authorization=Bearer {{secret.bot_token}}",
		"--body-content-type", "application/json",
		"--body-template", `{"msg_type":"text","content":{"text":"{{task.task_slug}}"}}`,
		"--secret-ref", "bot_token=integrations.feishu.bot_token",
	})
	if err != nil {
		t.Fatalf("notification sink add error = %v", err)
	}
	var sink map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &sink); err != nil {
		t.Fatalf("sink json error = %v: %s", err, stdout.String())
	}
	sinkID, _ := sink["id"].(string)
	if sinkID == "" || sink["type"] != "http_template" {
		t.Fatalf("sink = %#v", sink)
	}
	if strings.Contains(stdout.String(), "bot_token") && strings.Contains(stdout.String(), "secret-token") {
		t.Fatalf("sink output leaks secret: %s", stdout.String())
	}

	stdout.Reset()
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "notification", "sink", "disable", sinkID}); err != nil {
		t.Fatalf("sink disable error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Disabled notification sink") {
		t.Fatalf("disable output = %q", stdout.String())
	}
}

func TestCLIReminderRuleLifecycle(t *testing.T) {
	var stdout, stderr bytes.Buffer
	db := setupHookTestDB(t)
	opts := setupHookTestOpts(&stdout, &stderr)

	cmd := NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "user", "add", "alice"}); err != nil {
		t.Fatalf("user add error = %v", err)
	}
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "notification", "sink", "add", "openclaw", "--url", "https://example.com/notify"}); err != nil {
		t.Fatalf("sink add error = %v", err)
	}

	stdout.Reset()
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "--json", "reminder", "rule", "add", "due-before", "--trigger", "due_before", "--offset", "4h", "--audience", "assignees", "--sink", "openclaw"}); err != nil {
		t.Fatalf("reminder rule add error = %v", err)
	}
	var rule map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &rule); err != nil {
		t.Fatalf("rule json error = %v: %s", err, stdout.String())
	}
	ruleID, _ := rule["id"].(string)
	if ruleID == "" || rule["trigger_type"] != "due_before" {
		t.Fatalf("rule = %#v", rule)
	}

	stdout.Reset()
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "reminder", "rule", "list"}); err != nil {
		t.Fatalf("reminder rule list error = %v", err)
	}
	if !strings.Contains(stdout.String(), "due-before") {
		t.Fatalf("rule list output = %q", stdout.String())
	}
}
