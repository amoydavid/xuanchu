package app

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/query"
	"git.dajee.net/dajee/xuanchu/internal/recurrence"
	"git.dajee.net/dajee/xuanchu/internal/report"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
	"git.dajee.net/dajee/xuanchu/internal/uda"
	"git.dajee.net/dajee/xuanchu/internal/urgency"
)

type Service struct {
	store                     *storage.Store
	repo                      *storage.TaskRepository
	projectRepo               *storage.ProjectRepository
	configRepo                *storage.ConfigRepository
	configDefRepo             *storage.ConfigDefinitionRepository
	userRepo                  *storage.UserRepository
	workspaceRepo             *storage.WorkspaceRepository
	memberRepo                *storage.MemberRepository
	auditRepo                 auditAppenderLister
	tokenRepo                 *storage.TokenRepository
	adminActingSessionRepo    adminActingSessionStore
	contextRepo               *storage.ContextRepository
	udaRepo                   *storage.UDARepository
	hookRepo                  *storage.HookRepository
	hookDeliveryRepo          hookDeliveryEnqueuer
	notificationSinkRepo      *storage.NotificationSinkRepository
	reminderRuleRepo          *storage.ReminderRuleRepository
	eventNotificationRuleRepo *storage.EventNotificationRuleRepository
	notificationDeliveryRepo  *storage.NotificationDeliveryRepository
	projectAutomationRuleRepo     *storage.ProjectAutomationRuleRepository
	projectAutomationDeliveryRepo *storage.ProjectAutomationDeliveryRepository
	extIDRepo                 *storage.ExternalIDRepository
	runtimeConfig             map[string]string
	runtimeOverrides          map[string]string
	runtimeUDAs               map[string]uda.Definition
	activeContextOverride     *string
	runtime                   RuntimeContext
	requestScope              *RequestScope
	workspaceID               string
	clock                     Clock
	reports                   report.Registry
	disableContext            bool
	// sinkTestClient 用于 notification sink 测试投递；nil 时使用 SSRF-safe 默认 client。
	// 测试可注入 httptest.Server.Client() 以便命中本地服务。
	sinkTestClient *http.Client
	// sinkTestResolver 覆盖默认 DNS 解析器，便于测试绕过 loopback 限制。
	sinkTestResolver HookHostResolver
	// tokenSecretKey 加密可恢复的 API token 明文，供 Web Console 按需 reveal。
	tokenSecretKey []byte
	// requireTokenSecret 为 true 时，缺少 secret key 的 token 创建请求会失败。
	requireTokenSecret bool
}

// adminActingSessionStore 是 admin acting session 仓储在 app 层的最小接口。
// 生产环境绑定 storage.AdminActingSessionRepository；测试可注入替代品。
type adminActingSessionStore interface {
	Create(storage.AdminActingSessionEntry) error
	GetByPrefix(prefix string) (storage.AdminActingSessionEntry, error)
	GetByID(id string) (storage.AdminActingSessionEntry, error)
	TouchLastUsed(id string, ts int64) error
	Revoke(id string, ts int64) error
}

type AddInput struct {
	Title       string
	Description *string
	Project     *string
	Priority    *string
	Due         *int64
	Assignees   []string
	Depends     []string
	Wait        *int64
	Scheduled   *int64
	Until       *int64
	Recur       *string
	Tags        []string
	UDAs        map[string]string
	Parent      *string
}

type ListInput struct {
	Target     *string
	Status     string
	Sort       string
	Query      query.Expr
	ReportMode bool
	NoContext  bool
	Limit      int
	Offset     int
}

type ExportInput struct {
	ProjectID *string
}

type ModifyInput struct {
	Title            *string
	Description      *string
	ClearDescription bool
	Project          *string
	ClearProject     bool
	Priority         *string
	ClearPriority    bool
	Due              *int64
	ClearDue         bool
	Wait             *int64
	ClearWait        bool
	Scheduled        *int64
	ClearScheduled   bool
	Until            *int64
	ClearUntil       bool
	AddDepends       []string
	ClearDepends     bool
	Recur            *string
	ClearRecur       bool
	AddAssignees     []string
	RemoveAssignees  []string
	ClearAssignees   bool
	AddTags          []string
	RemoveTags       []string
	UDAs             map[string]string
	ClearUDAs        []string
}

type ReportInput struct {
	Name      string
	Query     query.Expr
	NoContext bool
	Limit     int
	Offset    int
}

type ReportResult struct {
	Tasks []task.Task
}

type auditAppenderLister interface {
	Append(storage.AuditLogEntry) error
	List(storage.AuditListOptions) ([]storage.AuditLogEntry, error)
}

type hookDeliveryEnqueuer interface {
	Enqueue(rows []storage.HookDelivery) error
	ListByHook(hookID string, status string, limit int, offset int) ([]storage.HookDelivery, error)
	GetByID(id string) (storage.HookDelivery, error)
	Requeue(id string, now int64) error
}

func NewService(opts ServiceOptions) (*Service, error) {
	if opts.Clock == nil {
		opts.Clock = RealClock{}
	}
	userRepo := storage.NewUserRepository(opts.Store.DB())
	workspaceRepo := storage.NewWorkspaceRepository(opts.Store.DB())
	memberRepo := storage.NewMemberRepository(opts.Store.DB())
	auditRepo := storage.NewAuditRepository(opts.Store.DB())
	rt := RuntimeContext{}
	if opts.Runtime != nil {
		rt = *opts.Runtime
	} else {
		var err error
		rt, err = ResolveRuntimeContext(opts.Store, userRepo, workspaceRepo, memberRepo, opts.ActorRef, opts.WorkspaceRef)
		if err != nil {
			return nil, err
		}
	}
	runtimeConfig := cloneStringMap(opts.RuntimeConfig)
	runtimeUDAs, err := udaDefinitionsFromConfig(runtimeConfig)
	if err != nil {
		return nil, err
	}
	svc := &Service{
		store:                     opts.Store,
		repo:                      storage.NewTaskRepository(opts.Store.DB()),
		projectRepo:               storage.NewProjectRepository(opts.Store.DB()),
		configRepo:                storage.NewConfigRepository(opts.Store.DB()),
		configDefRepo:             storage.NewConfigDefinitionRepository(opts.Store.DB()),
		userRepo:                  userRepo,
		workspaceRepo:             workspaceRepo,
		memberRepo:                memberRepo,
		auditRepo:                 auditRepo,
		tokenRepo:                 storage.NewTokenRepository(opts.Store.DB()),
		adminActingSessionRepo:    storage.NewAdminActingSessionRepository(opts.Store.DB()),
		contextRepo:               storage.NewContextRepository(opts.Store.DB()),
		udaRepo:                   storage.NewUDARepository(opts.Store.DB()),
		hookRepo:                  storage.NewHookRepository(opts.Store.DB()),
		hookDeliveryRepo:          storage.NewHookDeliveryRepository(opts.Store.DB()),
		notificationSinkRepo:      storage.NewNotificationSinkRepository(opts.Store.DB()),
		reminderRuleRepo:          storage.NewReminderRuleRepository(opts.Store.DB()),
		eventNotificationRuleRepo: storage.NewEventNotificationRuleRepository(opts.Store.DB()),
		notificationDeliveryRepo:  storage.NewNotificationDeliveryRepository(opts.Store.DB()),
		projectAutomationRuleRepo:     storage.NewProjectAutomationRuleRepository(opts.Store.DB()),
		projectAutomationDeliveryRepo: storage.NewProjectAutomationDeliveryRepository(opts.Store.DB()),
		extIDRepo:                 storage.NewExternalIDRepository(opts.Store.DB()),
		runtimeConfig:             runtimeConfig,
		runtimeOverrides:          cloneStringMap(opts.RuntimeOverrides),
		runtimeUDAs:               runtimeUDAs,
		runtime:                   rt,
		requestScope:              cloneRequestScope(opts.RequestScope),
		workspaceID:               rt.WorkspaceID,
		clock:                     opts.Clock,
		reports:                   report.DefaultRegistry(),
		disableContext:            opts.NoContext,
		sinkTestClient:            opts.SinkTestClient,
		sinkTestResolver:          opts.SinkTestResolver,
		tokenSecretKey:            append([]byte(nil), opts.TokenSecretKey...),
		requireTokenSecret:        opts.RequireTokenSecret,
	}
	if !opts.DisableScopeBootstrap {
		if err := svc.ensureBuiltinConfigDefinitions(rt.WorkspaceID); err != nil {
			return nil, err
		}
	}
	return svc, nil
}

func (s *Service) Clock() Clock {
	return s.clock
}

func normalizeOptionalText(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func optionalTextValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (s *Service) withStore(store *storage.Store) (*Service, error) {
	clone := *s
	clone.store = store
	clone.repo = storage.NewTaskRepository(store.DB())
	clone.projectRepo = storage.NewProjectRepository(store.DB())
	clone.configRepo = storage.NewConfigRepository(store.DB())
	clone.configDefRepo = storage.NewConfigDefinitionRepository(store.DB())
	clone.userRepo = storage.NewUserRepository(store.DB())
	clone.workspaceRepo = storage.NewWorkspaceRepository(store.DB())
	clone.memberRepo = storage.NewMemberRepository(store.DB())
	// Tests can inject a custom audit repo to force append failures; keep it
	// attached while production services get a tx-bound repository.
	if _, ok := s.auditRepo.(*storage.AuditRepository); ok || s.auditRepo == nil {
		clone.auditRepo = storage.NewAuditRepository(store.DB())
	}
	clone.tokenRepo = storage.NewTokenRepository(store.DB())
	// 测试可注入自定义 acting session 仓储；生产路径使用 tx 绑定的仓储。
	if prod, ok := s.adminActingSessionRepo.(*storage.AdminActingSessionRepository); ok || s.adminActingSessionRepo == nil {
		_ = prod
		clone.adminActingSessionRepo = storage.NewAdminActingSessionRepository(store.DB())
	} else {
		clone.adminActingSessionRepo = s.adminActingSessionRepo
	}
	clone.contextRepo = storage.NewContextRepository(store.DB())
	clone.udaRepo = storage.NewUDARepository(store.DB())
	clone.hookRepo = storage.NewHookRepository(store.DB())
	// Tests can inject a custom delivery repo to force enqueue failures; keep it
	// attached while production services get a tx-bound repository.
	if _, ok := s.hookDeliveryRepo.(*storage.HookDeliveryRepository); ok || s.hookDeliveryRepo == nil {
		clone.hookDeliveryRepo = storage.NewHookDeliveryRepository(store.DB())
	}
	clone.notificationSinkRepo = storage.NewNotificationSinkRepository(store.DB())
	clone.reminderRuleRepo = storage.NewReminderRuleRepository(store.DB())
	clone.eventNotificationRuleRepo = storage.NewEventNotificationRuleRepository(store.DB())
	clone.notificationDeliveryRepo = storage.NewNotificationDeliveryRepository(store.DB())
	clone.projectAutomationRuleRepo = storage.NewProjectAutomationRuleRepository(store.DB())
	clone.projectAutomationDeliveryRepo = storage.NewProjectAutomationDeliveryRepository(store.DB())
	// token secret key 在事务克隆中必须保留，否则 withAudit 内创建/解密 token 会失败。
	clone.tokenSecretKey = append([]byte(nil), s.tokenSecretKey...)
	clone.requireTokenSecret = s.requireTokenSecret
	return &clone, nil
}

func (s *Service) Runtime() RuntimeContext {
	return s.runtime
}

func (s *Service) Projects(includeArchived bool) ([]string, error) {
	projects, err := s.ListProjects(includeArchived)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(projects))
	for _, project := range projects {
		out = append(out, project.Slug)
	}
	return out, nil
}

