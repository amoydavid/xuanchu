package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/projecttemplate"
	"git.dajee.net/dajee/xuanchu/internal/query"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
	"git.dajee.net/dajee/xuanchu/internal/taskseries"
	"git.dajee.net/dajee/xuanchu/internal/urgency"
)

const (
	projectTemplateCandidateDefaultLimit = 50
	projectTemplateCandidateMaxLimit     = 100
)

type TaskCandidateListInput struct {
	SourceProjectRef string
	Refs             []string
	Q                string
	Status           string
	Priority         string
	Assignees        []string
	Tags             []string
	DueAfter         string
	DueBefore        string
	Query            string
	Sort             string
	Limit            int
	Offset           int
}

type SeriesCandidateListInput struct {
	SourceProjectRef string
	Refs             []string
	Q                string
	Status           string
	Assignee         string
	Sort             string
	Limit            int
	Offset           int
}

type ConfigCandidateListInput struct {
	SourceProjectRef string
	Refs             []string
	Q                string
	Mode             string
	Limit            int
	Offset           int
}

type AutomationCandidateListInput struct {
	SourceProjectRef string
	Refs             []string
	Q                string
	Status           string
	TriggerType      string
	Limit            int
	Offset           int
}

type TaskCandidateView struct {
	Ref          string          `json:"ref"`
	ProjectID    string          `json:"project_id"`
	ProjectSeq   *int64          `json:"project_seq,omitempty"`
	SeriesID     *string         `json:"series_id,omitempty"`
	Title        string          `json:"title"`
	Status       string          `json:"status"`
	Priority     *string         `json:"priority,omitempty"`
	Due          *int64          `json:"due,omitempty"`
	Assignees    []task.UserInfo `json:"assignees"`
	WarningCount int             `json:"warning_count"`
}

type TaskCandidatePage struct {
	Items  []TaskCandidateView `json:"items"`
	Total  int64               `json:"total"`
	Limit  int                 `json:"limit"`
	Offset int                 `json:"offset"`
}

type SeriesCandidateView struct {
	Ref            string          `json:"ref"`
	ProjectID      string          `json:"project_id"`
	ProjectSeq     *int64          `json:"project_seq,omitempty"`
	Title          string          `json:"title"`
	Status         string          `json:"status"`
	RecurrenceRule string          `json:"recurrence_rule"`
	FirstDue       int64           `json:"first_due"`
	Assignees      []task.UserInfo `json:"assignees"`
	CreatedBy      task.UserInfo   `json:"created_by"`
	WarningCount   int             `json:"warning_count"`
}

type SeriesCandidatePage struct {
	Items  []SeriesCandidateView `json:"items"`
	Total  int64                 `json:"total"`
	Limit  int                   `json:"limit"`
	Offset int                   `json:"offset"`
}

type ConfigCandidateView struct {
	Ref          string `json:"ref"`
	Key          string `json:"key"`
	Label        string `json:"label"`
	Mode         string `json:"mode"`
	ValueType    string `json:"value_type"`
	WarningCount int    `json:"warning_count"`
}

type ConfigCandidatePage struct {
	Items  []ConfigCandidateView `json:"items"`
	Total  int64                 `json:"total"`
	Limit  int                   `json:"limit"`
	Offset int                   `json:"offset"`
}

type AutomationCandidateView struct {
	Ref          string        `json:"ref"`
	ID           string        `json:"id"`
	ProjectID    string        `json:"project_id"`
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	Enabled      bool          `json:"enabled"`
	TriggerType  string        `json:"trigger_type"`
	CreatedBy    task.UserInfo `json:"created_by"`
	CreatedAt    int64         `json:"created_at"`
	WarningCount int           `json:"warning_count"`
}

type AutomationCandidatePage struct {
	Items  []AutomationCandidateView `json:"items"`
	Total  int64                     `json:"total"`
	Limit  int                       `json:"limit"`
	Offset int                       `json:"offset"`
}

