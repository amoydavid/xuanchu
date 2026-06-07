package cli

import "testing"

func TestIsPlainTargetArgRecognizesTaskRefs(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{name: "numeric", args: []string{"1"}, want: true},
		{name: "huge numeric", args: []string{"999999999999999999999999999999"}, want: true},
		{name: "task slug", args: []string{"api-1"}, want: true},
		{name: "uuid", args: []string{"00000000-0000-0000-0000-000000000001"}, want: true},
		{name: "filter", args: []string{"project:api"}, want: false},
		{name: "multiple args", args: []string{"api-1", "+next"}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isPlainTargetArg(tc.args); got != tc.want {
				t.Fatalf("isPlainTargetArg(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}
