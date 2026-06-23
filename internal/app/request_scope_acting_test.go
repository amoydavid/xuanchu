package app

import (
	"encoding/json"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

// TestAdminActingAuditTrace 验证 acting mode 下普通操作的 audit 行能追溯 server admin 来源。
// RuntimeContext 携带的 AdminActingSessionID / DelegatorAdminTokenID / DelegatorAdminTokenName
// 必须落到 audit_logs 的对应列，且不污染普通 user-agent 的 delegator 字段。
func TestAdminActingAuditTrace(t *testing.T) {
	store := newTestStore(t)
	svc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: 100}})
	if err != nil {
		t.Fatal(err)
	}
	// 模拟 acting 鉴权分支构造出的 runtime：复用解析出的 actor/workspace/role，
	// 只叠加 server admin acting trace 字段。
	rt := svc.Runtime()
	rt.AdminActingSessionID = "act-1"
	rt.DelegatorAdminTokenID = "admin-token-1"
	rt.DelegatorAdminTokenName = "ops-primary"
	svc.runtime = rt

	if _, err := svc.Add(AddInput{Title: "via acting"}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	rows, err := storage.NewAuditRepository(store.DB()).List(storage.AuditListOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 || rows[0].Action != "task.add" {
		t.Fatalf("audit rows = %#v", rows)
	}
	row := rows[0]
	if row.AdminActingSessionID == nil || *row.AdminActingSessionID != "act-1" {
		t.Fatalf("admin_acting_session_id = %#v", row.AdminActingSessionID)
	}
	if row.DelegatorAdminTokenID == nil || *row.DelegatorAdminTokenID != "admin-token-1" {
		t.Fatalf("delegator_admin_token_id = %#v", row.DelegatorAdminTokenID)
	}
	if row.DelegatorAdminTokenName != "ops-primary" {
		t.Fatalf("delegator_admin_token_name = %q", row.DelegatorAdminTokenName)
	}
	// 普通 user-agent delegator 字段必须保持独立，不被 acting trace 污染。
	if row.DelegatorTokenID != nil {
		t.Fatalf("delegator_token_id leaked into acting trace: %#v", row.DelegatorTokenID)
	}
	if row.DelegatorUserID != nil {
		t.Fatalf("delegator_user_id leaked into acting trace: %#v", row.DelegatorUserID)
	}
	// payload 不应包含 raw acting token。
	var payload map[string]any
	if err := json.Unmarshal([]byte(row.PayloadJSON), &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["token"]; ok {
		t.Fatalf("payload leaked token: %#v", payload)
	}
}
