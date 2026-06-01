package mcpserver

import (
	"fmt"
	"strings"
	"testing"

	"github.com/dajee/taskg/internal/app"
)

func TestSuccessResultReturnsEnvelope(t *testing.T) {
	result, envelope, err := successResult(map[string]any{"ok": true}, "created task")
	if err != nil {
		t.Fatalf("successResult() error = %v", err)
	}
	if result == nil {
		t.Fatal("result = nil")
	}
	if got := renderedText(result); got != "created task" {
		t.Fatalf("rendered text = %q, want %q", got, "created task")
	}
	if envelope.Rendered != "created task" {
		t.Fatalf("rendered = %q, want %q", envelope.Rendered, "created task")
	}
	if data, ok := envelope.Data.(map[string]any); !ok || data["ok"] != true {
		t.Fatalf("data = %#v, want ok=true", envelope.Data)
	}
}

func TestToolErrorPreservesRuntimeCode(t *testing.T) {
	result := businessErrorResult(app.RuntimeError{Code: "task_not_found", Message: "task not found"})
	if result == nil {
		t.Fatal("result = nil")
	}
	if !result.IsError {
		t.Fatal("IsError = false, want true")
	}
	text := renderedText(result)
	if !strings.Contains(text, "task_not_found") || !strings.Contains(text, "task not found") {
		t.Fatalf("rendered text = %q, want code and message", text)
	}
	payload, ok := result.StructuredContent.(ToolError)
	if !ok {
		t.Fatalf("structured content = %T, want ToolError", result.StructuredContent)
	}
	if payload.Code != "task_not_found" || payload.Message != "task not found" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestToolErrorMapsInternalErrorWithoutDetails(t *testing.T) {
	result := businessErrorResult(fmt.Errorf("boom"))
	if result == nil {
		t.Fatal("result = nil")
	}
	if !result.IsError {
		t.Fatal("IsError = false, want true")
	}
	text := renderedText(result)
	if !strings.Contains(text, "mcp_internal") || strings.Contains(text, "boom") {
		t.Fatalf("rendered text = %q, want generic mcp_internal without original details", text)
	}
	payload, ok := result.StructuredContent.(ToolError)
	if !ok {
		t.Fatalf("structured content = %T, want ToolError", result.StructuredContent)
	}
	if payload.Code != "mcp_internal" || strings.Contains(payload.Message, "boom") {
		t.Fatalf("payload = %#v", payload)
	}
}
