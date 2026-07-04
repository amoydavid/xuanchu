package storage

import (
	"path/filepath"
	"testing"
)

func TestAuditRepositoryListFiltersByTargetAndAction(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := NewAuditRepository(store.DB())
	workspaceID := "ws-1"

	// 两条不同 task 的 task.modify，加一条 task.add。
	entries := []AuditLogEntry{
		{
			ActorType:   "user",
			WorkspaceID: &workspaceID,
			Action:      "task.modify",
			TargetType:  "task",
			TargetID:    "task-1",
			PayloadJSON: "{}",
			CreatedAt:   100,
		},
		{
			ActorType:   "user",
			WorkspaceID: &workspaceID,
			Action:      "task.modify",
			TargetType:  "task",
			TargetID:    "task-2",
			PayloadJSON: "{}",
			CreatedAt:   200,
		},
		{
			ActorType:   "user",
			WorkspaceID: &workspaceID,
			Action:      "task.add",
			TargetType:  "task",
			TargetID:    "task-1",
			PayloadJSON: "{}",
			CreatedAt:   300,
		},
	}
	for _, entry := range entries {
		// 让 DB 自增 ID，避免重复主键。
		entry.ID = 0
		if err := repo.Append(entry); err != nil {
			t.Fatalf("Append() error = %v", err)
		}
	}

	targetType := "task"
	targetID := "task-1"
	action := "task.modify"
	rows, err := repo.List(AuditListOptions{
		WorkspaceID: &workspaceID,
		TargetType:  &targetType,
		TargetID:    &targetID,
		Action:      &action,
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].TargetID != "task-1" || rows[0].Action != "task.modify" {
		t.Fatalf("rows[0] = %#v", rows[0])
	}
}