type CandidateSelectionQuery struct {
	SourceProjectRef string
	Kind             string
	Task             *TaskCandidateListInput
	Series           *SeriesCandidateListInput
	Config           *ConfigCandidateListInput
	Automation       *AutomationCandidateListInput
}

type ResolvedCandidateSelection struct {
	Refs       []string `json:"refs"`
	Total      int      `json:"total"`
	SourceHash string   `json:"source_hash"`
}

func (s *Service) ListProjectTemplateTaskCandidates(input TaskCandidateListInput) (TaskCandidatePage, error) {
	limit, offset, err := normalizeProjectTemplateCandidatePage(input.Limit, input.Offset)
	if err != nil {
		return TaskCandidatePage{}, err
	}
	return s.listProjectTemplateTaskCandidates(input, limit, offset)
}

func (s *Service) listProjectTemplateTaskCandidates(input TaskCandidateListInput, limit, offset int) (TaskCandidatePage, error) {
	if err := s.requireProjectTemplateCandidate(PermissionTaskRead); err != nil {
		return TaskCandidatePage{}, err
	}
	project, err := s.ResolveProject(input.SourceProjectRef)
	if err != nil {
		return TaskCandidatePage{}, err
	}
	status := strings.TrimSpace(input.Status)
	if len(input.Refs) > 0 {
		status = "all"
	}
	if status == "" {
		status = task.StatusPending
	}
	if status != task.StatusPending && status != task.StatusWaiting && status != task.StatusCompleted && status != "all" {
		return TaskCandidatePage{}, candidateInputError("task status must be pending|waiting|completed|all")
	}
	priority := strings.ToUpper(strings.TrimSpace(input.Priority))
	if len(input.Refs) > 0 {
		priority = "ALL"
		input.Q = ""
		input.Query = ""
		input.Assignees, input.Tags = nil, nil
		input.DueAfter, input.DueBefore = "", ""
	}
	if priority != "" && priority != "ALL" && priority != "H" && priority != "M" && priority != "L" {
		return TaskCandidatePage{}, candidateInputError("task priority must be H|M|L|all")
	}
	if priority == "ALL" {
		priority = "all"
	}
	sortKey := strings.TrimSpace(input.Sort)
	if sortKey == "" {
		sortKey = "urgency"
	}
	if sortKey != "urgency" && sortKey != "entry" && sortKey != "due" && sortKey != "wait" && sortKey != "completed" && sortKey != "source" {
		return TaskCandidatePage{}, candidateInputError("unsupported task candidate sort")
	}
	var expr query.Expr
	if source := strings.TrimSpace(input.Query); source != "" {
		expr, err = query.ParseQuery(source)
		if err != nil {
			return TaskCandidatePage{}, candidateInputError(err.Error())
		}
		expr, err = s.resolveTaskQueryPredicates(expr)
		if err != nil {
			return TaskCandidatePage{}, err
		}
	}
	assigneeIDs := make([]string, 0, len(input.Assignees))
	if len(input.Assignees) > 0 {
		resolved, resolveErr := s.resolveAssigneeRefs(input.Assignees)
		if resolveErr != nil {
			return TaskCandidatePage{}, resolveErr
		}
		assigneeIDs = extractAssigneeIDs(resolved)
	}
	dueAfter, err := candidateDateBoundary(input.DueAfter, s.clock.Unix(), s.clock.Location(), false)
	if err != nil {
		return TaskCandidatePage{}, candidateInputError(err.Error())
	}
	dueBefore, err := candidateDateBoundary(input.DueBefore, s.clock.Unix(), s.clock.Location(), true)
	if err != nil {
		return TaskCandidatePage{}, candidateInputError(err.Error())
	}
	udaDefs, err := s.udaDefinitionTypes()
	if err != nil {
		return TaskCandidatePage{}, err
	}
	var urgencyScore func(task.Task, bool, bool) float64
	if sortKey == "urgency" {
		urgencyOpts, configErr := s.urgencyConfig()
		if configErr != nil {
			return TaskCandidatePage{}, configErr
		}
		urgencyOpts.NowUnix = s.clock.Unix()
		urgencyScore = func(row task.Task, blocked, blocking bool) float64 {
			opts := urgencyOpts
			opts.Blocked, opts.Blocking = blocked, blocking
			return urgency.Explain(row, opts).Total
		}
	}
	page, err := s.repo.ListCandidatePage(storage.TaskCandidateListOptions{
		WorkspaceID: s.workspaceID, ProjectID: project.ID, Refs: input.Refs, Q: input.Q, Status: status, Priority: priority,
		AssigneeUserIDs: assigneeIDs, Tags: input.Tags, DueAfter: dueAfter, DueBefore: dueBefore,
		Query: expr, Sort: sortKey, NowUnix: s.clock.Unix(), UDADefinitions: udaDefs, Dialect: s.store.Dialect(),
		UrgencyScore: urgencyScore,
	}, limit, offset)
	if err != nil {
		return TaskCandidatePage{}, mapProjectQueryCompileError(err)
	}
	userIDs := make([]string, 0, len(page.Items)*2)
	for _, row := range page.Items {
		for _, assignee := range row.Assignees {
			userIDs = append(userIDs, assignee.UserID)
		}
	}
	users, err := s.resolveUserInfos(userIDs)
	if err != nil {
		return TaskCandidatePage{}, err
	}
	items := make([]TaskCandidateView, 0, len(page.Items))
	for _, row := range page.Items {
		projectID := ""
		if row.ProjectID != nil {
			projectID = *row.ProjectID
		}
		items = append(items, TaskCandidateView{
			Ref: row.UUID, ProjectID: projectID, ProjectSeq: row.ProjectSeq, SeriesID: row.SeriesID,
			Title: row.Title, Status: row.Status, Priority: row.Priority, Due: row.Due,
			Assignees: userInfoList(row.Assignees, users), WarningCount: descriptionAttachmentWarningCount(row.Description),
		})
	}
	return TaskCandidatePage{Items: items, Total: page.Total, Limit: limit, Offset: offset}, nil
}

