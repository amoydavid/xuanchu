package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/auth"
)

// workspaceAutomationRuleRequest 是 Workspace 自动化规则的 HTTP 请求体 DTO，
// 字段对齐 app.AutomationRuleInput。
type workspaceAutomationRuleRequest struct {
	Name                string                             `json:"name"`
	Description         string                             `json:"description"`
	Enabled             bool                               `json:"enabled"`
	TriggerType         string                             `json:"trigger_type"`
	TriggerConfig       app.ProjectAutomationTriggerConfig `json:"trigger_config"`
	Condition           app.ProjectAutomationCondition     `json:"condition"`
	Action              app.ProjectAutomationActionConfig  `json:"action"`
	Context             app.ProjectAutomationContextConfig `json:"context"`
	InstructionTemplate string                             `json:"instruction_template"`
	SystemPrompt        string                             `json:"system_prompt"`
}

func workspaceAutomationRuleInput(req workspaceAutomationRuleRequest) app.AutomationRuleInput {
	return app.AutomationRuleInput{
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

func workspaceAutomationModifyInput(req workspaceAutomationRuleRequest) app.AutomationRuleModifyInput {
	return app.AutomationRuleModifyInput{
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
}

// scopedWorkspaceAutomationService 校验 workspace + hook 双重 scope；
// write=true 时使用 workspace:write + hook:write。
// browser session 的 scope 是交互层人为收紧，capability 检查在 AuthorizeTokenRequest
// 中对 browser session 自动跳过（真实授权由 role 决定）。
func (s *Server) scopedWorkspaceAutomationService(r *http.Request, write bool) (*app.Service, error) {
	wsScope := auth.ScopeWorkspaceRead
	wsPermission := app.PermissionWorkspaceRead
	hookScope := auth.ScopeHookRead
	hookPermission := app.PermissionHookRead
	if write {
		wsScope = auth.ScopeWorkspaceWrite
		wsPermission = app.PermissionWorkspaceModify
		hookScope = auth.ScopeHookWrite
		hookPermission = app.PermissionHookWrite
	}
	scoped, _, err := s.scopedService(r, wsScope, wsPermission, "")
	if err != nil {
		return nil, err
	}
	if _, _, err := s.scopedService(r, hookScope, hookPermission, ""); err != nil {
		return nil, err
	}
	return scoped, nil
}

func (s *Server) handleWorkspaceAutomationList(w http.ResponseWriter, r *http.Request) {
	scoped, err := s.scopedWorkspaceAutomationService(r, false)
	if err != nil {
		writeAppError(w, err)
		return
	}
	rows, err := scoped.ListWorkspaceAutomationRules(r.URL.Query().Get("all") == "true")
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, rows, nil)
}

func (s *Server) handleWorkspaceAutomationCreate(w http.ResponseWriter, r *http.Request) {
	var req workspaceAutomationRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	scoped, err := s.scopedWorkspaceAutomationService(r, true)
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.AddWorkspaceAutomationRule(workspaceAutomationRuleInput(req))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, view, nil)
}

func (s *Server) handleWorkspaceAutomationInfo(w http.ResponseWriter, r *http.Request) {
	scoped, err := s.scopedWorkspaceAutomationService(r, false)
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.WorkspaceAutomationRuleInfo(chi.URLParam(r, "ruleID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

func (s *Server) handleWorkspaceAutomationModify(w http.ResponseWriter, r *http.Request) {
	var req workspaceAutomationRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	scoped, err := s.scopedWorkspaceAutomationService(r, true)
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.ModifyWorkspaceAutomationRule(chi.URLParam(r, "ruleID"), workspaceAutomationModifyInput(req))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

func (s *Server) handleWorkspaceAutomationDelete(w http.ResponseWriter, r *http.Request) {
	scoped, err := s.scopedWorkspaceAutomationService(r, true)
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := scoped.DeleteWorkspaceAutomationRule(chi.URLParam(r, "ruleID")); err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]bool{"deleted": true}, nil)
}

func (s *Server) handleWorkspaceAutomationEnable(w http.ResponseWriter, r *http.Request) {
	s.enableDisableWorkspaceAutomation(w, r, true)
}

func (s *Server) handleWorkspaceAutomationDisable(w http.ResponseWriter, r *http.Request) {
	s.enableDisableWorkspaceAutomation(w, r, false)
}

func (s *Server) enableDisableWorkspaceAutomation(w http.ResponseWriter, r *http.Request, enable bool) {
	scoped, err := s.scopedWorkspaceAutomationService(r, true)
	if err != nil {
		writeAppError(w, err)
		return
	}
	var view app.AutomationRuleView
	if enable {
		view, err = scoped.EnableWorkspaceAutomationRule(chi.URLParam(r, "ruleID"))
	} else {
		view, err = scoped.DisableWorkspaceAutomationRule(chi.URLParam(r, "ruleID"))
	}
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

func (s *Server) handleAutomationTemplateVars(w http.ResponseWriter, r *http.Request) {
	// template vars 只描述可用变量，不读取 sample Project/config/secret；
	// 仍要求 Automation 读权限避免向无权用户暴露变量名结构。
	if _, err := s.scopedWorkspaceAutomationService(r, false); err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, app.WorkspaceAutomationTemplateVars(), nil)
}

// handleWorkspaceAutomationDeliveryList 列出当前 Workspace 的所有 Delivery。
// 支持 rule_id/project_ref/status/trigger_type/q/limit/offset 过滤。
func (s *Server) handleWorkspaceAutomationDeliveryList(w http.ResponseWriter, r *http.Request) {
	scoped, err := s.scopedWorkspaceAutomationService(r, false)
	if err != nil {
		writeAppError(w, err)
		return
	}
	input := automationDeliveryListInputFromQuery(r)
	rows, total, err := scoped.ListAutomationDeliveries(app.AutomationScopeWorkspaceValue, input)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, rows, map[string]any{"total": total, "limit": input.Limit, "offset": input.Offset})
}

func (s *Server) handleWorkspaceAutomationDeliveryInfo(w http.ResponseWriter, r *http.Request) {
	scoped, err := s.scopedWorkspaceAutomationService(r, false)
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.AutomationDeliveryInfo(app.AutomationScopeWorkspaceValue, chi.URLParam(r, "deliveryID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

func (s *Server) handleWorkspaceAutomationDeliveryReplay(w http.ResponseWriter, r *http.Request) {
	scoped, err := s.scopedWorkspaceAutomationService(r, true)
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.ReplayAutomationDelivery(app.AutomationScopeWorkspaceValue, chi.URLParam(r, "deliveryID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

// automationDeliveryListInputFromQuery 解析 delivery list 的过滤参数。
// 非法 limit/offset 返回默认值，避免静默忽略。
func automationDeliveryListInputFromQuery(r *http.Request) app.AutomationDeliveryListInput {
	input := app.AutomationDeliveryListInput{
		RuleID:      r.URL.Query().Get("rule_id"),
		Status:      r.URL.Query().Get("status"),
		TriggerType: r.URL.Query().Get("trigger_type"),
		Q:           r.URL.Query().Get("q"),
	}
	if projectRef := r.URL.Query().Get("project_ref"); projectRef != "" {
		input.ProjectRef = projectRef
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	input.Limit = limit
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	input.Offset = offset
	return input
}
