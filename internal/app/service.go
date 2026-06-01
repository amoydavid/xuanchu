package app

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/dajee/taskg/internal/query"
	"github.com/dajee/taskg/internal/recurrence"
	"github.com/dajee/taskg/internal/report"
	"github.com/dajee/taskg/internal/storage/sqlite"
	"github.com/dajee/taskg/internal/task"
	"github.com/dajee/taskg/internal/uda"
	"github.com/dajee/taskg/internal/urgency"
)

type Service struct {
	store                 *sqlite.Store
	repo                  *sqlite.TaskRepository
	projectRepo           *sqlite.ProjectRepository
	configRepo            *sqlite.ConfigRepository
	userRepo              *sqlite.UserRepository
	workspaceRepo         *sqlite.WorkspaceRepository
	memberRepo            *sqlite.MemberRepository
	auditRepo             auditAppenderLister
	tokenRepo             *sqlite.TokenRepository
	contextRepo           *sqlite.ContextRepository
	udaRepo               *sqlite.UDARepository
	runtimeConfig         map[string]string
	runtimeOverrides      map[string]string
	runtimeUDAs           map[string]uda.Definition
	activeContextOverride *string
	runtime               RuntimeContext
	requestScope          *RequestScope
	workspaceID           string
	clock                 Clock
	reports               report.Registry
	disableContext        bool
}

type AddInput struct {
	Description string
	Project     *string
	Priority    *string
	Due         *int64
	Depends     []string
	Wait        *int64
	Scheduled   *int64
	Until       *int64
	Recur       *string
	Tags        []string
	UDAs        map[string]string
}

type ListInput struct {
	Target     *string
	Status     string
	Sort       string
	Query      query.Expr
	ReportMode bool
	NoContext  bool
}

type ExportInput struct {
	ProjectID *string
}

type ModifyInput struct {
	Description    *string
	Project        *string
	ClearProject   bool
	Priority       *string
	ClearPriority  bool
	Due            *int64
	ClearDue       bool
	Wait           *int64
	ClearWait      bool
	Scheduled      *int64
	ClearScheduled bool
	Until          *int64
	ClearUntil     bool
	AddDepends     []string
	ClearDepends   bool
	Recur          *string
	ClearRecur     bool
	AddTags        []string
	RemoveTags     []string
	UDAs           map[string]string
	ClearUDAs      []string
}

type ReportInput struct {
	Name      string
	Query     query.Expr
	NoContext bool
}

type ReportResult struct {
	Tasks []task.Task
}

type auditAppenderLister interface {
	Append(sqlite.AuditLogEntry) error
	List(sqlite.AuditListOptions) ([]sqlite.AuditLogEntry, error)
}

func NewService(opts ServiceOptions) (*Service, error) {
	if opts.Clock == nil {
		opts.Clock = realClock{}
	}
	userRepo := sqlite.NewUserRepository(opts.Store.DB())
	workspaceRepo := sqlite.NewWorkspaceRepository(opts.Store.DB())
	memberRepo := sqlite.NewMemberRepository(opts.Store.DB())
	auditRepo := sqlite.NewAuditRepository(opts.Store.DB())
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
	return &Service{
		store:            opts.Store,
		repo:             sqlite.NewTaskRepository(opts.Store.DB()),
		projectRepo:      sqlite.NewProjectRepository(opts.Store.DB()),
		configRepo:       sqlite.NewConfigRepository(opts.Store.DB()),
		userRepo:         userRepo,
		workspaceRepo:    workspaceRepo,
		memberRepo:       memberRepo,
		auditRepo:        auditRepo,
		tokenRepo:        sqlite.NewTokenRepository(opts.Store.DB()),
		contextRepo:      sqlite.NewContextRepository(opts.Store.DB()),
		udaRepo:          sqlite.NewUDARepository(opts.Store.DB()),
		runtimeConfig:    runtimeConfig,
		runtimeOverrides: cloneStringMap(opts.RuntimeOverrides),
		runtimeUDAs:      runtimeUDAs,
		runtime:          rt,
		requestScope:     cloneRequestScope(opts.RequestScope),
		workspaceID:      rt.WorkspaceID,
		clock:            opts.Clock,
		reports:          report.DefaultRegistry(),
		disableContext:   opts.NoContext,
	}, nil
}