func (s *Service) ListProjectTemplateSeriesCandidates(input SeriesCandidateListInput) (SeriesCandidatePage, error) {
	limit, offset, err := normalizeProjectTemplateCandidatePage(input.Limit, input.Offset)
	if err != nil {
		return SeriesCandidatePage{}, err
	}
	return s.listProjectTemplateSeriesCandidates(input, limit, offset)
}

func (s *Service) listProjectTemplateSeriesCandidates(input SeriesCandidateListInput, limit, offset int) (SeriesCandidatePage, error) {
	if err := s.requireProjectTemplateCandidate(PermissionTaskRead); err != nil {
		return SeriesCandidatePage{}, err
	}
	project, err := s.ResolveProject(input.SourceProjectRef)
	if err != nil {
		return SeriesCandidatePage{}, err
	}
	status := strings.TrimSpace(input.Status)
	if len(input.Refs) > 0 {
		status = "all"
		input.Q, input.Assignee = "", ""
	}
	if status == "" {
		status = taskseries.StatusActive
	}
	if status != taskseries.StatusActive && status != taskseries.StatusEnded && status != taskseries.StatusStopped && status != "all" {
		return SeriesCandidatePage{}, candidateInputError("series status must be active|ended|stopped|all")
	}
	sortKey := strings.TrimSpace(input.Sort)
	if sortKey == "" {
		sortKey = "next"
	}
	if sortKey != "next" && sortKey != "title" && sortKey != "modified" && sortKey != "source" {
		return SeriesCandidatePage{}, candidateInputError("unsupported series candidate sort")
	}
	assigneeID := ""
	if ref := strings.TrimSpace(input.Assignee); ref != "" {
		resolved, resolveErr := s.resolveAssigneeRefs([]string{ref})
		if resolveErr != nil {
			return SeriesCandidatePage{}, resolveErr
		}
		assigneeID = resolved[0].UserID
	}
	page, err := s.taskSeriesRepo.ListCandidatePage(storage.TaskSeriesListOptions{
		WorkspaceID: s.workspaceID, ProjectID: project.ID, Refs: input.Refs, Status: status, Q: input.Q, AssigneeUserID: assigneeID, Sort: sortKey,
		NextRecurrenceAt: func(series taskseries.Series) *int64 { return computeNextRecurrenceAt(series, s.clock) },
	}, limit, offset)
	if err != nil {
		return SeriesCandidatePage{}, err
	}
	userIDs := make([]string, 0, len(page.Items)*2)
	for _, row := range page.Items {
		userIDs = append(userIDs, row.CreatedBy)
		userIDs = append(userIDs, row.AssigneeIDs...)
	}
	users, err := s.resolveUserInfos(userIDs)
	if err != nil {
		return SeriesCandidatePage{}, err
	}
	items := make([]SeriesCandidateView, 0, len(page.Items))
	for _, row := range page.Items {
		items = append(items, SeriesCandidateView{
			Ref: row.ID, ProjectID: row.ProjectID, ProjectSeq: row.ProjectSeq, Title: row.Title, Status: row.Status,
			RecurrenceRule: row.RecurrenceRule, FirstDue: row.FirstDue, Assignees: seriesUserInfoList(row, users),
			CreatedBy: candidateUserInfo(row.CreatedBy, users), WarningCount: descriptionAttachmentWarningCount(row.Description),
		})
	}
	return SeriesCandidatePage{Items: items, Total: page.Total, Limit: limit, Offset: offset}, nil
}

