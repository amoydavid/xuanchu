package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/query"
	domain "git.dajee.net/dajee/xuanchu/internal/task"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TaskRepository struct {
	db           *gorm.DB
	taskLinkRepo *TaskLinkRepository
}

type ListOptions struct {
	Status         string
	Sort           string
	Query          query.Expr
	NowUnix        int64
	UDADefinitions map[string]string
	Limit          int
	Offset         int
	Dialect        string
}

// TaskCandidateListOptions 是项目模板普通任务候选的数据库筛选条件。
// WorkspaceID、ProjectID 和 normal/non-deleted 边界由 repository 强制施加，
// Query 只能在该边界内继续收窄结果。
type TaskCandidateListOptions struct {
	WorkspaceID     string
	ProjectID       string
	Refs            []string
	Q               string
	Status          string
	Priority        string
	AssigneeUserIDs []string
	Tags            []string
	DueAfter        *int64
	DueBefore       *int64
	Query           query.Expr
	Sort            string
	NowUnix         int64
	UDADefinitions  map[string]string
	Dialect         string
	UrgencyScore    func(domain.Task, bool, bool) float64
}

type TaskCandidatePage struct {
	Items  []domain.Task
	Total  int64
	Limit  int
	Offset int
}

func NewTaskRepository(db *gorm.DB) *TaskRepository {
	return &TaskRepository{db: db, taskLinkRepo: NewTaskLinkRepository(db)}
}

func (r *TaskRepository) preloadAssociations() *gorm.DB {
	return r.db.Preload("Tags").Preload("Annotations").Preload("Depends").Preload("Assignees").Preload("UDAs")
}

func (r *TaskRepository) Create(tsk domain.Task) (domain.Task, error) {
	if err := tsk.Validate(); err != nil {
		return domain.Task{}, err
	}
	model := toModel(tsk)
	if err := r.db.Create(&model).Error; err != nil {
		return domain.Task{}, err
	}
	usersByID, err := r.loadAssigneeUsers([]Task{model})
	if err != nil {
		return domain.Task{}, err
	}
	linksByTask, err := r.loadLinksByTask([]Task{model})
	if err != nil {
		return domain.Task{}, err
	}
	return fromModel(model, usersByID, linksByTask), nil
}

func (r *TaskRepository) List(workspaceID string, opts ListOptions) ([]domain.Task, error) {
	var models []Task
	q := r.preloadAssociations()
	if opts.Status != "" {
		q = q.Where("status = ?", opts.Status)
	}
	if opts.Query != nil {
		q = ApplyQuery(q, opts.Query, QueryCompileOptions{WorkspaceID: workspaceID, NowUnix: opts.NowUnix, UDADefinitions: opts.UDADefinitions, Dialect: opts.Dialect})
	} else {
		q = q.Where("workspace_id = ?", workspaceID)
	}
	switch opts.Sort {
	case "next":
		q = q.Order("due IS NULL ASC").Order("due ASC").Order("CASE priority WHEN 'H' THEN 3 WHEN 'M' THEN 2 WHEN 'L' THEN 1 ELSE 0 END DESC").Order("entry ASC")
	case "completed":
		q = q.Order("end_ts DESC").Order("modified DESC")
	case "due":
		q = q.Order("due IS NULL ASC").Order("due ASC")
	case "wait":
		q = q.Order("wait IS NULL ASC").Order("wait ASC")
	case "start":
		q = q.Order("start DESC")
	default:
		q = q.Order("entry ASC")
	}
	if opts.Limit > 0 {
		q = q.Limit(opts.Limit)
	}
	if opts.Offset > 0 {
		q = q.Offset(opts.Offset)
	}
	if err := q.Find(&models).Error; err != nil {
		return nil, err
	}
	usersByID, err := r.loadAssigneeUsers(models)
	if err != nil {
		return nil, err
	}
	linksByTask, err := r.loadLinksByTask(models)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Task, 0, len(models))
	for _, model := range models {
		out = append(out, fromModel(model, usersByID, linksByTask))
	}
	return out, nil
}

