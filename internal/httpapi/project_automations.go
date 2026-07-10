package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/auth"
)

// projectAutomationRuleRequest 是 HTTP 请求体 DTO，对齐 app.ProjectAutomationRuleAddInput。
type projectAutomationRuleRequest struct {
	Name                string                            `json:"name"`
	Description         string                            `json:"description"`
	Enabled             bool                              `json:"enabled"`
	TriggerType         string                            `json:"trigger_type"`
	TriggerConfig       app.ProjectAutomationTriggerConfig `json:"trigger_config"`
	Condition           app.ProjectAutomationCondition     `json:"condition"`
	Action              app.ProjectAutomationActionConfig  `json:"action"`
	Context             app.ProjectAutomationContextConfig `json:"context"`
	InstructionTemplate string                            `json:"instruction_template"`
	SystemPrompt        string                            `json:"system_prompt"`
}

func projectAutomationAddInput(req projectAutomationRuleRequest) app.ProjectAutomationRuleAddInput {
	return app.ProjectAutomationRuleAddInput{
		Name:                req.Name,
		Description:         req.Description,
		Enabled:             req.Enabled,
		TriggerType:         req.TriggerType,
		TriggerConfig:       req.TriggerConfig,
		Condition:           req.Condition,
		Action:              req.Action,
		Context:             req.Context,
		InstructionTemplate: req.InstructionTemplate,
		SystemPrompt:        req.SystemPrompt,
	}
}

// scopedProjectAutomationService 校验 project + hook 双重 scope，write=true 时使用写权限。
func (s *Server) scopedProjectAutomationService(r *http.Request, projectRef string, write bool) (*app.Service, error) {
	projectScope := auth.ScopeProjectRead
	projectPermission := app.PermissionProjectRead
	hookScope := auth.ScopeHookRead
	hookPermission := app.PermissionHookRead
	if write {
		projectScope = auth.ScopeProjectWrite
		projectPermission = app.PermissionProjectManage
		hookScope = auth.ScopeHookWrite
		hookPermission = app.PermissionHookWrite
	}
	scoped, _, err := s.scopedService(r, projectScope, projectPermission, projectRef)
	if err != nil {
		return nil, err
	}
	if _, _, err := s.scopedService(r, hookScope, hookPermission, projectRef); err != nil {
		return nil, err
	}
	return scoped, nil
}

func (s *Server) handleProjectAutomationList(w http.ResponseWriter, r *http.Request) {
	projectRef := chi.URLParam(r, "projectRef")
	scoped, err := s.scopedProjectAutomationService(r, projectRef, false)
	if err != nil {
		writeAppError(w, err)
		return
	}
	rows, err := scoped.ListProjectAutomationRules(projectRef, r.URL.Query().Get("all") == "true")
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, rows, nil)
}

func (s *Server) handleProjectAutomationCreate(w http.ResponseWriter, r *http.Request) {
	var req projectAutomationRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	projectRef := chi.URLParam(r, "projectRef")
	scoped, err := s.scopedProjectAutomationService(r, projectRef, true)
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.AddProjectAutomationRule(projectRef, projectAutomationAddInput(req))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, view, nil)
}

