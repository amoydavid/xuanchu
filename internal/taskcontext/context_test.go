package taskcontext

import "testing"

func TestValidateName(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		for _, name := range []string{
			"work",
			"home.v2",
			"dev_ops-1",
			"  trimmed.name  ",
		} {
			if !ValidateName(name) {
				t.Fatalf("ValidateName(%q) = false, want true", name)
			}
		}
	})

	t.Run("invalid", func(t *testing.T) {
		for _, name := range []string{
			"",
			"   ",
			"with space",
			"slash/name",
			"中文",
			"name!",
		} {
			if ValidateName(name) {
				t.Fatalf("ValidateName(%q) = true, want false", name)
			}
		}
	})
}
