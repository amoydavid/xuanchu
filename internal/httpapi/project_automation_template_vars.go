package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"git.dajee.net/dajee/xuanchu/internal/app"
)

// handleProjectAutomationTemplateVars 返回按触发器分组的可用模板变量列表。
func (s *Server) handleProjectAutomationTemplateVars(w http.ResponseWriter, r *http.Request) {
	projectRef := chi.URLParam(r, "projectRef")
	scoped, err := s.scopedProjectAutomationService(r, projectRef, false)
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	_ = scoped
	view := app.AutomationTemplateVars()
	writeSuccess(w, http.StatusOK, view, nil)
}
