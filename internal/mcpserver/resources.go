package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/dajee/taskg/internal/app"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func RegisterResources(s *mcp.Server, opts Options) {
	// workspace/current — 静态 resource
	s.AddResource(&mcp.Resource{
		Name:        "workspace-current",
		Title:       "当前工作区",
		Description: "返回当前生效的 workspace 信息，包括 ID、slug、名称、角色和可见 project 摘要。",
		URI:         "taskg://workspace/current",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		svc, err := resourceService(ctx, opts, req, RequestScopeInput{}, "workspace:read", app.PermissionWorkspaceRead)
		if err != nil {
			return nil, err
		}
		data, err := buildWorkspaceCurrentData(svc)
		if err != nil {
			return nil, err
		}
		return jsonResource("taskg://workspace/current", data)
	})

	// workspace/{workspace_id} — template resource
	s.AddResourceTemplate(&mcp.ResourceTemplate{
		Name:        "workspace-by-id",
		Title:       "指定工作区",
		Description: "按 ID 或 slug 返回指定 workspace 信息。",
		URITemplate: "taskg://workspace/{workspace_id}",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		workspaceID := uriParam(req.Params.URI)
		if workspaceID == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		svc, err := resourceService(ctx, opts, req, RequestScopeInput{Workspace: workspaceID}, "workspace:read", app.PermissionWorkspaceRead)
		if err != nil {
			return nil, err
		}
		info, err := svc.WorkspaceInfo(workspaceID)
		if err != nil {
			return nil, err
		}
		projects, err := svc.ListProjects(false)
		if err != nil {
			return nil, err
		}
		data := buildWorkspaceData(info, projects)
		return jsonResource(req.Params.URI, data)
	})

	// project/{project_id} — template resource
	s.AddResourceTemplate(&mcp.ResourceTemplate{
		Name:        "project-by-id",
		Title:       "指定项目",
		Description: "按 ID 或 slug 返回指定 project 详情，仅暴露 agent.* 配置项。",
		URITemplate: "taskg://project/{project_id}",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		projectRef := uriParam(req.Params.URI)
		if projectRef == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		if _, err := uuid.Parse(projectRef); err != nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		svc, err := resourceService(ctx, opts, req, RequestScopeInput{ProjectID: projectRef}, "project:read", app.PermissionProjectRead)
		if err != nil {
			return nil, err
		}
		info, err := svc.ProjectInfo(projectRef)
		if err != nil {
			return nil, err
		}
		agentConfig, err := filterAgentConfig(svc, projectRef)
		if err != nil {
			return nil, err
		}
		data := buildProjectData(info, agentConfig)
		return jsonResource(req.Params.URI, data)
	})

	// context/current — 静态 resource
	s.AddResource(&mcp.Resource{
		Name:        "context-current",
		Title:       "当前上下文",
		Description: "返回当前活跃 context 的名称、过滤表达式和生效范围。",
		URI:         "taskg://context/current",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		svc, err := resourceService(ctx, opts, req, RequestScopeInput{}, "context:read", app.PermissionContextUse)
		if err != nil {
			return nil, err
		}
		data, err := buildContextCurrentData(svc)
		if err != nil {
			return nil, err
		}
		return jsonResource("taskg://context/current", data)
	})
}

// ---- data 构造 ----

type workspaceResourceData struct {
	ID       string           `json:"id"`
	Slug     string           `json:"slug"`
	Name     string           `json:"name"`
	Role     string           `json:"role"`
	Context  *contextSummary  `json:"context,omitempty"`
	Projects []projectSummary `json:"projects"`
}

type contextSummary struct {
	Name   string `json:"name,omitempty"`
	Filter string `json:"filter,omitempty"`
}

