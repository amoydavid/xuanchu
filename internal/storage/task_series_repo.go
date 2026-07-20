package storage

import (
	"errors"
	"sort"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/task"
	"git.dajee.net/dajee/xuanchu/internal/taskseries"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TaskSeriesRepository 负责 task_series 聚合的持久化（spec §7.1）。
type TaskSeriesRepository struct {
	db *gorm.DB
}

// NewTaskSeriesRepository 构造 TaskSeriesRepository。
func NewTaskSeriesRepository(db *gorm.DB) *TaskSeriesRepository {
	return &TaskSeriesRepository{db: db}
}

// TaskSeriesListOptions 是 Series 候选过滤参数。
// 普通列表只使用过滤字段；模板候选的动态 next 排序由 App 注入领域计算器，
// Storage 负责排序键落临时表以及最终 SQL 分页。
type TaskSeriesListOptions struct {
	WorkspaceID      string
	ProjectID        string
	Status           string // active|ended|stopped|all；空或 all 表示不筛选
	Q                string // title/description 大小写不敏感包含
	AssigneeUserID   string // 按负责人 user id 过滤
	Sort             string // next|title|modified|source
	NextRecurrenceAt func(taskseries.Series) *int64
}

type TaskSeriesCandidatePage struct {
	Items  []taskseries.Series
	Total  int64
	Limit  int
	Offset int
}

// Create 在一个事务内创建 series 行、初始 rule version、关联字段。
func (r *TaskSeriesRepository) Create(s taskseries.Series) (taskseries.Series, error) {
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	now := time.Now().Unix()
	if s.CreatedAt == 0 {
		s.CreatedAt = now
	}
	if s.ModifiedAt == 0 {
		s.ModifiedAt = now
	}
	model := seriesToModel(s)
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&model.TaskSeries).Error; err != nil {
			return err
		}
		// 初始 rule version。
		if len(s.RuleVersions) == 0 {
			rv := TaskSeriesRuleVersion{
				ID:             uuid.NewString(),
				SeriesID:       model.TaskSeries.ID,
				EffectiveFrom:  s.FirstDue,
				RecurrenceRule: s.RecurrenceRule,
				CreatedBy:      s.CreatedBy,
				CreatedAt:      s.CreatedAt,
			}
			if err := tx.Create(&rv).Error; err != nil {
				return err
			}
		}
		for _, rv := range model.RuleVersions {
			if rv.ID == "" {
				rv.ID = uuid.NewString()
			}
			rv.SeriesID = model.TaskSeries.ID
			if err := tx.Create(&rv).Error; err != nil {
				return err
			}
		}
		// 关联字段（assignees/tags/udas）。
		for _, a := range model.Assignees {
			if err := tx.Create(&a).Error; err != nil {
				return err
			}
		}
		for _, tg := range model.Tags {
			if err := tx.Create(&tg).Error; err != nil {
				return err
			}
		}
		for _, u := range model.UDAValues {
			if err := tx.Create(&u).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return taskseries.Series{}, err
	}
	return r.Get(s.WorkspaceID, model.TaskSeries.ID)
}

// Get 按 workspace + id 读取 series 含所有关联。
func (r *TaskSeriesRepository) Get(workspaceID, seriesID string) (taskseries.Series, error) {
	var model TaskSeries
	err := r.db.
		Where("id = ? AND workspace_id = ?", seriesID, workspaceID).
		Preload("RuleVersions").
		Preload("Assignees").
		Preload("Tags").
		Preload("UDAValues").
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return taskseries.Series{}, ErrSeriesNotFound
		}
		return taskseries.Series{}, err
	}
	if err := r.enrichProjectSlug(workspaceID, &model); err != nil {
		return taskseries.Series{}, err
	}
	return seriesFromModel(model), nil
}

