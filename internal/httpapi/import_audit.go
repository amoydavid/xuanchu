package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

type auditResponse struct {
	ID                      int64                   `json:"id"`
	ActorType               string                  `json:"actor_type,omitempty"`
	Actor                   *task.JSONUserInfo      `json:"actor,omitempty"`
	ActorToken              *tokenActorJSON         `json:"actor_token,omitempty"`
	WorkspaceID             *string                 `json:"workspace_id"`
	ProjectID               *string                 `json:"project_id"`
	Action                  string                  `json:"action"`
	TargetType              string                  `json:"target_type"`
	TargetID                string                  `json:"target_id"`
	Payload                 json.RawMessage         `json:"payload,omitempty"`
	Changes                 []taskFieldChangeJSON   `json:"changes,omitempty"`
	DelegatorTokenID        *string                 `json:"delegator_token_id,omitempty"`
	DelegatorUser           *task.JSONUserInfo      `json:"delegator_user,omitempty"`
	AdminActingSessionID    *string                 `json:"admin_acting_session_id,omitempty"`
	DelegatorAdminTokenID   *string                 `json:"delegator_admin_token_id,omitempty"`
	DelegatorAdminTokenName string                  `json:"delegator_admin_token_name,omitempty"`
	CreatedAt               int64                   `json:"created_at"`
}

// taskFieldChangeJSON 是字段级变更的 HTTP 输出结构。
// 注意：根据 spec §4.3，标量 previous/current 和集合 added/removed 都
// 不能加 omitempty——标量要保留 raw 为 null 的显式语义，集合空数组也要
// 输出为 []，前端类型把这两组都声明为非可选。
type taskFieldChangeJSON struct {
	Field    string                  `json:"field"`
	Kind     string                  `json:"kind"`
	LabelKey string                  `json:"label_key"`
	Previous *taskChangeDisplayJSON  `json:"previous,omitempty"`
	Current  *taskChangeDisplayJSON  `json:"current,omitempty"`
	Added    []taskChangeDisplayJSON `json:"added"`
	Removed  []taskChangeDisplayJSON `json:"removed"`
}

// taskChangeDisplayJSON 用 json.RawMessage 承载 raw，
// 这样 nil 会序列化成真正的 JSON null，而不是被 omitempty 丢掉。
type taskChangeDisplayJSON struct {
	Raw  json.RawMessage `json:"raw"`
	Text string          `json:"text"`
}

type tokenActorJSON struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Prefix string `json:"prefix"`
}

const auditMaxLimit = 1000

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	projectRef := requestProjectRef(r)
	scoped, _, err := s.scopedService(r, auth.ScopeTaskRead, app.PermissionTaskRead, projectRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	var input app.ExportInput
	if projectRef != "" {
		project, err := scoped.ProjectInfo(projectRef)
		if err != nil {
			writeAppError(w, err)
			return
		}
		input.ProjectID = &project.ID
	}
	rows, err := scoped.ExportWithInput(input)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, tasksToJSON(rows), nil)
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, auth.ScopeTaskWrite, app.PermissionTaskWrite, requestProjectRef(r))
	if err != nil {
		writeAppError(w, err)
		return
	}
	var rows []task.JSONTask
	if err := json.NewDecoder(r.Body).Decode(&rows); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	count, err := scoped.Import(rows)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]int{"imported": count}, nil)
}

func (s *Server) handleAuditList(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "api_bad_limit", "invalid limit", nil)
			return
		}
		if parsed > auditMaxLimit {
			writeError(w, http.StatusBadRequest, "api_bad_limit", fmt.Sprintf("limit must be <= %d", auditMaxLimit), nil)
			return
		}
		limit = parsed
	}
	scoped, _, err := s.scopedService(r, auth.ScopeAuditRead, app.PermissionAuditRead, requestProjectRef(r))
	if err != nil {
		writeAppError(w, err)
		return
	}
	projectRef := r.URL.Query().Get("project")
	if projectRef == "" {
		projectRef = r.URL.Query().Get("project_id")
	}
	rows, err := scoped.ListAudit(app.AuditListInput{ProjectRef: projectRef, Limit: limit})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, auditRowsToResponse(rows), nil)
}

