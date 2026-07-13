package cli

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/remote"
	"git.dajee.net/dajee/xuanchu/internal/task"
	"git.dajee.net/dajee/xuanchu/internal/taskseries"
	"github.com/spf13/cobra"
)

// TestSeriesCommandRegistered 验证 series 命令树包含所有子命令。
func TestSeriesCommandRegistered(t *testing.T) {
	root := &cobra.Command{}
	root.AddCommand(newSeriesCommand(Options{}))
	series, _, err := root.Find([]string{"series"})
	if err != nil {
		t.Fatalf("Find series: %v", err)
	}
	want := map[string]bool{
		"add": true, "list": true, "info": true, "modify": true,
		"occurrences": true, "stop": true, "skip": true,
	}
	for _, c := range series.Commands() {
		name := strings.Fields(c.Use)
		if len(name) > 0 {
			delete(want, name[0])
		}
	}
	if len(want) > 0 {
		t.Fatalf("缺少子命令: %v", want)
	}
}

// TestSeriesCommandHelpInChinese 验证 series 命令使用中文描述。
func TestSeriesCommandHelpInChinese(t *testing.T) {
	cmd := newSeriesCommand(Options{})
	if !strings.Contains(cmd.Short, "循环") {
		t.Fatalf("series Short 应含'循环': %q", cmd.Short)
	}
}

// TestParseSeriesDateFlag 验证日期 flag 解析。
func TestParseSeriesDateFlag(t *testing.T) {
	// unix 时间戳。
	ts, err := parseSeriesDateFlag("123")
	if err != nil || ts != 123 {
		t.Fatalf("unix: ts=%d err=%v", ts, err)
	}
	// YYYY-MM-DD。
	ts2, err := parseSeriesDateFlag("2030-01-15")
	if err != nil {
		t.Fatalf("date: %v", err)
	}
	if ts2 <= 0 {
		t.Fatalf("date ts = %d", ts2)
	}
	// 非法。
	if _, err := parseSeriesDateFlag("not-a-date"); err == nil {
		t.Fatal("非法日期应失败")
	}
}

// TestIsLikelyUUID 验证 UUID 判断。
func TestIsLikelyUUID(t *testing.T) {
	if !isLikelyUUID("11111111-1111-1111-1111-111111111111") {
		t.Fatal("标准 UUID 应识别为 UUID")
	}
	if isLikelyUUID("ops") {
		t.Fatal("slug 不应识别为 UUID")
	}
}

func TestRemoteSeriesDTOToViewKeepsProtocolFields(t *testing.T) {
	description, priority, until := "说明", "H", int64(9000)
	dto := remote.TaskSeriesDTO{
		ID: "series-1", WorkspaceID: "workspace-1", ProjectID: "project-1",
		Title: "每日巡检", Description: &description, Status: "active",
		RecurrenceRule: "daily", FirstDue: 5000, Until: &until, Priority: &priority,
		Tags: []string{"ops"}, UDAs: map[string]string{"estimate": "3"},
		CreatedBy: task.JSONUserInfo{ID: "user-1", Name: "alice"},
		Assignees: []task.JSONUserInfo{{ID: "user-2", Name: "bob"}},
		CreatedAt: 100, ModifiedAt: 200,
	}
	view := remoteSeriesDTOToView(dto)
	if view.ID != dto.ID || view.Title != dto.Title || view.RecurrenceRule != dto.RecurrenceRule || view.FirstDue != dto.FirstDue {
		t.Fatalf("series fields lost: %#v", view)
	}
	if !reflect.DeepEqual(view.Tags, dto.Tags) || view.UDAs["estimate"] != "3" || view.CreatedBy.ID != "user-1" || len(view.Assignees) != 1 {
		t.Fatalf("series retained fields lost: %#v", view)
	}
}

func TestRemoteOccurrenceDTOToViewKeepsProtocolFields(t *testing.T) {
	description, project, projectID, parent := "说明", "ops", "project-1", "parent-1"
	dto := remote.TaskOccurrenceDTO{
		ID: "occ:s1:5000", WorkspaceID: "workspace-1", ProjectID: &projectID, Project: &project,
		Title: "每日巡检", Description: &description, Status: "pending", Parent: &parent,
		Assignees: []task.JSONUserInfo{{ID: "user-1", Name: "alice"}},
		Depends:   []string{"dep-1"}, UDAs: map[string]string{"estimate": "3"},
	}
	view := remoteOccurrenceDTOToView(dto)
	if view.WorkspaceID != dto.WorkspaceID || view.ProjectID == nil || view.Description == nil || view.Parent == nil {
		t.Fatalf("occurrence fields lost: %#v", view)
	}
	if len(view.Assignees) != 1 || !reflect.DeepEqual(view.Depends, dto.Depends) || view.UDAs["estimate"].Raw != "3" {
		t.Fatalf("occurrence retained fields lost: %#v", view)
	}
}

func TestRenderSeriesJSONUsesProtocolFieldNames(t *testing.T) {
	var out bytes.Buffer
	renderSeriesDetail(&out, true, app.TaskSeriesDetailView{Series: app.TaskSeriesView{
		Series: taskseries.Series{ID: "series-1", Title: "每日巡检", Status: "active", RecurrenceRule: "daily", FirstDue: 5000},
	}})
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["id"] != "series-1" || payload["recurrence_rule"] != "daily" || payload["first_due"] == nil {
		t.Fatalf("protocol JSON = %#v", payload)
	}
	for _, retiredShape := range []string{"Series", "RecurrenceRule", "FirstDue"} {
		if _, ok := payload[retiredShape]; ok {
			t.Fatalf("JSON 不应包含 Go 字段名 %s: %#v", retiredShape, payload)
		}
	}
}

func TestRenderSeriesDetailShowsOccurrenceGroups(t *testing.T) {
	var out bytes.Buffer
	renderSeriesDetail(&out, false, app.TaskSeriesDetailView{
		Series:          app.TaskSeriesView{Series: taskseries.Series{ID: "series-1", Title: "每日巡检", Status: "active", RecurrenceRule: "daily", FirstDue: 5000}},
		OpenOccurrences: []app.TaskOccurrenceView{{ID: "occ:s1:5000", Title: "本次巡检", Status: "pending"}},
		RecentCompleted: []app.TaskOccurrenceView{{ID: "occ:s1:4000", Title: "昨日巡检", Status: "completed"}},
		RecentSkipped:   []app.TaskOccurrenceView{{ID: "occ:s1:3000", Title: "跳过巡检", Status: "deleted"}},
	})
	for _, want := range []string{"未完成实例", "最近完成", "最近跳过", "本次巡检", "昨日巡检", "跳过巡检"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("detail 缺少 %q: %s", want, out.String())
		}
	}
}

// 集成测试在 tests/integration 中通过二进制执行 series 全流程。
