package app

import "testing"

func TestResourceURLs(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"project", ProjectURL("dajee", "agentapi"), "/workspaces/dajee/projects/agentapi"},
		{"project task", ProjectTaskURL("dajee", "agentapi", "agentapi-17"), "/workspaces/dajee/projects/agentapi/tasks/agentapi-17"},
		{"projected occurrence", ProjectTaskURL("dajee", "agentapi", "occ:series-1:1784303999"), "/workspaces/dajee/projects/agentapi/tasks/occ%3Aseries-1%3A1784303999"},
		{"standalone task", StandaloneTaskURL("7b4d901e-5e4f-4cfa-b42c-38fc7089880a"), "/tasks/7b4d901e-5e4f-4cfa-b42c-38fc7089880a"},
		{"series", TaskSeriesURL("dajee", "agentapi", "agentapi-s-3"), "/workspaces/dajee/projects/agentapi/series/agentapi-s-3"},
		{"encoded segments", ProjectTaskURL("workspace/name", "project?name", "task#1"), "/workspaces/workspace%2Fname/projects/project%3Fname/tasks/task%231"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("URL = %q, want %q", tt.got, tt.want)
			}
		})
	}
}
