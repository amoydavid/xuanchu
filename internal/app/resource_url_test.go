package app

import "testing"

func TestResourceURLs(t *testing.T) {
	base := "https://xuanchu.example.com/admin"
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"project", ProjectURL(base, "dajee", "agentapi"), base + "/workspaces/dajee/projects/agentapi"},
		{"project task", ProjectTaskURL(base, "dajee", "agentapi", "agentapi-17"), base + "/workspaces/dajee/projects/agentapi/tasks/agentapi-17"},
		{"projected occurrence", ProjectTaskURL(base, "dajee", "agentapi", "occ:series-1:1784303999"), base + "/workspaces/dajee/projects/agentapi/tasks/occ%3Aseries-1%3A1784303999"},
		{"standalone task", StandaloneTaskURL(base, "7b4d901e-5e4f-4cfa-b42c-38fc7089880a"), base + "/tasks/7b4d901e-5e4f-4cfa-b42c-38fc7089880a"},
		{"series", TaskSeriesURL(base, "dajee", "agentapi", "agentapi-s-3"), base + "/workspaces/dajee/projects/agentapi/series/agentapi-s-3"},
		{"encoded segments", ProjectTaskURL(base, "workspace/name", "project?name", "task#1"), base + "/workspaces/workspace%2Fname/projects/project%3Fname/tasks/task%231"},
		{"empty project", ProjectURL("", "dajee", "agentapi"), ""},
		{"empty project task", ProjectTaskURL("", "dajee", "agentapi", "agentapi-17"), ""},
		{"empty standalone task", StandaloneTaskURL("", "task-1"), ""},
		{"empty series", TaskSeriesURL("", "dajee", "agentapi", "agentapi-s-3"), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("URL = %q, want %q", tt.got, tt.want)
			}
		})
	}
}
