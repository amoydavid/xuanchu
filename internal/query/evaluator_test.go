package query

import (
	"testing"
	"time"
)

func TestMatchTaskValueStatusAndProject(t *testing.T) {
	expr, err := ParseQuery(`status:pending project:ops`)
	if err != nil {
		t.Fatal(err)
	}
	proj := "ops"
	tv := TaskValue{Status: "pending", Project: &proj}
	ok, err := MatchTaskValue(expr, tv, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("应匹配")
	}
	// 不匹配的 project。
	other := "other"
	tv2 := TaskValue{Status: "pending", Project: &other}
	ok2, _ := MatchTaskValue(expr, tv2, time.Local)
	if ok2 {
		t.Fatal("project=other 不应匹配 project:ops")
	}
}

func TestMatchTaskValueTaskType(t *testing.T) {
	expr, err := ParseQuery(`task_type:occurrence`)
	if err != nil {
		t.Fatal(err)
	}
	tt := "occurrence"
	tv := TaskValue{TaskType: &tt}
	ok, _ := MatchTaskValue(expr, tv, time.Local)
	if !ok {
		t.Fatal("task_type:occurrence 应匹配")
	}
}

func TestMatchTaskValueSeriesID(t *testing.T) {
	expr, err := ParseQuery(`series_id:s1`)
	if err != nil {
		t.Fatal(err)
	}
	sid := "s1"
	tv := TaskValue{SeriesID: &sid}
	ok, _ := MatchTaskValue(expr, tv, time.Local)
	if !ok {
		t.Fatal("series_id:s1 应匹配")
	}
}

func TestMatchTaskValueDateIsNull(t *testing.T) {
	// projected occurrence 的 entry 为 nil，应命中 entry.isnull。
	expr, err := ParseQuery(`entry.isnull`)
	if err != nil {
		t.Fatal(err)
	}
	tv := TaskValue{Entry: nil}
	ok, _ := MatchTaskValue(expr, tv, time.Local)
	if !ok {
		t.Fatal("entry.isnull 应匹配 nil entry")
	}
	// materialized 有 entry。
	e := int64(100)
	tv2 := TaskValue{Entry: &e}
	ok2, _ := MatchTaskValue(expr, tv2, time.Local)
	if ok2 {
		t.Fatal("entry.isnull 不应匹配有值的 entry")
	}
}

func TestMatchTaskValueOr(t *testing.T) {
	expr, err := ParseQuery(`(status:pending or status:waiting)`)
	if err != nil {
		t.Fatal(err)
	}
	tv := TaskValue{Status: "waiting"}
	ok, _ := MatchTaskValue(expr, tv, time.Local)
	if !ok {
		t.Fatal("waiting 应匹配 (pending or waiting)")
	}
	tv2 := TaskValue{Status: "completed"}
	ok2, _ := MatchTaskValue(expr, tv2, time.Local)
	if ok2 {
		t.Fatal("completed 不应匹配 (pending or waiting)")
	}
}

func TestMatchTaskValueNot(t *testing.T) {
	expr, err := ParseQuery(`not status:completed`)
	if err != nil {
		t.Fatal(err)
	}
	tv := TaskValue{Status: "pending"}
	ok, _ := MatchTaskValue(expr, tv, time.Local)
	if !ok {
		t.Fatal("not status:completed 应匹配 pending")
	}
}

func TestMatchTaskValueStringNotEqual(t *testing.T) {
	expr := Predicate{Attribute: AttrStatus, Operator: OpNotEqual, Value: StringValue("deleted")}
	ok, err := MatchTaskValue(expr, TaskValue{Status: "pending"}, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("status != deleted 应匹配 pending")
	}
	ok, err = MatchTaskValue(expr, TaskValue{Status: "deleted"}, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("status != deleted 不应匹配 deleted")
	}
}

func TestMatchTaskValueTagAndAssignee(t *testing.T) {
	expr, err := ParseQuery(`+daily assignee:alice`)
	if err != nil {
		t.Fatal(err)
	}
	tv := TaskValue{Tags: []string{"daily"}, AssigneeIDs: []string{"alice"}}
	ok, _ := MatchTaskValue(expr, tv, time.Local)
	if !ok {
		t.Fatal("应匹配 +daily assignee:alice")
	}
}

func TestMatchTaskValueBare(t *testing.T) {
	expr, err := ParseQuery(`巡检`)
	if err != nil {
		t.Fatal(err)
	}
	tv := TaskValue{Title: "每日巡检"}
	ok, _ := MatchTaskValue(expr, tv, time.Local)
	if !ok {
		t.Fatal("bare text 应匹配标题包含")
	}
}

func TestMatchTaskValueUUID(t *testing.T) {
	expr, err := ParseQuery(`uuid:abc-123`)
	if err != nil {
		t.Fatal(err)
	}
	uuid := "abc-123"
	tv := TaskValue{UUID: &uuid}
	ok, _ := MatchTaskValue(expr, tv, time.Local)
	if !ok {
		t.Fatal("uuid 应匹配")
	}
}

func TestMatchTaskValueNilExpr(t *testing.T) {
	ok, err := MatchTaskValue(nil, TaskValue{Status: "pending"}, time.Local)
	if err != nil || !ok {
		t.Fatal("nil expr 应总是匹配")
	}
}

func TestMatchTaskValueAtUsesInjectedClockForRelativeDates(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, loc).Unix()
	due := time.Date(2025, 6, 1, 23, 59, 59, 0, loc).Unix()
	expr, err := ParseQuery(`due:today`)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := MatchTaskValueAt(expr, TaskValue{Due: &due}, now, loc)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("due:today must resolve against injected clock")
	}
}