func (s *Service) Clock() Clock {
	return s.clock
}

func (s *Service) withStore(store *sqlite.Store) (*Service, error) {
	clone := *s
	clone.store = store
	clone.repo = sqlite.NewTaskRepository(store.DB())
	clone.projectRepo = sqlite.NewProjectRepository(store.DB())
	clone.configRepo = sqlite.NewConfigRepository(store.DB())
	clone.userRepo = sqlite.NewUserRepository(store.DB())
	clone.workspaceRepo = sqlite.NewWorkspaceRepository(store.DB())
	clone.memberRepo = sqlite.NewMemberRepository(store.DB())
	// Tests can inject a custom audit repo to force append failures; keep it
	// attached while production services get a tx-bound repository.
	if _, ok := s.auditRepo.(*sqlite.AuditRepository); ok || s.auditRepo == nil {
		clone.auditRepo = sqlite.NewAuditRepository(store.DB())
	}
	clone.tokenRepo = sqlite.NewTokenRepository(store.DB())
	clone.contextRepo = sqlite.NewContextRepository(store.DB())
	clone.udaRepo = sqlite.NewUDARepository(store.DB())
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
	err := s.withAudit("task.add", func(tx *Service) (AuditEntry, error) {
		var (
			err    error
			change projectChange
		)
		created, change, err = tx.addLocked(input)
		if err != nil {
			return AuditEntry{}, err
		}
		return taskAuditEntry("task.add", created.UUID, change), nil
	})
	return created, err
}

func (s *Service) addLocked(input AddInput) (task.Task, projectChange, error) {
	now := s.clock.Unix()
	if input.Recur != nil {
		return s.createRecurringParent(input, now)
	}
	depends, err := s.resolveDependencyTargets(input.Depends)
	if err != nil {
		return task.Task{}, projectChange{}, err
	}
	tsk := task.Task{
		UUID: uuid.NewString(), WorkspaceID: s.workspaceID, Description: input.Description,
		Status: task.StatusPending, Entry: now, Modified: now,
		Due: input.Due, Priority: input.Priority, Tags: input.Tags,
		Depends: depends,
		Wait:    input.Wait, Scheduled: input.Scheduled, Until: input.Until, Recur: input.Recur,
	}
	projChange, err := s.applyProjectBinding(&tsk, input.Project)
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
	resolvedInputQuery, err := s.resolveProjectPredicates(input.Query)
	if err != nil {
		return nil, err
	}
	queryExpr := query.And(s.projectScopeExpr(), query.And(contextExpr, resolvedInputQuery))
	udaDefs, err := s.udaDefinitionTypes()
	if err != nil {
		return nil, err
	}
	tasks, err := s.repo.List(s.workspaceID, sqlite.ListOptions{
		Status:         status,
		Sort:           input.Sort,
		Query:          queryExpr,
		NowUnix:        s.clock.Unix(),
		UDADefinitions: udaDefs,
	})
	if err != nil {
		return nil, mapProjectQueryCompileError(err)
	}
	if !input.ReportMode {
		tasks = filterExpiredUntil(tasks, s.clock.Unix())
	}
	return tasks, nil
}

func (s *Service) ListReport(name string, input ListInput) ([]task.Task, error) {
	if input.Target != nil {
		return s.List(input)
	}
	result, err := s.RunReport(ReportInput{Name: name, Query: input.Query, NoContext: input.NoContext})
	if err != nil {
		return nil, err
	}
	return result.Tasks, nil
}

func (s *Service) Info(target string) (task.Task, error) {
	if err := s.Require(PermissionTaskRead); err != nil {
		return task.Task{}, err
	}
	return s.resolveTargetForRead(target)
}

func (s *Service) ResolveTarget(target string) (task.Task, error) {
	return s.resolveTargetForRead(target)
}

