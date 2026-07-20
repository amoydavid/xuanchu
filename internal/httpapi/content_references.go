package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"git.dajee.net/dajee/xuanchu/internal/app"
)

// handleContentReferenceSuggestions 暴露 GET /api/v1/content-references/suggestions。
//
// type=user 返回当前 workspace active member；
// type=task 返回 request scope 内可读的实际任务，当前项目优先。
func (s *Server) handleContentReferenceSuggestions(w http.ResponseWriter, r *http.Request) {
	refType := strings.TrimSpace(r.URL.Query().Get("type"))
	if refType != "user" && refType != "task" {
		writeError(w, http.StatusBadRequest, "content_reference_query_invalid", "type must be user or task", nil)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeError(w, http.StatusBadRequest, "content_reference_query_invalid", "q is required", nil)
		return
	}
	limit := 20
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	scoped, _, err := s.scopedService(r, scopeForRefType(refType), permissionForRefType(refType), "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	results, err := scoped.SuggestContentReferences(r.Context(), app.ContentReferenceSuggestionInput{
		Type:       refType,
		Query:      query,
		ProjectRef: strings.TrimSpace(r.URL.Query().Get("project")),
		Limit:      limit,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]any{"results": results, "count": len(results)}, nil)
}

// handleContentReferenceResolve 暴露 POST /api/v1/content-references/resolve。
//
// 最多 200 个引用，逐项独立判断；不可读/不存在/越权统一返回 unavailable。
func (s *Server) handleContentReferenceResolve(w http.ResponseWriter, r *http.Request) {
	var req struct {
		References []struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		} `json:"references"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	if len(req.References) > 200 {
		writeError(w, http.StatusBadRequest, "description_reference_limit_exceeded", "too many references; max 200", nil)
		return
	}
	// resolve 不要求单一 capability：混合 user/task/attachment 引用由 App 层逐项
	// 判断 workspace:read 与 task:read，缺少一种时只返回对应项 unavailable。
	scoped, _, err := s.scopedService(r, "", "", "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	keys := make([]app.ContentReferenceKeyInput, 0, len(req.References))
	for _, ref := range req.References {
		keys = append(keys, app.ContentReferenceKeyInput{Type: ref.Type, ID: ref.ID})
	}
	results, err := scoped.ResolveContentReferences(r.Context(), keys)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]any{"results": results}, nil)
}

func scopeForRefType(refType string) string {
	if refType == "user" {
		// member 列表读取复用 workspace 读取 capability（无独立 member:read capability）。
		return "workspace:read"
	}
	return "task:read"
}

func permissionForRefType(refType string) app.Permission {
	if refType == "user" {
		// member 列表读取复用 workspace 读取权限（无独立 member:read permission）。
		return app.PermissionWorkspaceRead
	}
	return app.PermissionTaskRead
}

// 占位：确保 chi 已被使用（path params 在后续迭代会用到）。
var _ = chi.URLParam
