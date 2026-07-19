package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"git.dajee.net/dajee/xuanchu/internal/app"
)

// handleContentReferenceSuggestions 暴露 GET /api/v1/content-references/suggestions。
//
// 当前实现返回最小可用响应（空数据），完整查询逻辑由 app 层后续迭代补齐。
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
	scoped, _, err := s.scopedService(r, scopeForRefType(refType), permissionForRefType(refType), "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	results, err := scoped.SuggestContentReferences(r.Context(), refType, query)
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
	results := make([]map[string]any, 0, len(req.References))
	for _, ref := range req.References {
		// 当前实现统一返回 unavailable；完整解析逻辑由 app 层后续迭代补齐。
		results = append(results, map[string]any{
			"type":   ref.Type,
			"id":     ref.ID,
			"status": "unavailable",
		})
	}
	writeSuccess(w, http.StatusOK, map[string]any{"results": results}, nil)
}

func scopeForRefType(refType string) string {
	if refType == "user" {
		return "member:read"
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
