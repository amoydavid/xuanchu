package app

import (
	"context"
	"fmt"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	domain "git.dajee.net/dajee/xuanchu/internal/task"
)

// ContentReferenceSuggestionInput 是 suggest 的输入。
type ContentReferenceSuggestionInput struct {
	Type       string // user|task
	Query      string
	ProjectRef string
	Limit      int
}

// ContentReferenceKeyInput 是 resolve 的单项输入。
type ContentReferenceKeyInput struct {
	Type string
	ID   string
}

// ContentReferenceSuggestion 是 suggest 的单项结果。
type ContentReferenceSuggestion struct {
	Type string               `json:"type"`
	User *domain.JSONUserInfo `json:"user,omitempty"`
	Task *TaskReferenceView   `json:"task,omitempty"`
}

// TaskReferenceView 是 task 引用的稳定视图。
type TaskReferenceView struct {
	ID       string               `json:"id"`
	Title    string               `json:"title"`
	TaskSlug string               `json:"task_slug"`
	Status   string               `json:"status"`
	Project  TaskReferenceProject `json:"project"`
	URL      string               `json:"url"`
}

// TaskReferenceProject 是 task 引用中的 project 摘要。
type TaskReferenceProject struct {
	ID   string `json:"id,omitempty"`
	Slug string `json:"slug,omitempty"`
	Name string `json:"name,omitempty"`
}

// ContentReferenceResolution 是 resolve 的单项结果。
type ContentReferenceResolution struct {
	Type       string               `json:"type"`
	ID         string               `json:"id"`
	Status     string               `json:"status"`
	User       *domain.JSONUserInfo `json:"user,omitempty"`
	Task       *TaskReferenceView   `json:"task,omitempty"`
	Attachment *AttachmentView      `json:"attachment,omitempty"`
}

// buildUserMentionedEventIfNeeded 比较前后 description 中的 user 引用集合，
// 仅当存在新增 user mention 时返回一个 task.user_mentioned 事件。
//
// spec §16.1：label/移动/同次删除再插入都不触发；先删保存、后再次添加保存会再次触发。
func (s *Service) buildUserMentionedEventIfNeeded(before, after domain.Task, now int64) (HookEvent, bool) {
	beforeRefs, err := domain.ParseContentReferences(optionalTextValue(before.Description))
	if err != nil {
		return HookEvent{}, false
	}
	afterRefs, err := domain.ParseContentReferences(optionalTextValue(after.Description))
	if err != nil {
		return HookEvent{}, false
	}
	added, _ := domain.DiffContentReferenceKeys(beforeRefs, afterRefs)
	addedUserIDs := uniqueUserIDs(added)
	if len(addedUserIDs) == 0 {
		return HookEvent{}, false
	}

	// 解析为完整 UserInfo。未找到的 fallback 为 {ID, Name: id}，不出错。
	addedInfos, _ := s.resolveUserInfos(addedUserIDs)
	currentIDs, _ := domain.MentionedUserIDs(optionalTextValue(after.Description))
	currentInfos, _ := s.resolveUserInfos(currentIDs)

	addedList := orderedUserInfoList(addedUserIDs, addedInfos)
	currentList := orderedUserInfoList(currentIDs, currentInfos)

	event := buildTaskHookEvent("task.user_mentioned", after, s.runtime, now)
	event.Data["source_field"] = "description"
	event.Data["mentioned_users"] = domain.UserInfoListToJSON(addedList)
	event.Data["current_mentioned_users"] = domain.UserInfoListToJSON(currentList)
	return event, true
}

func uniqueUserIDs(keys []domain.ContentReferenceKey) []string {
	seen := map[string]struct{}{}
	var ids []string
	for _, k := range keys {
		if k.Kind != domain.ContentReferenceUser {
			continue
		}
		if _, ok := seen[k.ID]; ok {
			continue
		}
		seen[k.ID] = struct{}{}
		ids = append(ids, k.ID)
	}
	return ids
}

func orderedUserInfoList(ids []string, infos map[string]domain.UserInfo) []domain.UserInfo {
	out := make([]domain.UserInfo, 0, len(ids))
	for _, id := range ids {
		info, ok := infos[id]
		if !ok {
			info = domain.UserInfo{ID: id, Name: id}
		}
		out = append(out, info)
	}
	return out
}