func (s *Service) Modify(target string, input ModifyInput) error {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return err
	}
	return s.withAudit("task.modify", func(tx *Service) (AuditEntry, error) {
		targetID, change, err := tx.modifyLocked(target, input)
		if err != nil {
			return AuditEntry{}, err
		}
		return taskAuditEntry("task.modify", targetID, change), nil
	})
}

func (s *Service) modifyLocked(target string, input ModifyInput) (string, projectChange, error) {
	tsk, err := s.resolveTargetForWrite(target)
	if err != nil {
		return "", projectChange{}, err
	}
	now := s.clock.Unix()
	change := projectChangeForTask(tsk)
	if input.Description != nil {
		tsk.Description = *input.Description
	}
	if input.Project != nil {
		change, err = s.applyProjectBinding(&tsk, input.Project)
		if err != nil {
			return "", projectChange{}, err
		}
	}
	if input.ClearProject {
		empty := ""
		change, err = s.applyProjectBinding(&tsk, &empty)
		if err != nil {
			return "", projectChange{}, err
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
			return "", projectChange{}, err
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
			return "", projectChange{}, err
		}
	}
	if len(input.UDAs) > 0 || len(input.ClearUDAs) > 0 {
		udas, err := s.normalizeUDAModifications(tsk.UDAs, input.UDAs, input.ClearUDAs, false)
		if err != nil {
			return "", projectChange{}, err
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
		return "", projectChange{}, err
	}
	return tsk.UUID, change, nil
}

func (s *Service) Done(target string) error {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return err
	}
	return s.withAuditEntries(func(tx *Service) ([]AuditEntry, error) {
		targetID, change, extraEntries, err := tx.doneLocked(target)
		if err != nil {
			return nil, err
		}
		entries := []AuditEntry{taskAuditEntry("task.done", targetID, change)}
		entries = append(entries, extraEntries...)
		return entries, nil
	})
}

func (s *Service) doneLocked(target string) (string, projectChange, []AuditEntry, error) {
	tsk, err := s.resolveTargetForWrite(target)
	if err != nil {
		return "", projectChange{}, nil, err
	}
	change := projectChangeForTask(tsk)
	tsk.Complete(s.clock.Unix())
	if err := s.repo.Update(tsk); err != nil {
		return "", projectChange{}, nil, err
	}
	if tsk.Parent != nil {
		parent, err := s.repo.GetByUUID(s.workspaceID, *tsk.Parent)
		if err != nil {
			return "", projectChange{}, nil, err
		}
		if parent.Status != task.StatusRecurring {
			return tsk.UUID, change, nil, nil
		}
		_, warningEntry, err := s.createNextRecurringChild(parent, &tsk, s.clock.Unix())
		if err != nil {
			return "", projectChange{}, nil, err
		}
		if warningEntry != nil {
			return tsk.UUID, change, []AuditEntry{*warningEntry}, nil
		}
	}
	return tsk.UUID, change, nil, nil
}

func (s *Service) Delete(target string) error {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return err
	}
	return s.withAudit("task.delete", func(tx *Service) (AuditEntry, error) {
		targetID, change, err := tx.deleteLocked(target)
		if err != nil {
			return AuditEntry{}, err
		}
		return taskAuditEntry("task.delete", targetID, change), nil
	})
}

func (s *Service) deleteLocked(target string) (string, projectChange, error) {
	tsk, err := s.resolveTargetForWrite(target)
	if err != nil {
		return "", projectChange{}, err
	}
	change := projectChangeForTask(tsk)
	if tsk.Status == task.StatusCompleted || tsk.Status == task.StatusDeleted {
		return "", projectChange{}, fmt.Errorf("cannot delete %s task", tsk.Status)
	}
	tsk.Delete(s.clock.Unix())
	if err := s.repo.Update(tsk); err != nil {
		return "", projectChange{}, err
	}
	return tsk.UUID, change, nil
}

