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

func projectPath(workspaceSlug, projectSlug string) string {
	return "/workspaces/" + webPathSegment(workspaceSlug) + "/projects/" + webPathSegment(projectSlug)
}

func resourceURL(resourceBaseURL, path string) string {
	base := strings.TrimRight(strings.TrimSpace(resourceBaseURL), "/")
	if base == "" {
		return ""
	}
	return base + path
}

// ProjectURL 返回项目的 Web Console 绝对 URL；未配置 base 时返回空字符串。
func ProjectURL(resourceBaseURL, workspaceSlug, projectSlug string) string {
	return resourceURL(resourceBaseURL, projectPath(workspaceSlug, projectSlug))
}

// ProjectTaskURL 返回项目内任务或 occurrence 的 Web Console 绝对 URL。
func ProjectTaskURL(resourceBaseURL, workspaceSlug, projectSlug, taskRef string) string {
	return resourceURL(resourceBaseURL, projectPath(workspaceSlug, projectSlug)+"/tasks/"+webPathSegment(taskRef))
}

// StandaloneTaskURL 返回无项目任务的 Web Console 绝对 URL。
func StandaloneTaskURL(resourceBaseURL, taskRef string) string {
	return resourceURL(resourceBaseURL, "/tasks/"+webPathSegment(taskRef))
}

// TaskSeriesURL 返回循环系列的 Web Console 绝对 URL。
func TaskSeriesURL(resourceBaseURL, workspaceSlug, projectSlug, seriesSlug string) string {
	return resourceURL(resourceBaseURL, projectPath(workspaceSlug, projectSlug)+"/series/"+webPathSegment(seriesSlug))
}
