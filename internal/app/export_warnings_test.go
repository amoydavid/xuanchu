package app

import (
	"testing"
)

func TestExportWithWarningsCollectsAttachmentIDs(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()

	// 创建两个 task，description 中引用两个不同 attachment ID。
	id1 := "40af0185-316f-42bb-b52b-545d21f6f012"
	id2 := "a801f977-c745-4f47-95a4-7893a9317aba"
	desc1 := "![](ref://attachment/" + id1 + ")"
	desc2 := "[文档](ref://attachment/" + id2 + ")"
	tsk1, err := svc.Add(AddInput{Title: "t1", Description: &desc1})
	if err != nil {
		t.Fatal(err)
	}
	_ = tsk1
	if _, err := svc.Add(AddInput{Title: "t2", Description: &desc2}); err != nil {
		t.Fatal(err)
	}

	result, err := svc.ExportWithWarnings(ExportInput{})
	if err != nil {
		t.Fatalf("ExportWithWarnings: %v", err)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
	warning := result.Warnings[0]
	if warning.Type != "binary_not_included" {
		t.Fatalf("type = %q", warning.Type)
	}
	if len(warning.AttachmentIDs) != 2 {
		t.Fatalf("ids = %#v", warning.AttachmentIDs)
	}
	// 应按 id 排序、去重。
	if warning.AttachmentIDs[0] != id1 || warning.AttachmentIDs[1] != id2 {
		t.Fatalf("ids order = %#v", warning.AttachmentIDs)
	}
}

func TestExportWithWarningsOmitsWhenNoAttachments(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()

	if _, err := svc.Add(AddInput{Title: "no-attachment"}); err != nil {
		t.Fatal(err)
	}
	result, err := svc.ExportWithWarnings(ExportInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings should be empty, got %#v", result.Warnings)
	}
}

func TestExportWithWarningsDeduplicatesAttachmentIDs(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()

	id := "40af0185-316f-42bb-b52b-545d21f6f012"
	desc := "![](ref://attachment/" + id + ")"
	if _, err := svc.Add(AddInput{Title: "t1", Description: &desc}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(AddInput{Title: "t2", Description: &desc}); err != nil {
		t.Fatal(err)
	}
	result, err := svc.ExportWithWarnings(ExportInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 1 || len(result.Warnings[0].AttachmentIDs) != 1 {
		t.Fatalf("expected 1 unique id, got %#v", result.Warnings)
	}
}
