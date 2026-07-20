package app

import "testing"

// 事务内 helper 提取后，公开 Add 仍必须保留“首槽已到期就立即物化”的既有行为。
func TestTaskSeriesAddStillMaterializesFirstOccurrence(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	project, err := svc.AddProject(AddProjectInput{Slug: "seriesadd", Name: "Series Add"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日检查", ProjectID: project.ID, RecurrenceRule: "daily", FirstDue: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.FirstOccurrence == nil || result.FirstOccurrence.UUID == nil || result.FirstOccurrence.RecurrenceInfo == nil || result.FirstOccurrence.RecurrenceInfo.SeriesID != result.Series.ID {
		t.Fatalf("first occurrence = %#v, series = %#v", result.FirstOccurrence, result.Series)
	}
}