// ListCandidatePage 在数据库中完成模板普通任务候选的筛选、计数与分页。
func (r *TaskRepository) ListCandidatePage(opts TaskCandidateListOptions, limit, offset int) (TaskCandidatePage, error) {
	if opts.Sort == "urgency" {
		return r.listUrgencyCandidatePage(opts, limit, offset)
	}
	base := taskCandidateQuery(r.db, opts)
	var total int64
	if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return TaskCandidatePage{}, err
	}
	var models []Task
	pageQuery := base.Session(&gorm.Session{}).
		Preload("Tags").Preload("Annotations").Preload("Depends").Preload("Assignees").Preload("UDAs")
	pageQuery = orderTaskCandidateQuery(pageQuery, opts.Sort).Limit(limit).Offset(offset)
	if err := pageQuery.Find(&models).Error; err != nil {
		return TaskCandidatePage{}, err
	}
	return r.taskCandidatePage(models, total, limit, offset)
}

func taskCandidateQuery(db *gorm.DB, opts TaskCandidateListOptions) *gorm.DB {
	base := db.Model(&Task{}).
		Where("workspace_id = ? AND project_id = ? AND series_id IS NULL AND status <> ?", opts.WorkspaceID, opts.ProjectID, domain.StatusDeleted)
	if len(opts.Refs) > 0 {
		base = base.Where("uuid IN ?", opts.Refs)
	}
	if opts.Status != "" && opts.Status != "all" {
		base = base.Where("status = ?", opts.Status)
	}
	if q := strings.TrimSpace(opts.Q); q != "" {
		like := "%" + q + "%"
		base = base.Where("(LOWER(title) LIKE LOWER(?) OR LOWER(COALESCE(description, '')) LIKE LOWER(?))", like, like)
	}
	if priority := strings.TrimSpace(opts.Priority); priority != "" && priority != "all" {
		base = base.Where("priority = ?", priority)
	}
	if len(opts.AssigneeUserIDs) > 0 {
		base = base.Where("uuid IN (SELECT task_uuid FROM task_assignees WHERE user_id IN ?)", opts.AssigneeUserIDs)
	}
	for _, tag := range opts.Tags {
		if tag = strings.TrimSpace(tag); tag != "" {
			base = base.Where("uuid IN (SELECT task_uuid FROM task_tags WHERE tag = ?)", tag)
		}
	}
	if opts.DueAfter != nil {
		base = base.Where("due >= ?", *opts.DueAfter)
	}
	if opts.DueBefore != nil {
		base = base.Where("due < ?", *opts.DueBefore)
	}
	if opts.Query != nil {
		base = ApplyQuery(base, opts.Query, QueryCompileOptions{WorkspaceID: opts.WorkspaceID, NowUnix: opts.NowUnix, UDADefinitions: opts.UDADefinitions, Dialect: opts.Dialect})
	}
	return base
}

func (r *TaskRepository) listUrgencyCandidatePage(opts TaskCandidateListOptions, limit, offset int) (TaskCandidatePage, error) {
	if opts.UrgencyScore == nil {
		return TaskCandidatePage{}, fmt.Errorf("task candidate urgency score is required")
	}
	var total int64
	var models []Task
	err := withCandidateSortKeyTable(r.db, func(tx *gorm.DB, tableName string) error {
		base := taskCandidateQuery(tx, opts)
		if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
			return err
		}
		if err := populateTaskCandidateUrgencySortKeys(tx, opts, tableName); err != nil {
			return err
		}
		quotedName := quoteCandidateSortKeyTable(tableName)
		pageQuery := base.Session(&gorm.Session{}).
			Select("tasks.*").
			Joins("JOIN " + quotedName + " AS candidate_sort_keys ON candidate_sort_keys.candidate_id = tasks.uuid").
			Preload("Tags").Preload("Annotations").Preload("Depends").Preload("Assignees").Preload("UDAs").
			Order("candidate_sort_keys.sort_key DESC").Order("tasks.entry ASC").Order("tasks.uuid ASC").
			Limit(limit).Offset(offset)
		return pageQuery.Find(&models).Error
	})
	if err != nil {
		return TaskCandidatePage{}, err
	}
	return r.taskCandidatePage(models, total, limit, offset)
}

