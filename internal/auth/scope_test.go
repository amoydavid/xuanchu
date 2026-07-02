package auth

import (
	"testing"
)

func TestExpandScopes_ResourceWildcard(t *testing.T) {
	set, err := ParseScopes([]string{"task:*"})
	if err != nil {
		t.Fatal(err)
	}
	if !set.Has("task:read") || !set.Has("task:write") {
		t.Fatalf("expected task:read and task:write, got %v", set.Values())
	}
	if set.Has("project:read") {
		t.Fatalf("should not contain project:read")
	}
}

func TestExpandScopes_ActionWildcard(t *testing.T) {
	set, err := ParseScopes([]string{"*:read"})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"task:read", "project:read", "context:read", "config:read", "workspace:read", "audit:read", "token:read", "hook:read", "notification:read", "reminder:read"} {
		if !set.Has(s) {
			t.Fatalf("missing %s", s)
		}
	}
	if set.Has("task:write") {
		t.Fatalf("should not contain task:write")
	}
}

func TestExpandScopes_StarWildcard(t *testing.T) {
	set, err := ParseScopes([]string{"*"})
	if err != nil {
		t.Fatal(err)
	}
	if len(set) != len(scopeRegistry) {
		t.Fatalf("expected %d scopes, got %d", len(scopeRegistry), len(set))
	}
}

func TestExpandScopes_Mixed(t *testing.T) {
	set, err := ParseScopes([]string{"task:*", "hook:read"})
	if err != nil {
		t.Fatal(err)
	}
	if len(set) != 3 {
		t.Fatalf("expected 3 scopes, got %d: %v", len(set), set.Values())
	}
}

func TestExpandScopes_InvalidResource(t *testing.T) {
	_, err := ParseScopes([]string{"foo:*"})
	if err == nil {
		t.Fatal("expected error for foo:*")
	}
}

func TestExpandScopes_InvalidAction(t *testing.T) {
	_, err := ParseScopes([]string{"*:execute"})
	if err == nil {
		t.Fatal("expected error for *:execute")
	}
}

func TestExpandScopes_PlainScopeStillWorks(t *testing.T) {
	set, err := ParseScopes([]string{"task:read"})
	if err != nil {
		t.Fatal(err)
	}
	if !set.Has("task:read") || len(set) != 1 {
		t.Fatalf("expected only task:read, got %v", set.Values())
	}
}

func TestScopeRegistryValues(t *testing.T) {
	values := ScopeRegistryValues()
	if len(values) != 24 {
		t.Fatalf("expected 24 scopes, got %d", len(values))
	}
	if values[0] != "task:read" {
		t.Fatalf("expected first scope task:read, got %s", values[0])
	}
}
