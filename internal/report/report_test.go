package report

import "testing"

func TestDefaultRegistryHasM1Reports(t *testing.T) {
	reg := DefaultRegistry()
	for _, name := range []string{"list", "next", "all", "completed", "deleted", "overdue"} {
		if _, ok := reg.Get(name); !ok {
			t.Fatalf("missing report %s", name)
		}
	}
	for _, name := range []string{"waiting", "active", "ready", "blocked", "blocking"} {
		if _, ok := reg.Get(name); ok {
			t.Fatalf("M1 should not register report %s", name)
		}
	}
}