func (s *Service) Tags() ([]string, error) {
	tasks, err := s.List(ListInput{ReportMode: true})
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, tsk := range tasks {
		for _, tag := range tsk.Tags {
			seen[tag] = true
		}
	}
	out := make([]string, 0, len(seen))
	for tag := range seen {
		out = append(out, tag)
	}
	sort.Strings(out)
	return out, nil
}

func (s *Service) UUIDs(input ListInput) ([]string, error) {
	tasks, err := s.List(input)
	if err != nil {
		return nil, err
	}
	uuids := make([]string, len(tasks))
	for i, tsk := range tasks {
		uuids[i] = tsk.UUID
	}
	return uuids, nil
}

func (s *Service) IDs(input ListInput) ([]int, error) {
	filtered, err := s.List(input)
	if err != nil {
		return nil, err
	}
	if len(filtered) == 0 {
		return nil, nil
	}
	matched := make(map[string]struct{}, len(filtered))
	for _, tsk := range filtered {
		matched[tsk.UUID] = struct{}{}
	}
	workingSet, err := s.defaultWorkingSet()
	if err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(filtered))
	for i, tsk := range workingSet {
		if _, ok := matched[tsk.UUID]; ok {
			ids = append(ids, i+1)
		}
	}
	return ids, nil
}

// WorkingSetIDs returns one ID per task in the same order as the input slice.
// IDs are 1-based positions in the default working set; tasks not present in
// the working set (e.g. completed/deleted/until-expired) get 0.
func (s *Service) WorkingSetIDs(tasks []task.Task) ([]int, error) {
	if len(tasks) == 0 {
		return nil, nil
	}
	workingSet, err := s.defaultWorkingSet()
	if err != nil {
		return nil, err
	}
	index := make(map[string]int, len(workingSet))
	for i, tsk := range workingSet {
		index[tsk.UUID] = i + 1
	}
	ids := make([]int, len(tasks))
	for i, tsk := range tasks {
		ids[i] = index[tsk.UUID]
	}
	return ids, nil
}

func (s *Service) Add(input AddInput) (task.Task, error) {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return task.Task{}, err
	}
	var created task.Task
	err := s.withAuditAndEvents(func(tx *Service) (*AuditEntry, []HookEvent, error) {
		var (
			err    error
			change projectChange
		)
		created, change, err = tx.addLocked(input)
		if err != nil {
			return nil, nil, err
		}
		event := buildTaskHookEvent("task.created", created, tx.runtime, tx.clock.Unix())
		blocked := tx.detectBlockedEventsAfterAdd(created)
		events := []HookEvent{event}
		events = append(events, blocked...)
		entry := taskAuditEntry("task.add", created.UUID, change)
		return &entry, events, nil
	})
	return created, err
}

func (s *Service) AddWithAnnotations(input AddInput, annotations []string) (task.Task, error) {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return task.Task{}, err
	}
	if len(annotations) == 0 {
		return s.Add(input)
	}
	var created task.Task
	err := s.withAuditEntriesAndEvents(func(tx *Service) ([]AuditEntry, []HookEvent, error) {
		var (
			err    error
			change projectChange
		)
		created, change, err = tx.addLocked(input)
		if err != nil {
			return nil, nil, err
		}
		entries := []AuditEntry{taskAuditEntry("task.add", created.UUID, change)}
		for _, annotation := range annotations {
			targetTask, annChange, err := tx.annotateLocked(created.UUID, annotation)
			if err != nil {
				return nil, nil, err
			}
			entries = append(entries, taskAuditEntry("task.annotate", targetTask.UUID, annChange))
			_ = targetTask // annotation events not required in M8 v1
		}
		created, err = tx.repo.GetByUUID(tx.workspaceID, created.UUID)
		if err != nil {
			return nil, nil, err
		}
		event := buildTaskHookEvent("task.created", created, tx.runtime, tx.clock.Unix())
		blocked := tx.detectBlockedEventsAfterAdd(created)
		events := []HookEvent{event}
		events = append(events, blocked...)
		return entries, events, nil
	})
	return created, err
}

func (s *Service) addLocked(input AddInput) (task.Task, projectChange, error) {
	now := s.clock.Unix()
	if input.Recur != nil {
		return s.createRecurringParent(input, now)
	}
	assignees, err := s.resolveAssigneeRefs(input.Assignees)
	if err != nil {
		return task.Task{}, projectChange{}, err
	}
	depends, err := s.resolveDependencyTargets(input.Depends)
	if err != nil {
		return task.Task{}, projectChange{}, err
	}
	parentUUID, parentProject, err := s.resolveParentForAdd(input.Parent, input.Project)
	if err != nil {
		return task.Task{}, projectChange{}, err
	}
	tsk := task.Task{
		UUID: uuid.NewString(), WorkspaceID: s.workspaceID, Title: strings.TrimSpace(input.Title), Description: normalizeOptionalText(input.Description),
		Status: task.StatusPending, Entry: now, Modified: now,
		Due: input.Due, Priority: input.Priority, Tags: input.Tags,
		Assignees: assignees, Depends: depends,
		Wait: input.Wait, Scheduled: input.Scheduled, Until: input.Until, Recur: input.Recur,
		Parent: parentUUID,
	}
	projChange, err := s.applyProjectBinding(&tsk, parentProject)
	if err != nil {
		return task.Task{}, projectChange{}, err
	}
	udas, err := s.normalizeUDAModifications(nil, input.UDAs, nil, false)
	if err != nil {
		return task.Task{}, projChange, err
	}
	tsk.UDAs = udas
	if input.Wait != nil && *input.Wait > now {
		tsk.Status = task.StatusWaiting
	}
	created, err := s.repo.Create(tsk)
	return created, projChange, err
}

func (s *Service) List(input ListInput) ([]task.Task, error) {
	if err := s.Require(PermissionTaskRead); err != nil {
		return nil, err
	}
	if input.Target != nil {
		tsk, err := s.ResolveTarget(*input.Target)
		if err != nil {
			return nil, err
		}
		return []task.Task{tsk}, nil
	}
	if err := s.refreshAutomaticState(); err != nil {
		return nil, err
	}
	status := input.Status
	if status == "" && input.Query == nil && !input.ReportMode {
		status = task.StatusPending
	}
	contextExpr, err := s.activeContextFilter(input.NoContext)
	if err != nil {
		return nil, err
	}
	resolvedInputQuery, err := s.resolveTaskQueryPredicates(input.Query)
	if err != nil {
		return nil, err
	}
	queryExpr := query.And(s.projectScopeExpr(), query.And(contextExpr, resolvedInputQuery))
	udaDefs, err := s.udaDefinitionTypes()
	if err != nil {
		return nil, err
	}
	tasks, err := s.repo.List(s.workspaceID, storage.ListOptions{
		Status:         status,
		Sort:           input.Sort,
		Query:          queryExpr,
		NowUnix:        s.clock.Unix(),
		UDADefinitions: udaDefs,
		Limit:          input.Limit,
		Offset:         input.Offset,
		Dialect:        s.store.Dialect(),
	})
	if err != nil {
		return nil, mapProjectQueryCompileError(err)
	}
	if !input.ReportMode {
		tasks = filterExpiredUntil(tasks, s.clock.Unix())
	}
	if err := s.enrichTaskLinkActors(tasks); err != nil {
		return nil, err
	}
	for _, tsk := range tasks {
		if err := s.validateTaskProjectInvariant(tsk); err != nil {
			return nil, err
		}
	}
	return tasks, nil
}

func (s *Service) ListReport(name string, input ListInput) ([]task.Task, error) {
	if input.Target != nil {
		return s.List(input)
	}
	result, err := s.RunReport(ReportInput{Name: name, Query: input.Query, NoContext: input.NoContext, Limit: input.Limit, Offset: input.Offset})
	if err != nil {
		return nil, err
	}
	return result.Tasks, nil
}

func (s *Service) Info(target string) (task.Task, error) {
	if err := s.Require(PermissionTaskRead); err != nil {
		return task.Task{}, err
	}
	tsk, err := s.resolveTargetForRead(target)
	if err != nil {
		return task.Task{}, err
	}
	if err := s.enrichTaskLinkActors([]task.Task{tsk}); err != nil {
		return task.Task{}, err
	}
	return tsk, nil
}