func (s *Service) Start(target string) error {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return err
	}
	return s.withAudit("task.start", func(tx *Service) (AuditEntry, error) {
		targetID, change, err := tx.startLocked(target)
		if err != nil {
			return AuditEntry{}, err
		}
		return taskAuditEntry("task.start", targetID, change), nil
	})
}

func (s *Service) startLocked(target string) (string, projectChange, error) {
	tsk, err := s.resolveTargetForWrite(target)
	if err != nil {
		return "", projectChange{}, err
	}
	change := projectChangeForTask(tsk)
	if tsk.Status == task.StatusCompleted || tsk.Status == task.StatusDeleted || tsk.Status == task.StatusRecurring {
		return "", projectChange{}, fmt.Errorf("cannot start %s task", tsk.Status)
	}
	if tsk.Start != nil {
		return "", projectChange{}, fmt.Errorf("task is already active")
	}
	tsk.StartTask(s.clock.Unix())
	if err := s.repo.Update(tsk); err != nil {
		return "", projectChange{}, err
	}
	return tsk.UUID, change, nil
}

func (s *Service) Stop(target string) error {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return err
	}
	return s.withAudit("task.stop", func(tx *Service) (AuditEntry, error) {
		targetID, change, err := tx.stopLocked(target)
		if err != nil {
			return AuditEntry{}, err
		}
		return taskAuditEntry("task.stop", targetID, change), nil
	})
}

func (s *Service) stopLocked(target string) (string, projectChange, error) {
	tsk, err := s.resolveTargetForWrite(target)
	if err != nil {
		return "", projectChange{}, err
	}
	change := projectChangeForTask(tsk)
	if tsk.Status == task.StatusCompleted || tsk.Status == task.StatusDeleted {
		return "", projectChange{}, fmt.Errorf("cannot stop %s task", tsk.Status)
	}
	tsk.StopTask(s.clock.Unix())
	if err := s.repo.Update(tsk); err != nil {
		return "", projectChange{}, err
	}
	return tsk.UUID, change, nil
}

func (s *Service) Annotate(target, description string) error {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return err
	}
	return s.withAudit("task.annotate", func(tx *Service) (AuditEntry, error) {
		targetID, change, err := tx.annotateLocked(target, description)
		if err != nil {
			return AuditEntry{}, err
		}
		return taskAuditEntry("task.annotate", targetID, change), nil
	})
}

func (s *Service) annotateLocked(target, description string) (string, projectChange, error) {
	description = strings.TrimSpace(description)
	if description == "" {
		return "", projectChange{}, fmt.Errorf("annotation description is required")
	}
	if strings.ContainsAny(description, "\n\r") {
		return "", projectChange{}, fmt.Errorf("annotation description must not contain newlines")
	}
	now := s.clock.Unix()
	for attempts := 0; attempts < 3; attempts++ {
		tsk, err := s.resolveTargetForWrite(target)
		if err != nil {
			return "", projectChange{}, err
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
		if err := s.repo.AddAnnotation(s.workspaceID, tsk.UUID, task.Annotation{Entry: entry, Description: description}, entry); err != nil {
			if sqlite.IsUniqueConstraintError(err) {
				now = entry + 1
				continue
			}
			return "", projectChange{}, err
		}
		return tsk.UUID, change, nil
	}
	return "", projectChange{}, fmt.Errorf("annotation conflict could not be resolved")
}

func (s *Service) Denotate(target string, index int) error {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return err
	}
	return s.withAudit("task.denotate", func(tx *Service) (AuditEntry, error) {
		targetID, change, err := tx.denotateLocked(target, index)
		if err != nil {
			return AuditEntry{}, err
		}
		return taskAuditEntry("task.denotate", targetID, change), nil
	})
}

