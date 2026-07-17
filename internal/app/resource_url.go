package app

import (
	"net/url"
	"strings"
)

// webPathSegment 按单个 URL path segment 编码资源引用。
// QueryEscape 会把空格写成加号；Web Console 路由需要标准的 %20。
func webPathSegment(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}

// ProjectURL 返回项目的 Web Console 相对路径。
func ProjectURL(workspaceSlug, projectSlug string) string {
	return "/workspaces/" + webPathSegment(workspaceSlug) + "/projects/" + webPathSegment(projectSlug)
}

// ProjectTaskURL 返回项目内任务或 occurrence 的 Web Console 相对路径。
func ProjectTaskURL(workspaceSlug, projectSlug, taskRef string) string {
	return ProjectURL(workspaceSlug, projectSlug) + "/tasks/" + webPathSegment(taskRef)
}

// StandaloneTaskURL 返回无项目任务的 Web Console 相对路径。
func StandaloneTaskURL(taskRef string) string {
	return "/tasks/" + webPathSegment(taskRef)
}

// TaskSeriesURL 返回循环系列的 Web Console 相对路径。
func TaskSeriesURL(workspaceSlug, projectSlug, seriesSlug string) string {
	return ProjectURL(workspaceSlug, projectSlug) + "/series/" + webPathSegment(seriesSlug)
}
