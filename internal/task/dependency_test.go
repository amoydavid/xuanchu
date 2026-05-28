package task

import "testing"

func TestWouldCreateDependencyCycle(t *testing.T) {
	graph := map[string][]string{
		"a": {"b"},
		"b": {"c"},
	}
	if !WouldCreateDependencyCycle(graph, "c", "a") {
		t.Fatal("expected c -> a to create cycle")
	}
	if WouldCreateDependencyCycle(graph, "c", "d") {
		t.Fatal("c -> d should not create cycle")
	}
	if !WouldCreateDependencyCycle(graph, "a", "a") {
		t.Fatal("self-dependency should be a cycle")
	}
}
