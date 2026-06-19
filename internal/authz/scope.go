package authz

import (
	"slices"
	"strings"
)

func (s RequestScope) HasCapability(capability string) bool {
	if strings.TrimSpace(capability) == "" {
		return true
	}
	return slices.Contains(s.Capabilities, capability)
}

func (s RequestScope) RestrictsWorkspaces() bool {
	return len(s.WorkspaceIDs) > 0
}

func (s RequestScope) RestrictsProjects() bool {
	return len(s.ProjectIDs) > 0
}

func (s RequestScope) AllowsWorkspace(id string) bool {
	if !s.RestrictsWorkspaces() {
		return true
	}
	return slices.Contains(s.WorkspaceIDs, id)
}

func (s RequestScope) AllowsProject(id string) bool {
	if !s.RestrictsProjects() {
		return true
	}
	return slices.Contains(s.ProjectIDs, id)
}