func populateTaskCandidateUrgencySortKeys(tx *gorm.DB, opts TaskCandidateListOptions, tableName string) error {
	lastUUID := ""
	for {
		var models []Task
		q := taskCandidateQuery(tx, opts).
			Preload("Tags").Preload("Annotations").Preload("Depends").Preload("UDAs").
			Order("uuid ASC").Limit(candidateSortKeyBatchSize)
		if lastUUID != "" {
			q = q.Where("uuid > ?", lastUUID)
		}
		if err := q.Find(&models).Error; err != nil {
			return err
		}
		if len(models) == 0 {
			return nil
		}
		uuids := make([]string, 0, len(models))
		for _, model := range models {
			uuids = append(uuids, model.UUID)
		}
		blocked, blocking, err := candidateTaskDependencyState(tx, opts.WorkspaceID, opts.NowUnix, models, uuids)
		if err != nil {
			return err
		}
		keys := make([]candidateSortKey, 0, len(models))
		for _, model := range models {
			score := opts.UrgencyScore(fromModel(model, nil, nil), blocked[model.UUID], blocking[model.UUID])
			keys = append(keys, candidateSortKey{CandidateID: model.UUID, SortKey: &score})
		}
		if err := tx.Table(tableName).Create(&keys).Error; err != nil {
			return err
		}
		lastUUID = models[len(models)-1].UUID
	}
}

func candidateTaskDependencyState(tx *gorm.DB, workspaceID string, now int64, models []Task, uuids []string) (map[string]bool, map[string]bool, error) {
	eligible := make(map[string]bool, len(models))
	for _, model := range models {
		eligible[model.UUID] = candidateTaskDependencyEligible(model.Status, model.Until, now)
	}
	var blockedIDs []string
	if err := tx.Table("task_dependencies AS dependencies").
		Select("DISTINCT dependencies.task_uuid").
		Joins("JOIN tasks AS targets ON targets.uuid = dependencies.depends_on").
		Where("dependencies.task_uuid IN ?", uuids).
		Where("targets.workspace_id = ? AND targets.status IN ?", workspaceID, []string{domain.StatusPending, domain.StatusWaiting}).
		Where("targets.until IS NULL OR targets.until > ?", now).
		Pluck("dependencies.task_uuid", &blockedIDs).Error; err != nil {
		return nil, nil, err
	}
	var blockingIDs []string
	if err := tx.Table("task_dependencies AS dependencies").
		Select("DISTINCT dependencies.depends_on").
		Joins("JOIN tasks AS dependents ON dependents.uuid = dependencies.task_uuid").
		Where("dependencies.depends_on IN ?", uuids).
		Where("dependents.workspace_id = ? AND dependents.status IN ?", workspaceID, []string{domain.StatusPending, domain.StatusWaiting}).
		Where("dependents.until IS NULL OR dependents.until > ?", now).
		Pluck("dependencies.depends_on", &blockingIDs).Error; err != nil {
		return nil, nil, err
	}
	blocked := make(map[string]bool, len(blockedIDs))
	for _, id := range blockedIDs {
		blocked[id] = eligible[id]
	}
	blocking := make(map[string]bool, len(blockingIDs))
	for _, id := range blockingIDs {
		blocking[id] = eligible[id]
	}
	return blocked, blocking, nil
}

func candidateTaskDependencyEligible(status string, until *int64, now int64) bool {
	return (status == domain.StatusPending || status == domain.StatusWaiting) && (until == nil || *until > now)
}

