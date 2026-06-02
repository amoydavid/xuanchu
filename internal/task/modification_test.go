package task

import "testing"

func TestModificationEmptyTreatsAssigneeChangesAsNonEmpty(t *testing.T) {
	cases := []Modification{
		{AddAssignees: []string{"alice"}},
		{RemoveAssignees: []string{"bob"}},
		{ClearAssignees: true},
	}

	for _, tc := range cases {
		if tc.Empty() {
			t.Fatalf("Modification.Empty() = true for %#v, want false", tc)
		}
	}
}