// ListChildren 列出指定任务的直接子任务。
// includeClosed=false 时过滤 completed/deleted（spec §8.3）。
// 不复用 List + parent:<uuid> query，因为带 query 后默认 pending 状态过滤失效（service.go List 默认 pending 仅在 Query==nil 时生效）。
func (s *Service) ListChildren(target string, includeClosed bool) ([]task.Task, error) {
	if err := s.Require(PermissionTaskRead); err != nil {
		return nil, err
	}
	parent, err := s.ResolveProtocolTarget(target)
	if err != nil {
		return nil, err
	}
	children, err := s.repo.Children(s.workspaceID, parent.UUID)
	if err != nil {
		return nil, err
	}
	if includeClosed {
		return children, nil
	}
	out := children[:0]
	for _, c := range children {
		if c.Status == task.StatusCompleted || c.Status == task.StatusDeleted {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

func (s *Service) ResolveTarget(target string) (task.Task, error) {
	tsk, err := s.resolveTargetForRead(target)
	if err != nil {
		return task.Task{}, err
	}
	if err := s.enrichTaskLinkActors([]task.Task{tsk}); err != nil {
		return task.Task{}, err
	}
	return tsk, nil
}

func (s *Service) enrichTaskLinkActors(tasks []task.Task) error {
	userIDs := make([]string, 0)
	for _, tsk := range tasks {
		for _, link := range tsk.Links {
			if link.CreatedBy.Type == auth.TokenTypeTenantAccess {
				continue
			}
			if link.CreatedBy.User != nil && link.CreatedBy.User.ID != "" {
				userIDs = append(userIDs, link.CreatedBy.User.ID)
				continue
			}
			if link.CreatedBy.ID != "" {
				userIDs = append(userIDs, link.CreatedBy.ID)
			}
		}
	}
	userInfos, err := s.resolveUserInfos(userIDs)
	if err != nil {
		return err
	}
	for taskIndex := range tasks {
		for linkIndex := range tasks[taskIndex].Links {
			link := &tasks[taskIndex].Links[linkIndex]
			if link.CreatedBy.Type == auth.TokenTypeTenantAccess {
				continue
			}
			userID := link.CreatedBy.ID
			if link.CreatedBy.User != nil && link.CreatedBy.User.ID != "" {
				userID = link.CreatedBy.User.ID
			}
			if ui := userInfos[userID]; ui.ID != "" {
				link.CreatedBy = task.ActorInfo{Type: actorTypeUser, ID: ui.ID, Name: ui.Name, User: &ui}
			}
		}
	}
	return nil
}

func (s *Service) Modify(target string, input ModifyInput) error {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return err
	}
	return s.withAuditAndEvents(func(tx *Service) (*AuditEntry, []HookEvent, error) {
		before, err := tx.resolveTargetForWrite(target)
		if err != nil {
			return nil, nil, err
		}
		modified, change, err := tx.modifyLocked(target, input)
		if err != nil {
			return nil, nil, err
		}
		now := tx.clock.Unix()
		diff := diffTaskChanges(before, modified)
		diff = tx.hydrateAssigneeDiff(diff, before.Assignees, modified.Assignees)
		fineGrained := buildFineGrainedEvents(diff, modified, tx.runtime, now)
		blocked := tx.detectBlockedEventsAfterModify(before, modified)
		events := []HookEvent{buildTaskHookEvent("task.modified", modified, tx.runtime, now)}
		events = append(events, fineGrained...)
		events = append(events, blocked...)
		entry := taskModifyAuditEntry(modified.UUID, change, diff)
		return &entry, events, nil
	})
}

func (s *Service) modifyLocked(target string, input ModifyInput) (task.Task, projectChange, error) {
	tsk, err := s.resolveTargetForWrite(target)
	if err != nil {
		return task.Task{}, projectChange{}, err
	}
	now := s.clock.Unix()
	change := projectChangeForTask(tsk)
	if input.Title != nil {
		tsk.Title = strings.TrimSpace(*input.Title)
	}
	if input.Description != nil {
		tsk.Description = normalizeOptionalText(input.Description)
	}
	if input.ClearDescription {
		tsk.Description = nil
	}
	if input.Project != nil {
		change, err = s.applyProjectBinding(&tsk, input.Project)
		if err != nil {
			return task.Task{}, projectChange{}, err
		}
	}
	if input.ClearProject {
		empty := ""
		change, err = s.applyProjectBinding(&tsk, &empty)
		if err != nil {
			return task.Task{}, projectChange{}, err
		}
	}
	if input.Priority != nil {
		tsk.Priority = input.Priority
	}
	if input.ClearPriority {
		tsk.Priority = nil
	}
	if input.Due != nil {
		tsk.Due = input.Due
	}
	if input.ClearDue {
		tsk.Due = nil
	}
	if input.Wait != nil {
		tsk.Wait = input.Wait
		if *input.Wait > now && tsk.Status == task.StatusPending {
			tsk.Status = task.StatusWaiting
		}
	}
	if input.ClearWait {
		tsk.Wait = nil
		if tsk.Status == task.StatusWaiting {
			tsk.Status = task.StatusPending
		}
	}
	if input.Scheduled != nil {
		tsk.Scheduled = input.Scheduled
	}
	if input.ClearScheduled {
		tsk.Scheduled = nil
	}
	if input.Until != nil {
		tsk.Until = input.Until
	}
	if input.ClearUntil {
		tsk.Until = nil
	}
	if input.Recur != nil {
		tsk.Recur = input.Recur
	}
	if input.ClearRecur {
		tsk.Recur = nil
	}
	if input.ClearDepends {
		tsk.Depends = nil
	}
	if len(input.AddDepends) > 0 {
		depends, err := s.resolveDependencyTargets(input.AddDepends)
		if err != nil {
			return task.Task{}, projectChange{}, err
		}
		depSet := map[string]bool{}
		for _, d := range tsk.Depends {
			depSet[d] = true
		}
		for _, depUUID := range depends {
			if !depSet[depUUID] {
				tsk.Depends = append(tsk.Depends, depUUID)
				depSet[depUUID] = true
			}
		}
		if err := s.validateDependencyCycles(tsk.UUID, tsk.Depends); err != nil {
			return task.Task{}, projectChange{}, err
		}
	}
	if input.ClearAssignees || len(input.AddAssignees) > 0 || len(input.RemoveAssignees) > 0 {
		assignees, err := s.applyAssigneeModifications(tsk.Assignees, input.AddAssignees, input.RemoveAssignees, input.ClearAssignees)
		if err != nil {
			return task.Task{}, projectChange{}, err
		}
		tsk.Assignees = assignees
	}
	if len(input.UDAs) > 0 || len(input.ClearUDAs) > 0 {
		udas, err := s.normalizeUDAModifications(tsk.UDAs, input.UDAs, input.ClearUDAs, false)
		if err != nil {
			return task.Task{}, projectChange{}, err
		}
		tsk.UDAs = udas
	}
	// Handle tags
	tagSet := map[string]bool{}
	for _, tag := range tsk.Tags {
		tagSet[tag] = true
	}
	for _, tag := range input.AddTags {
		tagSet[tag] = true
	}
	for _, tag := range input.RemoveTags {
		delete(tagSet, tag)
	}
	newTags := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		newTags = append(newTags, tag)
	}
	tsk.Tags = newTags
	tsk.Modified = now
	if err := s.repo.Update(tsk); err != nil {
		return task.Task{}, projectChange{}, err
	}
	return tsk, change, nil
}

func (s *Service) Done(target string) error {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return err
	}
	return s.withAuditEntriesAndEvents(func(tx *Service) ([]AuditEntry, []HookEvent, error) {
		beforeTasks, err := tx.repo.List(tx.workspaceID, storage.ListOptions{
			NowUnix: tx.clock.Unix(),
			Query:   tx.projectScopeExpr(),
			Dialect: tx.store.Dialect(),
		})
		if err != nil {
			return nil, nil, err
		}
		beforeBlocked, _ := buildDependencyState(beforeTasks, tx.clock.Unix())
		doneTask, change, extraEntries, err := tx.doneLocked(target)
		if err != nil {
			return nil, nil, err
		}
		now := tx.clock.Unix()
		events := []HookEvent{buildTaskHookEvent("task.completed", doneTask, tx.runtime, now)}
		unblocked, err := tx.taskUnblockedEventsAfterDone(beforeBlocked, doneTask, now)
		if err != nil {
			return nil, nil, err
		}
		events = append(events, unblocked...)
		entries := []AuditEntry{taskAuditEntry("task.done", doneTask.UUID, change)}
		entries = append(entries, extraEntries...)
		return entries, events, nil
	})
}

func (s *Service) taskUnblockedEventsAfterDone(beforeBlocked map[string]bool, doneTask task.Task, now int64) ([]HookEvent, error) {
	afterTasks, err := s.repo.List(s.workspaceID, storage.ListOptions{
		NowUnix: now,
		Query:   s.projectScopeExpr(),
		Dialect: s.store.Dialect(),
	})
	if err != nil {
		return nil, err
	}
	afterBlocked, _ := buildDependencyState(afterTasks, now)
	events := []HookEvent{}
	for _, candidate := range afterTasks {
		if !taskDependsOn(candidate, doneTask.UUID) {
			continue
		}
		if !beforeBlocked[candidate.UUID] || afterBlocked[candidate.UUID] {
			continue
		}
		if !isDependencyEligible(candidate, now) {
			continue
		}
		events = append(events, buildTaskUnblockedHookEvent(candidate, doneTask, s.runtime, now))
	}
	return events, nil
}

func taskDependsOn(tsk task.Task, depUUID string) bool {
	for _, current := range tsk.Depends {
		if current == depUUID {
			return true
		}
	}
	return false
}

func (s *Service) doneLocked(target string) (task.Task, projectChange, []AuditEntry, error) {
	tsk, err := s.resolveTargetForWrite(target)
	if err != nil {
		return task.Task{}, projectChange{}, nil, err
	}
	change := projectChangeForTask(tsk)
	tsk.Complete(s.clock.Unix())
	if err := s.repo.Update(tsk); err != nil {
		return task.Task{}, projectChange{}, nil, err
	}
	if tsk.Parent != nil {
		parent, err := s.repo.GetByUUID(s.workspaceID, *tsk.Parent)
		if err != nil {
			return task.Task{}, projectChange{}, nil, err
		}
		if parent.Status != task.StatusRecurring {
			return tsk, change, nil, nil
		}
		_, warningEntry, err := s.createNextRecurringChild(parent, &tsk, s.clock.Unix())
		if err != nil {
			return task.Task{}, projectChange{}, nil, err
		}
		if warningEntry != nil {
			return tsk, change, []AuditEntry{*warningEntry}, nil
		}
	}
	return tsk, change, nil, nil
}

