package httpapi

import (
	"net/http"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/auth"
)

// handleNotificationTemplateVars 返回按 trigger × field 分组的模板变量描述。
// 只读端点，用于 web console 在规则编辑侧显示可用变量。
func (s *Server) handleNotificationTemplateVars(w http.ResponseWriter, r *http.Request) {
	if _, _, err := s.scopedService(r, auth.ScopeNotificationRead, app.PermissionNotificationRead, ""); err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, app.NotificationTemplateVarsView(), nil)
}