// ListByIDs 批量读取当前 workspace 内的 Series 聚合。
// 返回值不保证与输入顺序一致，调用方按 source project_seq/ID 做规范排序。
func (r *TaskSeriesRepository) ListByIDs(workspaceID string, seriesIDs []string) ([]taskseries.Series, error) {
	if len(seriesIDs) == 0 {
		return nil, nil
	}
	var models []TaskSeries
	if err := r.db.Where("workspace_id = ? AND id IN ?", workspaceID, seriesIDs).
		Preload("RuleVersions").Preload("Assignees").Preload("Tags").Preload("UDAValues").
		Find(&models).Error; err != nil {
		return nil, err
	}
	if err := enrichProjectSlugs(workspaceID, models, r.db); err != nil {
		return nil, err
	}
	out := make([]taskseries.Series, 0, len(models))
	for _, model := range models {
		out = append(out, seriesFromModel(model))
	}
	return out, nil
}

// GetByProjectSeq 按 workspace + project_id + series project_seq 读取 series（series_slug 解析路径）。
func (r *TaskSeriesRepository) GetByProjectSeq(workspaceID, projectID string, seq int64) (taskseries.Series, error) {
	var model TaskSeries
	err := r.db.
		Where("workspace_id = ? AND project_id = ? AND project_seq = ?", workspaceID, projectID, seq).
		Preload("RuleVersions").
		Preload("Assignees").
		Preload("Tags").
		Preload("UDAValues").
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return taskseries.Series{}, ErrSeriesNotFound
		}
		return taskseries.Series{}, err
	}
	if err := r.enrichProjectSlug(workspaceID, &model); err != nil {
		return taskseries.Series{}, err
	}
	return seriesFromModel(model), nil
}

// enrichProjectSlug 从 projects 表查 slug 回填到 model 的瞬态字段（series_slug 派生用，不落 series 表）。
func (r *TaskSeriesRepository) enrichProjectSlug(workspaceID string, model *TaskSeries) error {
	var project Project
	err := r.db.
		Select("slug").
		Where("workspace_id = ? AND id = ?", workspaceID, model.ProjectID).
		First(&project).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrSeriesNotFound
		}
		return err
	}
	model.ProjectSlugTransient = project.Slug
	return nil
}

// enrichProjectSlugs 批量回填 project slug（避免 N+1）。
func enrichProjectSlugs(workspaceID string, models []TaskSeries, db *gorm.DB) error {
	if len(models) == 0 {
		return nil
	}
	ids := make([]string, 0, len(models))
	for _, m := range models {
		ids = append(ids, m.ProjectID)
	}
	var projects []Project
	if err := db.Select("id, slug").Where("workspace_id = ? AND id IN ?", workspaceID, ids).Find(&projects).Error; err != nil {
		return err
	}
	slugByID := make(map[string]string, len(projects))
	for _, p := range projects {
		slugByID[p.ID] = p.Slug
	}
	for i := range models {
		if slug, ok := slugByID[models[i].ProjectID]; ok {
			models[i].ProjectSlugTransient = slug
		}
	}
	return nil
}

// ListCandidates 返回过滤后的候选 series（含关联），不分页、不排序。
func (r *TaskSeriesRepository) ListCandidates(opts TaskSeriesListOptions) ([]taskseries.Series, error) {
	q := taskSeriesCandidateQuery(r.db, opts).Preload("RuleVersions").Preload("Assignees").Preload("Tags").Preload("UDAValues")
	var models []TaskSeries
	if err := q.Order("id ASC").Find(&models).Error; err != nil {
		return nil, err
	}
	if err := enrichProjectSlugs(opts.WorkspaceID, models, r.db); err != nil {
		return nil, err
	}
	out := make([]taskseries.Series, 0, len(models))
	for _, m := range models {
		out = append(out, seriesFromModel(m))
	}
	return out, nil
}

func taskSeriesCandidateQuery(db *gorm.DB, opts TaskSeriesListOptions) *gorm.DB {
	q := db.Model(&TaskSeries{})
	if opts.WorkspaceID != "" {
		q = q.Where("workspace_id = ?", opts.WorkspaceID)
	}
	if opts.ProjectID != "" {
		q = q.Where("project_id = ?", opts.ProjectID)
	}
	if opts.Status != "" && opts.Status != "all" {
		q = q.Where("status = ?", opts.Status)
	}
	if trimmed := trimSpace(opts.Q); trimmed != "" {
		like := "%" + trimmed + "%"
		q = q.Where("(LOWER(title) LIKE LOWER(?) OR LOWER(COALESCE(description, '')) LIKE LOWER(?))", like, like)
	}
	if opts.AssigneeUserID != "" {
		q = q.Where("id IN (SELECT series_id FROM task_series_assignees WHERE user_id = ?)", opts.AssigneeUserID)
	}
	return q
}

