package app

import (
	"reflect"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/task"
)

func TestTaskModifyAuditPayloadIncludesStableChanges(t *testing.T) {
	beforeProject := "ops"
	afterProject := "agentapi"
	beforeProjectID := "project-before"
	afterProjectID := "project-after"
	previousDue := int64(1783036800)
	currentPriority := "H"
	previousDescription := "旧描述"
	currentDescription := "新描述"

	payload := taskModifyAuditPayload(projectChange{
		Before: projectBinding{ID: &beforeProjectID, Slug: &beforeProject},
		After:  projectBinding{ID: &afterProjectID, Slug: &afterProject},
	}, TaskChangeDiff{
		AssigneesChanged: true,
		AddedAssignees:   []task.UserInfo{{ID: "u2", Name: "lisi", DisplayName: "李四"}},
		RemovedAssignees: []task.UserInfo{{ID: "u1", Name: "zhangsan", DisplayName: "张三"}},
		DueChanged:       true,
		PreviousDue:      &previousDue,
		CurrentDue:       nil,
		PriorityChanged:  true,
		PreviousPriority: nil,
		CurrentPriority:  &currentPriority,
		ProjectChanged:   true,
		PreviousProject:  &beforeProject,
		CurrentProject:   &afterProject,
		TagsChanged:      true,
		AddedTags:        []string{"dashboard"},
		RemovedTags:      []string{"ads"},
		TitleChanged:     true,
		PreviousTitle:    "旧标题",
		CurrentTitle:     "新标题",
		DescriptionChanged:  true,
		PreviousDescription: &previousDescription,
		CurrentDescription:  &currentDescription,
	})

	// 旧 project 字段保留。
	if payload["before_project_slug"] != "ops" || payload["after_project_slug"] != "agentapi" {
		t.Fatalf("project payload changed: %#v", payload)
	}

	changes, ok := payload["changes"].([]map[string]any)
	if !ok {
		t.Fatalf("changes type = %T", payload["changes"])
	}

	// 固定顺序断言。
	gotFields := make([]string, 0, len(changes))
	for _, change := range changes {
		gotFields = append(gotFields, change["field"].(string))
	}
	wantFields := []string{"assignees", "due", "priority", "project", "tags", "title", "description"}
	if !reflect.DeepEqual(gotFields, wantFields) {
		t.Fatalf("fields = %#v, want %#v", gotFields, wantFields)
	}

	// assignees 必须含 display_name，不能只输出裸 UUID。
	added := changes[0]["added"].([]any)[0].(map[string]any)
	if added["name"] != "lisi" || added["display_name"] != "李四" {
		t.Fatalf("assignee payload = %#v", added)
	}

	// due 清空场景：current key 必须保留（即使值为 nil）。
	dueChange := changes[1]
	if _, ok := dueChange["current"]; !ok {
		t.Fatal("due current key missing; explicit null must be preserved")
	}
	if dueChange["current"] != nil {
		t.Fatalf("due current = %#v, want nil", dueChange["current"])
	}

	// priority 新增场景：previous 为 nil 也必须保留 key。
	priorityChange := changes[2]
	if _, ok := priorityChange["previous"]; !ok {
		t.Fatal("priority previous key missing")
	}
}

func TestTaskModifyAuditPayloadEmptyChangesIsArray(t *testing.T) {
	// 没有任何字段变化时，changes 必须是空数组而不是 nil，
	// 让新写入的 payload 与历史缺字段区分开。
	payload := taskModifyAuditPayload(projectChange{}, TaskChangeDiff{})
	changes, ok := payload["changes"].([]map[string]any)
	if !ok {
		t.Fatalf("changes type = %T", payload["changes"])
	}
	if len(changes) != 0 {
		t.Fatalf("changes = %#v, want empty", changes)
	}
}

func TestTaskModifyAuditPayloadSkipsEmptySetChanges(t *testing.T) {
	// assignees 标记变化但 added/removed 都空（理论不应发生）时不写入。
	payload := taskModifyAuditPayload(projectChange{}, TaskChangeDiff{
		AssigneesChanged: true,
	})
	changes := payload["changes"].([]map[string]any)
	if len(changes) != 0 {
		t.Fatalf("changes = %#v, want empty for empty set", changes)
	}
}

func TestTaskModifyAuditPayloadIncludesLowFrequencyFields(t *testing.T) {
	// 覆盖 wait/scheduled/until/depends/udas 低频字段。
	wait := int64(1_783_036_800)
	scheduled := int64(1_783_123_200)
	until := int64(1_783_209_600)
	payload := taskModifyAuditPayload(projectChange{}, TaskChangeDiff{
		WaitChanged:      true,
		PreviousWait:     nil,
		CurrentWait:      &wait,
		ScheduledChanged: true,
		PreviousScheduled: &scheduled,
		CurrentScheduled: nil,
		UntilChanged:     true,
		PreviousUntil:    nil,
		CurrentUntil:     &until,
		DependsChanged:   true,
		AddedDepends:     []string{"dep-1"},
		RemovedDepends:   []string{"dep-0"},
		UDAsChanged:      true,
		AddedUDAKeys:     []string{"effort"},
		ChangedUDAs: []UDAValueChange{{
			Name:  "effort",
			After: &task.UDAValue{Name: "effort", Raw: "2h", Type: "string"},
		}},
	})

	changes, ok := payload["changes"].([]map[string]any)
	if !ok {
		t.Fatalf("changes type = %T", payload["changes"])
	}
	// 完整顺序断言：低频字段排在 7 个标准字段之后。
	gotFields := make([]string, 0, len(changes))
	for _, change := range changes {
		gotFields = append(gotFields, change["field"].(string))
	}
	wantFields := []string{"wait", "scheduled", "until", "depends", "udas"}
	if !reflect.DeepEqual(gotFields, wantFields) {
		t.Fatalf("fields = %#v, want %#v", gotFields, wantFields)
	}

	// wait 标量：previous nil 也必须保留 key。
	waitChange := changes[0]
	if _, ok := waitChange["previous"]; !ok {
		t.Fatal("wait previous key missing")
	}

	// depends 集合：added/removed 都要存在。
	dependsChange := changes[3]
	_, hasAdded := dependsChange["added"]
	_, hasRemoved := dependsChange["removed"]
	if !hasAdded || !hasRemoved {
		t.Fatalf("depends change missing added/removed: %#v", dependsChange)
	}

	// udas：用 entries 承载。
	udasChange := changes[4]
	entries, ok := udasChange["entries"].([]map[string]any)
	if !ok || len(entries) != 1 {
		t.Fatalf("udas entries = %#v", udasChange["entries"])
	}
	if entries[0]["name"] != "effort" {
		t.Fatalf("uda entry name = %#v", entries[0]["name"])
	}
	// 新增 UDA：previous 必须为 nil，current 为值。
	if entries[0]["previous"] != nil {
		t.Fatalf("uda previous = %#v, want nil", entries[0]["previous"])
	}
	if entries[0]["current"] != "2h" {
		t.Fatalf("uda current = %#v, want 2h", entries[0]["current"])
	}
}
