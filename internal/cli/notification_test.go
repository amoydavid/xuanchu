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

func TestCLIReminderRuleScheduleFilterLifecycle(t *testing.T) {
	var stdout, stderr bytes.Buffer
	db := setupHookTestDB(t)
	opts := setupHookTestOpts(&stdout, &stderr)

	cmd := NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "notification", "sink", "add", "openclaw", "--url", "https://example.com/notify"}); err != nil {
		t.Fatalf("sink add error = %v", err)
	}

	stdout.Reset()
	cmd = NewRootCommand(opts)
	filter := "end.isnull and start.isnull and due.after:now and due.before:now+24h"
	if err := Execute(cmd, opts, []string{"--db", db, "--json", "reminder", "rule", "add", "due-soon-24h", "--schedule", "daily@08:50", "--filter", filter, "--audience", "assignees", "--sink", "openclaw"}); err != nil {
		t.Fatalf("reminder rule add error = %v", err)
	}
	var rule map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &rule); err != nil {
		t.Fatalf("rule json error = %v: %s", err, stdout.String())
	}
	if rule["schedule_type"] != "daily_at" || rule["schedule_value"] != "08:50" || rule["filter_source"] != filter {
		t.Fatalf("rule schedule/filter = %#v", rule)
	}

	stdout.Reset()
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "reminder", "rule", "list"}); err != nil {
		t.Fatalf("reminder rule list error = %v", err)
	}
	if !strings.Contains(stdout.String(), "daily@08:50") {
		t.Fatalf("rule list output = %q", stdout.String())
	}

	stdout.Reset()
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "--json", "reminder", "rule", "modify", rule["id"].(string), "--schedule", "daily@09:30", "--filter", "end.isnull and due.before:now"}); err != nil {
		t.Fatalf("reminder rule modify error = %v", err)
	}
	var modified map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &modified); err != nil {
		t.Fatalf("modified rule json error = %v: %s", err, stdout.String())
	}
	if modified["schedule_type"] != "daily_at" || modified["schedule_value"] != "09:30" || modified["filter_source"] != "end.isnull and due.before:now" {
		t.Fatalf("modified schedule/filter = %#v", modified)
	}
}

func TestCLINotificationRuleLifecycle(t *testing.T) {
	var stdout, stderr bytes.Buffer
	db := setupHookTestDB(t)
	opts := setupHookTestOpts(&stdout, &stderr)

	cmd := NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "notification", "sink", "add", "openclaw", "--url", "https://example.com/notify"}); err != nil {
		t.Fatalf("sink add error = %v", err)
	}

	stdout.Reset()
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "--json", "notification", "rule", "add", "task-unblocked-openclaw",
		"--event", "task.unblocked",
		"--filter", "end.isnull",
		"--audience", "assignees",
		"--sink", "openclaw",
		"--template-subject", "任务已解除阻塞",
		"--template-body", "{{task.description}}",
	}); err != nil {
		t.Fatalf("notification rule add error = %v", err)
	}
	var rule map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &rule); err != nil {
		t.Fatalf("rule json error = %v: %s", err, stdout.String())
	}
	ruleID, _ := rule["id"].(string)
	if ruleID == "" || rule["event_type"] != "task.unblocked" || rule["audience_type"] != "assignees" || rule["filter_source"] != "end.isnull" {
		t.Fatalf("rule = %#v", rule)
	}

	stdout.Reset()
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "notification", "rule", "list"}); err != nil {
		t.Fatalf("notification rule list error = %v", err)
	}
	if !strings.Contains(stdout.String(), "task-unblocked-openclaw") {
		t.Fatalf("rule list output = %q", stdout.String())
	}

	stdout.Reset()
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "--json", "notification", "rule", "modify", ruleID, "--name", "task-unblocked-renamed", "--audience", "actor"}); err != nil {
		t.Fatalf("notification rule modify error = %v", err)
	}
	var modified map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &modified); err != nil {
		t.Fatalf("modified rule json error = %v: %s", err, stdout.String())
	}
	if modified["name"] != "task-unblocked-renamed" || modified["audience_type"] != "actor" {
		t.Fatalf("modified rule = %#v", modified)
	}

	stdout.Reset()
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "notification", "rule", "disable", ruleID}); err != nil {
		t.Fatalf("notification rule disable error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Disabled notification rule") {
		t.Fatalf("disable output = %q", stdout.String())
	}

	stdout.Reset()
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "notification", "rule", "enable", ruleID}); err != nil {
		t.Fatalf("notification rule enable error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Enabled notification rule") {
		t.Fatalf("enable output = %q", stdout.String())
	}

	stdout.Reset()
	cmd = NewRootCommand(opts)
	if err := Execute(cmd, opts, []string{"--db", db, "notification", "rule", "delete", ruleID}); err != nil {
		t.Fatalf("notification rule delete error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Deleted notification rule") {
		t.Fatalf("delete output = %q", stdout.String())
	}
}