func (s *Service) denotateLocked(target string, index int) (string, projectChange, error) {
	tsk, err := s.resolveTargetForWrite(target)
	if err != nil {
		return "", projectChange{}, err
	}
	change := projectChangeForTask(tsk)
	sort.Slice(tsk.Annotations, func(i, j int) bool {
		return tsk.Annotations[i].Entry < tsk.Annotations[j].Entry
	})
	if index < 1 || index > len(tsk.Annotations) {
		return "", projectChange{}, fmt.Errorf("annotation %d not found", index)
	}
	tsk.Annotations = append(tsk.Annotations[:index-1], tsk.Annotations[index:]...)
	tsk.Modified = s.clock.Unix()
	if err := s.repo.Update(tsk); err != nil {
		return "", projectChange{}, err
	}
	return tsk.UUID, change, nil
}

func (s *Service) AppendDescription(target, suffix string) error {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return err
	}
	return s.withAudit("task.append", func(tx *Service) (AuditEntry, error) {
		targetID, change, err := tx.appendDescriptionLocked(target, suffix)
		if err != nil {
			return AuditEntry{}, err
		}
		return taskAuditEntry("task.append", targetID, change), nil
	})
}

func (s *Service) appendDescriptionLocked(target, suffix string) (string, projectChange, error) {
	tsk, err := s.resolveTargetForWrite(target)
	if err != nil {
		return "", projectChange{}, err
	}
	change := projectChangeForTask(tsk)
	suffix = strings.TrimSpace(suffix)
	if suffix == "" {
		return "", projectChange{}, fmt.Errorf("description text is required")
	}
	tsk.Description = tsk.Description + " " + suffix
	tsk.Modified = s.clock.Unix()
	if err := s.repo.Update(tsk); err != nil {
		return "", projectChange{}, err
	}
	return tsk.UUID, change, nil
}

func (s *Service) PrependDescription(target, prefix string) error {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return err
	}
	return s.withAudit("task.prepend", func(tx *Service) (AuditEntry, error) {
		targetID, change, err := tx.prependDescriptionLocked(target, prefix)
		if err != nil {
			return AuditEntry{}, err
		}
		return taskAuditEntry("task.prepend", targetID, change), nil
	})
}

func (s *Service) prependDescriptionLocked(target, prefix string) (string, projectChange, error) {
	tsk, err := s.resolveTargetForWrite(target)
	if err != nil {
		return "", projectChange{}, err
	}
	change := projectChangeForTask(tsk)
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "", projectChange{}, fmt.Errorf("description text is required")
	}
	tsk.Description = prefix + " " + tsk.Description
	tsk.Modified = s.clock.Unix()
	if err := s.repo.Update(tsk); err != nil {
		return "", projectChange{}, err
	}
	return tsk.UUID, change, nil
}

func (s *Service) ReplaceEditableTask(target string, edited task.Task) error {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return err
	}
	return s.withAudit("task.edit", func(tx *Service) (AuditEntry, error) {
		targetID, change, err := tx.replaceEditableTaskLocked(target, edited)
		if err != nil {
			return AuditEntry{}, err
		}
		return taskAuditEntry("task.edit", targetID, change), nil
	})
}

