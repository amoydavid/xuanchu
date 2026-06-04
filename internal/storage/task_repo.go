package storage

import (
	"errors"
	"sort"
	"strings"

	"github.com/dajee/taskg/internal/query"
	domain "github.com/dajee/taskg/internal/task"
	"gorm.io/gorm"
)

type TaskRepository struct {
	db          *gorm.DB
	taskLinkRepo *TaskLinkRepository
}

type ListOptions struct {
	Status         string
	Sort           string
	Query          query.Expr
	NowUnix        int64
	UDADefinitions map[string]string
	Limit          int
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

func (r *TaskRepository) CreateRecurringChild(tsk domain.Task) (domain.Task, bool, error) {
	if tsk.Parent == nil || tsk.Due == nil {
		created, err := r.Create(tsk)
		return created, false, err
	}
	created, err := r.Create(tsk)
	if err == nil {
		return created, false, nil
	}
	if !isUniqueConstraintError(err) {
		return domain.Task{}, false, err
	}
	var model Task
	findErr := r.preloadAssociations().
		Where("workspace_id = ? AND parent = ? AND due = ? AND status IN ?", tsk.WorkspaceID, *tsk.Parent, *tsk.Due, []string{domain.StatusPending, domain.StatusWaiting}).
		First(&model).Error
	if errors.Is(findErr, gorm.ErrRecordNotFound) {
		return domain.Task{}, false, err
	}
	if findErr != nil {
		return domain.Task{}, false, findErr
	}
	usersByID, err := r.loadAssigneeUsers([]Task{model})
	if err != nil {
		return domain.Task{}, false, err
	}
	linksByTask, err := r.loadLinksByTask([]Task{model})
	if err != nil {
		return domain.Task{}, false, err
	}
	return fromModel(model, usersByID, linksByTask), true, nil
}

func (r *TaskRepository) List(workspaceID string, opts ListOptions) ([]domain.Task, error) {
	var models []Task
	q := r.preloadAssociations()
	if opts.Status != "" {
		q = q.Where("status = ?", opts.Status)
	}
	if opts.Query != nil {
		q = ApplyQuery(q, opts.Query, QueryCompileOptions{WorkspaceID: workspaceID, NowUnix: opts.NowUnix, UDADefinitions: opts.UDADefinitions})
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

func (r *TaskRepository) Update(tsk domain.Task) error {
	if err := tsk.Validate(); err != nil {
		return err
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		model := toModel(tsk)
		if err := tx.Model(&Task{}).Where("uuid = ? AND workspace_id = ?", tsk.UUID, tsk.WorkspaceID).Updates(map[string]any{
			"description": model.Description,
			"status":      model.Status,
			"modified":    model.Modified,
			"end_ts":      model.EndTS,
			"due":         model.Due,
			"project":     model.Project,
			"project_id":  model.ProjectID,
			"priority":    model.Priority,
			"start":       model.Start,
			"wait":        model.Wait,
			"scheduled":   model.Scheduled,
			"until":       model.Until,
			"recur":       model.Recur,
			"parent":      model.Parent,
			"mask":        model.Mask,
			"i_mask":      model.IMask,
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
			if err := tx.Create(&TaskAnnotation{TaskUUID: tsk.UUID, Entry: a.Entry, Description: a.Description}).Error; err != nil {
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
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&TaskAnnotation{
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

var ErrNotFound = errors.New("task not found")

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

func (r *TaskRepository) RecurringParents(workspaceID string) ([]domain.Task, error) {
	var models []Task
	if err := r.preloadAssociations().
		Where("workspace_id = ? AND status = ?", workspaceID, domain.StatusRecurring).
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
		annotations = append(annotations, TaskAnnotation{TaskUUID: tsk.UUID, Entry: a.Entry, Description: a.Description})
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
		UUID: tsk.UUID, WorkspaceID: tsk.WorkspaceID, Description: tsk.Description,
		Status: tsk.Status, Entry: tsk.Entry, Modified: tsk.Modified,
		EndTS: tsk.End, Due: tsk.Due, Project: tsk.Project, ProjectID: tsk.ProjectID, Priority: tsk.Priority,
		Tags:  tags,
		Start: tsk.Start, Wait: tsk.Wait, Scheduled: tsk.Scheduled, Until: tsk.Until,
		Recur: tsk.Recur, Parent: tsk.Parent, Mask: tsk.Mask, IMask: tsk.IMask,
		Assignees: assignees, Annotations: annotations, Depends: depends, UDAs: udas,
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
		annotations = append(annotations, domain.Annotation{Entry: a.Entry, Description: a.Description})
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
		UUID: model.UUID, WorkspaceID: model.WorkspaceID, Description: model.Description,
		Status: model.Status, Entry: model.Entry, Modified: model.Modified,
		End: model.EndTS, Due: model.Due, Project: model.Project, ProjectID: model.ProjectID, Priority: model.Priority,
		Tags:  tags,
		Start: model.Start, Wait: model.Wait, Scheduled: model.Scheduled, Until: model.Until,
		Recur: model.Recur, Parent: model.Parent, Mask: model.Mask, IMask: model.IMask,
		Assignees: assignees, Annotations: annotations, Depends: depends,
		Links: linksByTask[model.UUID],
		UDAs: udas,
	}
}

type assigneeUserData struct {
	Name        string
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
			ExternalID: eid.ExternalID,
		})
	}
	usersByID := make(map[string]assigneeUserData, len(users))
	for _, user := range users {
		usersByID[user.ID] = assigneeUserData{
			Name:        user.Name,
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
				CreatedBy: domain.UserInfo{ID: l.CreatedBy},
			})
		}
		result[uuid] = infos
	}
	return result, nil
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
