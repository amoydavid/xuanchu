package storage

import (
	"encoding/json"
	"errors"
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