// SuggestContentReferences 查询用户或任务引用建议。
//
// query 允许为空：空 query 时 user 返回当前 workspace 的全部 active member、
// task 返回 request scope 内可读的实际任务（均按 limit 截断），让用户敲 @ / #
// 就看到候选列表；非空 query 则按 display_name/name/email（user）或 title/slug（task）过滤。
func (s *Service) SuggestContentReferences(ctx context.Context, input ContentReferenceSuggestionInput) ([]ContentReferenceSuggestion, error) {
	query := strings.TrimSpace(input.Query)
	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	switch input.Type {
	case "user":
		if err := s.Require(PermissionWorkspaceRead); err != nil {
			return nil, err
		}
		members, err := s.memberRepo.SearchActiveMembers(s.workspaceID, query, limit)
		if err != nil {
			return nil, err
		}
		// 加载 external IDs 以构造完整 UserInfo。
		userIDs := make([]string, 0, len(members))
		for _, m := range members {
			userIDs = append(userIDs, m.User.ID)
		}
		infos, _ := s.resolveUserInfos(userIDs)
		out := make([]ContentReferenceSuggestion, 0, len(members))
		for _, m := range members {
			info := infos[m.User.ID]
			if info.ID == "" {
				info = domain.UserInfo{ID: m.User.ID, Name: m.User.Name, DisplayName: m.User.DisplayName, Email: m.User.Email}
			}
			out = append(out, ContentReferenceSuggestion{Type: "user", User: jsonUserInfoPtr(info)})
		}
		return out, nil
	case "task":
		if err := s.Require(PermissionTaskRead); err != nil {
			return nil, err
		}
		projectID := ""
		if input.ProjectRef != "" {
			if proj, err := s.resolveProjectRef(input.ProjectRef); err == nil {
				projectID = proj.ID
			}
		}
		tasks, err := s.repo.SearchTasksByTitleOrSlug(s.workspaceID, query, projectID, limit)
		if err != nil {
			return nil, err
		}
		// 应用 project allowlist / scope。
		out := make([]ContentReferenceSuggestion, 0, len(tasks))
		for _, tsk := range tasks {
			if !s.allowsProjectScopeForRead(tsk) {
				continue
			}
			out = append(out, ContentReferenceSuggestion{Type: "task", Task: s.taskReferenceView(tsk)})
		}
		return out, nil
	default:
		return nil, RuntimeError{Code: "content_reference_query_invalid", Message: "type must be user or task"}
	}
}

// ResolveContentReferences 批量解析引用，保持输入顺序。
//
// 每项独立判断；不可读/不存在/越权统一返回 unavailable（spec §13.3 不区分 403/404）。
func (s *Service) ResolveContentReferences(ctx context.Context, keys []ContentReferenceKeyInput) ([]ContentReferenceResolution, error) {
	if len(keys) > 200 {
		return nil, RuntimeError{Code: "description_reference_limit_exceeded", Message: "too many references; max 200"}
	}

	// 收集各类 ID 做批量查询。
	var userIDs, taskIDs, attachmentIDs []string
	for _, k := range keys {
		switch k.Type {
		case "user":
			userIDs = append(userIDs, k.ID)
		case "task":
			taskIDs = append(taskIDs, k.ID)
		case "attachment":
			attachmentIDs = append(attachmentIDs, k.ID)
		}
	}
	// resolve 是混合批量 API：缺少某一类资源的读取能力不能泄露目标，也不能
	// 让其它类型一起失败。先算出每类资源可用的能力，再在各自分支内返回
	// unavailable（而不是依赖 project allowlist 作为唯一 read 判断）。
	canReadWorkspace := s.canResolveReferenceWith(PermissionWorkspaceRead, auth.ScopeWorkspaceRead)
	canReadTask := s.canResolveReferenceWith(PermissionTaskRead, auth.ScopeTaskRead)

	// user 解析：必须是当前 workspace active member。
	userInfos := map[string]domain.UserInfo{}
	if canReadWorkspace && len(userIDs) > 0 {
		infos, _ := s.resolveUserInfos(userIDs)
		members, _ := s.memberRepo.ListMembersByUserIDs(s.workspaceID, userIDs)
		activeMember := map[string]struct{}{}
		for _, m := range members {
			activeMember[m.User.ID] = struct{}{}
		}
		for id, info := range infos {
			if _, ok := activeMember[id]; ok {
				userInfos[id] = info
			}
		}
	}

	// task 解析：当前 workspace 且 request scope 可读。
	taskViews := map[string]*TaskReferenceView{}
	if canReadTask && len(taskIDs) > 0 {
		tasks, _ := s.repo.ListByUUIDs(s.workspaceID, taskIDs)
		for _, tsk := range tasks {
			if !s.allowsProjectScopeForRead(tsk) {
				continue
			}
			taskViews[tsk.UUID] = s.taskReferenceView(tsk)
		}
	}

	// attachment 解析：通过 attachment target handler 校验读取权限。
	attachmentViews := map[string]*AttachmentView{}
	if canReadTask && s.attachmentRuntime != nil {
		for _, id := range attachmentIDs {
			view, err := s.GetAttachment(id)
			if err != nil {
				continue
			}
			attachmentViews[id] = &view
		}
	}

	out := make([]ContentReferenceResolution, 0, len(keys))
	for _, k := range keys {
		res := ContentReferenceResolution{Type: k.Type, ID: k.ID, Status: "unavailable"}
		switch k.Type {
		case "user":
			if info, ok := userInfos[k.ID]; ok {
				res.Status = "resolved"
				u := domain.UserInfoToJSON(info)
				res.User = &u
			}
		case "task":
			if tv, ok := taskViews[k.ID]; ok {
				res.Status = "resolved"
				res.Task = tv
			}
		case "attachment":
			if av, ok := attachmentViews[k.ID]; ok {
				res.Status = "resolved"
				res.Attachment = av
			}
		}
		out = append(out, res)
	}
	return out, nil
}