func (r *TaskRepository) taskCandidatePage(models []Task, total int64, limit, offset int) (TaskCandidatePage, error) {
	usersByID, err := r.loadAssigneeUsers(models)
	if err != nil {
		return TaskCandidatePage{}, err
	}
	linksByTask, err := r.loadLinksByTask(models)
	if err != nil {
		return TaskCandidatePage{}, err
	}
	items := make([]domain.Task, 0, len(models))
	for _, model := range models {
		items = append(items, fromModel(model, usersByID, linksByTask))
	}
	return TaskCandidatePage{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

func orderTaskCandidateQuery(db *gorm.DB, sortKey string) *gorm.DB {
	switch sortKey {
	case "source":
		db = db.Order("project_seq IS NULL ASC").Order("project_seq ASC")
	case "due":
		db = db.Order("due IS NULL ASC").Order("due ASC")
	case "wait":
		db = db.Order("wait IS NULL ASC").Order("wait ASC")
	case "completed":
		db = db.Order("end_ts IS NULL ASC").Order("end_ts DESC")
	default:
		db = db.Order("entry ASC")
	}
	return db.Order("uuid ASC")
}

// ListDependents 返回依赖指定任务（depends_on = taskUUID）的活任务列表，
// 即被 taskUUID 阻塞的任务。用于任务详情页的反向关系展示。
func (r *TaskRepository) ListDependents(workspaceID, taskUUID string) ([]domain.Task, error) {
	var depRows []TaskDependency
	if err := r.db.
		Where("depends_on = ?", taskUUID).
		Find(&depRows).Error; err != nil {
		return nil, err
	}
	if len(depRows) == 0 {
		return nil, nil
	}
	dependentUUIDs := make([]string, 0, len(depRows))
	for _, row := range depRows {
		dependentUUIDs = append(dependentUUIDs, row.TaskUUID)
	}
	return r.ListByUUIDs(workspaceID, dependentUUIDs)
}

func (r *TaskRepository) GetByUUID(workspaceID, uuid string) (domain.Task, error) {
	var model Task
	err := r.preloadAssociations().Where("workspace_id = ? AND uuid = ?", workspaceID, uuid).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Task{}, ErrNotFound
	}
	if err != nil {
		return domain.Task{}, err
	}
	usersByID, err := r.loadAssigneeUsers([]Task{model})
	if err != nil {
		return domain.Task{}, err
	}
	linksByTask, err := r.loadLinksByTask([]Task{model})
	if err != nil {
		return domain.Task{}, err
	}
	return fromModel(model, usersByID, linksByTask), nil
}

// ListByUUIDs 批量按 UUID 返回任务（仅限当前 workspace，不含 deleted）。
// 用于解析 depends/parent 等任务引用的可读信息。找不到的 UUID 不会出现在结果中。
func (r *TaskRepository) ListByUUIDs(workspaceID string, uuids []string) ([]domain.Task, error) {
	if len(uuids) == 0 {
		return nil, nil
	}
	var models []Task
	if err := r.preloadAssociations().
		Where("workspace_id = ? AND uuid IN ? AND status <> ?", workspaceID, uuids, domain.StatusDeleted).
		Find(&models).Error; err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return nil, nil
	}
	usersByID, err := r.loadAssigneeUsers(models)
	if err != nil {
		return nil, err
	}
	linksByTask, err := r.loadLinksByTask(models)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Task, 0, len(models))
	for _, model := range models {
		out = append(out, fromModel(model, usersByID, linksByTask))
	}
	return out, nil
}

func (r *TaskRepository) GetByProjectSeq(workspaceID, projectID string, seq int64) (domain.Task, error) {
	var model Task
	err := r.preloadAssociations().
		Where("workspace_id = ? AND project_id = ? AND project_seq = ?", workspaceID, projectID, seq).
		First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Task{}, ErrNotFound
	}
	if err != nil {
		return domain.Task{}, err
	}
	usersByID, err := r.loadAssigneeUsers([]Task{model})
	if err != nil {
		return domain.Task{}, err
	}
	linksByTask, err := r.loadLinksByTask([]Task{model})
	if err != nil {
		return domain.Task{}, err
	}
	return fromModel(model, usersByID, linksByTask), nil
}