// auditRowsToResponse 把 app AuditLogView 转成 HTTP response，
// 供 handleAuditList 和 task audit handler 共用。
func auditRowsToResponse(rows []app.AuditLogView) []auditResponse {
	out := make([]auditResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, auditRowToResponse(row))
	}
	return out
}

func auditRowToResponse(row app.AuditLogView) auditResponse {
	item := auditResponse{
		ID:                      row.ID,
		ActorType:               row.ActorType,
		WorkspaceID:             row.WorkspaceID,
		ProjectID:               row.ProjectID,
		Action:                  row.Action,
		TargetType:              row.TargetType,
		TargetID:                row.TargetID,
		Changes:                 taskFieldChangesToJSON(row.Changes),
		DelegatorTokenID:        row.DelegatorTokenID,
		AdminActingSessionID:    row.AdminActingSessionID,
		DelegatorAdminTokenID:   row.DelegatorAdminTokenID,
		DelegatorAdminTokenName: row.DelegatorAdminTokenName,
		CreatedAt:               row.CreatedAt,
	}
	if row.Actor != nil {
		jui := task.UserInfoToJSON(*row.Actor)
		item.Actor = &jui
	}
	if row.ActorToken != nil {
		item.ActorToken = &tokenActorJSON{ID: row.ActorToken.ID, Name: row.ActorToken.Name, Prefix: row.ActorToken.Prefix}
	}
	if row.DelegatorUser != nil {
		jui := task.UserInfoToJSON(*row.DelegatorUser)
		item.DelegatorUser = &jui
	}
	if row.PayloadJSON != "" {
		item.Payload = json.RawMessage(row.PayloadJSON)
	}
	return item
}

// taskFieldChangesToJSON 把 app 层 change view 转成 HTTP DTO。
// 标量 previous/current 用 json.RawMessage 承载 raw，
// 保证 nil 值序列化为 JSON null，而不是被 omitempty 省略。
func taskFieldChangesToJSON(changes []app.TaskFieldChange) []taskFieldChangeJSON {
	if len(changes) == 0 {
		return nil
	}
	out := make([]taskFieldChangeJSON, 0, len(changes))
	for _, change := range changes {
		dto := taskFieldChangeJSON{
			Field:    change.Field,
			Kind:     change.Kind,
			LabelKey: change.LabelKey,
		}
		if change.Kind == "set" {
			dto.Added = displayValuesToJSON(change.Added)
			dto.Removed = displayValuesToJSON(change.Removed)
		} else {
			// 标量：previous/current 必须保留 presence。
			// 即使 Raw 为 nil，也要输出 {"raw":null,"text":""}。
			dto.Previous = displayValueToJSONPtr(change.Previous)
			dto.Current = displayValueToJSONPtr(change.Current)
		}
		out = append(out, dto)
	}
	return out
}

func displayValuesToJSON(values []app.TaskChangeDisplayValue) []taskChangeDisplayJSON {
	out := make([]taskChangeDisplayJSON, 0, len(values))
	for _, v := range values {
		out = append(out, displayValueToJSON(v))
	}
	return out
}

func displayValueToJSONPtr(v *app.TaskChangeDisplayValue) *taskChangeDisplayJSON {
	if v == nil {
		return nil
	}
	j := displayValueToJSON(*v)
	return &j
}

func displayValueToJSON(v app.TaskChangeDisplayValue) taskChangeDisplayJSON {
	raw := rawValueToJSON(v.Raw)
	return taskChangeDisplayJSON{Raw: raw, Text: v.Text}
}

// rawValueToJSON 把任意 raw 值序列化成 json.RawMessage。
// nil 值会得到 "null"，保证显式 null 不被 omitempty 丢掉。
func rawValueToJSON(raw any) json.RawMessage {
	if raw == nil {
		return json.RawMessage("null")
	}
	if msg, ok := raw.(json.RawMessage); ok {
		return msg
	}
	data, err := json.Marshal(raw)
	if err != nil {
		// 序列化失败时退化为字符串表示，避免整个响应失败。
		data = []byte(`""`)
	}
	return data
}