func (s *Service) Delete(target string) error {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return err
	}
	return s.withAuditAndEvents(func(tx *Service) (*AuditEntry, []HookEvent, error) {
		deletedTask, change, err := tx.deleteLocked(target)
		if err != nil {
			return nil, nil, err
		}
		event := buildTaskHookEvent("task.deleted", deletedTask, tx.runtime, tx.clock.Unix())
		entry := taskAuditEntry("task.delete", deletedTask.UUID, change)
		return &entry, []HookEvent{event}, nil
	})
}

func (s *Service) deleteLocked(target string) (task.Task, projectChange, error) {
	tsk, err := s.resolveTargetForWrite(target)
	if err != nil {
		return task.Task{}, projectChange{}, err
	}
	change := projectChangeForTask(tsk)
	if tsk.Status == task.StatusCompleted || tsk.Status == task.StatusDeleted {
		return task.Task{}, projectChange{}, fmt.Errorf("cannot delete %s task", tsk.Status)
	}
	tsk.Delete(s.clock.Unix())
	if err := s.repo.Update(tsk); err != nil {
		return task.Task{}, projectChange{}, err
	}
	return tsk, change, nil
}

func (s *Service) Start(target string) error {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return err
	}
	return s.withAuditAndEvents(func(tx *Service) (*AuditEntry, []HookEvent, error) {
		startedTask, change, err := tx.startLocked(target)
		if err != nil {
			return nil, nil, err
		}
		event := buildTaskHookEvent("task.started", startedTask, tx.runtime, tx.clock.Unix())
		entry := taskAuditEntry("task.start", startedTask.UUID, change)
		return &entry, []HookEvent{event}, nil
	})
}

func (s *Service) startLocked(target string) (task.Task, projectChange, error) {
	tsk, err := s.resolveTargetForWrite(target)
	if err != nil {
		return task.Task{}, projectChange{}, err
	}
	change := projectChangeForTask(tsk)
	if tsk.Status == task.StatusCompleted || tsk.Status == task.StatusDeleted || tsk.Status == task.StatusRecurring {
		return task.Task{}, projectChange{}, fmt.Errorf("cannot start %s task", tsk.Status)
	}
	if tsk.Start != nil {
		return task.Task{}, projectChange{}, fmt.Errorf("task is already active")
	}
	tsk.StartTask(s.clock.Unix())
	if err := s.repo.Update(tsk); err != nil {
		return task.Task{}, projectChange{}, err
	}
	return tsk, change, nil
}

func (s *Service) Stop(target string) error {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return err
	}
	return s.withAuditAndEvents(func(tx *Service) (*AuditEntry, []HookEvent, error) {
		stoppedTask, change, err := tx.stopLocked(target)
		if err != nil {
			return nil, nil, err
		}
		event := buildTaskHookEvent("task.stopped", stoppedTask, tx.runtime, tx.clock.Unix())
		entry := taskAuditEntry("task.stop", stoppedTask.UUID, change)
		return &entry, []HookEvent{event}, nil
	})
}

func (s *Service) stopLocked(target string) (task.Task, projectChange, error) {
	tsk, err := s.resolveTargetForWrite(target)
	if err != nil {
		return task.Task{}, projectChange{}, err
	}
	change := projectChangeForTask(tsk)
	if tsk.Status == task.StatusCompleted || tsk.Status == task.StatusDeleted {
		return task.Task{}, projectChange{}, fmt.Errorf("cannot stop %s task", tsk.Status)
	}
	tsk.StopTask(s.clock.Unix())
	if err := s.repo.Update(tsk); err != nil {
		return task.Task{}, projectChange{}, err
	}
	return tsk, change, nil
}

// Reopen 把已完成（completed）任务恢复为 pending。
// 仅允许 completed 状态，其余状态报错。deleted 不在本轮支持范围。
func (s *Service) Reopen(target string) error {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return err
	}
	return s.withAuditAndEvents(func(tx *Service) (*AuditEntry, []HookEvent, error) {
		reopenedTask, change, err := tx.reopenLocked(target)
		if err != nil {
			return nil, nil, err
		}
		event := buildTaskHookEvent("task.reopened", reopenedTask, tx.runtime, tx.clock.Unix())
		entry := taskAuditEntry("task.reopen", reopenedTask.UUID, change)
		return &entry, []HookEvent{event}, nil
	})
}

func (s *Service) reopenLocked(target string) (task.Task, projectChange, error) {
	tsk, err := s.resolveTargetForWrite(target)
	if err != nil {
		return task.Task{}, projectChange{}, err
	}
	change := projectChangeForTask(tsk)
	if tsk.Status != task.StatusCompleted {
		return task.Task{}, projectChange{}, fmt.Errorf("cannot reopen %s task", tsk.Status)
	}
	tsk.Reopen(s.clock.Unix())
	if err := s.repo.Update(tsk); err != nil {
		return task.Task{}, projectChange{}, err
	}
	return tsk, change, nil
}

// ListAnnotations 按 entry 倒序分页返回某任务的注解，total 为该任务注解总数。
// 读操作：只需 task:read 权限。
func (s *Service) ListAnnotations(target string, offset, limit int) ([]task.Annotation, int, error) {
	if err := s.Require(PermissionTaskRead); err != nil {
		return nil, 0, err
	}
	tsk, err := s.resolveTargetForRead(target)
	if err != nil {
		return nil, 0, err
	}
	return s.repo.ListAnnotations(s.workspaceID, tsk.UUID, offset, limit)
}

// ResolveTaskRefs 把一组任务 UUID 解析为带标题和 task_slug 的轻量引用，
// 用于 depends_info/parent_info 等可读展示。找不到或已删除的 UUID 被忽略。
func (s *Service) ResolveTaskRefs(uuids []string) ([]task.JSONTaskRef, error) {
	if len(uuids) == 0 {
		return nil, nil
	}
	tasks, err := s.repo.ListByUUIDs(s.workspaceID, uuids)
	if err != nil {
		return nil, err
	}
	return tasksToRefs(tasks), nil
}

// ResolveDependents 返回依赖指定任务的任务（反向依赖），即被 taskUUID 阻塞的任务。
// 用于任务详情页「阻塞了」反向关系展示。
func (s *Service) ResolveDependents(taskUUID string) ([]task.JSONTaskRef, error) {
	tasks, err := s.repo.ListDependents(s.workspaceID, taskUUID)
	if err != nil {
		return nil, err
	}
	return tasksToRefs(tasks), nil
}

// tasksToRefs 把领域任务列表转为轻量 JSONTaskRef（带标题 + task_slug）。
func tasksToRefs(tasks []task.Task) []task.JSONTaskRef {
	out := make([]task.JSONTaskRef, 0, len(tasks))
	for _, tsk := range tasks {
		ref := task.JSONTaskRef{UUID: tsk.UUID, Title: tsk.Title}
		if slug := taskSlugOf(tsk); slug != "" {
			s := slug
			ref.TaskSlug = &s
		}
		out = append(out, ref)
	}
	return out
}

// taskSlugOf 计算任务的稳定短标识（与 task.ToJSON 一致）。
func taskSlugOf(tsk task.Task) string {
	if tsk.Project == nil || *tsk.Project == "" || tsk.ProjectSeq == nil {
		return ""
	}
	return fmt.Sprintf("%s-%d", *tsk.Project, *tsk.ProjectSeq)
}

func (s *Service) Annotate(target, description string) error {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return err
	}
	return s.withAuditAndEvents(func(tx *Service) (*AuditEntry, []HookEvent, error) {
		annotatedTask, change, err := tx.annotateLocked(target, description)
		if err != nil {
			return nil, nil, err
		}
		event := buildTaskHookEvent("task.modified", annotatedTask, tx.runtime, tx.clock.Unix())
		entry := taskAuditEntry("task.annotate", annotatedTask.UUID, change)
		return &entry, []HookEvent{event}, nil
	})
}

func (s *Service) annotateLocked(target, description string) (task.Task, projectChange, error) {
	description = strings.TrimSpace(description)
	if description == "" {
		return task.Task{}, projectChange{}, fmt.Errorf("annotation description is required")
	}

	now := s.clock.Unix()
	for attempts := 0; attempts < 3; attempts++ {
		tsk, err := s.resolveTargetForWrite(target)
		if err != nil {
			return task.Task{}, projectChange{}, err
		}
		change := projectChangeForTask(tsk)
		entry := now
		for {
			conflict := false
			for _, annotation := range tsk.Annotations {
				if annotation.Entry == entry && annotation.Description == description {
					conflict = true
					break
				}
			}
			if !conflict {
				break
			}
			entry++
		}
		if err := s.repo.AddAnnotation(s.workspaceID, tsk.UUID, task.Annotation{ID: uuid.NewString(), Entry: entry, Description: description}, entry); err != nil {
			if storage.IsUniqueConstraintError(err) {
				now = entry + 1
				continue
			}
			return task.Task{}, projectChange{}, err
		}
		updated, err := s.repo.GetByUUID(s.workspaceID, tsk.UUID)
		if err != nil {
			return task.Task{}, projectChange{}, err
		}
		return updated, change, nil
	}
	return task.Task{}, projectChange{}, fmt.Errorf("annotation conflict could not be resolved")
}

func (s *Service) Denotate(target string, annotationID string) error {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return err
	}
	return s.withAuditAndEvents(func(tx *Service) (*AuditEntry, []HookEvent, error) {
		denotatedTask, change, err := tx.denotateLocked(target, annotationID)
		if err != nil {
			return nil, nil, err
		}
		event := buildTaskHookEvent("task.modified", denotatedTask, tx.runtime, tx.clock.Unix())
		entry := taskAuditEntry("task.denotate", denotatedTask.UUID, change)
		return &entry, []HookEvent{event}, nil
	})
}

func (s *Service) UpdateAnnotation(target, annotationID, description string) error {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return err
	}
	return s.withAuditAndEvents(func(tx *Service) (*AuditEntry, []HookEvent, error) {
		updatedTask, change, err := tx.updateAnnotationLocked(target, annotationID, description)
		if err != nil {
			return nil, nil, err
		}
		event := buildTaskHookEvent("task.modified", updatedTask, tx.runtime, tx.clock.Unix())
		entry := taskAuditEntry("task.annotation.update", updatedTask.UUID, change)
		return &entry, []HookEvent{event}, nil
	})
}

