package httpapi

import (
	"net/http"

	"git.dajee.net/dajee/xuanchu/internal/branding"
)

// handleBranding 返回平台品牌名（中/英），供前端 OEM 显示。
// 公开端点（无鉴权）：登录页首屏就要用，必须能匿名访问。
// 品牌名在二进制生命周期内不变，附加短缓存头降低首屏延迟。
func (s *Server) handleBranding(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, http.StatusOK, branding.Current())
}
