package authz

import "testing"

func TestRequestScopeAllowsCapabilityWorkspaceAndProject(t *testing.T) {
	scope := RequestScope{
		TokenID:      "tok-1",
		TokenType:    "agent",
		WorkspaceIDs: []string{"ws-1"},
		ProjectIDs:   []string{"p-1"},
		Capabilities: []string{"task:read", "project:read"},
	}
	if !scope.HasCapability("task:read") {
		t.Fatal("HasCapability(task:read) = false")
	}
	if scope.HasCapability("task:write") {
		t.Fatal("HasCapability(task:write) = true")
	}
	if !scope.AllowsWorkspace("ws-1") || scope.AllowsWorkspace("ws-2") {
		t.Fatalf("workspace allowlist mismatch")
	}
	if !scope.AllowsProject("p-1") || scope.AllowsProject("p-2") {
		t.Fatalf("project allowlist mismatch")
	}
}

func TestRequestScopeEmptyAllowlistsAreUnrestricted(t *testing.T) {
	scope := RequestScope{}
	if !scope.HasCapability("") {
		t.Fatal("empty capability should be allowed")
	}
	if !scope.AllowsWorkspace("any-ws") {
		t.Fatal("empty workspace allowlist should be unrestricted")
	}
	if !scope.AllowsProject("any-project") {
		t.Fatal("empty project allowlist should be unrestricted")
	}
}