func (s *Service) updateAnnotationLocked(target, annotationID, description string) (task.Task, projectChange, error) {
	annotationID = strings.TrimSpace(annotationID)
	if annotationID == "" {
		return task.Task{}, projectChange{}, fmt.Errorf("annotation id is required")
	}
	description = strings.TrimSpace(description)
	if description == "" {
		return task.Task{}, projectChange{}, fmt.Errorf("annotation description is required")
	}
	tsk, err := s.resolveTargetForWrite(target)
	if err != nil {
		return task.Task{}, projectChange{}, err
	}
	change := projectChangeForTask(tsk)
	modified := s.clock.Unix()
	if err := s.repo.UpdateAnnotation(s.workspaceID, tsk.UUID, annotationID, description, modified); err != nil {
		if err == storage.ErrNotFound {
			return task.Task{}, projectChange{}, fmt.Errorf("annotation %s not found", annotationID)
		}
		return task.Task{}, projectChange{}, err
	}
	updated, err := s.repo.GetByUUID(s.workspaceID, tsk.UUID)
	if err != nil {
		return task.Task{}, projectChange{}, err
	}
	return updated, change, nil
}

func (s *Service) denotateLocked(target string, annotationID string) (task.Task, projectChange, error) {
	annotationID = strings.TrimSpace(annotationID)
	if annotationID == "" {
		return task.Task{}, projectChange{}, fmt.Errorf("annotation id is required")
	}
	tsk, err := s.resolveTargetForWrite(target)
	if err != nil {
		return task.Task{}, projectChange{}, err
	}
	change := projectChangeForTask(tsk)
	modified := s.clock.Unix()
	if err := s.repo.DeleteAnnotation(s.workspaceID, tsk.UUID, annotationID, modified); err != nil {
		if err == storage.ErrNotFound {
			return task.Task{}, projectChange{}, fmt.Errorf("annotation %s not found", annotationID)
		}
		return task.Task{}, projectChange{}, err
	}
	updated, err := s.repo.GetByUUID(s.workspaceID, tsk.UUID)
	if err != nil {
		return task.Task{}, projectChange{}, err
	}
	return updated, change, nil
}

func (s *Service) AppendDescription(target, suffix string) error {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return err
	}
	return s.withAuditAndEvents(func(tx *Service) (*AuditEntry, []HookEvent, error) {
		appendedTask, change, err := tx.appendDescriptionLocked(target, suffix)
		if err != nil {
			return nil, nil, err
		}
		event := buildTaskHookEvent("task.modified", appendedTask, tx.runtime, tx.clock.Unix())
		entry := taskAuditEntry("task.append", appendedTask.UUID, change)
		return &entry, []HookEvent{event}, nil
	})
}

func (s *Service) appendDescriptionLocked(target, suffix string) (task.Task, projectChange, error) {
	tsk, err := s.resolveTargetForWrite(target)
	if err != nil {
		return task.Task{}, projectChange{}, err
	}
	change := projectChangeForTask(tsk)
	suffix = strings.TrimSpace(suffix)
	if suffix == "" {
		return task.Task{}, projectChange{}, fmt.Errorf("title text is required")
	}
	tsk.Title = strings.TrimSpace(tsk.Title + " " + suffix)
	tsk.Modified = s.clock.Unix()
	if err := s.repo.Update(tsk); err != nil {
		return task.Task{}, projectChange{}, err
	}
	return tsk, change, nil
}

func (s *Service) PrependDescription(target, prefix string) error {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return err
	}
	return s.withAuditAndEvents(func(tx *Service) (*AuditEntry, []HookEvent, error) {
		prependedTask, change, err := tx.prependDescriptionLocked(target, prefix)
		if err != nil {
			return nil, nil, err
		}
		event := buildTaskHookEvent("task.modified", prependedTask, tx.runtime, tx.clock.Unix())
		entry := taskAuditEntry("task.prepend", prependedTask.UUID, change)
		return &entry, []HookEvent{event}, nil
	})
}

func (s *Service) prependDescriptionLocked(target, prefix string) (task.Task, projectChange, error) {
	tsk, err := s.resolveTargetForWrite(target)
	if err != nil {
		return task.Task{}, projectChange{}, err
	}
	change := projectChangeForTask(tsk)
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return task.Task{}, projectChange{}, fmt.Errorf("title text is required")
	}
	tsk.Title = strings.TrimSpace(prefix + " " + tsk.Title)
	tsk.Modified = s.clock.Unix()
	if err := s.repo.Update(tsk); err != nil {
		return task.Task{}, projectChange{}, err
	}
	return tsk, change, nil
}

func (s *Service) ReplaceEditableTask(target string, edited task.Task) error {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return err
	}
	return s.withAuditAndEvents(func(tx *Service) (*AuditEntry, []HookEvent, error) {
		replacedTask, change, err := tx.replaceEditableTaskLocked(target, edited)
		if err != nil {
			return nil, nil, err
		}
		event := buildTaskHookEvent("task.modified", replacedTask, tx.runtime, tx.clock.Unix())
		entry := taskAuditEntry("task.edit", replacedTask.UUID, change)
		return &entry, []HookEvent{event}, nil
	})
}

func (s *Service) replaceEditableTaskLocked(target string, edited task.Task) (task.Task, projectChange, error) {
	original, err := s.resolveTargetForWrite(target)
	if err != nil {
		return task.Task{}, projectChange{}, err
	}
	edited.UUID = original.UUID
	edited.WorkspaceID = original.WorkspaceID
	edited.Entry = original.Entry
	edited.Modified = s.clock.Unix()
	change, err := s.applyProjectBindingFrom(&edited, edited.Project, projectBindingFromTask(original))
	if err != nil {
		return task.Task{}, projectChange{}, err
	}
	if edited.Wait != nil && *edited.Wait > s.clock.Unix() && edited.Status == task.StatusPending {
		edited.Status = task.StatusWaiting
	}
	if edited.Wait == nil && edited.Status == task.StatusWaiting {
		edited.Status = task.StatusPending
	}
	if err := edited.Validate(); err != nil {
		return task.Task{}, projectChange{}, err
	}
	if err := s.repo.Update(edited); err != nil {
		return task.Task{}, projectChange{}, err
	}
	return edited, change, nil
}

func (s *Service) Export() ([]task.Task, error) {
	return s.ExportWithInput(ExportInput{})
}

func (s *Service) ExportWithInput(input ExportInput) ([]task.Task, error) {
	if err := s.Require(PermissionTaskRead); err != nil {
		return nil, err
	}
	queryExpr := s.projectScopeExpr()
	if input.ProjectID != nil && strings.TrimSpace(*input.ProjectID) != "" {
		queryExpr = query.And(queryExpr, query.Predicate{Attribute: query.AttrProjectID, Operator: query.OpEqual, Value: query.StringValue(strings.TrimSpace(*input.ProjectID))})
	}
	tasks, err := s.repo.List(s.workspaceID, storage.ListOptions{Query: queryExpr, Dialect: s.store.Dialect()})
	if err != nil {
		return nil, err
	}
	for _, tsk := range tasks {
		if err := s.validateTaskProjectInvariant(tsk); err != nil {
			return nil, err
		}
	}
	return tasks, nil
}