// ListCandidatePage 在数据库中完成 series 候选的筛选、计数与分页。
func (r *TaskSeriesRepository) ListCandidatePage(opts TaskSeriesListOptions, limit, offset int) (TaskSeriesCandidatePage, error) {
	if opts.Sort == "next" {
		return r.listNextCandidatePage(opts, limit, offset)
	}
	base := taskSeriesCandidateQuery(r.db, opts)
	var total int64
	if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return TaskSeriesCandidatePage{}, err
	}
	var models []TaskSeries
	q := base.Session(&gorm.Session{}).Preload("RuleVersions").Preload("Assignees").Preload("Tags").Preload("UDAValues")
	switch opts.Sort {
	case "source":
		q = q.Order("project_seq IS NULL ASC").Order("project_seq ASC")
	case "title":
		q = q.Order("title ASC")
	default:
		q = q.Order("modified_at DESC")
	}
	if err := q.Order("id ASC").Limit(limit).Offset(offset).Find(&models).Error; err != nil {
		return TaskSeriesCandidatePage{}, err
	}
	if err := enrichProjectSlugs(opts.WorkspaceID, models, r.db); err != nil {
		return TaskSeriesCandidatePage{}, err
	}
	out := make([]taskseries.Series, 0, len(models))
	for _, m := range models {
		out = append(out, seriesFromModel(m))
	}
	return TaskSeriesCandidatePage{Items: out, Total: total, Limit: limit, Offset: offset}, nil
}

func (r *TaskSeriesRepository) listNextCandidatePage(opts TaskSeriesListOptions, limit, offset int) (TaskSeriesCandidatePage, error) {
	if opts.NextRecurrenceAt == nil {
		return TaskSeriesCandidatePage{}, errors.New("task series candidate next recurrence calculator is required")
	}
	var total int64
	var models []TaskSeries
	err := withCandidateSortKeyTable(r.db, func(tx *gorm.DB, tableName string) error {
		base := taskSeriesCandidateQuery(tx, opts)
		if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
			return err
		}
		if err := populateTaskSeriesCandidateNextSortKeys(tx, opts, tableName); err != nil {
			return err
		}
		quotedName := quoteCandidateSortKeyTable(tableName)
		q := base.Session(&gorm.Session{}).
			Select("task_series.*").
			Joins("JOIN " + quotedName + " AS candidate_sort_keys ON candidate_sort_keys.candidate_id = task_series.id").
			Preload("RuleVersions").Preload("Assignees").Preload("Tags").Preload("UDAValues").
			Order("candidate_sort_keys.sort_key IS NULL ASC").Order("candidate_sort_keys.sort_key ASC").Order("task_series.id ASC").
			Limit(limit).Offset(offset)
		return q.Find(&models).Error
	})
	if err != nil {
		return TaskSeriesCandidatePage{}, err
	}
	if err := enrichProjectSlugs(opts.WorkspaceID, models, r.db); err != nil {
		return TaskSeriesCandidatePage{}, err
	}
	out := make([]taskseries.Series, 0, len(models))
	for _, model := range models {
		out = append(out, seriesFromModel(model))
	}
	return TaskSeriesCandidatePage{Items: out, Total: total, Limit: limit, Offset: offset}, nil
}

func populateTaskSeriesCandidateNextSortKeys(tx *gorm.DB, opts TaskSeriesListOptions, tableName string) error {
	lastID := ""
	for {
		var models []TaskSeries
		q := taskSeriesCandidateQuery(tx, opts).Preload("RuleVersions").Order("id ASC").Limit(candidateSortKeyBatchSize)
		if lastID != "" {
			q = q.Where("id > ?", lastID)
		}
		if err := q.Find(&models).Error; err != nil {
			return err
		}
		if len(models) == 0 {
			return nil
		}
		keys := make([]candidateSortKey, 0, len(models))
		for _, model := range models {
			next := opts.NextRecurrenceAt(seriesFromModel(model))
			var sortKey *float64
			if next != nil {
				value := float64(*next)
				sortKey = &value
			}
			keys = append(keys, candidateSortKey{CandidateID: model.ID, SortKey: sortKey})
		}
		if err := tx.Table(tableName).Create(&keys).Error; err != nil {
			return err
		}
		lastID = models[len(models)-1].ID
	}
}