func (s *Service) ListProjectTemplateConfigCandidates(input ConfigCandidateListInput) (ConfigCandidatePage, error) {
	limit, offset, err := normalizeProjectTemplateCandidatePage(input.Limit, input.Offset)
	if err != nil {
		return ConfigCandidatePage{}, err
	}
	return s.listProjectTemplateConfigCandidates(input, limit, offset)
}

func (s *Service) listProjectTemplateConfigCandidates(input ConfigCandidateListInput, limit, offset int) (ConfigCandidatePage, error) {
	if err := s.requireProjectTemplateCandidate(PermissionProjectConfigRead); err != nil {
		return ConfigCandidatePage{}, err
	}
	project, err := s.ResolveProject(input.SourceProjectRef)
	if err != nil {
		return ConfigCandidatePage{}, err
	}
	mode := strings.TrimSpace(input.Mode)
	if len(input.Refs) > 0 {
		mode, input.Q = "all", ""
	}
	if mode == "" {
		mode = "all"
	}
	if mode != "all" && mode != "literal" && mode != "secret" {
		return ConfigCandidatePage{}, candidateInputError("config mode must be all|literal|secret")
	}
	page, err := s.configRepo.ListCandidatePage(storage.ConfigCandidateListOptions{WorkspaceID: s.workspaceID, ProjectID: project.ID, Refs: input.Refs, Q: input.Q, Mode: mode}, limit, offset)
	if err != nil {
		return ConfigCandidatePage{}, err
	}
	items := make([]ConfigCandidateView, 0, len(page.Items))
	for _, row := range page.Items {
		itemMode := "literal"
		if row.Definition.Secret {
			itemMode = "secret"
		}
		items = append(items, ConfigCandidateView{Ref: row.Config.Key, Key: row.Config.Key, Label: row.Definition.Label, Mode: itemMode, ValueType: row.Definition.ValueType})
	}
	return ConfigCandidatePage{Items: items, Total: page.Total, Limit: limit, Offset: offset}, nil
}

func (s *Service) ListProjectTemplateAutomationCandidates(input AutomationCandidateListInput) (AutomationCandidatePage, error) {
	limit, offset, err := normalizeProjectTemplateCandidatePage(input.Limit, input.Offset)
	if err != nil {
		return AutomationCandidatePage{}, err
	}
	return s.listProjectTemplateAutomationCandidates(input, limit, offset)
}

