package remote

import "testing"

func TestProjectDTOToViewKeepsURL(t *testing.T) {
	view := projectDTOToView(projectDTO{ID: "p1", Slug: "ops", URL: "https://xuanchu.example.com/workspaces/local/projects/ops"})
	if view.URL != "https://xuanchu.example.com/workspaces/local/projects/ops" {
		t.Fatalf("URL = %q", view.URL)
	}
}