// Update 更新 series 行 + 关联字段（不含 rule versions，rule version 追加用 AppendRuleVersion）。
func (r *TaskSeriesRepository) Update(s taskseries.Series) error {
	model := seriesToModel(s)
	model.TaskSeries.ModifiedAt = time.Now().Unix()
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&model.TaskSeries).Error; err != nil {
			return err
		}
		// 重建关联字段（assignees/tags/udas）。
		if err := tx.Where("series_id = ?", model.TaskSeries.ID).Delete(&TaskSeriesAssignee{}).Error; err != nil {
			return err
		}
		for _, a := range model.Assignees {
			if err := tx.Create(&a).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("series_id = ?", model.TaskSeries.ID).Delete(&TaskSeriesTag{}).Error; err != nil {
			return err
		}
		for _, tg := range model.Tags {
			if err := tx.Create(&tg).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("series_id = ?", model.TaskSeries.ID).Delete(&TaskSeriesUDAValue{}).Error; err != nil {
			return err
		}
		for _, u := range model.UDAValues {
			if err := tx.Create(&u).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// AppendRuleVersion 追加新 rule version 段（spec §7.1）。
func (r *TaskSeriesRepository) AppendRuleVersion(seriesID string, rv taskseries.RuleVersion) error {
	if rv.ID == "" {
		rv.ID = uuid.NewString()
	}
	rv.SeriesID = seriesID
	if rv.CreatedAt == 0 {
		rv.CreatedAt = time.Now().Unix()
	}
	model := TaskSeriesRuleVersion{
		ID: rv.ID, SeriesID: rv.SeriesID, EffectiveFrom: rv.EffectiveFrom,
		RecurrenceRule: rv.RecurrenceRule, CreatedBy: rv.CreatedBy, CreatedAt: rv.CreatedAt,
	}
	return r.db.Create(&model).Error
}

// ListActive 返回 active series（scheduler 用，按 id 稳定排序，支持分页）。
func (r *TaskSeriesRepository) ListActive(workspaceID string, limit, offset int) ([]taskseries.Series, error) {
	q := r.db.Model(&TaskSeries{}).Where("status = ?", taskseries.StatusActive).Preload("RuleVersions").Preload("Assignees").Preload("Tags").Preload("UDAValues")
	if workspaceID != "" {
		q = q.Where("workspace_id = ?", workspaceID)
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	var models []TaskSeries
	if err := q.Order("id ASC").Find(&models).Error; err != nil {
		return nil, err
	}
	if err := enrichProjectSlugs(workspaceID, models, r.db); err != nil {
		return nil, err
	}
	out := make([]taskseries.Series, 0, len(models))
	for _, m := range models {
		out = append(out, seriesFromModel(m))
	}
	return out, nil
}

// StopProjectSeries 把项目下所有 active series 标记为 stopped（spec §19）。
// 返回受影响的 series 列表。
func (r *TaskSeriesRepository) StopProjectSeries(workspaceID, projectID string, at int64, reason string) ([]taskseries.Series, error) {
	var affected []TaskSeries
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("workspace_id = ? AND project_id = ? AND status = ?", workspaceID, projectID, taskseries.StatusActive).
			Find(&affected).Error; err != nil {
			return err
		}
		for i := range affected {
			affected[i].Status = taskseries.StatusStopped
			affected[i].EffectiveEndAt = &at
			affected[i].StopReason = &reason
			affected[i].ModifiedAt = at
			if err := tx.Save(&affected[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := enrichProjectSlugs(workspaceID, affected, r.db); err != nil {
		return nil, err
	}
	out := make([]taskseries.Series, 0, len(affected))
	for _, m := range affected {
		out = append(out, seriesFromModel(m))
	}
	return out, nil
}

// --- model mapping ---

type seriesModelBundle struct {
	TaskSeries   TaskSeries
	RuleVersions []TaskSeriesRuleVersion
	Assignees    []TaskSeriesAssignee
	Tags         []TaskSeriesTag
	UDAValues    []TaskSeriesUDAValue
}

func seriesToModel(s taskseries.Series) seriesModelBundle {
	b := seriesModelBundle{
		TaskSeries: TaskSeries{
			ID: s.ID, WorkspaceID: s.WorkspaceID, ProjectID: s.ProjectID, Title: s.Title,
			Description: s.Description, Status: s.Status, RecurrenceRule: s.RecurrenceRule,
			FirstDue: s.FirstDue, Until: s.Until, EffectiveEndAt: s.EffectiveEndAt,
			StopReason: s.StopReason, Priority: s.Priority, ProjectSeq: s.ProjectSeq,
			CreatedBy: s.CreatedBy, CreatedAt: s.CreatedAt, ModifiedAt: s.ModifiedAt,
		},
	}
	for _, rv := range s.RuleVersions {
		b.RuleVersions = append(b.RuleVersions, TaskSeriesRuleVersion{
			ID: rv.ID, SeriesID: s.ID, EffectiveFrom: rv.EffectiveFrom,
			RecurrenceRule: rv.RecurrenceRule, CreatedBy: rv.CreatedBy, CreatedAt: rv.CreatedAt,
		})
	}
	for _, uid := range sortedUniqueStrings(s.AssigneeIDs) {
		b.Assignees = append(b.Assignees, TaskSeriesAssignee{SeriesID: s.ID, UserID: uid})
	}
	for _, tag := range sortedUniqueStrings(s.Tags) {
		b.Tags = append(b.Tags, TaskSeriesTag{SeriesID: s.ID, Tag: tag})
	}
	for name, value := range s.UDAs {
		b.UDAValues = append(b.UDAValues, TaskSeriesUDAValue{
			SeriesID: s.ID, Name: name, Value: value,
		})
	}
	return b
}

func seriesFromModel(m TaskSeries) taskseries.Series {
	s := taskseries.Series{
		ID: m.ID, WorkspaceID: m.WorkspaceID, ProjectID: m.ProjectID, Title: m.Title,
		Description: m.Description, Status: m.Status, RecurrenceRule: m.RecurrenceRule,
		FirstDue: m.FirstDue, Until: m.Until, EffectiveEndAt: m.EffectiveEndAt,
		StopReason: m.StopReason, Priority: m.Priority, ProjectSeq: m.ProjectSeq,
		ProjectSlug: m.ProjectSlugTransient,
		CreatedBy:   m.CreatedBy, CreatedAt: m.CreatedAt, ModifiedAt: m.ModifiedAt,
	}
	rvs := make([]taskseries.RuleVersion, 0, len(m.RuleVersions))
	for _, rv := range m.RuleVersions {
		rvs = append(rvs, taskseries.RuleVersion{
			ID: rv.ID, SeriesID: rv.SeriesID, EffectiveFrom: rv.EffectiveFrom,
			RecurrenceRule: rv.RecurrenceRule, CreatedBy: rv.CreatedBy, CreatedAt: rv.CreatedAt,
		})
	}
	sort.Slice(rvs, func(i, j int) bool { return rvs[i].EffectiveFrom < rvs[j].EffectiveFrom })
	s.RuleVersions = rvs
	assigneeIDs := make([]string, 0, len(m.Assignees))
	for _, a := range m.Assignees {
		assigneeIDs = append(assigneeIDs, a.UserID)
	}
	sort.Strings(assigneeIDs)
	s.AssigneeIDs = assigneeIDs
	tags := make([]string, 0, len(m.Tags))
	for _, t := range m.Tags {
		tags = append(tags, t.Tag)
	}
	sort.Strings(tags)
	s.Tags = tags
	udas := make(map[string]string, len(m.UDAValues))
	for _, u := range m.UDAValues {
		udas[u.Name] = u.Value
	}
	s.UDAs = udas
	return s
}

// --- 错误 ---

// ErrSeriesNotFound 表示 series 不存在或不在 scope 内。
var ErrSeriesNotFound = errors.New("task series not found")

// --- 辅助 ---

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

func sortedUniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// 保持 task 包引用（series 不依赖 task，但 occurrence repo 会用）。
var _ = task.StatusPending