type projectSummary struct {
	ID     string `json:"id"`
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type projectResourceData struct {
	ID          string            `json:"id"`
	Slug        string            `json:"slug"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Status      string            `json:"status"`
	Archived    bool              `json:"archived"`
	AgentConfig map[string]string `json:"agent_config"`
}

type contextResourceData struct {
	Name              string `json:"name,omitempty"`
	Filter            string `json:"filter,omitempty"`
	WorkspaceID       string `json:"workspace_id,omitempty"`
	WorkspaceSlug     string `json:"workspace_slug,omitempty"`
	ProjectScopeCount int    `json:"project_scope_count,omitempty"`
}

func buildWorkspaceCurrentData(svc *app.Service) (*workspaceResourceData, error) {
	rt := svc.Runtime()

	wsInfo, err := svc.WorkspaceInfo(rt.WorkspaceSlug)
	if err != nil {
		return nil, err
	}

	ctxSummary, err := contextSummaryFromService(svc)
	if err != nil {
		return nil, err
	}

	projects, err := svc.ListProjects(false)
	if err != nil {
		return nil, err
	}

	summaries := make([]projectSummary, 0, len(projects))
	for _, p := range projects {
		summaries = append(summaries, projectSummary{
			ID:     p.ID,
			Slug:   p.Slug,
			Name:   p.Name,
			Status: p.Status,
		})
	}

	return &workspaceResourceData{
		ID:       wsInfo.ID,
		Slug:     wsInfo.Slug,
		Name:     wsInfo.Name,
		Role:     string(wsInfo.Role),
		Context:  ctxSummary,
		Projects: summaries,
	}, nil
}

func buildWorkspaceData(info app.WorkspaceView, projects []app.ProjectView) *workspaceResourceData {
	summaries := make([]projectSummary, 0, len(projects))
	for _, p := range projects {
		summaries = append(summaries, projectSummary{
			ID:     p.ID,
			Slug:   p.Slug,
			Name:   p.Name,
			Status: p.Status,
		})
	}
	return &workspaceResourceData{
		ID:       info.ID,
		Slug:     info.Slug,
		Name:     info.Name,
		Role:     string(info.Role),
		Projects: summaries,
	}
}

func buildProjectData(info app.ProjectView, agentConfig map[string]string) *projectResourceData {
	archived := info.ArchivedAt != nil
	return &projectResourceData{
		ID:          info.ID,
		Slug:        info.Slug,
		Name:        info.Name,
		Description: info.Description,
		Status:      info.Status,
		Archived:    archived,
		AgentConfig: agentConfig,
	}
}

func buildContextCurrentData(svc *app.Service) (*contextResourceData, error) {
	name, ok, err := svc.ActiveContextName()
	if err != nil {
		return nil, err
	}
	var filter string
	if ok {
		show, err := svc.ContextShow()
		if err != nil {
			return nil, err
		}
		parts := strings.SplitN(show, " ", 2)
		if len(parts) == 2 {
			filter = parts[1]
		}
	}

	rt := svc.Runtime()

	projects, projErr := svc.ListProjects(false)
	if projErr != nil {
		return nil, projErr
	}

	return &contextResourceData{
		Name:              name,
		Filter:            filter,
		WorkspaceID:       rt.WorkspaceID,
		WorkspaceSlug:     rt.WorkspaceSlug,
		ProjectScopeCount: len(projects),
	}, nil
}

// ---- 辅助函数 ----

// scopedService 根据 transport mode 构造 scoped app.Service。
func resourceService(ctx context.Context, opts Options, req *mcp.ReadResourceRequest, input RequestScopeInput, capability string, permission app.Permission) (*app.Service, error) {
	factory := RuntimeFactory{Store: opts.Store, Clock: opts.Clock}
	if opts.Mode == ModeHTTP {
		return factory.ServiceForHTTP(requestForResource(ctx, req, opts), input, capability, permission)
	}
	return factory.ServiceForStdio(ctx, input, capability, permission)
}

func requestForResource(ctx context.Context, req *mcp.ReadResourceRequest, opts Options) *http.Request {
	if opts.Request != nil {
		return opts.Request.WithContext(ctx)
	}
	r, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/mcp", nil)
	if req != nil && req.Extra != nil && req.Extra.Header != nil {
		r.Header = req.Extra.Header.Clone()
	}
	return r
}

// contextSummaryFromService 从 service 获取 active context 摘要。
func contextSummaryFromService(svc *app.Service) (*contextSummary, error) {
	name, ok, err := svc.ActiveContextName()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	show, err := svc.ContextShow()
	if err != nil {
		return nil, err
	}
	// ContextShow 格式: "<name> <filter>"
	parts := strings.SplitN(show, " ", 2)
	filter := ""
	if len(parts) == 2 {
		filter = parts[1]
	}
	return &contextSummary{Name: name, Filter: filter}, nil
}

// filterAgentConfig 只保留 spec 允许暴露给 Agent 的 project 配置项。
func filterAgentConfig(svc *app.Service, projectRef string) (map[string]string, error) {
	all, err := svc.ProjectConfigList(projectRef)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string)
	for k, v := range all {
		if isAllowedAgentKey(k) {
			out[k] = v
		}
	}
	return out, nil
}

// jsonResource 将任意数据序列化为 JSON text resource content。
func jsonResource(uri string, data any) (*mcp.ReadResourceResult, error) {
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal resource: %w", err)
	}
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      uri,
				MIMEType: "application/json",
				Text:     string(raw),
			},
		},
	}, nil
}

// uriParam 从 taskg://segment/value 形式的 URI path 中提取 value。
func uriParam(uri string) string {
	// URI 格式: taskg://workspace/{workspace_id} 或 taskg://project/{project_id}
	// SDK 在 template 匹配时会把 {param} 替换为实际值
	// 所以实际收到的 URI 是 taskg://workspace/xxx 或 taskg://project/xxx
	parts := strings.Split(strings.TrimPrefix(uri, "taskg://"), "/")
	// /workspace/xxx -> ["workspace", "xxx"]
	// /project/xxx   -> ["project", "xxx"]
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

// allResourceURIs 返回所有注册的 resource URI，用于测试验证。
func allResourceURIs() []string {
	return []string{
		"taskg://workspace/current",
		"taskg://context/current",
	}
}

// allResourceTemplateURIs 返回所有注册的 resource template URI，用于测试验证。
func allResourceTemplateURIs() []string {
	return []string{
		"taskg://workspace/{workspace_id}",
		"taskg://project/{project_id}",
	}
}

// allowedAgentKeys 是 project resource 允许暴露的 agent.* 配置键白名单。
// 只包含当前 app 层 projectConfigKeys 中 agent.* 前缀的键。
var allowedAgentKeys = []string{
	"agent.background",
	"agent.constraints",
	"agent.default_context",
	"agent.handoff",
}

// isAllowedAgentKey 检查键是否在允许的 agent.* 白名单中。
func isAllowedAgentKey(key string) bool {
	return slices.Contains(allowedAgentKeys, key)
}
