package projecttemplate

import (
	"strings"
	"testing"
)

func TestRewriteCaptureTaskReferencesUsesLocalRefsAndPreservesCode(t *testing.T) {
	taskID := "40af0185-316f-42bb-b52b-545d21f6f012"
	attachmentID := "a801f977-c745-4f47-95a4-7893a9317aba"
	source := "[发布任务](ref://task/" + taskID + ") [官网](https://example.test) ``[内联](ref://task/" + taskID + ")``\n\n![附件](ref://attachment/" + attachmentID + ")\n\n```\n[代码](ref://task/" + taskID + ")\n```"

	got, issues, err := RewriteCaptureTaskReferences(source, map[string]string{taskID: "task-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "[发布任务](ref://task/task-1)") || !strings.Contains(got, "[官网](https://example.test)") {
		t.Fatalf("rewritten markdown = %q", got)
	}
	if !strings.Contains(got, "[代码](ref://task/"+taskID+")") {
		t.Fatalf("code block was rewritten: %q", got)
	}
	if !strings.Contains(got, "``[内联](ref://task/"+taskID+")``") {
		t.Fatalf("inline code was rewritten: %q", got)
	}
	if len(issues) != 1 || issues[0].Code != "project_template_attachment_unsupported" || issues[0].TargetRef != attachmentID {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestRewriteInstantiateTaskReferencesRequiresValidPreallocation(t *testing.T) {
	newTaskID := "f2c0f5c1-4b17-4b7f-a5bf-15c4f4bdeef4"
	source := "[发布任务](ref://task/task-1)\n\n```\n[代码](ref://task/task-1)\n```"
	got, err := RewriteInstantiateTaskReferences(source, map[string]string{"task-1": newTaskID})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "[发布任务](ref://task/"+newTaskID+")") || !strings.Contains(got, "[代码](ref://task/task-1)") {
		t.Fatalf("rewritten markdown = %q", got)
	}
	if _, err := RewriteInstantiateTaskReferences("[bad](ref://task/task-0)", map[string]string{}); ErrorCode(err) != "project_template_snapshot_invalid" {
		t.Fatalf("invalid local ref error = %v", err)
	}
}