func (r *TaskRepository) Update(tsk domain.Task) error {
	if err := tsk.Validate(); err != nil {
		return err
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		model := toModel(tsk)
		if err := tx.Model(&Task{}).Where("uuid = ? AND workspace_id = ?", tsk.UUID, tsk.WorkspaceID).Updates(map[string]any{
			"title":                     model.Title,
			"description":               model.Description,
			"status":                    model.Status,
			"modified":                  model.Modified,
			"end_ts":                    model.EndTS,
			"due":                       model.Due,
			"project":                   model.Project,
			"project_id":                model.ProjectID,
			"project_seq":               model.ProjectSeq,
			"priority":                  model.Priority,
			"start":                     model.Start,
			"wait":                      model.Wait,
			"scheduled":                 model.Scheduled,
			"until":                     model.Until,
			"parent":                    model.Parent,
			"series_id":                 model.SeriesID,
			"recurrence_at":             model.RecurrenceAt,
			"recurrence_rule_snapshot":  model.RecurrenceRuleSnapshot,
			"recurrence_overrides_json": model.RecurrenceOverridesJSON,
		}).Error; err != nil {
			return err
		}
		if err := tx.Where("task_uuid = ?", tsk.UUID).Delete(&TaskTag{}).Error; err != nil {
			return err
		}
		for _, tag := range sortedUnique(tsk.Tags) {
			if err := tx.Create(&TaskTag{TaskUUID: tsk.UUID, Tag: tag}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("task_uuid = ?", tsk.UUID).Delete(&TaskAnnotation{}).Error; err != nil {
			return err
		}
		for _, a := range tsk.Annotations {
			if a.ID == "" {
				a.ID = uuid.NewString()
			}
			if err := tx.Create(&TaskAnnotation{ID: a.ID, TaskUUID: tsk.UUID, Entry: a.Entry, Description: a.Description}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("task_uuid = ?", tsk.UUID).Delete(&TaskDependency{}).Error; err != nil {
			return err
		}
		for _, d := range sortedUnique(tsk.Depends) {
			if err := tx.Create(&TaskDependency{TaskUUID: tsk.UUID, DependsOn: d}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("task_uuid = ?", tsk.UUID).Delete(&TaskAssignee{}).Error; err != nil {
			return err
		}
		for _, userID := range sortedUniqueAssigneeUserIDs(tsk.Assignees) {
			if err := tx.Create(&TaskAssignee{TaskUUID: tsk.UUID, UserID: userID}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("workspace_id = ? AND task_uuid = ?", tsk.WorkspaceID, tsk.UUID).Delete(&TaskUDAValue{}).Error; err != nil {
			return err
		}
		for name, value := range tsk.UDAs {
			if value.Raw == "" {
				continue
			}
			if err := tx.Create(&TaskUDAValue{
				WorkspaceID: tsk.WorkspaceID,
				TaskUUID:    tsk.UUID,
				Name:        name,
				Value:       value.Raw,
				ValueType:   value.Type,
				Orphan:      value.Orphan,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *TaskRepository) AddAnnotation(workspaceID, taskUUID string, annotation domain.Annotation, modified int64) error {
	if annotation.ID == "" {
		annotation.ID = uuid.NewString()
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&TaskAnnotation{
			ID:          annotation.ID,
			TaskUUID:    taskUUID,
			Entry:       annotation.Entry,
			Description: annotation.Description,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&Task{}).
			Where("workspace_id = ? AND uuid = ?", workspaceID, taskUUID).
			Update("modified", modified).Error
	})
}

func (r *TaskRepository) DeleteAnnotation(workspaceID, taskUUID, annotationID string, modified int64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		result := tx.
			Where("id = ? AND task_uuid = ? AND EXISTS (SELECT 1 FROM tasks WHERE tasks.uuid = task_annotations.task_uuid AND tasks.workspace_id = ?)", annotationID, taskUUID, workspaceID).
			Delete(&TaskAnnotation{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return tx.Model(&Task{}).
			Where("workspace_id = ? AND uuid = ?", workspaceID, taskUUID).
			Update("modified", modified).Error
	})
}

func (r *TaskRepository) UpdateAnnotation(workspaceID, taskUUID, annotationID, description string, modified int64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&TaskAnnotation{}).
			Where("id = ? AND task_uuid = ? AND EXISTS (SELECT 1 FROM tasks WHERE tasks.uuid = task_annotations.task_uuid AND tasks.workspace_id = ?)", annotationID, taskUUID, workspaceID).
			Update("description", description)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return tx.Model(&Task{}).
			Where("workspace_id = ? AND uuid = ?", workspaceID, taskUUID).
			Update("modified", modified).Error
	})
}

var ErrNotFound = errors.New("task not found")

// ListAnnotations 按 entry 倒序分页返回某任务的注解，total 为该任务注解总数。
// 校验注解归属当前 workspace 下的任务，避免跨 workspace 泄漏。
func (r *TaskRepository) ListAnnotations(workspaceID, taskUUID string, offset, limit int) ([]domain.Annotation, int, error) {
	scope := "EXISTS (SELECT 1 FROM tasks WHERE tasks.uuid = task_annotations.task_uuid AND tasks.workspace_id = ?)"
	var total int64
	if err := r.db.Model(&TaskAnnotation{}).
		Where("task_uuid = ? AND "+scope, taskUUID, workspaceID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 20
	}
	var rows []TaskAnnotation
	if err := r.db.
		Where("task_uuid = ? AND "+scope, taskUUID, workspaceID).
		Order("entry DESC").
		Offset(offset).
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]domain.Annotation, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.Annotation{ID: row.ID, Entry: row.Entry, Description: row.Description})
	}
	return out, int(total), nil
}

func (r *TaskRepository) Projects(workspaceID string) ([]string, error) {
	var projects []string
	err := r.db.Model(&Task{}).
		Where("workspace_id = ? AND project IS NOT NULL AND project != ''", workspaceID).
		Distinct("project").
		Order("project ASC").
		Pluck("project", &projects).Error
	return projects, err
}

func (r *TaskRepository) Tags(workspaceID string) ([]string, error) {
	var tags []string
	err := r.db.Model(&TaskTag{}).
		Joins("JOIN tasks ON tasks.uuid = task_tags.task_uuid").
		Where("tasks.workspace_id = ?", workspaceID).
		Distinct("tag").
		Order("tag ASC").
		Pluck("tag", &tags).Error
	return tags, err
}

// SearchTasksByTitleOrSlug 在当前 workspace 按 title 子串匹配实际任务。
//
// 用于 content reference #任务 suggestion：当前项目优先，其次按 modified 倒序、title 升序。
// occurrence_ref 关联的 projected 任务不出现在结果中（spec §5.5）。
// task_slug 在数据库中没有独立列，由 project slug + seq 派生；为避免复杂 join，
// 这里只按 title 匹配，slug 匹配留到 resolve 阶段（直接命中 project_seq）。
func (r *TaskRepository) SearchTasksByTitleOrSlug(workspaceID, query, projectID string, limit int) ([]domain.Task, error) {
	if limit <= 0 {
		limit = 20
	}
	pattern := "%" + query + "%"
	var models []Task
	q := r.preloadAssociations().
		Where("workspace_id = ?", workspaceID).
		Where("series_id IS NULL"). // 排除 projected occurrence（spec §5.5）
		Where("LOWER(title) LIKE LOWER(?)", pattern)
	if projectID != "" {
		// 当前项目优先：通过 CASE 排序实现。
		q = q.Order(fmt.Sprintf("CASE WHEN project_id = '%s' THEN 0 ELSE 1 END", escapeSQLString(projectID)))
	}
	q = q.Order("modified DESC").Order("title ASC").Limit(limit)
	if err := q.Find(&models).Error; err != nil {
		return nil, err
	}
	usersByID, err := r.loadAssigneeUsers(models)
	if err != nil {
		return nil, err
	}
	linksByTask, err := r.loadLinksByTask(models)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Task, 0, len(models))
	for _, model := range models {
		out = append(out, fromModel(model, usersByID, linksByTask))
	}
	return out, nil
}

// escapeSQLString 做最小转义，避免 projectID 在 ORDER BY 表达式里注入。
// projectID 由 server 生成（UUID），但仍做防御性转义。
func escapeSQLString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func (r *TaskRepository) Children(workspaceID, parentUUID string) ([]domain.Task, error) {
	var models []Task
	if err := r.preloadAssociations().
		Where("workspace_id = ? AND parent = ?", workspaceID, parentUUID).
		Order("entry ASC").
		Find(&models).Error; err != nil {
		return nil, err
	}
	usersByID, err := r.loadAssigneeUsers(models)
	if err != nil {
		return nil, err
	}
	linksByTask, err := r.loadLinksByTask(models)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Task, 0, len(models))
	for _, model := range models {
		out = append(out, fromModel(model, usersByID, linksByTask))
	}
	return out, nil
}

func toModel(tsk domain.Task) Task {
	tags := make([]TaskTag, 0, len(tsk.Tags))
	for _, tag := range sortedUnique(tsk.Tags) {
		tags = append(tags, TaskTag{TaskUUID: tsk.UUID, Tag: tag})
	}
	annotations := make([]TaskAnnotation, 0, len(tsk.Annotations))
	for _, a := range tsk.Annotations {
		if a.ID == "" {
			a.ID = uuid.NewString()
		}
		annotations = append(annotations, TaskAnnotation{ID: a.ID, TaskUUID: tsk.UUID, Entry: a.Entry, Description: a.Description})
	}
	depends := make([]TaskDependency, 0, len(tsk.Depends))
	for _, d := range sortedUnique(tsk.Depends) {
		depends = append(depends, TaskDependency{TaskUUID: tsk.UUID, DependsOn: d})
	}
	assignees := make([]TaskAssignee, 0, len(tsk.Assignees))
	for _, userID := range sortedUniqueAssigneeUserIDs(tsk.Assignees) {
		assignees = append(assignees, TaskAssignee{TaskUUID: tsk.UUID, UserID: userID})
	}
	udas := make([]TaskUDAValue, 0, len(tsk.UDAs))
	for name, value := range tsk.UDAs {
		if value.Raw == "" {
			continue
		}
		udas = append(udas, TaskUDAValue{
			WorkspaceID: tsk.WorkspaceID,
			TaskUUID:    tsk.UUID,
			Name:        name,
			Value:       value.Raw,
			ValueType:   value.Type,
			Orphan:      value.Orphan,
		})
	}
	return Task{
		UUID: tsk.UUID, WorkspaceID: tsk.WorkspaceID, Title: tsk.Title, Description: tsk.Description,
		Status: tsk.Status, Entry: tsk.Entry, Modified: tsk.Modified,
		EndTS: tsk.End, Due: tsk.Due, Project: tsk.Project, ProjectID: tsk.ProjectID, ProjectSeq: tsk.ProjectSeq, Priority: tsk.Priority,
		Tags:  tags,
		Start: tsk.Start, Wait: tsk.Wait, Scheduled: tsk.Scheduled, Until: tsk.Until,
		Parent:    tsk.Parent,
		Assignees: assignees, Annotations: annotations, Depends: depends, UDAs: udas,
		SeriesID:                tsk.SeriesID,
		RecurrenceAt:            tsk.RecurrenceAt,
		RecurrenceRuleSnapshot:  tsk.RecurrenceRuleSnapshot,
		RecurrenceOverridesJSON: overridesToJSON(tsk.RecurrenceOverrides),
	}
}

func fromModel(model Task, usersByID map[string]assigneeUserData, linksByTask map[string][]domain.TaskLinkInfo) domain.Task {
	tags := make([]string, 0, len(model.Tags))
	for _, tag := range model.Tags {
		tags = append(tags, tag.Tag)
	}
	sort.Strings(tags)
	annotations := make([]domain.Annotation, 0, len(model.Annotations))
	for _, a := range model.Annotations {
		annotations = append(annotations, domain.Annotation{ID: a.ID, Entry: a.Entry, Description: a.Description})
	}
	sort.Slice(annotations, func(i, j int) bool {
		if annotations[i].Entry != annotations[j].Entry {
			return annotations[i].Entry < annotations[j].Entry
		}
		return annotations[i].Description < annotations[j].Description
	})
	depends := make([]string, 0, len(model.Depends))
	for _, d := range model.Depends {
		depends = append(depends, d.DependsOn)
	}
	sort.Strings(depends)
	assignees := make([]domain.AssigneeInfo, 0, len(model.Assignees))
	for _, assignee := range model.Assignees {
		info := domain.AssigneeInfo{UserID: assignee.UserID}
		if user, ok := usersByID[assignee.UserID]; ok {
			info.Name = user.Name
			info.DisplayName = user.DisplayName
			info.Email = user.Email
			info.ExternalIDs = user.ExternalIDs
		}
		assignees = append(assignees, info)
	}
	domain.SortAssigneeInfos(assignees)
	udas := make(map[string]domain.UDAValue, len(model.UDAs))
	for _, value := range model.UDAs {
		udas[value.Name] = domain.UDAValue{Name: value.Name, Raw: value.Value, Type: value.ValueType, Orphan: value.Orphan}
	}
	return domain.Task{
		UUID: model.UUID, WorkspaceID: model.WorkspaceID, Title: model.Title, Description: model.Description,
		Status: model.Status, Entry: model.Entry, Modified: model.Modified,
		End: model.EndTS, Due: model.Due, Project: model.Project, ProjectID: model.ProjectID, ProjectSeq: model.ProjectSeq, Priority: model.Priority,
		Tags:  tags,
		Start: model.Start, Wait: model.Wait, Scheduled: model.Scheduled, Until: model.Until,
		Parent:    model.Parent,
		Assignees: assignees, Annotations: annotations, Depends: depends,
		Links:                  linksByTask[model.UUID],
		UDAs:                   udas,
		SeriesID:               model.SeriesID,
		RecurrenceAt:           model.RecurrenceAt,
		RecurrenceRuleSnapshot: model.RecurrenceRuleSnapshot,
		RecurrenceOverrides:    overridesFromJSON(model.RecurrenceOverridesJSON),
	}
}

// overridesToJSON 把 override 字段列表序列化为 JSON 文本（spec §7.5）。
// 使用文本 JSON 保持 SQLite/PostgreSQL 一致，不依赖数据库专属 JSON 运算。
func overridesToJSON(in []string) *string {
	normalized := domain.NormalizeRecurrenceOverrides(in)
	data, err := json.Marshal(normalized)
	if err != nil {
		// 理论上不可能：[]string 一定能序列化。回退到空数组保证 NOT NULL。
		empty := "[]"
		return &empty
	}
	s := string(data)
	return &s
}

// overridesFromJSON 反序列化 override JSON 文本。
func overridesFromJSON(in *string) []string {
	if in == nil || *in == "" {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal([]byte(*in), &out); err != nil {
		return []string{}
	}
	return domain.NormalizeRecurrenceOverrides(out)
}

type assigneeUserData struct {
	Name        string
	DisplayName string
	Email       *string
	ExternalIDs []domain.ExternalIDInfo
}

func (r *TaskRepository) loadAssigneeUsers(models []Task) (map[string]assigneeUserData, error) {
	userIDs := make([]string, 0)
	seen := map[string]bool{}
	for _, model := range models {
		for _, assignee := range model.Assignees {
			if assignee.UserID == "" || seen[assignee.UserID] {
				continue
			}
			seen[assignee.UserID] = true
			userIDs = append(userIDs, assignee.UserID)
		}
	}
	if len(userIDs) == 0 {
		return nil, nil
	}
	var users []User
	if err := r.db.Where("id IN ?", userIDs).Find(&users).Error; err != nil {
		return nil, err
	}
	var extIDs []UserExternalID
	if err := r.db.Where("user_id IN ?", userIDs).Find(&extIDs).Error; err != nil {
		return nil, err
	}
	extByUser := make(map[string][]domain.ExternalIDInfo)
	for _, eid := range extIDs {
		extByUser[eid.UserID] = append(extByUser[eid.UserID], domain.ExternalIDInfo{
			Provider:   eid.Provider,
			UserType:   eid.UserType,
			ExternalID: eid.ExternalID,
		})
	}
	usersByID := make(map[string]assigneeUserData, len(users))
	for _, user := range users {
		usersByID[user.ID] = assigneeUserData{
			Name:        user.Name,
			DisplayName: user.DisplayName,
			Email:       user.Email,
			ExternalIDs: extByUser[user.ID],
		}
	}
	return usersByID, nil
}

func (r *TaskRepository) loadLinksByTask(models []Task) (map[string][]domain.TaskLinkInfo, error) {
	taskUUIDs := make([]string, 0, len(models))
	for _, m := range models {
		taskUUIDs = append(taskUUIDs, m.UUID)
	}
	if len(taskUUIDs) == 0 {
		return nil, nil
	}
	linkRepo := r.taskLinkRepo
	linksMap, err := linkRepo.LoadByTaskUUIDs(taskUUIDs)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]domain.TaskLinkInfo, len(linksMap))
	for uuid, links := range linksMap {
		infos := make([]domain.TaskLinkInfo, 0, len(links))
		for _, l := range links {
			infos = append(infos, domain.TaskLinkInfo{
				ID: l.ID, Type: l.Type, URL: l.URL,
				Title: l.Title, CreatedAt: l.CreatedAt,
				CreatedBy: taskLinkActorInfo(l),
			})
		}
		result[uuid] = infos
	}
	return result, nil
}

func taskLinkActorInfo(row TaskLink) domain.ActorInfo {
	actorType := row.CreatedByActorType
	if actorType == "" {
		actorType = "user"
	}
	if actorType == "tenant_access_token" {
		token := domain.TokenActorInfo{
			ID:     derefString(row.CreatedByTokenID),
			Name:   derefString(row.CreatedByTokenName),
			Prefix: derefString(row.CreatedByTokenPrefix),
		}
		return domain.ActorInfo{
			Type:  actorType,
			ID:    token.ID,
			Name:  token.Name,
			Token: &token,
		}
	}
	userID := row.CreatedBy
	if row.CreatedByUserID != nil && *row.CreatedByUserID != "" {
		userID = *row.CreatedByUserID
	}
	return domain.ActorInfo{Type: "user", ID: userID, Name: userID, User: &domain.UserInfo{ID: userID, Name: userID}}
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func sortedUnique(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func sortedUniqueAssigneeUserIDs(values []domain.AssigneeInfo) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value.UserID == "" || seen[value.UserID] {
			continue
		}
		seen[value.UserID] = true
		out = append(out, value.UserID)
	}
	sort.Strings(out)
	return out
}

func isUniqueConstraintError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func IsUniqueConstraintError(err error) bool {
	return isUniqueConstraintError(err)
}