func (s *Server) handleProjectAutomationInfo(w http.ResponseWriter, r *http.Request) {
	projectRef := chi.URLParam(r, "projectRef")
	scoped, err := s.scopedProjectAutomationService(r, projectRef, false)
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.ProjectAutomationRuleInfo(projectRef, chi.URLParam(r, "ruleID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

func (s *Server) handleProjectAutomationModify(w http.ResponseWriter, r *http.Request) {
	var req projectAutomationRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	projectRef := chi.URLParam(r, "projectRef")
	scoped, err := s.scopedProjectAutomationService(r, projectRef, true)
	if err != nil {
		writeAppError(w, err)
		return
	}
	input := app.ProjectAutomationRuleModifyInput{
		Name:                &req.Name,
		Description:         &req.Description,
		Enabled:             &req.Enabled,
		TriggerType:         &req.TriggerType,
		TriggerConfig:       &req.TriggerConfig,
		Condition:           &req.Condition,
		Action:              &req.Action,
		Context:             &req.Context,
		InstructionTemplate: &req.InstructionTemplate,
		SystemPrompt:        &req.SystemPrompt,
	}
	view, err := scoped.ModifyProjectAutomationRule(projectRef, chi.URLParam(r, "ruleID"), input)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

func (s *Server) handleProjectAutomationDelete(w http.ResponseWriter, r *http.Request) {
	projectRef := chi.URLParam(r, "projectRef")
	scoped, err := s.scopedProjectAutomationService(r, projectRef, true)
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := scoped.DeleteProjectAutomationRule(projectRef, chi.URLParam(r, "ruleID")); err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]bool{"deleted": true}, nil)
}

func (s *Server) handleProjectAutomationEnable(w http.ResponseWriter, r *http.Request) {
	s.enableDisableProjectAutomation(w, r, true)
}

func (s *Server) handleProjectAutomationDisable(w http.ResponseWriter, r *http.Request) {
	s.enableDisableProjectAutomation(w, r, false)
}

func (s *Server) enableDisableProjectAutomation(w http.ResponseWriter, r *http.Request, enable bool) {
	projectRef := chi.URLParam(r, "projectRef")
	scoped, err := s.scopedProjectAutomationService(r, projectRef, true)
	if err != nil {
		writeAppError(w, err)
		return
	}
	var view app.ProjectAutomationRuleView
	if enable {
		view, err = scoped.EnableProjectAutomationRule(projectRef, chi.URLParam(r, "ruleID"))
	} else {
		view, err = scoped.DisableProjectAutomationRule(projectRef, chi.URLParam(r, "ruleID"))
	}
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

func (s *Server) handleProjectAutomationPreview(w http.ResponseWriter, r *http.Request) {
	var req projectAutomationRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	projectRef := chi.URLParam(r, "projectRef")
	scoped, err := s.scopedProjectAutomationService(r, projectRef, true)
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.PreviewProjectAutomation(projectRef, app.ProjectAutomationPreviewInput(projectAutomationAddInput(req)))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

func (s *Server) handleProjectAutomationSavedPreview(w http.ResponseWriter, r *http.Request) {
	projectRef := chi.URLParam(r, "projectRef")
	scoped, err := s.scopedProjectAutomationService(r, projectRef, true)
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.PreviewSavedProjectAutomation(projectRef, chi.URLParam(r, "ruleID"), nil)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

func (s *Server) handleProjectAutomationTest(w http.ResponseWriter, r *http.Request) {
	projectRef := chi.URLParam(r, "projectRef")
	scoped, err := s.scopedProjectAutomationService(r, projectRef, true)
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.TestProjectAutomationRule(projectRef, chi.URLParam(r, "ruleID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, view, nil)
}

func (s *Server) handleProjectAutomationDeliveryList(w http.ResponseWriter, r *http.Request) {
	projectRef := chi.URLParam(r, "projectRef")
	scoped, err := s.scopedProjectAutomationService(r, projectRef, false)
	if err != nil {
		writeAppError(w, err)
		return
	}
	input := app.ProjectAutomationDeliveryListInput{
		RuleID: r.URL.Query().Get("rule"),
		Status: r.URL.Query().Get("status"),
		Limit:  queryAutomationLimit(r),
	}
	rows, err := scoped.ListProjectAutomationDeliveries(projectRef, input)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, rows, nil)
}

func (s *Server) handleProjectAutomationDeliveryInfo(w http.ResponseWriter, r *http.Request) {
	projectRef := chi.URLParam(r, "projectRef")
	scoped, err := s.scopedProjectAutomationService(r, projectRef, false)
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.ProjectAutomationDeliveryInfo(projectRef, chi.URLParam(r, "deliveryID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

func (s *Server) handleProjectAutomationDeliveryReplay(w http.ResponseWriter, r *http.Request) {
	projectRef := chi.URLParam(r, "projectRef")
	scoped, err := s.scopedProjectAutomationService(r, projectRef, true)
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.ReplayProjectAutomationDelivery(projectRef, chi.URLParam(r, "deliveryID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

// queryAutomationLimit 解析 list 接口的 limit 参数，默认 50，上限 200。
func queryAutomationLimit(r *http.Request) int {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		return 50
	}
	return limit
}
