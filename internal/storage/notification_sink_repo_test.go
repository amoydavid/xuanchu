package storage

import (
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func newNotificationTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	return store, ws.ID
}

func makeNotificationSink(wsID, name string, overrides ...func(*NotificationSink)) NotificationSink {
	enabled := true
	row := NotificationSink{
		ID:                  uuid.NewString(),
		WorkspaceID:         wsID,
		Name:                name,
		Type:                "webhook",
		EndpointMode:        "static_url",
		URL:                 "https://example.com/xuanchu/notifications",
		AllowedHostsJSON:    `["example.com"]`,
		HTTPMethod:          "POST",
		HeaderTemplatesJSON: `[]`,
		BodyTemplate:        "",
		BodyContentType:     "",
		SecretRefsJSON:      `{}`,
		Secret:              "secret",
		Enabled:             &enabled,
		TimeoutSeconds:      10,
		MaxAttempts:         5,
		CreatedBy:           "user-1",
		CreatedAt:           100,
		ModifiedAt:          100,
	}
	for _, fn := range overrides {
		fn(&row)
	}
	return row
}

func TestNotificationSinkRepositoryCRUD(t *testing.T) {
	store, wsID := newNotificationTestStore(t)
	repo := NewNotificationSinkRepository(store.DB())

	sink := makeNotificationSink(wsID, "openclaw")
	if err := repo.Create(sink); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := repo.GetByID(sink.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Name != "openclaw" || got.WorkspaceID != wsID {
		t.Fatalf("GetByID() = %#v", got)
	}

	rows, err := repo.List(wsID, false)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(rows) != 1 || rows[0].ID != sink.ID {
		t.Fatalf("List() = %#v", rows)
	}

	got.Secret = "new-secret"
	got.ModifiedAt = 200
	if err := repo.Update(got); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	updated, err := repo.GetByID(sink.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Secret != "new-secret" || updated.ModifiedAt != 200 {
		t.Fatalf("updated row = %#v", updated)
	}

	if err := repo.Delete(sink.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := repo.GetByID(sink.ID); err != ErrNotFound {
		t.Fatalf("GetByID(deleted) error = %v, want ErrNotFound", err)
	}
}

func TestNotificationSinkRepositoryNameUniquePerWorkspace(t *testing.T) {
	store, wsID := newNotificationTestStore(t)
	repo := NewNotificationSinkRepository(store.DB())

	if err := repo.Create(makeNotificationSink(wsID, "openclaw")); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(makeNotificationSink(wsID, "openclaw")); err == nil {
		t.Fatal("Create duplicate name error = nil, want unique constraint error")
	}
}

func TestNotificationSinkRepositoryStoresHTTPTemplateFields(t *testing.T) {
	store, wsID := newNotificationTestStore(t)
	repo := NewNotificationSinkRepository(store.DB())

	sink := makeNotificationSink(wsID, "feishu", func(s *NotificationSink) {
		s.Type = "http_template"
		s.EndpointMode = "config_value"
		s.URL = ""
		s.ConfigKey = "integrations.feishu.webhook_url"
		s.AllowedHostsJSON = `["open.feishu.cn"]`
		s.HeaderTemplatesJSON = `[{"name":"Content-Type","value":"application/json"},{"name":"Authorization","value":"Bearer {{secret.feishu_bot_token}}"}]`
		s.BodyTemplate = `{"msg_type":"text","content":{"text":"任务 {{task.task_slug}} 即将到期：{{task.title}}"}}`
		s.BodyContentType = "application/json"
		s.SecretRefsJSON = `{"feishu_bot_token":{"config_key":"integrations.feishu.bot_token"}}`
	})
	if err := repo.Create(sink); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := repo.GetByID(sink.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Type != "http_template" {
		t.Fatalf("Type = %q, want http_template", got.Type)
	}
	if got.HeaderTemplatesJSON != sink.HeaderTemplatesJSON {
		t.Fatalf("HeaderTemplatesJSON = %q, want %q", got.HeaderTemplatesJSON, sink.HeaderTemplatesJSON)
	}
	if got.BodyTemplate != sink.BodyTemplate {
		t.Fatalf("BodyTemplate = %q, want %q", got.BodyTemplate, sink.BodyTemplate)
	}
	if got.BodyContentType != "application/json" {
		t.Fatalf("BodyContentType = %q, want application/json", got.BodyContentType)
	}
	if got.SecretRefsJSON != sink.SecretRefsJSON {
		t.Fatalf("SecretRefsJSON = %q, want %q", got.SecretRefsJSON, sink.SecretRefsJSON)
	}
}