// canResolveReferenceWith 同时检查角色权限和 HTTP/MCP 注入的 token scope。
// 普通 Request handler 会在入口检查一个主 capability；resolve 是混合资源接口，
// 必须在这里逐项检查，否则 workspace:read PAT 会因 owner/admin 角色间接读到 task。
func (s *Service) canResolveReferenceWith(permission Permission, capability string) bool {
	if s.Require(permission) != nil {
		return false
	}
	return s.requestScope == nil || s.requestScope.HasCapability(capability)
}

// allowsProjectScopeForRead 判断 task 是否在当前 request scope 可读。
func (s *Service) allowsProjectScopeForRead(tsk domain.Task) bool {
	if tsk.WorkspaceID != s.workspaceID {
		return false
	}
	if !s.hasProjectScope() {
		return true
	}
	if tsk.ProjectID == nil {
		return false
	}
	return s.requestScope.AllowsProject(*tsk.ProjectID)
}

// taskReferenceView 把 domain.Task 转为 TaskReferenceView。
func (s *Service) taskReferenceView(tsk domain.Task) *TaskReferenceView {
	view := &TaskReferenceView{
		ID:       tsk.UUID,
		Title:    tsk.Title,
		TaskSlug: deriveTaskSlug(tsk),
		Status:   tsk.Status,
	}
	if tsk.Project != nil {
		view.Project.Slug = *tsk.Project
	}
	if tsk.ProjectID != nil {
		view.Project.ID = *tsk.ProjectID
		if proj, err := s.projectRepo.GetByID(*tsk.ProjectID); err == nil {
			view.Project.Slug = proj.Slug
			view.Project.Name = proj.Name
		}
	}
	view.URL = s.taskCanonicalURL(tsk)
	return view
}

// taskCanonicalURL 返回任务的 Web Console 绝对 URL（复用 ProjectTaskURL/StandaloneTaskURL）。
func (s *Service) taskCanonicalURL(tsk domain.Task) string {
	slug := deriveTaskSlug(tsk)
	if slug != "" && tsk.Project != nil {
		return ProjectTaskURL(s.resourceBaseURL, s.runtime.WorkspaceSlug, *tsk.Project, slug)
	}
	return StandaloneTaskURL(s.resourceBaseURL, tsk.UUID)
}

// deriveTaskSlug 用 project slug + project seq 派生 task slug（spec §5.5）。
func deriveTaskSlug(tsk domain.Task) string {
	if tsk.Project == nil || tsk.ProjectSeq == nil {
		return ""
	}
	return fmt.Sprintf("%s-%d", *tsk.Project, *tsk.ProjectSeq)
}

// resolveProjectRef 通过 ProjectRepository.GetByRef 解析（支持 slug 或 UUID）。
func (s *Service) resolveProjectRef(ref string) (*storage.Project, error) {
	proj, err := s.projectRepo.GetByRef(s.workspaceID, strings.TrimSpace(ref))
	if err != nil {
		return nil, err
	}
	return &proj, nil
}

// jsonUserInfoPtr 把 UserInfo 转 *JSONUserInfo。
func jsonUserInfoPtr(info domain.UserInfo) *domain.JSONUserInfo {
	j := domain.UserInfoToJSON(info)
	return &j
}
