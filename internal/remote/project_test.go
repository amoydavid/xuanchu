package remote

import "testing"

func TestProjectDTOToViewKeepsURL(t *testing.T) {
	view := projectDTOToView(projectDTO{ID: "p1", Slug: "ops", URL: "/workspaces/local/projects/ops"})
	if view.URL != "/workspaces/local/projects/ops" {
		t.Fatalf("URL = %q", view.URL)
	}
}
