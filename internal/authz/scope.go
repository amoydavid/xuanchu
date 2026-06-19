package authz

import (
	"slices"
	"strings"
)

// HasCapability 判断当前请求范围是否具备某个 capability。
// 空 capability 表示无 token scope 要求，直接放行。
func (s RequestScope) HasCapability(capability string) bool {
	if strings.TrimSpace(capability) == "" {
		return true
	}
	return slices.Contains(s.Capabilities, capability)
}

// RestrictsWorkspaces 表示是否按 workspace allowlist 收窄。
func (s RequestScope) RestrictsWorkspaces() bool {
	return len(s.WorkspaceIDs) > 0
}

// RestrictsProjects 表示是否按 project allowlist 收窄。
func (s RequestScope) RestrictsProjects() bool {
	return len(s.ProjectIDs) > 0
}

// AllowsWorkspace 判断当前请求范围是否允许访问某个 workspace。
// 空 allowlist 表示不按该维度收窄。
func (s RequestScope) AllowsWorkspace(id string) bool {
	if !s.RestrictsWorkspaces() {
		return true
	}
	return slices.Contains(s.WorkspaceIDs, id)
}

// AllowsProject 判断当前请求范围是否允许访问某个 project。
// 空 allowlist 表示不按该维度收窄。
func (s RequestScope) AllowsProject(id string) bool {
	if !s.RestrictsProjects() {
		return true
	}
	return slices.Contains(s.ProjectIDs, id)
}