func (s *Service) listProjectTemplateAutomationCandidates(input AutomationCandidateListInput, limit, offset int) (AutomationCandidatePage, error) {
	if err := s.requireProjectTemplateCandidate(PermissionHookRead); err != nil {
		return AutomationCandidatePage{}, err
	}
	project, err := s.ResolveProject(input.SourceProjectRef)
	if err != nil {
		return AutomationCandidatePage{}, err
	}
	status := strings.TrimSpace(input.Status)
	if len(input.Refs) > 0 {
		status, input.Q, input.TriggerType = "all", "", "all"
	}
	if status == "" {
		status = "all"
	}
	if status != "all" && status != "enabled" && status != "disabled" {
		return AutomationCandidatePage{}, candidateInputError("automation status must be enabled|disabled|all")
	}
	trigger := strings.TrimSpace(input.TriggerType)
	if trigger == "" {
		trigger = "all"
	}
	if trigger != "all" && trigger != "schedule" && trigger != "event" {
		return AutomationCandidatePage{}, candidateInputError("automation trigger type must be schedule|event|all")
	}
	page, err := s.projectAutomationRuleRepo.ListCandidatePage(storage.ProjectAutomationCandidateListOptions{
		WorkspaceID: s.workspaceID, ProjectID: project.ID, Refs: input.Refs, Q: input.Q, Enabled: status, TriggerType: trigger,
	}, limit, offset)
	if err != nil {
		return AutomationCandidatePage{}, err
	}
	userIDs := make([]string, 0, len(page.Items))
	for _, row := range page.Items {
		userIDs = append(userIDs, valueOrEmpty(row.CreatedByUserID))
	}
	users, err := s.resolveUserInfos(userIDs)
	if err != nil {
		return AutomationCandidatePage{}, err
	}
	items := make([]AutomationCandidateView, 0, len(page.Items))
	for _, row := range page.Items {
		items = append(items, AutomationCandidateView{
			Ref: row.ID, ID: row.ID, ProjectID: row.ProjectID, Name: row.Name, Description: row.Description,
			Enabled: row.Enabled != nil && *row.Enabled, TriggerType: row.TriggerType,
			CreatedBy: candidateUserInfo(valueOrEmpty(row.CreatedByUserID), users), CreatedAt: row.CreatedAt,
		})
	}
	return AutomationCandidatePage{Items: items, Total: page.Total, Limit: limit, Offset: offset}, nil
}

// ResolveProjectTemplateCandidateSelection 将当前筛选展开成规范顺序的显式引用。
func (s *Service) ResolveProjectTemplateCandidateSelection(input CandidateSelectionQuery) (ResolvedCandidateSelection, error) {
	kind := strings.TrimSpace(input.Kind)
	refs := []string{}
	var total int64
	switch kind {
	case "task":
		candidate := TaskCandidateListInput{}
		if input.Task != nil {
			candidate = *input.Task
		}
		candidate.SourceProjectRef, candidate.Sort = input.SourceProjectRef, "source"
		limit := projecttemplate.DefaultLimits.MaxTasks
		page, err := s.listProjectTemplateTaskCandidates(candidate, limit+1, 0)
		if err != nil {
			return ResolvedCandidateSelection{}, err
		}
		total = page.Total
		for _, item := range page.Items {
			refs = append(refs, item.Ref)
		}
		if total > int64(limit) {
			return ResolvedCandidateSelection{}, candidateLimitError(kind, total, limit)
		}
	case "series":
		candidate := SeriesCandidateListInput{}
		if input.Series != nil {
			candidate = *input.Series
		}
		candidate.SourceProjectRef, candidate.Sort = input.SourceProjectRef, "source"
		limit := projecttemplate.DefaultLimits.MaxSeries
		page, err := s.listProjectTemplateSeriesCandidates(candidate, limit+1, 0)
		if err != nil {
			return ResolvedCandidateSelection{}, err
		}
		total = page.Total
		for _, item := range page.Items {
			refs = append(refs, item.Ref)
		}
		if total > int64(limit) {
			return ResolvedCandidateSelection{}, candidateLimitError(kind, total, limit)
		}
	case "config":
		candidate := ConfigCandidateListInput{}
		if input.Config != nil {
			candidate = *input.Config
		}
		candidate.SourceProjectRef = input.SourceProjectRef
		limit := projecttemplate.DefaultLimits.MaxConfigs
		page, err := s.listProjectTemplateConfigCandidates(candidate, limit+1, 0)
		if err != nil {
			return ResolvedCandidateSelection{}, err
		}
		total = page.Total
		for _, item := range page.Items {
			refs = append(refs, item.Ref)
		}
		if total > int64(limit) {
			return ResolvedCandidateSelection{}, candidateLimitError(kind, total, limit)
		}
	case "automation":
		candidate := AutomationCandidateListInput{}
		if input.Automation != nil {
			candidate = *input.Automation
		}
		candidate.SourceProjectRef = input.SourceProjectRef
		limit := projecttemplate.DefaultLimits.MaxAutomations
		page, err := s.listProjectTemplateAutomationCandidates(candidate, limit+1, 0)
		if err != nil {
			return ResolvedCandidateSelection{}, err
		}
		total = page.Total
		for _, item := range page.Items {
			refs = append(refs, item.Ref)
		}
		if total > int64(limit) {
			return ResolvedCandidateSelection{}, candidateLimitError(kind, total, limit)
		}
	default:
		return ResolvedCandidateSelection{}, candidateInputError("candidate kind must be task|series|config|automation")
	}
	sourceProject, err := s.ResolveProject(input.SourceProjectRef)
	if err != nil {
		return ResolvedCandidateSelection{}, err
	}
	hash, err := projectTemplateSelectionSourceHash(kind, sourceProject.ID, refs)
	if err != nil {
		return ResolvedCandidateSelection{}, err
	}
	return ResolvedCandidateSelection{Refs: refs, Total: int(total), SourceHash: hash}, nil
}