func (s *Service) Import(tasks []task.JSONTask) (int, error) {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return 0, err
	}
	count := 0
	err := s.withAuditAndEvents(func(tx *Service) (*AuditEntry, []HookEvent, error) {
		imported, err := tx.importLocked(tasks)
		if err != nil {
			return nil, nil, err
		}
		count = imported
		entry := AuditEntry{
			Action:     "task.import",
			TargetType: "task",
			Payload: map[string]any{
				"count": imported,
			},
		}
		return &entry, nil, nil
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Service) importLocked(tasks []task.JSONTask) (int, error) {
	count := 0
	for _, dto := range tasks {
		if err := s.importOneLocked(dto); err != nil {
			return 0, err
		}
		count++
	}
	return count, nil
}

func (s *Service) importOneLocked(dto task.JSONTask) error {
	tsk, err := task.FromJSONStrict(dto)
	if err != nil {
		return err
	}
	tsk.WorkspaceID = s.workspaceID
	if tsk.UUID == "" {
		tsk.UUID = uuid.NewString()
	}
	existing, err := s.repo.GetByUUID(s.workspaceID, tsk.UUID)
	if err == storage.ErrNotFound {
		if err := s.normalizeTaskProjectFields(&tsk); err != nil {
			return err
		}
		if tsk.Status == "" {
			tsk.Status = task.StatusPending
		}
		if tsk.Entry == 0 {
			tsk.Entry = s.clock.Unix()
		}
		if tsk.Modified == 0 {
			tsk.Modified = s.clock.Unix()
		}
		ensureAnnotationIDs(tsk.Annotations)
		if tsk.UDAs != nil {
			normalized, err := s.normalizeImportedUDAs(tsk.UDAs)
			if err != nil {
				return err
			}
			tsk.UDAs = normalized
		}
		if tsk.Assignees != nil {
			resolvedAssignees, err := s.resolveImportedAssignees(tsk.Assignees)
			if err != nil {
				return err
			}
			tsk.Assignees = resolvedAssignees
		}
		if err := s.ensureWritableTaskScope(tsk); err != nil {
			return err
		}
		_, err = s.repo.Create(tsk)
		return err
	}
	if err != nil {
		return err
	}
	if err := s.ensureWritableTaskScope(existing); err != nil {
		return err
	}
	if err := s.normalizeImportedProjectUpdate(existing, &tsk); err != nil {
		return err
	}
	if tsk.Title != "" {
		existing.Title = tsk.Title
	}
	if dto.Description != nil {
		existing.Description = normalizeOptionalText(dto.Description)
	}
	if tsk.Status != "" {
		existing.Status = tsk.Status
	}
	existing.Modified = s.clock.Unix()
	if tsk.Project != nil {
		existing.Project = tsk.Project
		existing.ProjectID = tsk.ProjectID
		existing.ProjectSeq = tsk.ProjectSeq
	}
	if tsk.Project == nil {
		existing.Project = nil
		existing.ProjectID = nil
		existing.ProjectSeq = nil
	}
	if tsk.Priority != nil {
		existing.Priority = tsk.Priority
	}
	if tsk.Due != nil {
		existing.Due = tsk.Due
	}
	if tsk.Start != nil {
		existing.Start = tsk.Start
	}
	if tsk.Wait != nil {
		existing.Wait = tsk.Wait
	}
	if tsk.Scheduled != nil {
		existing.Scheduled = tsk.Scheduled
	}
	if tsk.Until != nil {
		existing.Until = tsk.Until
	}
	if tsk.Recur != nil {
		existing.Recur = tsk.Recur
	}
	if tsk.Parent != nil {
		existing.Parent = tsk.Parent
	}
	if tsk.Mask != nil {
		existing.Mask = tsk.Mask
	}
	if tsk.IMask != nil {
		existing.IMask = tsk.IMask
	}
	if tsk.Tags != nil {
		existing.Tags = tsk.Tags
	}
	if tsk.Annotations != nil {
		ensureAnnotationIDs(tsk.Annotations)
		existing.Annotations = tsk.Annotations
	}
	if tsk.Depends != nil {
		existing.Depends = tsk.Depends
	}
	if tsk.Assignees != nil {
		resolvedAssignees, err := s.resolveImportedAssignees(tsk.Assignees)
		if err != nil {
			return err
		}
		existing.Assignees = resolvedAssignees
	}
	if tsk.UDAs != nil {
		normalized, err := s.normalizeImportedUDAs(tsk.UDAs)
		if err != nil {
			return err
		}
		existing.UDAs = normalized
	}
	if err := s.ensureWritableTaskScope(existing); err != nil {
		return err
	}
	return s.repo.Update(existing)
}

func ensureAnnotationIDs(annotations []task.Annotation) {
	for i := range annotations {
		if strings.TrimSpace(annotations[i].ID) == "" {
			annotations[i].ID = uuid.NewString()
		}
	}
}

func (s *Service) resolveImportedAssignees(values []task.AssigneeInfo) ([]task.AssigneeInfo, error) {
	if values == nil {
		return nil, nil
	}
	if len(values) == 0 {
		return []task.AssigneeInfo{}, nil
	}
	refs := make([]string, 0, len(values))
	for _, value := range values {
		switch {
		case strings.TrimSpace(value.UserID) != "":
			refs = append(refs, strings.TrimSpace(value.UserID))
		case value.Email != nil && strings.TrimSpace(*value.Email) != "":
			refs = append(refs, strings.TrimSpace(*value.Email))
		case strings.TrimSpace(value.Name) != "":
			refs = append(refs, strings.TrimSpace(value.Name))
		default:
			return nil, fmt.Errorf("assignee reference is required")
		}
	}
	return s.resolveAssigneeRefs(refs)
}

func (s *Service) dependencyGraph() (map[string][]string, error) {
	tasks, err := s.repo.List(s.workspaceID, storage.ListOptions{Dialect: s.store.Dialect()})
	if err != nil {
		return nil, err
	}
	graph := make(map[string][]string, len(tasks))
	for _, tsk := range tasks {
		graph[tsk.UUID] = append([]string(nil), tsk.Depends...)
	}
	return graph, nil
}

func (s *Service) RunReport(input ReportInput) (ReportResult, error) {
	if err := s.Require(PermissionTaskRead); err != nil {
		return ReportResult{}, err
	}
	def, ok := s.reports.Get(input.Name)
	if !ok {
		return ReportResult{}, fmt.Errorf("unknown report %q", input.Name)
	}
	if err := s.refreshAutomaticState(); err != nil {
		return ReportResult{}, err
	}
	contextExpr, err := s.activeContextFilter(input.NoContext)
	if err != nil {
		return ReportResult{}, err
	}
	resolvedReportQuery, err := s.resolveTaskQueryPredicates(input.Query)
	if err != nil {
		return ReportResult{}, err
	}
	merged := query.And(s.projectScopeExpr(), query.And(contextExpr, query.And(def.Filter, resolvedReportQuery)))
	now := s.clock.Unix()
	udaDefs, err := s.udaDefinitionTypes()
	if err != nil {
		return ReportResult{}, err
	}
	tasks, err := s.repo.List(s.workspaceID, storage.ListOptions{
		Query:          merged,
		Sort:           def.Sort,
		NowUnix:        now,
		UDADefinitions: udaDefs,
		Limit:          input.Limit,
		Offset:         input.Offset,
		Dialect:        s.store.Dialect(),
	})
	if err != nil {
		return ReportResult{}, mapProjectQueryCompileError(err)
	}
	allTasks, err := s.repo.List(s.workspaceID, storage.ListOptions{NowUnix: now, Query: s.projectScopeExpr(), Dialect: s.store.Dialect()})
	if err != nil {
		return ReportResult{}, err
	}
	blocked, blocking := buildDependencyState(allTasks, now)
	tasks = applyReportScope(tasks, def.Scope, now, blocked, blocking)
	if def.Sort == "urgency" {
		urgencyOptions, err := s.urgencyConfig()
		if err != nil {
			return ReportResult{}, err
		}
		type taskWithUrgency struct {
			Task  task.Task
			Total float64
		}
		withUrgency := make([]taskWithUrgency, len(tasks))
		for i, tsk := range tasks {
			opts := urgencyOptions
			opts.NowUnix = now
			opts.Blocked = blocked[tsk.UUID]
			opts.Blocking = blocking[tsk.UUID]
			explain := urgency.Explain(tsk, opts)
			withUrgency[i] = taskWithUrgency{Task: tsk, Total: explain.Total}
		}
		sort.SliceStable(withUrgency, func(i, j int) bool {
			return withUrgency[i].Total > withUrgency[j].Total
		})
		for i, wu := range withUrgency {
			tasks[i] = wu.Task
		}
	}
	if input.Limit > 0 && len(tasks) > input.Limit {
		tasks = tasks[:input.Limit]
	}
	return ReportResult{Tasks: tasks}, nil
}

func (s *Service) resolveTaskQueryPredicates(expr query.Expr) (query.Expr, error) {
	resolved, err := s.resolveProjectPredicates(expr)
	if err != nil {
		return nil, err
	}
	return s.resolveAssigneePredicates(resolved)
}

func (s *Service) resolveAssigneePredicates(expr query.Expr) (query.Expr, error) {
	switch e := expr.(type) {
	case nil:
		return nil, nil
	case query.Predicate:
		return s.resolveAssigneePredicate(e)
	case query.Binary:
		left, err := s.resolveAssigneePredicates(e.Left)
		if err != nil {
			return nil, err
		}
		right, err := s.resolveAssigneePredicates(e.Right)
		if err != nil {
			return nil, err
		}
		return query.Binary{Op: e.Op, Left: left, Right: right}, nil
	case query.Unary:
		resolved, err := s.resolveAssigneePredicates(e.Expr)
		if err != nil {
			return nil, err
		}
		return query.Unary{Op: e.Op, Expr: resolved}, nil
	default:
		return nil, fmt.Errorf("unsupported query expr %T", expr)
	}
}

func (s *Service) resolveAssigneePredicate(p query.Predicate) (query.Expr, error) {
	if p.Attribute != query.AttrAssignee {
		return p, nil
	}
	switch p.Operator {
	case query.OpIsNull, query.OpNotNull:
		return p, nil
	case query.OpEqual:
		value := strings.TrimSpace(p.Value.Text)
		if value == "" {
			return query.Predicate{Attribute: query.AttrAssignee, Operator: query.OpIsNull, Value: query.StringValue("")}, nil
		}
		if strings.EqualFold(value, "me") {
			if s.runtime.IsTenantActor() {
				return nil, tenantActorNotUserError()
			}
			return query.Predicate{
				Attribute: query.AttrAssignee,
				Operator:  query.OpEqual,
				Value:     query.StringValue(s.runtime.ActorUserID),
			}, nil
		}
		info, err := s.resolveAssigneeRef(value)
		if err != nil {
			return nil, err
		}
		return query.Predicate{
			Attribute: query.AttrAssignee,
			Operator:  query.OpEqual,
			Value:     query.StringValue(info.UserID),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported assignee predicate operator %q", p.Operator)
	}
}

func (s *Service) resolveAssigneeRefs(refs []string) ([]task.AssigneeInfo, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	seen := make(map[string]bool, len(refs))
	out := make([]task.AssigneeInfo, 0, len(refs))
	for _, ref := range refs {
		info, err := s.resolveAssigneeRef(ref)
		if err != nil {
			return nil, err
		}
		if seen[info.UserID] {
			continue
		}
		seen[info.UserID] = true
		out = append(out, info)
	}
	normalizeAssigneeInfos(out)
	return out, nil
}

func (s *Service) resolveAssigneeRef(ref string) (task.AssigneeInfo, error) {
	if strings.EqualFold(strings.TrimSpace(ref), "me") && s.runtime.IsTenantActor() {
		return task.AssigneeInfo{}, tenantActorNotUserError()
	}
	user, err := s.resolveUser(ref)
	if err != nil {
		if rtErr, ok := err.(RuntimeError); ok && rtErr.Code == "user_not_found" {
			return task.AssigneeInfo{}, RuntimeError{
				Code:    "assignee_not_found",
				Message: fmt.Sprintf("assignee %q not found", strings.TrimSpace(ref)),
			}
		}
		return task.AssigneeInfo{}, err
	}
	if _, err := s.memberRepo.Get(user.ID, s.workspaceID); err == storage.ErrNotFound {
		return task.AssigneeInfo{}, RuntimeError{
			Code:    "assignee_not_member",
			Message: fmt.Sprintf("user %q is not a member of workspace %q", user.Name, s.runtime.WorkspaceSlug),
		}
	} else if err != nil {
		return task.AssigneeInfo{}, err
	}
	return task.AssigneeInfo{
		UserID:      user.ID,
		Name:        user.Name,
		DisplayName: user.DisplayName,
		Email:       cloneStringPtr(user.Email),
	}, nil
}

func (s *Service) applyAssigneeModifications(existing []task.AssigneeInfo, addRefs, removeRefs []string, clear bool) ([]task.AssigneeInfo, error) {
	assigneeByUserID := make(map[string]task.AssigneeInfo, len(existing))
	if !clear {
		for _, assignee := range existing {
			if assignee.UserID == "" {
				continue
			}
			assigneeByUserID[assignee.UserID] = task.AssigneeInfo{
				UserID:      assignee.UserID,
				Name:        assignee.Name,
				DisplayName: assignee.DisplayName,
				Email:       cloneStringPtr(assignee.Email),
			}
		}
	}
	added, err := s.resolveAssigneeRefs(addRefs)
	if err != nil {
		return nil, err
	}
	for _, assignee := range added {
		assigneeByUserID[assignee.UserID] = assignee
	}
	removed, err := s.resolveAssigneeRefs(removeRefs)
	if err != nil {
		return nil, err
	}
	for _, assignee := range removed {
		delete(assigneeByUserID, assignee.UserID)
	}
	out := make([]task.AssigneeInfo, 0, len(assigneeByUserID))
	for _, assignee := range assigneeByUserID {
		out = append(out, assignee)
	}
	normalizeAssigneeInfos(out)
	return out, nil
}

func normalizeAssigneeInfos(assignees []task.AssigneeInfo) {
	task.SortAssigneeInfos(assignees)
}

func cloneAssigneeInfos(assignees []task.AssigneeInfo) []task.AssigneeInfo {
	if assignees == nil {
		return nil
	}
	out := make([]task.AssigneeInfo, len(assignees))
	for i, assignee := range assignees {
		out[i] = task.AssigneeInfo{
			UserID:      assignee.UserID,
			Name:        assignee.Name,
			DisplayName: assignee.DisplayName,
			Email:       cloneStringPtr(assignee.Email),
		}
	}
	return out
}

func (s *Service) ExplainUrgency(target string) (urgency.ExplainResult, error) {
	if err := s.Require(PermissionTaskRead); err != nil {
		return urgency.ExplainResult{}, err
	}
	if err := s.refreshAutomaticState(); err != nil {
		return urgency.ExplainResult{}, err
	}
	tsk, err := s.resolveTargetForRead(target)
	if err != nil {
		return urgency.ExplainResult{}, err
	}
	allTasks, err := s.repo.List(s.workspaceID, storage.ListOptions{NowUnix: s.clock.Unix(), Query: s.projectScopeExpr(), Dialect: s.store.Dialect()})
	if err != nil {
		return urgency.ExplainResult{}, err
	}
	blocked, blocking := buildDependencyState(allTasks, s.clock.Unix())
	opts, err := s.urgencyConfig()
	if err != nil {
		return urgency.ExplainResult{}, err
	}
	opts.NowUnix = s.clock.Unix()
	opts.Blocked = blocked[tsk.UUID]
	opts.Blocking = blocking[tsk.UUID]
	return urgency.Explain(tsk, opts), nil
}

func (s *Service) urgencyConfig() (urgency.Options, error) {
	values, err := s.mergedConfigValues()
	if err != nil {
		return urgency.Options{}, err
	}
	opts := urgency.Options{
		UDACoefficients:      map[string]float64{},
		UDAValueCoefficients: map[string]float64{},
	}
	const prefix = "urgency.uda."
	const suffix = ".coefficient"
	for key, value := range values {
		if !strings.HasPrefix(key, prefix) || !strings.HasSuffix(key, suffix) {
			continue
		}
		body := strings.TrimSuffix(strings.TrimPrefix(key, prefix), suffix)
		if body == "" {
			continue
		}
		coef, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return urgency.Options{}, fmt.Errorf("invalid urgency coefficient %q: %w", key, err)
		}
		if name, udaValue, ok := strings.Cut(body, "."); ok && name != "" && udaValue != "" {
			opts.UDAValueCoefficients[name+"."+udaValue] = coef
			continue
		}
		opts.UDACoefficients[body] = coef
	}
	return opts, nil
}

func (s *Service) resolveDependencyTargets(targets []string) ([]string, error) {
	if len(targets) == 0 {
		return nil, nil
	}
	depends := make([]string, 0, len(targets))
	seen := map[string]bool{}
	for _, target := range targets {
		dep, err := s.resolveTargetForWrite(target)
		if err != nil {
			return nil, fmt.Errorf("dependency %q not found: %w", target, err)
		}
		if !seen[dep.UUID] {
			depends = append(depends, dep.UUID)
			seen[dep.UUID] = true
		}
	}
	return depends, nil
}

// resolveParentForAdd 解析创建子任务时的 parent 引用。
// 返回 (parentUUID 指针, 子任务应使用的 project ref)。
// 当 parent 为空时返回 (nil, inputProject)，行为与无 parent 一致。
// 拒绝：parent 不存在/跨 workspace、deleted/completed/recurring parent、显式 project 与父 project 不一致。
func (s *Service) resolveParentForAdd(parentRef *string, inputProject *string) (*string, *string, error) {
	if parentRef == nil || strings.TrimSpace(*parentRef) == "" {
		return nil, inputProject, nil
	}
	parent, err := s.resolveTargetForWrite(*parentRef)
	if err != nil {
		// resolveTargetForWrite 对跨 workspace/project scope 已返回 task_not_found/project_scope_denied，
		// 统一转成面向使用者的 task_invalid_parent。
		return nil, nil, RuntimeError{Code: "task_invalid_parent", Message: "parent task not found"}
	}
	switch parent.Status {
	case task.StatusDeleted:
		return nil, nil, RuntimeError{Code: "task_parent_deleted", Message: "parent task is deleted"}
	case task.StatusCompleted:
		// 完成任务下继续拆任务容易造成状态语义混乱；文案给出替代动作（spec §8.2 规则 6）。
		return nil, nil, RuntimeError{Code: "task_parent_completed", Message: "parent task is completed; please reopen the parent before adding sub-tasks"}
	case task.StatusRecurring:
		// recurring parent 不允许手动子任务（spec §8.1 / §9.1）。
		return nil, nil, RuntimeError{Code: "task_parent_recurring", Message: "manual sub-tasks cannot be added to a recurring parent"}
	}
	// 显式 project 必须与父任务 project 一致（spec §8.2 规则 3）。
	if inputProject != nil && strings.TrimSpace(*inputProject) != "" &&
		parent.Project != nil && strings.TrimSpace(*parent.Project) != "" &&
		*inputProject != *parent.Project {
		return nil, nil, RuntimeError{Code: "task_invalid_parent", Message: "sub-task project must match parent project"}
	}
	// 子任务默认继承父任务 project；父任务无 project 时沿用调用方传入的 project（可为 nil）。
	if parent.Project == nil || strings.TrimSpace(*parent.Project) == "" {
		return &parent.UUID, inputProject, nil
	}
	return &parent.UUID, parent.Project, nil
}

func (s *Service) validateDependencyCycles(taskUUID string, depends []string) error {
	graph, err := s.dependencyGraph()
	if err != nil {
		return err
	}
	graph[taskUUID] = append([]string(nil), depends...)
	for _, depUUID := range depends {
		if task.WouldCreateDependencyCycle(graph, taskUUID, depUUID) {
			return fmt.Errorf("invalid dependency: cycle detected")
		}
	}
	return nil
}

func (s *Service) refreshAutomaticState() error {
	now := s.clock.Unix()
	waitingTasks, err := s.repo.List(s.workspaceID, storage.ListOptions{
		Status:  task.StatusWaiting,
		NowUnix: now,
		Dialect: s.store.Dialect(),
	})
	if err != nil {
		return err
	}
	for _, tsk := range waitingTasks {
		if tsk.Wait != nil && *tsk.Wait <= now {
			tsk.Wait = nil
			tsk.Status = task.StatusPending
			tsk.Modified = now
			if err := s.repo.Update(tsk); err != nil {
				return err
			}
		}
	}
	if err := s.ensureRecurringChildren(); err != nil {
		return err
	}
	return nil
}

func applyReportScope(tasks []task.Task, scope report.ScopeKind, now int64, blocked map[string]bool, blocking map[string]bool) []task.Task {
	switch scope {
	case report.ScopeHideUntilExpired:
		return filterExpiredUntil(tasks, now)
	case report.ScopeReady:
		filtered := make([]task.Task, 0, len(tasks))
		for _, tsk := range filterExpiredUntil(tasks, now) {
			if tsk.Status != task.StatusPending || tsk.Start != nil || blocked[tsk.UUID] {
				continue
			}
			if tsk.Wait != nil && *tsk.Wait > now {
				continue
			}
			if tsk.Scheduled != nil && *tsk.Scheduled > now {
				continue
			}
			filtered = append(filtered, tsk)
		}
		return filtered
	case report.ScopeBlocked:
		filtered := make([]task.Task, 0, len(tasks))
		for _, tsk := range filterExpiredUntil(tasks, now) {
			if tsk.Status == task.StatusPending && blocked[tsk.UUID] {
				filtered = append(filtered, tsk)
			}
		}
		return filtered
	case report.ScopeBlocking:
		filtered := make([]task.Task, 0, len(tasks))
		for _, tsk := range filterExpiredUntil(tasks, now) {
			if tsk.Status == task.StatusPending && blocking[tsk.UUID] {
				filtered = append(filtered, tsk)
			}
		}
		return filtered
	default:
		return tasks
	}
}

func filterExpiredUntil(tasks []task.Task, now int64) []task.Task {
	filtered := make([]task.Task, 0, len(tasks))
	for _, tsk := range tasks {
		if isUntilExpired(tsk, now) {
			continue
		}
		filtered = append(filtered, tsk)
	}
	return filtered
}

func isUntilExpired(tsk task.Task, now int64) bool {
	if tsk.Until == nil || *tsk.Until > now {
		return false
	}
	return tsk.Status == task.StatusPending || tsk.Status == task.StatusWaiting
}

func buildDependencyState(tasks []task.Task, now int64) (map[string]bool, map[string]bool) {
	byUUID := make(map[string]task.Task, len(tasks))
	for _, tsk := range tasks {
		byUUID[tsk.UUID] = tsk
	}
	blocked := map[string]bool{}
	blocking := map[string]bool{}
	for _, tsk := range tasks {
		if !isDependencyEligible(tsk, now) {
			continue
		}
		for _, depUUID := range tsk.Depends {
			dep, ok := byUUID[depUUID]
			if !ok || !isDependencyEligible(dep, now) {
				continue
			}
			blocked[tsk.UUID] = true
			blocking[depUUID] = true
		}
	}
	return blocked, blocking
}

func isDependencyEligible(tsk task.Task, now int64) bool {
	if isUntilExpired(tsk, now) {
		return false
	}
	return tsk.Status == task.StatusPending || tsk.Status == task.StatusWaiting
}

func (s *Service) defaultWorkingSet() ([]task.Task, error) {
	if err := s.refreshAutomaticState(); err != nil {
		return nil, err
	}
	tasks, err := s.repo.List(s.workspaceID, storage.ListOptions{
		NowUnix: s.clock.Unix(),
		Query:   s.projectScopeExpr(),
		Dialect: s.store.Dialect(),
	})
	if err != nil {
		return nil, err
	}
	filtered := make([]task.Task, 0, len(tasks))
	for _, tsk := range tasks {
		if tsk.Status != task.StatusPending && tsk.Status != task.StatusWaiting {
			continue
		}
		if isUntilExpired(tsk, s.clock.Unix()) {
			continue
		}
		filtered = append(filtered, tsk)
	}
	return filtered, nil
}

func (s *Service) createRecurringParent(input AddInput, now int64) (task.Task, projectChange, error) {
	if input.Wait != nil || input.Scheduled != nil || len(input.Depends) > 0 {
		return task.Task{}, projectChange{}, fmt.Errorf("recurring task does not accept wait, scheduled, or depends")
	}
	assignees, err := s.resolveAssigneeRefs(input.Assignees)
	if err != nil {
		return task.Task{}, projectChange{}, err
	}
	parent := task.Task{
		UUID: uuid.NewString(), WorkspaceID: s.workspaceID, Title: strings.TrimSpace(input.Title), Description: normalizeOptionalText(input.Description),
		Status: task.StatusRecurring, Entry: now, Modified: now,
		Due: input.Due, Priority: input.Priority, Tags: input.Tags,
		Assignees: assignees, Until: input.Until, Recur: input.Recur,
	}
	change, err := s.applyProjectBinding(&parent, input.Project)
	if err != nil {
		return task.Task{}, projectChange{}, err
	}
	udas, err := s.normalizeUDAModifications(nil, input.UDAs, nil, false)
	if err != nil {
		return task.Task{}, projectChange{}, err
	}
	parent.UDAs = udas
	createdParent, err := s.repo.Create(parent)
	if err != nil {
		return task.Task{}, projectChange{}, err
	}
	if _, warningEntry, err := s.createNextRecurringChild(createdParent, nil, now); err != nil {
		return task.Task{}, projectChange{}, err
	} else if warningEntry != nil {
		if err := s.appendAuditEntry(*warningEntry); err != nil {
			return task.Task{}, projectChange{}, err
		}
	}
	return createdParent, change, nil
}

func (s *Service) createNextRecurringChild(parent task.Task, previous *task.Task, now int64) (task.Task, *AuditEntry, error) {
	if parent.Status != task.StatusRecurring {
		return task.Task{}, nil, nil
	}
	children, err := s.repo.Children(s.workspaceID, parent.UUID)
	if err != nil {
		return task.Task{}, nil, err
	}
	for _, child := range children {
		if child.Status == task.StatusPending || child.Status == task.StatusWaiting {
			return child, nil, nil
		}
	}

	var due *int64
	if previous == nil {
		if parent.Due != nil {
			value := *parent.Due
			due = &value
		}
	} else if previous.Due != nil && parent.Recur != nil {
		nextDue, err := recurrence.Next(*previous.Due, *parent.Recur, s.clock.Location())
		if err != nil {
			return task.Task{}, nil, err
		}
		due = &nextDue
	}
	if due == nil {
		return task.Task{}, nil, fmt.Errorf("recurring parent requires due")
	}
	if parent.Until != nil && *due > *parent.Until {
		return task.Task{}, nil, nil
	}
	child := task.Task{
		UUID: uuid.NewString(), WorkspaceID: s.workspaceID, Title: parent.Title, Description: cloneStringPtr(parent.Description),
		Status: task.StatusPending, Entry: now, Modified: now,
		Due: due, Project: parent.Project, ProjectID: parent.ProjectID, Priority: parent.Priority, Tags: parent.Tags,
		Assignees: cloneAssigneeInfos(parent.Assignees), Until: parent.Until, Recur: parent.Recur,
		Parent: &parent.UUID,
		UDAs:   cloneUDAs(parent.UDAs),
	}
	if child.ProjectID != nil {
		seq, err := s.projectRepo.AllocateProjectTaskSeqLocked(child.WorkspaceID, *child.ProjectID)
		if err != nil {
			return task.Task{}, nil, err
		}
		child.ProjectSeq = &seq
	}
	created, _, err := s.repo.CreateRecurringChild(child)
	if err != nil {
		return task.Task{}, nil, err
	}
	warningEntry, err := s.recurringArchivedProjectWarning(parent, created)
	if err != nil {
		return task.Task{}, nil, err
	}
	return created, warningEntry, nil
}

func (s *Service) ensureRecurringChildren() error {
	parents, err := s.repo.RecurringParents(s.workspaceID)
	if err != nil {
		return err
	}
	for _, parent := range parents {
		children, err := s.repo.Children(s.workspaceID, parent.UUID)
		if err != nil {
			return err
		}
		var latest *task.Task
		hasOpen := false
		for i := range children {
			child := children[i]
			if latest == nil || child.Entry > latest.Entry {
				latest = &child
			}
			if child.Status == task.StatusPending || child.Status == task.StatusWaiting {
				hasOpen = true
			}
		}
		if hasOpen {
			continue
		}
		created, warningEntry, err := s.createNextRecurringChild(parent, latest, s.clock.Unix())
		if err != nil {
			return err
		}
		if warningEntry != nil && created.UUID != "" {
			if err := s.appendAuditEntry(*warningEntry); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) normalizeTaskProjectFields(tsk *task.Task) error {
	change, err := s.applyProjectBinding(tsk, tsk.Project)
	if err != nil {
		return err
	}
	_ = change
	return s.validateTaskProjectInvariant(*tsk)
}

func (s *Service) normalizeImportedProjectUpdate(existing task.Task, incoming *task.Task) error {
	if incoming.Project == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*incoming.Project)
	if trimmed == "" {
		incoming.Project = nil
		incoming.ProjectID = nil
		return nil
	}
	binding, err := s.resolveImportedProjectBinding(existing, trimmed)
	if err != nil {
		return err
	}
	incoming.Project = cloneStringPtr(binding.Slug)
	incoming.ProjectID = cloneStringPtr(binding.ID)
	if stringPtrEqual(existing.ProjectID, binding.ID) {
		incoming.ProjectSeq = cloneInt64Ptr(existing.ProjectSeq)
	} else if binding.ID != nil {
		seq, err := s.projectRepo.AllocateProjectTaskSeqLocked(incoming.WorkspaceID, *binding.ID)
		if err != nil {
			return err
		}
		incoming.ProjectSeq = &seq
	}
	return s.validateTaskProjectInvariant(*incoming)
}

func (s *Service) resolveImportedProjectBinding(existing task.Task, slug string) (projectBinding, error) {
	project, err := s.ResolveProject(slug)
	if err != nil {
		normalized, normalizeErr := normalizeProjectSlug(slug)
		if normalizeErr != nil || normalized == slug {
			return projectBinding{}, err
		}
		project, err = s.ResolveProject(normalized)
		if err != nil {
			return projectBinding{}, err
		}
	}
	binding := projectBindingFromProject(project)
	if !binding.Archived {
		return binding, nil
	}
	if existing.ProjectID != nil && existing.Project != nil && *existing.ProjectID == project.ID && *existing.Project == project.Slug {
		return binding, nil
	}
	return projectBinding{}, RuntimeError{
		Code:    "project_archived",
		Message: fmt.Sprintf("project %q is archived", project.Slug),
	}
}

func (s *Service) recurringArchivedProjectWarning(parent task.Task, child task.Task) (*AuditEntry, error) {
	if parent.ProjectID == nil || parent.Project == nil {
		return nil, nil
	}
	project, err := s.projectRepo.GetByID(*parent.ProjectID)
	if err != nil {
		return nil, RuntimeError{Code: "project_invariant_violation", Message: "project invariant violation"}
	}
	if !isProjectClosed(project) {
		return nil, nil
	}
	projectID := project.ID
	return &AuditEntry{
		Action:      "task.recurrence.archived_project",
		WorkspaceID: &parent.WorkspaceID,
		ProjectID:   &projectID,
		TargetType:  "task",
		TargetID:    child.UUID,
		Payload: map[string]any{
			"parent_uuid":  parent.UUID,
			"child_uuid":   child.UUID,
			"project_id":   project.ID,
			"project_slug": project.Slug,
		},
	}, nil
}

func (s *Service) appendAuditEntry(entry AuditEntry) error {
	workspaceID := &s.runtime.WorkspaceID
	if entry.WorkspaceID != nil {
		workspaceID = entry.WorkspaceID
	}
	payload, err := marshalAuditPayload(entry.Payload)
	if err != nil {
		return err
	}
	actorType := s.runtime.ActorType
	if actorType == "" {
		actorType = "user"
	}
	var actorUserID *string
	var actorTokenID *string
	var actorTokenName *string
	var actorTokenPrefix *string
	if s.runtime.IsTenantActor() {
		actorTokenID = stringPtr(s.runtime.ActorTokenID)
		actorTokenName = stringPtr(s.runtime.ActorTokenName)
		actorTokenPrefix = stringPtr(s.runtime.ActorTokenPrefix)
	} else {
		actorUserID = stringPtr(s.runtime.ActorUserID)
	}
	return s.auditRepo.Append(storage.AuditLogEntry{
		ActorType:        actorType,
		ActorUserID:      actorUserID,
		ActorTokenID:     actorTokenID,
		ActorTokenName:   actorTokenName,
		ActorTokenPrefix: actorTokenPrefix,
		WorkspaceID:      workspaceID,
		ProjectID:        entry.ProjectID,
		Action:           entry.Action,
		TargetType:       entry.TargetType,
		TargetID:         entry.TargetID,
		PayloadJSON:      payload,
		CreatedAt:        s.clock.Unix(),
	})
}
