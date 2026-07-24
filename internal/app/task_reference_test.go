package app

import (
	"encoding/json"
	"reflect"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

func TestResolveTaskReferenceForReadTreatsOccurrenceAliasesAsOneResource(t *testing.T) {
	svc, series, _, slot, _ := newOccurrenceMergeFixture(t)
	occurrenceRef := OccurrenceRef(series.ID, slot)
	materialized, existed, err := svc.MaterializeOccurrenceForWrite(occurrenceRef)
	if err != nil || existed {
		t.Fatalf("MaterializeOccurrenceForWrite = (%#v, %v, %v)", materialized, existed, err)
	}
	if materialized.Project == nil || materialized.ProjectSeq == nil {
		t.Fatalf("materialized project binding = (%#v, %#v)", materialized.Project, materialized.ProjectSeq)
	}
	taskSlug := taskSlugOf(materialized)
	if taskSlug == "" {
		t.Fatal("materialized task_slug is empty")
	}

	for _, ref := range []string{materialized.UUID, taskSlug, occurrenceRef} {
		t.Run(ref, func(t *testing.T) {
			got, err := svc.ResolveTaskReferenceForRead(ref)
			if err != nil {
				t.Fatalf("ResolveTaskReferenceForRead(%q): %v", ref, err)
			}
			if got.Kind != TaskResourceOccurrence {
				t.Fatalf("Kind = %q，期望 occurrence", got.Kind)
			}
			if got.StableID != occurrenceRef {
				t.Fatalf("StableID = %q，期望 %q", got.StableID, occurrenceRef)
			}
			if got.UUID == nil || *got.UUID != materialized.UUID {
				t.Fatalf("UUID = %#v，期望 %q", got.UUID, materialized.UUID)
			}
			if got.TaskSlug == nil || *got.TaskSlug != taskSlug {
				t.Fatalf("TaskSlug = %#v，期望 %q", got.TaskSlug, taskSlug)
			}
			if got.OccurrenceRef == nil || *got.OccurrenceRef != occurrenceRef {
				t.Fatalf("OccurrenceRef = %#v，期望 %q", got.OccurrenceRef, occurrenceRef)
			}
			if got.Materialization != "materialized" || got.View.RecurrenceInfo == nil {
				t.Fatalf("resolution view = %#v", got)
			}
		})
	}
}

func TestResolveTaskReferenceForReadKeepsProjectedReadOnly(t *testing.T) {
	svc, series, _, slot, _ := newOccurrenceMergeFixture(t)
	ref := OccurrenceRef(series.ID, slot)
	before := occurrenceRowCount(t, svc, svc.workspaceID)

	got, err := svc.ResolveTaskReferenceForRead(ref)
	if err != nil {
		t.Fatalf("ResolveTaskReferenceForRead(projected): %v", err)
	}
	if got.Kind != TaskResourceOccurrence || got.Materialization != "projected" {
		t.Fatalf("resolution = %#v", got)
	}
	if got.StableID != ref || got.UUID != nil || got.TaskSlug != nil || got.Task != nil {
		t.Fatalf("projected identity = %#v", got)
	}
	if after := occurrenceRowCount(t, svc, svc.workspaceID); after != before {
		t.Fatalf("projected read wrote rows: before=%d after=%d", before, after)
	}
}

func TestModifyOccurrenceAliasesRecordsOnlyChangedOverrides(t *testing.T) {
	svc, series, _, slot, _ := newOccurrenceMergeFixture(t)
	if err := svc.DefineUDA("effort", "string", "Effort", nil, ""); err != nil {
		t.Fatalf("DefineUDA: %v", err)
	}
	ref := OccurrenceRef(series.ID, slot)
	materialized, _, err := svc.MaterializeOccurrenceForWrite(ref)
	if err != nil {
		t.Fatalf("MaterializeOccurrenceForWrite: %v", err)
	}
	slug := taskSlugOf(materialized)
	description := "检查预算"
	priority := "H"
	due := slot + 3600

	changes := []struct {
		ref   string
		input ModifyInput
	}{
		{ref: ref, input: ModifyInput{Title: strptr("每日预算巡检"), Description: &description}},
		{ref: materialized.UUID, input: ModifyInput{Priority: &priority, Due: &due}},
		{ref: slug, input: ModifyInput{AddAssignees: []string{"local"}, AddTags: []string{"ops"}, UDAs: map[string]string{"effort": "1h"}}},
	}
	for _, change := range changes {
		if err := svc.Modify(change.ref, change.input); err != nil {
			t.Fatalf("Modify(%q): %v", change.ref, err)
		}
	}

	got, err := svc.ResolveTaskReferenceForRead(slug)
	if err != nil {
		t.Fatalf("ResolveTaskReferenceForRead: %v", err)
	}
	wantOverrides := []string{"assignees", "description", "due", "priority", "tags", "title", "udas"}
	if got.Task == nil || !reflect.DeepEqual(got.Task.RecurrenceOverrides, wantOverrides) {
		t.Fatalf("overrides = %#v，期望 %#v", got.Task, wantOverrides)
	}

	// no-op 修改不能制造新的 override。
	if err := svc.Modify(slug, ModifyInput{Title: strptr("每日预算巡检")}); err != nil {
		t.Fatalf("Modify(no-op): %v", err)
	}
	got, err = svc.ResolveTaskReferenceForRead(slug)
	if err != nil {
		t.Fatalf("ResolveTaskReferenceForRead after no-op: %v", err)
	}
	if !reflect.DeepEqual(got.Task.RecurrenceOverrides, wantOverrides) {
		t.Fatalf("no-op overrides = %#v，期望 %#v", got.Task.RecurrenceOverrides, wantOverrides)
	}
}

func TestDeleteMaterializedOccurrenceBySlugRecordsSkippedAudit(t *testing.T) {
	svc, series, _, slot, _ := newOccurrenceMergeFixture(t)
	ref := OccurrenceRef(series.ID, slot)
	materialized, _, err := svc.MaterializeOccurrenceForWrite(ref)
	if err != nil {
		t.Fatalf("MaterializeOccurrenceForWrite: %v", err)
	}
	slug := taskSlugOf(materialized)
	if err := svc.Delete(slug); err != nil {
		t.Fatalf("Delete(%q): %v", slug, err)
	}

	action := "task.recurrence.skipped"
	targetType := "task"
	targetID := materialized.UUID
	rows, err := svc.auditRepo.List(storage.AuditListOptions{
		WorkspaceID: &svc.workspaceID,
		TargetType:  &targetType,
		TargetID:    &targetID,
		Action:      &action,
	})
	if err != nil {
		t.Fatalf("List audit: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("skipped audit rows = %d，期望 1", len(rows))
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(rows[0].PayloadJSON), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["series_id"] != series.ID || int64(payload["recurrence_at"].(float64)) != slot {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestProjectedOccurrenceReadSubresourcesStayEmptyWithoutMaterializing(t *testing.T) {
	svc, series, _, slot, _ := newOccurrenceMergeFixture(t)
	ref := OccurrenceRef(series.ID, slot)
	before := occurrenceRowCount(t, svc, svc.workspaceID)

	children, err := svc.ListChildren(ref, true)
	if err != nil || len(children) != 0 {
		t.Fatalf("ListChildren(projected) = %#v, %v", children, err)
	}
	annotations, total, err := svc.ListAnnotations(ref, 0, 20)
	if err != nil || total != 0 || len(annotations) != 0 {
		t.Fatalf("ListAnnotations(projected) = %#v, %d, %v", annotations, total, err)
	}
	explain, err := svc.ExplainUrgency(ref)
	if err != nil {
		t.Fatalf("ExplainUrgency(projected): %v", err)
	}
	if explain.ID != ref || explain.UUID != nil || explain.Total <= 0 {
		t.Fatalf("ExplainUrgency(projected) = %#v", explain)
	}
	if after := occurrenceRowCount(t, svc, svc.workspaceID); after != before {
		t.Fatalf("projected reads wrote rows: before=%d after=%d", before, after)
	}
}