func (s *Service) requireProjectTemplateCandidate(permission Permission) error {
	if err := s.rejectProjectTemplateProjectScope(); err != nil {
		return err
	}
	if err := s.Require(PermissionProjectRead); err != nil {
		return err
	}
	return s.Require(permission)
}

func normalizeProjectTemplateCandidatePage(limit, offset int) (int, int, error) {
	if offset < 0 || limit < 0 {
		return 0, 0, candidateInputError("candidate limit and offset must not be negative")
	}
	if limit == 0 {
		limit = projectTemplateCandidateDefaultLimit
	}
	if limit > projectTemplateCandidateMaxLimit {
		limit = projectTemplateCandidateMaxLimit
	}
	return limit, offset, nil
}

func candidateDateBoundary(raw string, now int64, loc *time.Location, inclusiveEnd bool) (*int64, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	start, end, err := query.ResolveDateRange(query.ParseDateValue(strings.TrimSpace(raw)), now, loc)
	if err != nil {
		return nil, err
	}
	if inclusiveEnd {
		return &end, nil
	}
	return &start, nil
}

func candidateInputError(message string) RuntimeError {
	return RuntimeError{Code: "project_template_candidate_invalid", Message: message}
}

func candidateLimitError(kind string, total int64, limit int) RuntimeError {
	return RuntimeError{Code: "project_template_candidate_limit_exceeded", Message: fmt.Sprintf("%s candidate count %d exceeds snapshot limit %d", kind, total, limit)}
}

func candidateUserInfo(id string, users map[string]task.UserInfo) task.UserInfo {
	if user, ok := users[id]; ok {
		return user
	}
	return task.UserInfo{ID: id, Name: id}
}

func descriptionAttachmentWarningCount(description *string) int {
	if description == nil || *description == "" {
		return 0
	}
	refs, err := task.ParseContentReferences(*description)
	if err != nil {
		return 1
	}
	return len(uniqueAttachmentIDs(refs))
}

func projectTemplateSelectionSourceHash(kind, sourceProjectID string, refs []string) (string, error) {
	payload, err := json.Marshal(struct {
		Kind            string   `json:"kind"`
		SourceProjectID string   `json:"source_project_id"`
		Refs            []string `json:"refs"`
	}{Kind: kind, SourceProjectID: sourceProjectID, Refs: refs})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}