func (s *Service) replaceEditableTaskLocked(target string, edited task.Task) (string, projectChange, error) {
	original, err := s.resolveTargetForWrite(target)
	if err != nil {
		return "", projectChange{}, err
	}
	edited.UUID = original.UUID
	edited.WorkspaceID = original.WorkspaceID
	edited.Entry = original.Entry
	edited.Modified = s.clock.Unix()
	change, err := s.applyProjectBindingFrom(&edited, edited.Project, projectBindingFromTask(original))
	if err != nil {
		return "", projectChange{}, err
	}
	if edited.Wait != nil && *edited.Wait > s.clock.Unix() && edited.Status == task.StatusPending {
		edited.Status = task.StatusWaiting
	}
	if edited.Wait == nil && edited.Status == task.StatusWaiting {
		edited.Status = task.StatusPending
	}
	if err := edited.Validate(); err != nil {
		return "", projectChange{}, err
	}
	if err := s.repo.Update(edited); err != nil {
		return "", projectChange{}, err
	}
	return edited.UUID, change, nil
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
	tasks, err := s.repo.List(s.workspaceID, sqlite.ListOptions{Query: queryExpr})
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
	err := s.withAudit("task.import", func(tx *Service) (AuditEntry, error) {
		imported, err := tx.importLocked(tasks)
		if err != nil {
			return AuditEntry{}, err
		}
		count = imported
		return AuditEntry{
			TargetType: "task",
			Payload: map[string]any{
				"count": imported,
			},
		}, nil
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
	if err == sqlite.ErrNotFound {
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
		if tsk.UDAs != nil {
			normalized, err := s.normalizeImportedUDAs(tsk.UDAs)
			if err != nil {
				return err
			}
			tsk.UDAs = normalized
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
	if tsk.Description != "" {
		existing.Description = tsk.Description
	}
	if tsk.Status != "" {
		existing.Status = tsk.Status
	}
	existing.Modified = s.clock.Unix()
	if tsk.Project != nil {
		existing.Project = tsk.Project
		existing.ProjectID = tsk.ProjectID
	}
	if tsk.Project == nil {
		existing.Project = nil
		existing.ProjectID = nil
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
		existing.Annotations = tsk.Annotations
	}
	if tsk.Depends != nil {
		existing.Depends = tsk.Depends
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

func (s *Service) dependencyGraph() (map[string][]string, error) {
	tasks, err := s.repo.List(s.workspaceID, sqlite.ListOptions{})
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
	resolvedReportQuery, err := s.resolveProjectPredicates(input.Query)
	if err != nil {
		return ReportResult{}, err
	}
	merged := query.And(s.projectScopeExpr(), query.And(contextExpr, query.And(def.Filter, resolvedReportQuery)))
	now := s.clock.Unix()
	udaDefs, err := s.udaDefinitionTypes()
	if err != nil {
		return ReportResult{}, err
	}
	tasks, err := s.repo.List(s.workspaceID, sqlite.ListOptions{
		Query:          merged,
		Sort:           def.Sort,
		NowUnix:        now,
		UDADefinitions: udaDefs,
	})
	if err != nil {
		return ReportResult{}, mapProjectQueryCompileError(err)
	}
	allTasks, err := s.repo.List(s.workspaceID, sqlite.ListOptions{NowUnix: now, Query: s.projectScopeExpr()})
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
	return ReportResult{Tasks: tasks}, nil
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
	allTasks, err := s.repo.List(s.workspaceID, sqlite.ListOptions{NowUnix: s.clock.Unix(), Query: s.projectScopeExpr()})
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
	waitingTasks, err := s.repo.List(s.workspaceID, sqlite.ListOptions{
		Status:  task.StatusWaiting,
		NowUnix: now,
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
	tasks, err := s.repo.List(s.workspaceID, sqlite.ListOptions{
		NowUnix: s.clock.Unix(),
		Query:   s.projectScopeExpr(),
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
	parent := task.Task{
		UUID: uuid.NewString(), WorkspaceID: s.workspaceID, Description: input.Description,
		Status: task.StatusRecurring, Entry: now, Modified: now,
		Due: input.Due, Priority: input.Priority, Tags: input.Tags,
		Until: input.Until, Recur: input.Recur,
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
		UUID: uuid.NewString(), WorkspaceID: s.workspaceID, Description: parent.Description,
		Status: task.StatusPending, Entry: now, Modified: now,
		Due: due, Project: parent.Project, ProjectID: parent.ProjectID, Priority: parent.Priority, Tags: parent.Tags,
		Until: parent.Until, Recur: parent.Recur,
		Parent: &parent.UUID,
		UDAs:   cloneUDAs(parent.UDAs),
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
	if project.Status != string(sqlite.ProjectStatusArchived) && project.ArchivedAt == nil {
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
	return s.auditRepo.Append(sqlite.AuditLogEntry{
		ActorUserID: &s.runtime.ActorUserID,
		WorkspaceID: workspaceID,
		ProjectID:   entry.ProjectID,
		Action:      entry.Action,
		TargetType:  entry.TargetType,
		TargetID:    entry.TargetID,
		PayloadJSON: payload,
		CreatedAt:   s.clock.Unix(),
	})
}
