package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"git.dajee.net/dajee/xuanchu/internal/projecttemplate"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
	"git.dajee.net/dajee/xuanchu/internal/taskseries"
	"git.dajee.net/dajee/xuanchu/internal/uda"
)

var projectTemplateKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{2,31}$`)

type CaptureSelection struct {
	ConfigKeys        []string `json:"config_keys"`
	TaskRefs          []string `json:"task_refs"`
	SeriesRefs        []string `json:"series_refs"`
	AutomationRuleIDs []string `json:"automation_rule_ids"`
}

// SelectionPresence 保留协议层 required 数组是否真实出现，避免 nil/empty 抹平漏传。
type SelectionPresence struct {
	ConfigKeys        bool `json:"config_keys"`
	TaskRefs          bool `json:"task_refs"`
	SeriesRefs        bool `json:"series_refs"`
	AutomationRuleIDs bool `json:"automation_rule_ids"`
}

type TaskRelationResolution struct {
	SourceTaskRef string `json:"source_task_ref"`
	Relation      string `json:"relation"`
	TargetTaskRef string `json:"target_task_ref"`
}

type ContentRefResolution struct {
	SourceKind    string `json:"source_kind"`
	SourceRef     string `json:"source_ref"`
	TargetTaskRef string `json:"target_task_ref"`
}

type TaskDateOverride struct {
	SourceTaskRef string                               `json:"source_task_ref"`
	Field         string                               `json:"field"`
	Value         *projecttemplate.RelativeLocalTimeV1 `json:"value"`
}

type SeriesScheduleOverride struct {
	SourceSeriesRef string                               `json:"source_series_ref"`
	FirstDue        projecttemplate.RelativeLocalTimeV1  `json:"first_due"`
	Until           *projecttemplate.RelativeLocalTimeV1 `json:"until"`
	ClearUntil      bool                                 `json:"clear_until"`
}

type CaptureResolution struct {
	DropParentTaskRefs      []string                 `json:"drop_parent_task_refs"`
	DropDepends             []TaskRelationResolution `json:"drop_depends"`
	DropContentTaskRefs     []ContentRefResolution   `json:"drop_content_task_refs"`
	TaskDateOverrides       []TaskDateOverride       `json:"task_date_overrides"`
	SeriesScheduleOverrides []SeriesScheduleOverride `json:"series_schedule_overrides"`
}

type CaptureInput struct {
	SourceProjectRef   string                     `json:"source_project_ref"`
	AnchorDate         string                     `json:"anchor_date"`
	Selection          CaptureSelection           `json:"selection"`
	SelectionPresence  SelectionPresence          `json:"selection_presence"`
	ConfigPolicies     []CaptureConfigPolicyInput `json:"config_policies,omitempty"`
	Resolution         CaptureResolution          `json:"resolution"`
	ExpectedSourceHash string                     `json:"expected_source_hash"`
}

type CaptureConfigPolicyInput struct {
	Key      string `json:"key"`
	Strategy string `json:"strategy"`
	Required bool   `json:"required,omitempty"`
}

type CreateTemplateInput struct {
	Key         string       `json:"key"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Capture     CaptureInput `json:"capture"`
}

type CaptureIssue struct {
	Code       string         `json:"code"`
	SourceKind string         `json:"source_kind,omitempty"`
	SourceRef  string         `json:"source_ref,omitempty"`
	TargetRef  string         `json:"target_ref,omitempty"`
	User       *task.UserInfo `json:"user,omitempty"`
	Relation   string         `json:"relation,omitempty"`
	Field      string         `json:"field,omitempty"`
	Message    string         `json:"message"`
	userID     string
}

func (issue CaptureIssue) MarshalJSON() ([]byte, error) {
	type wireIssue struct {
		Code       string             `json:"code"`
		SourceKind string             `json:"source_kind,omitempty"`
		SourceRef  string             `json:"source_ref,omitempty"`
		TargetRef  string             `json:"target_ref,omitempty"`
		User       *task.JSONUserInfo `json:"user,omitempty"`
		Relation   string             `json:"relation,omitempty"`
		Field      string             `json:"field,omitempty"`
		Message    string             `json:"message"`
	}
	var user *task.JSONUserInfo
	if issue.User != nil {
		value := task.UserInfoToJSON(*issue.User)
		user = &value
	}
	return json.Marshal(wireIssue{Code: issue.Code, SourceKind: issue.SourceKind, SourceRef: issue.SourceRef, TargetRef: issue.TargetRef, User: user, Relation: issue.Relation, Field: issue.Field, Message: issue.Message})
}

type CapturePreview struct {
	Selection          CaptureSelection             `json:"selection"`
	RequiredConfigKeys []string                     `json:"required_config_keys"`
	SourceHash         string                       `json:"source_hash"`
	Counts             ComponentCounts              `json:"counts"`
	BlockingIssues     []CaptureIssue               `json:"blocking_issues"`
	Warnings           []CaptureIssue               `json:"warnings"`
	Snapshot           *ProjectTemplateSnapshotView `json:"snapshot"`
}

type captureSource struct {
	Project            storage.Project
	Selection          CaptureSelection
	RequiredConfigKeys []string
	Tasks              []task.Task
	Series             []taskseries.Series
	Configs            []storage.ConfigCandidate
	ConfigPolicies     map[string]CaptureConfigPolicyInput
	Automations        []storage.ProjectAutomationRule
	UDADefs            map[string]uda.Definition
	Members            []storage.MemberWithUser
	UserRefs           map[string][]string
}

type capturePlan struct {
	Source        captureSource
	SourceProject storage.Project
	Selection     CaptureSelection
	SourceHash    string
	Snapshot      projecttemplate.Snapshot
	SnapshotJSON  []byte
	SnapshotHash  string
	Issues        []CaptureIssue
	Warnings      []CaptureIssue
	Users         map[string]task.UserInfo
}

func (s *Service) PreviewProjectTemplateCapture(input CaptureInput) (CapturePreview, error) {
	plan, err := s.buildProjectTemplateCapture(input, false)
	if err != nil {
		return CapturePreview{}, err
	}
	return CapturePreview{
		Selection: plan.Selection, RequiredConfigKeys: append([]string{}, plan.Source.RequiredConfigKeys...), SourceHash: plan.SourceHash, Counts: componentCounts(plan.Snapshot),
		BlockingIssues: nonNilCaptureIssues(plan.Issues), Warnings: nonNilCaptureIssues(plan.Warnings),
		Snapshot: projectTemplateSnapshotView(plan.Snapshot, plan.Users),
	}, nil
}

func (s *Service) CreateProjectTemplate(input CreateTemplateInput) (ProjectTemplateView, error) {
	if err := s.rejectProjectTemplateProjectScope(); err != nil {
		return ProjectTemplateView{}, err
	}
	if err := s.Require(PermissionProjectManage); err != nil {
		return ProjectTemplateView{}, err
	}
	key := strings.TrimSpace(input.Key)
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return ProjectTemplateView{}, captureError("project_template_invalid_name", "template name is required")
	}
	description := strings.TrimSpace(input.Description)
	if key == "" {
		sourceProject, err := s.ResolveProject(input.Capture.SourceProjectRef)
		if err != nil {
			return ProjectTemplateView{}, err
		}
		key = sourceProject.Slug
	}
	if !projectTemplateKeyPattern.MatchString(key) {
		return ProjectTemplateView{}, captureError("project_template_key_invalid", "template key must match ^[a-z][a-z0-9-]{2,31}$")
	}
	now := s.clock.Unix()
	var templateID string
	for attempt := 0; attempt < 2; attempt++ {
		templateID = ""
		err := s.withProjectTemplateCaptureAudit(func(tx *Service) ([]AuditEntry, error) {
			if err := tx.beginProjectTemplateCaptureWrite(); err != nil {
				return nil, err
			}
			sourceLocked := false
			if tx.store.Dialect() == "sqlite" {
				if _, err := tx.lockProjectTemplateCaptureSource(input.Capture.SourceProjectRef); err != nil {
					return nil, err
				}
				sourceLocked = true
			}
			existing, getErr := tx.projectTemplateRepo.GetByRef(tx.workspaceID, key)
			if getErr != nil && !errors.Is(getErr, storage.ErrNotFound) {
				return nil, getErr
			}
			var locked *storage.ProjectTemplate
			if getErr == nil {
				row, err := tx.projectTemplateRepo.LockByRef(tx.workspaceID, existing.ID)
				if err != nil {
					return nil, err
				}
				if row.Status == "archived" {
					return nil, captureError("project_template_archived", "archived template cannot accept snapshots")
				}
				locked = &row
			}
			if !sourceLocked {
				if _, err := tx.lockProjectTemplateCaptureSource(input.Capture.SourceProjectRef); err != nil {
					return nil, err
				}
			}
			plan, err := tx.capturePlanForWrite(input.Capture)
			if err != nil {
				return nil, err
			}
			if err := tx.lockProjectTemplateCaptureRows(plan.Source); err != nil {
				return nil, err
			}
			actor := tx.runtime.actorColumns()
			if locked == nil {
				templateID = uuid.NewString()
				template := storage.ProjectTemplate{
					ID: templateID, WorkspaceID: tx.workspaceID, Key: key, Name: name, Description: description, Status: "active",
					CreatedByActorType: actor.Type, CreatedByUserID: actor.UserID, CreatedByTokenID: actor.TokenID,
					CreatedByTokenName: actor.TokenName, CreatedByTokenPrefix: actor.TokenPrefix, CreatedAt: now, ModifiedAt: now,
				}
				if err := tx.projectTemplateRepo.Create(template); err != nil {
					return nil, err
				}
				snapshotID := uuid.NewString()
				snapshot, err := tx.projectTemplateRepo.AppendSnapshotLocked(tx.workspaceID, templateID, captureSnapshotRow(snapshotID, templateID, plan, actor, now))
				if err != nil {
					return nil, err
				}
				payload := captureAuditPayload(templateID, snapshot, plan)
				return []AuditEntry{
					{Action: "project_template.create", WorkspaceID: &tx.workspaceID, TargetType: "project_template", TargetID: templateID, Payload: payload},
					{Action: "project_template.snapshot.create", WorkspaceID: &tx.workspaceID, TargetType: "project_template_snapshot", TargetID: snapshot.ID, Payload: payload},
				}, nil
			}

			templateID = locked.ID
			metadataChanged := locked.Name != name || locked.Description != description
			if metadataChanged {
				if err := tx.projectTemplateRepo.UpdateMetadata(tx.workspaceID, locked.ID, name, description, now); err != nil {
					return nil, err
				}
			}
			entries := []AuditEntry{}
			if metadataChanged {
				entries = append(entries, AuditEntry{
					Action: "project_template.modify", WorkspaceID: &tx.workspaceID, TargetType: "project_template", TargetID: locked.ID,
					Payload: map[string]any{"name_before": locked.Name, "name_after": name, "description_before": locked.Description, "description_after": description},
				})
			}
			versions, err := tx.projectTemplateRepo.ListSnapshots(tx.workspaceID, locked.ID)
			if err != nil {
				return nil, err
			}
			for _, version := range versions {
				if version.SnapshotHash != plan.SnapshotHash {
					continue
				}
				if locked.CurrentSnapshotID != nil && version.ID == *locked.CurrentSnapshotID {
					return entries, nil
				}
				return nil, captureError("project_template_snapshot_invalid", "snapshot duplicates a historical non-current version")
			}
			snapshotID := uuid.NewString()
			snapshot, err := tx.projectTemplateRepo.AppendSnapshotLocked(tx.workspaceID, locked.ID, captureSnapshotRow(snapshotID, locked.ID, plan, actor, now))
			if err != nil {
				return nil, err
			}
			entries = append(entries, AuditEntry{
				Action: "project_template.snapshot.create", WorkspaceID: &tx.workspaceID,
				TargetType: "project_template_snapshot", TargetID: snapshot.ID, Payload: captureAuditPayload(locked.ID, snapshot, plan),
			})
			return entries, nil
		})
		if errors.Is(err, storage.ErrProjectTemplateKeyConflict) && attempt == 0 {
			continue
		}
		if err != nil {
			return ProjectTemplateView{}, err
		}
		return s.ProjectTemplateInfo(templateID, nil)
	}
	return ProjectTemplateView{}, captureError("project_template_concurrency_conflict", "template was saved concurrently; retry the request")
}

func (s *Service) CreateProjectTemplateSnapshot(templateRef string, input CaptureInput) (ProjectTemplateView, error) {
	if err := s.rejectProjectTemplateProjectScope(); err != nil {
		return ProjectTemplateView{}, err
	}
	if err := s.Require(PermissionProjectManage); err != nil {
		return ProjectTemplateView{}, err
	}
	template, err := s.resolveProjectTemplate(templateRef)
	if err != nil {
		return ProjectTemplateView{}, err
	}
	if template.Status == "archived" {
		return ProjectTemplateView{}, captureError("project_template_archived", "archived template cannot accept snapshots")
	}
	now, snapshotID := s.clock.Unix(), uuid.NewString()
	var plan capturePlan
	err = s.withProjectTemplateCaptureAudit(func(tx *Service) ([]AuditEntry, error) {
		if err := tx.beginProjectTemplateCaptureWrite(); err != nil {
			return nil, err
		}
		lockedTemplate, err := tx.lockProjectTemplateForSnapshotAppend(template.ID)
		if err != nil {
			return nil, tx.projectTemplateStorageError(templateRef, err)
		}
		if lockedTemplate.Status == "archived" {
			return nil, captureError("project_template_archived", "archived template cannot accept snapshots")
		}
		if _, err := tx.lockProjectTemplateCaptureSource(input.SourceProjectRef); err != nil {
			return nil, err
		}
		plan, err = tx.capturePlanForWrite(input)
		if err != nil {
			return nil, err
		}
		if err := tx.lockProjectTemplateCaptureRows(plan.Source); err != nil {
			return nil, err
		}
		versions, err := tx.projectTemplateRepo.ListSnapshots(tx.workspaceID, template.ID)
		if err != nil {
			return nil, err
		}
		for _, version := range versions {
			if version.SnapshotHash != plan.SnapshotHash {
				continue
			}
			if lockedTemplate.CurrentSnapshotID != nil && version.ID == *lockedTemplate.CurrentSnapshotID {
				return []AuditEntry{}, nil
			}
			return nil, captureError("project_template_snapshot_invalid", "snapshot duplicates a historical non-current version")
		}
		actor := tx.runtime.actorColumns()
		snapshot, err := tx.projectTemplateRepo.AppendSnapshotLocked(tx.workspaceID, template.ID, captureSnapshotRow(snapshotID, template.ID, plan, actor, now))
		if err != nil {
			return nil, err
		}
		return []AuditEntry{{
			Action: "project_template.snapshot.create", WorkspaceID: &tx.workspaceID,
			TargetType: "project_template_snapshot", TargetID: snapshot.ID, Payload: captureAuditPayload(template.ID, snapshot, plan),
		}}, nil
	})
	if errors.Is(err, storage.ErrProjectTemplateHashConflict) {
		// 并发请求可能在本事务取得 append lock 前已经保存了同一 hash。
		// 只有它此刻确实是 current 才按幂等成功；历史重复仍必须拒绝。
		latest, getErr := s.projectTemplateRepo.GetByRef(s.workspaceID, template.ID)
		if getErr != nil {
			return ProjectTemplateView{}, s.projectTemplateStorageError(templateRef, getErr)
		}
		if latest.CurrentSnapshotID != nil {
			current, currentErr := s.projectTemplateRepo.GetSnapshot(s.workspaceID, template.ID, *latest.CurrentSnapshotID)
			if currentErr == nil && current.SnapshotHash == plan.SnapshotHash {
				return s.ProjectTemplateInfo(template.ID, nil)
			}
			if currentErr != nil {
				return ProjectTemplateView{}, currentErr
			}
		}
		return ProjectTemplateView{}, captureError("project_template_snapshot_invalid", "snapshot duplicates a historical non-current version")
	}
	if err != nil {
		return ProjectTemplateView{}, err
	}
	return s.ProjectTemplateInfo(template.ID, nil)
}

func (s *Service) beginProjectTemplateCaptureWrite() error {
	if s.store.Dialect() == "postgres" {
		return s.store.DB().Exec("SET TRANSACTION ISOLATION LEVEL REPEATABLE READ").Error
	}
	return nil
}

func (s *Service) withProjectTemplateCaptureAudit(fn func(*Service) ([]AuditEntry, error)) error {
	const maxAttempts = 3
	for attempt := 0; attempt < maxAttempts; attempt++ {
		err := s.withAuditEntries(fn)
		if !isProjectTemplateCaptureSerializationFailure(err) {
			return err
		}
	}
	return captureError("project_template_source_changed", "source changed concurrently during capture")
}

func isProjectTemplateCaptureSerializationFailure(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "40001"
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "sqlstate 40001") || strings.Contains(message, "could not serialize access")
}

// lockProjectTemplateCaptureSource 在最终写事务内建立一致的源读取边界。
// SQLite 先取得全库 writer lock；PostgreSQL 固定 Repeatable Read snapshot，
// 并锁住源 Project 行，防止项目本身在 Capture 提交前变化。
func (s *Service) lockProjectTemplateCaptureSource(sourceProjectRef string) (storage.Project, error) {
	db := s.store.DB()
	if db.Dialector.Name() == "sqlite" {
		ref := strings.TrimSpace(sourceProjectRef)
		rows, err := acquireSQLiteCaptureWriterLock(db, "UPDATE projects SET modified_at = modified_at WHERE workspace_id = ? AND (id = ? OR slug = ?)", s.workspaceID, ref, ref)
		if err != nil {
			return storage.Project{}, err
		}
		if rows == 0 {
			return storage.Project{}, captureError("project_not_found", "source project not found")
		}
		return s.ResolveProject(ref)
	}
	project, err := s.ResolveProject(sourceProjectRef)
	if err != nil {
		return storage.Project{}, err
	}
	if db.Dialector.Name() == "postgres" {
		var locked storage.Project
		if err := db.Clauses(clause.Locking{Strength: "SHARE"}).Where("workspace_id = ? AND id = ?", s.workspaceID, project.ID).First(&locked).Error; err != nil {
			return storage.Project{}, err
		}
		return locked, nil
	}
	return project, nil
}

// lockProjectTemplateCaptureRows 在 PostgreSQL 上为 fingerprint 覆盖的持久源行
// 取得 SHARE lock。Repeatable Read 保证多次批量查询来自同一 snapshot；这些锁再
// 保证正常 App 写路径不能在 Snapshot 提交前改掉已经读取的主资源/定义/成员行。
func (s *Service) lockProjectTemplateCaptureRows(source captureSource) error {
	if s.store.Dialect() != "postgres" {
		return nil
	}
	db := s.store.DB()
	lock := clause.Locking{Strength: "SHARE"}
	if len(source.Tasks) > 0 {
		ids := make([]string, 0, len(source.Tasks))
		for _, row := range source.Tasks {
			ids = append(ids, row.UUID)
		}
		var rows []storage.Task
		if err := db.Clauses(lock).Where("workspace_id = ? AND uuid IN ?", s.workspaceID, ids).Find(&rows).Error; err != nil {
			return err
		}
	}
	if len(source.Series) > 0 {
		ids := make([]string, 0, len(source.Series))
		for _, row := range source.Series {
			ids = append(ids, row.ID)
		}
		var rows []storage.TaskSeries
		if err := db.Clauses(lock).Where("workspace_id = ? AND id IN ?", s.workspaceID, ids).Find(&rows).Error; err != nil {
			return err
		}
	}
	if len(source.Configs) > 0 {
		keys := make([]string, 0, len(source.Configs))
		for _, item := range source.Configs {
			keys = append(keys, item.Config.Key)
		}
		var configs []storage.Config
		if err := db.Clauses(lock).Where("workspace_id = ? AND scope = ? AND scope_id = ? AND key IN ?", s.workspaceID, string(storage.ConfigScopeProject), source.Project.ID, keys).Find(&configs).Error; err != nil {
			return err
		}
		var definitions []storage.ConfigDefinition
		if err := db.Clauses(lock).Where("workspace_id = ? AND key IN ?", s.workspaceID, keys).Find(&definitions).Error; err != nil {
			return err
		}
	}
	if len(source.Automations) > 0 {
		ids := make([]string, 0, len(source.Automations))
		for _, row := range source.Automations {
			ids = append(ids, row.ID)
		}
		var rows []storage.ProjectAutomationRule
		if err := db.Clauses(lock).Where("workspace_id = ? AND id IN ?", s.workspaceID, ids).Find(&rows).Error; err != nil {
			return err
		}
	}
	if len(source.UDADefs) > 0 {
		names := make([]string, 0, len(source.UDADefs))
		for name := range source.UDADefs {
			names = append(names, name)
		}
		var rows []storage.UDADefinition
		if err := db.Clauses(lock).Where("workspace_id = ? AND name IN ?", s.workspaceID, names).Find(&rows).Error; err != nil {
			return err
		}
	}
	if len(source.Members) > 0 {
		ids := make([]string, 0, len(source.Members))
		for _, member := range source.Members {
			ids = append(ids, member.Membership.UserID)
		}
		var rows []storage.Membership
		if err := db.Clauses(lock).Where("workspace_id = ? AND user_id IN ?", s.workspaceID, ids).Find(&rows).Error; err != nil {
			return err
		}
	}
	return nil
}

// lockProjectTemplateForSnapshotAppend 在追加事务内先锁住 Template，再读取
// status/current pointer。这样 archive 与 append 的胜负由同一把数据库行锁决定：
// archive 先提交则追加看到 archived 并拒绝；追加先取得锁则其 current 切换先完成。
func (s *Service) lockProjectTemplateForSnapshotAppend(templateID string) (storage.ProjectTemplate, error) {
	db := s.store.DB()
	if db.Dialector.Name() == "sqlite" {
		rows, err := acquireSQLiteCaptureWriterLock(db, "UPDATE project_templates SET modified_at = modified_at WHERE workspace_id = ? AND id = ?", s.workspaceID, templateID)
		if err != nil {
			return storage.ProjectTemplate{}, err
		}
		if rows == 0 {
			return storage.ProjectTemplate{}, storage.ErrNotFound
		}
	}
	query := db.Where("workspace_id = ? AND id = ?", s.workspaceID, templateID)
	if db.Dialector.Name() == "postgres" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var row storage.ProjectTemplate
	if err := query.First(&row).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return storage.ProjectTemplate{}, storage.ErrNotFound
	} else if err != nil {
		return storage.ProjectTemplate{}, err
	}
	return row, nil
}

func acquireSQLiteCaptureWriterLock(db *gorm.DB, statement string, args ...any) (int64, error) {
	const attempts = 200
	for attempt := 0; attempt < attempts; attempt++ {
		result := db.Exec(statement, args...)
		if result.Error == nil {
			return result.RowsAffected, nil
		}
		message := strings.ToLower(result.Error.Error())
		if !strings.Contains(message, "database is locked") && !strings.Contains(message, "database table is locked") && !strings.Contains(message, "sqlite_busy") && !strings.Contains(message, "sqlite_locked") {
			return 0, result.Error
		}
		if attempt+1 == attempts {
			return 0, result.Error
		}
		time.Sleep(10 * time.Millisecond)
	}
	return 0, errors.New("unreachable SQLite capture writer lock retry")
}

func (s *Service) capturePlanForWrite(input CaptureInput) (capturePlan, error) {
	plan, err := s.buildProjectTemplateCapture(input, true)
	if err != nil {
		return capturePlan{}, err
	}
	if len(plan.Issues) > 0 {
		issue := plan.Issues[0]
		return capturePlan{}, captureError(issue.Code, issue.Message)
	}
	return plan, nil
}

func (s *Service) buildProjectTemplateCapture(input CaptureInput, enforceSourceHash bool) (capturePlan, error) {
	if err := s.rejectProjectTemplateProjectScope(); err != nil {
		return capturePlan{}, err
	}
	if err := s.Require(PermissionProjectManage); err != nil {
		return capturePlan{}, err
	}
	if !input.SelectionPresence.ConfigKeys || !input.SelectionPresence.TaskRefs || !input.SelectionPresence.SeriesRefs || !input.SelectionPresence.AutomationRuleIDs {
		return capturePlan{}, captureError("project_template_selection_invalid", "all four selection arrays are required")
	}
	if _, err := time.ParseInLocation("2006-01-02", input.AnchorDate, s.clock.Location()); err != nil {
		return capturePlan{}, captureError("project_template_date_out_of_range", "anchor_date must be YYYY-MM-DD")
	}
	selection := normalizeCaptureSelection(input.Selection)
	if len(selection.TaskRefs) > projecttemplate.DefaultLimits.MaxTasks || len(selection.SeriesRefs) > projecttemplate.DefaultLimits.MaxSeries || len(selection.ConfigKeys) > projecttemplate.DefaultLimits.MaxConfigs || len(selection.AutomationRuleIDs) > projecttemplate.DefaultLimits.MaxAutomations {
		return capturePlan{}, captureError("project_template_selection_invalid", "capture selection exceeds snapshot limits")
	}
	project, err := s.ResolveProject(input.SourceProjectRef)
	if err != nil {
		return capturePlan{}, err
	}
	if err := s.requireCaptureReadPermissions(selection); err != nil {
		return capturePlan{}, err
	}
	source, err := s.loadCaptureSource(project, selection, input.ConfigPolicies)
	if err != nil {
		return capturePlan{}, err
	}
	sourceHash, err := captureSourceHash(source)
	if err != nil {
		return capturePlan{}, err
	}
	if enforceSourceHash && (strings.TrimSpace(input.ExpectedSourceHash) == "" || input.ExpectedSourceHash != sourceHash) {
		return capturePlan{}, captureError("project_template_source_changed", "selected source resources changed after preview")
	}
	snapshot, issues, warnings, err := s.mapCaptureSnapshot(source, strings.TrimSpace(input.AnchorDate), input.Resolution)
	if err != nil {
		return capturePlan{}, err
	}
	persisted, err := projecttemplate.ToV2(snapshot)
	if err != nil {
		return capturePlan{}, mapProjectTemplateDomainError(err)
	}
	raw, snapshotHash, err := projecttemplate.EncodeV2(persisted, projecttemplate.DefaultLimits)
	if err == nil {
		// EncodeV2 是 Snapshot canonicalization 的唯一权威边界。Preview 和最终
		// detail 都从 canonical JSON 解码 typed view，避免预览 raw、落库 normalized。
		snapshot, err = projecttemplate.Decode(raw, projecttemplate.DefaultLimits)
	} else if len(issues) > 0 {
		// attachment/未选正文引用会让候选 Snapshot 暂时不能通过完整校验；仍以
		// EncodeV2 规范化其余字段，并只恢复 trim 后的待处理 description。
		snapshot, err = canonicalizeBlockingCaptureSnapshot(snapshot)
		raw, snapshotHash = nil, ""
	}
	if err != nil {
		return capturePlan{}, mapProjectTemplateDomainError(err)
	}
	userIDs := captureAssigneeIDs(source)
	for _, issue := range append(append([]CaptureIssue{}, issues...), warnings...) {
		if issue.userID != "" {
			userIDs = append(userIDs, issue.userID)
		}
	}
	userIDs = sortedUniqueCapture(userIDs)
	users, err := s.resolveUserInfos(userIDs)
	if err != nil {
		return capturePlan{}, err
	}
	resolveCaptureIssueUsers(issues, users)
	resolveCaptureIssueUsers(warnings, users)
	return capturePlan{Source: source, SourceProject: project, Selection: source.Selection, SourceHash: sourceHash, Snapshot: snapshot, SnapshotJSON: raw, SnapshotHash: snapshotHash, Issues: issues, Warnings: warnings, Users: users}, nil
}

func (s *Service) requireCaptureReadPermissions(selection CaptureSelection) error {
	if err := s.Require(PermissionProjectRead); err != nil {
		return err
	}
	if len(selection.TaskRefs) > 0 || len(selection.SeriesRefs) > 0 {
		if err := s.Require(PermissionTaskRead); err != nil {
			return err
		}
	}
	if len(selection.ConfigKeys) > 0 || len(selection.AutomationRuleIDs) > 0 {
		if err := s.Require(PermissionProjectConfigRead); err != nil {
			return err
		}
	}
	if len(selection.AutomationRuleIDs) > 0 {
		if err := s.Require(PermissionHookRead); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) loadCaptureSource(project storage.Project, selection CaptureSelection, policyInputs []CaptureConfigPolicyInput) (captureSource, error) {
	tasks, err := s.repo.ListByUUIDs(s.workspaceID, selection.TaskRefs)
	if err != nil {
		return captureSource{}, err
	}
	if len(tasks) != len(selection.TaskRefs) {
		return captureSource{}, captureError("project_template_selection_invalid", "one or more selected tasks do not exist or are deleted")
	}
	for _, row := range tasks {
		if row.ProjectID == nil || *row.ProjectID != project.ID || row.SeriesID != nil {
			return captureSource{}, captureError("project_template_selection_invalid", "selected task is outside source project or is an occurrence")
		}
	}
	sort.Slice(tasks, func(i, j int) bool {
		return lessSourceSeq(tasks[i].ProjectSeq, tasks[i].UUID, tasks[j].ProjectSeq, tasks[j].UUID)
	})
	for i := range tasks {
		normalizeCaptureTaskSource(&tasks[i])
	}

	series, err := s.taskSeriesRepo.ListByIDs(s.workspaceID, selection.SeriesRefs)
	if err != nil {
		return captureSource{}, err
	}
	if len(series) != len(selection.SeriesRefs) {
		return captureSource{}, captureError("project_template_selection_invalid", "one or more selected series do not exist")
	}
	for _, row := range series {
		if row.ProjectID != project.ID || (row.Status != taskseries.StatusActive && row.Status != taskseries.StatusEnded && row.Status != taskseries.StatusStopped) {
			return captureSource{}, captureError("project_template_selection_invalid", "selected series is outside source project or invalid")
		}
		if err := validateCaptureSeriesSource(row, s.clock.Location()); err != nil {
			return captureSource{}, captureError("project_template_selection_invalid", "selected series aggregate is invalid")
		}
	}
	sort.Slice(series, func(i, j int) bool {
		return lessSourceSeq(series[i].ProjectSeq, series[i].ID, series[j].ProjectSeq, series[j].ID)
	})

	configs := []storage.ConfigCandidate{}
	if len(selection.ConfigKeys) > 0 {
		configPage, err := s.configRepo.ListCandidatePage(storage.ConfigCandidateListOptions{
			WorkspaceID: s.workspaceID, ProjectID: project.ID, Refs: selection.ConfigKeys, Mode: "all",
		}, len(selection.ConfigKeys), 0)
		if err != nil {
			return captureSource{}, err
		}
		configs = configPage.Items
	}
	if len(configs) != len(selection.ConfigKeys) {
		return captureSource{}, captureError("project_template_config_invalid", "selected config definition is unavailable or does not allow project scope")
	}
	for i := range configs {
		item := &configs[i]
		if item.Definition.Key == "" {
			return captureSource{}, invalidCaptureConfig()
		}
		view, err := configDefinitionViewFromRow(item.Definition)
		if err != nil {
			return captureSource{}, invalidCaptureConfig()
		}
		if item.HasProjectValue {
			normalized, err := s.validateScopedConfigValue(view, storage.ConfigScopeProject, item.Config.Value)
			if err != nil {
				return captureSource{}, invalidCaptureConfig()
			}
			item.Config.Value = normalized
		}
	}

	automations, err := s.projectAutomationRuleRepo.ListByIDs(s.workspaceID, selection.AutomationRuleIDs)
	if err != nil {
		return captureSource{}, err
	}
	if len(automations) != len(selection.AutomationRuleIDs) {
		return captureSource{}, captureError("project_template_selection_invalid", "one or more selected automation rules do not exist")
	}
	for _, row := range automations {
		if row.ProjectID != project.ID {
			return captureSource{}, captureError("project_template_selection_invalid", "selected automation rule is outside source project")
		}
	}
	requiredConfigKeys, err := captureAutomationRequiredConfigKeys(automations)
	if err != nil {
		return captureSource{}, err
	}
	configs, err = s.appendRequiredCaptureConfigs(project, configs, requiredConfigKeys)
	if err != nil {
		return captureSource{}, err
	}
	requiredConfigKeys = captureRequiredExplicitConfigKeys(requiredConfigKeys, configs)
	if len(configs) > projecttemplate.DefaultLimits.MaxConfigs {
		return captureSource{}, captureError("project_template_selection_invalid", "automation dependencies exceed snapshot config limit")
	}
	configPolicies, err := normalizeCaptureConfigPolicies(policyInputs, configs, requiredConfigKeys)
	if err != nil {
		return captureSource{}, err
	}

	udaDefs, err := s.captureUDADefinitions(tasks, series)
	if err != nil {
		return captureSource{}, err
	}
	userRefs := make(map[string][]string)
	for _, row := range tasks {
		refs, err := captureMarkdownUserRefs(row.Description)
		if err != nil {
			return captureSource{}, err
		}
		if len(refs) > 0 {
			userRefs["task\x00"+row.UUID] = refs
		}
	}
	for _, row := range series {
		refs, err := captureMarkdownUserRefs(row.Description)
		if err != nil {
			return captureSource{}, err
		}
		if len(refs) > 0 {
			userRefs["series\x00"+row.ID] = refs
		}
	}
	memberIDs := captureAssigneeIDs(captureSource{Tasks: tasks, Series: series})
	for _, refs := range userRefs {
		memberIDs = append(memberIDs, refs...)
	}
	memberIDs = sortedUniqueCapture(memberIDs)
	members, err := s.memberRepo.ListMembersByUserIDs(s.workspaceID, memberIDs)
	if err != nil {
		return captureSource{}, err
	}
	sort.Slice(members, func(i, j int) bool { return members[i].Membership.UserID < members[j].Membership.UserID })
	normalizedSelection := CaptureSelection{ConfigKeys: make([]string, 0, len(configs)), TaskRefs: make([]string, 0, len(tasks)), SeriesRefs: make([]string, 0, len(series)), AutomationRuleIDs: make([]string, 0, len(automations))}
	for _, row := range configs {
		normalizedSelection.ConfigKeys = append(normalizedSelection.ConfigKeys, row.Definition.Key)
	}
	for _, row := range tasks {
		normalizedSelection.TaskRefs = append(normalizedSelection.TaskRefs, row.UUID)
	}
	for _, row := range series {
		normalizedSelection.SeriesRefs = append(normalizedSelection.SeriesRefs, row.ID)
	}
	for _, row := range automations {
		normalizedSelection.AutomationRuleIDs = append(normalizedSelection.AutomationRuleIDs, row.ID)
	}
	return captureSource{Project: project, Selection: normalizedSelection, RequiredConfigKeys: requiredConfigKeys, Tasks: tasks, Series: series, Configs: configs, ConfigPolicies: configPolicies, Automations: automations, UDADefs: udaDefs, Members: members, UserRefs: userRefs}, nil
}

func normalizeCaptureConfigPolicies(inputs []CaptureConfigPolicyInput, configs []storage.ConfigCandidate, requiredKeys []string) (map[string]CaptureConfigPolicyInput, error) {
	selected := make(map[string]storage.ConfigCandidate, len(configs))
	for _, config := range configs {
		selected[config.Definition.Key] = config
	}
	required := make(map[string]bool, len(requiredKeys))
	for _, key := range requiredKeys {
		required[key] = true
	}
	policies := make(map[string]CaptureConfigPolicyInput, len(configs))
	for _, input := range inputs {
		input.Key, input.Strategy = strings.TrimSpace(input.Key), strings.TrimSpace(input.Strategy)
		if _, ok := selected[input.Key]; !ok {
			return nil, captureError("project_template_config_policy_invalid", "config policy key is not selected")
		}
		if _, duplicate := policies[input.Key]; duplicate {
			return nil, captureError("project_template_config_policy_invalid", "config policy key is duplicated")
		}
		if input.Strategy != "fixed" && input.Strategy != "inherit" && input.Strategy != "prompt" {
			return nil, captureError("project_template_config_policy_invalid", "config policy strategy must be fixed, inherit, or prompt")
		}
		if input.Strategy != "prompt" && input.Required {
			return nil, captureError("project_template_config_policy_invalid", "only prompt config can be marked required")
		}
		if input.Strategy == "prompt" && required[input.Key] && !input.Required {
			return nil, captureError("project_template_config_policy_invalid", "automation dependency prompt must be required")
		}
		policies[input.Key] = input
	}
	for key, config := range selected {
		policy, ok := policies[key]
		if !ok {
			strategy := "inherit"
			if config.HasProjectValue {
				strategy = "fixed"
			}
			policy = CaptureConfigPolicyInput{Key: key, Strategy: strategy}
			policies[key] = policy
		}
		if policy.Strategy == "fixed" && !config.HasProjectValue {
			return nil, captureError("project_template_config_policy_invalid", "fixed config requires an explicit source project value")
		}
		if policy.Strategy == "inherit" && config.HasProjectValue {
			return nil, captureError("project_template_config_policy_invalid", "inherit config requires the source project value to be unset")
		}
	}
	return policies, nil
}

// captureAutomationRequiredConfigKeys 必须与 provider 校验的读取路径一致。它只返回
// automation 确实会读取的 key；继承 workspace/default 的 key 会在后续装载时自然跳过。
func captureAutomationRequiredConfigKeys(rows []storage.ProjectAutomationRule) ([]string, error) {
	keys := make([]string, 0, len(rows)*4)
	for index, row := range rows {
		blueprint, err := captureAutomationBlueprint(row, index+1)
		if err != nil {
			return nil, err
		}
		action := ProjectAutomationActionConfig(blueprint.Action)
		keys = append(keys, action.BaseURLConfigKey, action.APIKeyConfigKey)
		if strings.TrimSpace(action.ModelOverride) == "" {
			keys = append(keys, action.ModelConfigKey)
		}
		allowed := strings.TrimSpace(action.AllowedHostsConfigKey)
		if allowed == "" {
			allowed = "agent.provider.allowed_hosts"
		}
		keys = append(keys, allowed)
	}
	return sortedUniqueCapture(keys), nil
}

func (s *Service) appendRequiredCaptureConfigs(project storage.Project, selected []storage.ConfigCandidate, requiredKeys []string) ([]storage.ConfigCandidate, error) {
	selectedKeys := make(map[string]struct{}, len(selected))
	for _, item := range selected {
		selectedKeys[item.Definition.Key] = struct{}{}
	}
	for _, key := range requiredKeys {
		if _, exists := selectedKeys[key]; exists {
			continue
		}
		value, exists, err := s.configRepo.Get(storage.ConfigKey{WorkspaceID: s.workspaceID, Scope: storage.ConfigScopeProject, ScopeID: project.ID, Key: key})
		if err != nil {
			return nil, err
		}
		if !exists {
			continue // workspace/default 继承不写入项目模板。
		}
		definition, defined, err := s.configDefRepo.Get(s.workspaceID, key)
		if err != nil || !defined {
			return nil, invalidCaptureConfig()
		}
		view, err := configDefinitionViewFromRow(definition)
		if err != nil {
			return nil, invalidCaptureConfig()
		}
		normalized, err := s.validateScopedConfigValue(view, storage.ConfigScopeProject, value)
		if err != nil {
			return nil, invalidCaptureConfig()
		}
		selected = append(selected, storage.ConfigCandidate{
			Config:     storage.Config{WorkspaceID: s.workspaceID, Scope: string(storage.ConfigScopeProject), ScopeID: project.ID, Key: key, Value: normalized},
			Definition: definition, HasProjectValue: true, CanFixed: true, EffectiveSource: "project",
		})
		selectedKeys[key] = struct{}{}
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].Definition.Key < selected[j].Definition.Key })
	return selected, nil
}

func captureRequiredExplicitConfigKeys(required []string, configs []storage.ConfigCandidate) []string {
	explicit := make(map[string]struct{}, len(configs))
	for _, item := range configs {
		explicit[item.Definition.Key] = struct{}{}
	}
	out := make([]string, 0, len(required))
	for _, key := range required {
		if _, ok := explicit[key]; ok {
			out = append(out, key)
		}
	}
	return out
}

func (s *Service) captureUDADefinitions(tasks []task.Task, series []taskseries.Series) (map[string]uda.Definition, error) {
	names := map[string]bool{}
	for _, row := range tasks {
		for name := range row.UDAs {
			names[name] = true
		}
	}
	for _, row := range series {
		for name := range row.UDAs {
			names[name] = true
		}
	}
	defs := make(map[string]uda.Definition, len(names))
	for name := range names {
		def, err := s.udaDefinition(name)
		if err != nil {
			return nil, captureError("project_template_uda_invalid", fmt.Sprintf("UDA %q has no current definition", name))
		}
		defs[name] = def
	}
	for _, row := range tasks {
		for name, value := range row.UDAs {
			if value.Orphan {
				return nil, captureError("project_template_uda_invalid", fmt.Sprintf("UDA %q is orphaned", name))
			}
			if _, err := uda.NormalizeValue(defs[name], value.Raw); err != nil {
				return nil, captureError("project_template_uda_invalid", err.Error())
			}
		}
	}
	for _, row := range series {
		for name, value := range row.UDAs {
			if _, err := uda.NormalizeValue(defs[name], value); err != nil {
				return nil, captureError("project_template_uda_invalid", err.Error())
			}
		}
	}
	return defs, nil
}

func captureSourceHash(source captureSource) (string, error) {
	fingerprint, err := normalizedCaptureSourceFingerprint(source)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(fingerprint)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func validateCaptureSeriesSource(series taskseries.Series, loc *time.Location) error {
	if err := taskseries.ValidateSeries(series); err != nil {
		return err
	}
	switch series.Status {
	case taskseries.StatusStopped:
		if series.StopReason == nil || !validCaptureSeriesStopReason(*series.StopReason) {
			return errors.New("invalid stopped series reason")
		}
	case taskseries.StatusActive:
		if series.StopReason != nil {
			return errors.New("stop reason is only valid for stopped series")
		}
	case taskseries.StatusEnded:
		if series.StopReason != nil || series.Until == nil || series.EffectiveEndAt == nil || *series.EffectiveEndAt != *series.Until {
			return errors.New("ended series must end at until without stop reason")
		}
	}
	// ValidateSeries 负责聚合主体 invariant；RuleVersions 是同一 source aggregate
	// 的一部分，Capture 前也必须完整验证，不能因 v1 只保存 current rule 而跳过。
	if _, err := taskseries.FirstSlotAfter(series.RuleVersions, series.Until, series.FirstDue-1, loc); err != nil {
		return err
	}
	versions := append([]taskseries.RuleVersion(nil), series.RuleVersions...)
	sort.Slice(versions, func(i, j int) bool { return versions[i].EffectiveFrom < versions[j].EffectiveFrom })
	if len(versions) == 0 || versions[0].EffectiveFrom != series.FirstDue || versions[len(versions)-1].RecurrenceRule != series.RecurrenceRule {
		return errors.New("rule version boundaries do not match series")
	}
	return nil
}

func validCaptureSeriesStopReason(reason string) bool {
	switch reason {
	case taskseries.StopReasonUserStopped, taskseries.StopReasonProjectArchived, taskseries.StopReasonProjectCancelled:
		return true
	default:
		return false
	}
}

func invalidCaptureConfig() error {
	// Config validator 的底层错误可能包含 legacy row 的 raw value；Capture 对外只
	// 暴露稳定 code/message，且该错误也不会进入 audit/log payload。
	return captureError("project_template_config_invalid", "selected project config is invalid")
}

func canonicalizeBlockingCaptureSnapshot(snapshot projecttemplate.Snapshot) (projecttemplate.Snapshot, error) {
	surrogate := snapshot
	surrogate.Tasks = append([]projecttemplate.TaskBlueprintV1(nil), snapshot.Tasks...)
	surrogate.Series = append([]projecttemplate.SeriesBlueprintV1(nil), snapshot.Series...)
	for i := range surrogate.Tasks {
		surrogate.Tasks[i].Description = nil
	}
	for i := range surrogate.Series {
		surrogate.Series[i].Description = nil
	}
	persisted, err := projecttemplate.ToV2(surrogate)
	if err != nil {
		return projecttemplate.Snapshot{}, err
	}
	raw, _, err := projecttemplate.EncodeV2(persisted, projecttemplate.DefaultLimits)
	if err != nil {
		return projecttemplate.Snapshot{}, err
	}
	canonical, err := projecttemplate.Decode(raw, projecttemplate.DefaultLimits)
	if err != nil {
		return projecttemplate.Snapshot{}, err
	}
	for i := range canonical.Tasks {
		canonical.Tasks[i].Description = trimStringPointer(snapshot.Tasks[i].Description)
	}
	for i := range canonical.Series {
		canonical.Series[i].Description = trimStringPointer(snapshot.Series[i].Description)
	}
	return canonical, nil
}

type captureSourceFingerprint struct {
	ProjectID          string
	ProjectDescription string
	Selection          CaptureSelection
	Tasks              []captureTaskFingerprint
	Series             []taskseries.Series
	Configs            []captureConfigFingerprint
	Automations        []captureAutomationFingerprint
	UDADefs            map[string]uda.Definition
	Memberships        []storage.Membership
	UserRefs           map[string][]string
}

type captureTaskFingerprint struct {
	UUID                   string
	WorkspaceID            string
	Title                  string
	Description            *string
	Status                 string
	Entry                  int64
	Modified               int64
	End                    *int64
	Due                    *int64
	Project                *string
	ProjectID              *string
	ProjectSeq             *int64
	Priority               *string
	Tags                   []string
	Start                  *int64
	Wait                   *int64
	Scheduled              *int64
	Until                  *int64
	Annotations            []task.Annotation
	Depends                []string
	Parent                 *string
	AssigneeIDs            []string
	Links                  []captureTaskLinkFingerprint
	UDAs                   map[string]task.UDAValue
	SeriesID               *string
	RecurrenceAt           *int64
	RecurrenceRuleSnapshot *string
	RecurrenceOverrides    []string
}

type captureTaskLinkFingerprint struct {
	ID        string
	Type      string
	URL       string
	Title     string
	CreatedAt int64
	ActorType string
	ActorID   string
}

type captureConfigFingerprint struct {
	Key        string
	Value      string
	Definition ConfigDefinitionView
}

type captureAutomationFingerprint struct {
	ID                   string
	WorkspaceID          string
	ProjectID            string
	Enabled              bool
	Definition           projecttemplate.AutomationBlueprintV1
	CreatedByActorType   string
	CreatedByUserID      *string
	CreatedByTokenID     *string
	CreatedByTokenName   *string
	CreatedByTokenPrefix *string
	CreatedAt            int64
	ModifiedAt           int64
}

func normalizedCaptureSourceFingerprint(source captureSource) (captureSourceFingerprint, error) {
	fingerprint := captureSourceFingerprint{
		ProjectID: source.Project.ID, ProjectDescription: strings.TrimSpace(source.Project.Description), Selection: source.Selection,
		Tasks: make([]captureTaskFingerprint, 0, len(source.Tasks)), Series: append([]taskseries.Series{}, source.Series...),
		Configs: make([]captureConfigFingerprint, 0, len(source.Configs)), Automations: make([]captureAutomationFingerprint, 0, len(source.Automations)),
		UDADefs: source.UDADefs, Memberships: make([]storage.Membership, 0, len(source.Members)), UserRefs: source.UserRefs,
	}
	for _, row := range source.Tasks {
		links := make([]captureTaskLinkFingerprint, 0, len(row.Links))
		for _, link := range row.Links {
			links = append(links, captureTaskLinkFingerprint{ID: link.ID, Type: strings.TrimSpace(link.Type), URL: strings.TrimSpace(link.URL), Title: strings.TrimSpace(link.Title), CreatedAt: link.CreatedAt, ActorType: link.CreatedBy.Type, ActorID: link.CreatedBy.ID})
		}
		fingerprint.Tasks = append(fingerprint.Tasks, captureTaskFingerprint{
			UUID: row.UUID, WorkspaceID: row.WorkspaceID, Title: strings.TrimSpace(row.Title), Description: trimStringPointer(row.Description),
			Status: row.Status, Entry: row.Entry, Modified: row.Modified, End: row.End, Due: row.Due, Project: row.Project, ProjectID: row.ProjectID, ProjectSeq: row.ProjectSeq,
			Priority: row.Priority, Tags: row.Tags, Start: row.Start, Wait: row.Wait, Scheduled: row.Scheduled, Until: row.Until,
			Annotations: row.Annotations, Depends: row.Depends, Parent: row.Parent, AssigneeIDs: taskAssigneeIDs(row.Assignees), Links: links, UDAs: row.UDAs,
			SeriesID: row.SeriesID, RecurrenceAt: row.RecurrenceAt, RecurrenceRuleSnapshot: row.RecurrenceRuleSnapshot, RecurrenceOverrides: row.RecurrenceOverrides,
		})
	}
	for _, item := range source.Configs {
		definition, err := configDefinitionViewFromRow(item.Definition)
		if err != nil {
			return captureSourceFingerprint{}, captureError("project_template_config_invalid", err.Error())
		}
		definition.AllowedScopes = sortedUniqueCapture(definition.AllowedScopes)
		fingerprint.Configs = append(fingerprint.Configs, captureConfigFingerprint{Key: item.Config.Key, Value: item.Config.Value, Definition: definition})
	}
	for index, row := range source.Automations {
		definition, err := captureAutomationBlueprint(row, index+1)
		if err != nil {
			return captureSourceFingerprint{}, err
		}
		fingerprint.Automations = append(fingerprint.Automations, captureAutomationFingerprint{
			ID: row.ID, WorkspaceID: row.WorkspaceID, ProjectID: row.ProjectID, Enabled: row.Enabled == nil || *row.Enabled, Definition: definition,
			CreatedByActorType: row.CreatedByActorType, CreatedByUserID: row.CreatedByUserID, CreatedByTokenID: row.CreatedByTokenID,
			CreatedByTokenName: row.CreatedByTokenName, CreatedByTokenPrefix: row.CreatedByTokenPrefix, CreatedAt: row.CreatedAt, ModifiedAt: row.ModifiedAt,
		})
	}
	for _, member := range source.Members {
		fingerprint.Memberships = append(fingerprint.Memberships, member.Membership)
	}
	return fingerprint, nil
}

func trimStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}

func (s *Service) mapCaptureSnapshot(source captureSource, anchorDate string, resolution CaptureResolution) (projecttemplate.Snapshot, []CaptureIssue, []CaptureIssue, error) {
	localTasks := make(map[string]string, len(source.Tasks))
	for i, row := range source.Tasks {
		localTasks[row.UUID] = fmt.Sprintf("task-%d", i+1)
	}
	baseIssues, baseWarnings, err := s.capturePreviewIssues(source, anchorDate, localTasks)
	if err != nil {
		return projecttemplate.Snapshot{}, nil, nil, err
	}
	resolved, err := validateCaptureResolution(resolution, baseIssues, baseWarnings)
	if err != nil {
		return projecttemplate.Snapshot{}, nil, nil, err
	}

	snapshot := projecttemplate.Snapshot{
		Schema: projecttemplate.SnapshotSchemaV2, AnchorDate: anchorDate,
		Project: projecttemplate.ProjectBlueprintV1{Description: source.Project.Description},
		Configs: []projecttemplate.ConfigBlueprint{}, Tasks: []projecttemplate.TaskBlueprintV1{},
		Series: []projecttemplate.SeriesBlueprintV1{}, Automations: []projecttemplate.AutomationBlueprintV1{},
	}
	for _, item := range source.Configs {
		policy := source.ConfigPolicies[item.Definition.Key]
		if policy.Strategy == "prompt" {
			snapshot.Configs = append(snapshot.Configs, projecttemplate.ConfigBlueprint{
				Key: item.Definition.Key, Mode: "prompt", Prompt: &projecttemplate.ConfigPromptV2{Required: policy.Required},
			})
			continue
		}
		if policy.Strategy == "inherit" {
			snapshot.Configs = append(snapshot.Configs, projecttemplate.ConfigBlueprint{Key: item.Definition.Key, Mode: "inherit"})
			continue
		}
		if item.Definition.Secret {
			ciphertext, err := s.encryptProjectTemplateSecret(item.Config.Value)
			if err != nil {
				return projecttemplate.Snapshot{}, nil, nil, err
			}
			snapshot.Configs = append(snapshot.Configs, projecttemplate.ConfigBlueprint{Key: item.Definition.Key, Mode: "secret_copy", SecretCiphertext: &ciphertext})
		} else {
			value := item.Config.Value
			snapshot.Configs = append(snapshot.Configs, projecttemplate.ConfigBlueprint{Key: item.Definition.Key, Mode: "literal", Value: &value})
		}
	}
	for _, row := range source.Tasks {
		droppedContent := resolved.contentDrops["task\x00"+row.UUID]
		description, _, err := rewriteCaptureDescription(row.Description, localTasks, droppedContent)
		if err != nil {
			return projecttemplate.Snapshot{}, nil, nil, mapProjectTemplateDomainError(err)
		}
		blueprint := projecttemplate.TaskBlueprintV1{
			Ref: localTasks[row.UUID], Title: row.Title, Description: description, Priority: row.Priority,
			Tags: append([]string{}, row.Tags...), AssigneeIDs: taskAssigneeIDs(row.Assignees),
			UDAs: captureTaskUDAs(row.UDAs, source.UDADefs), Links: captureTaskLinks(row.Links),
		}
		blueprint.Dates, err = captureTaskDates(row, anchorDate, s.clock.Location(), resolved.taskDates[row.UUID])
		if err != nil {
			return projecttemplate.Snapshot{}, nil, nil, err
		}
		if row.Parent != nil && !resolved.parentDrops[row.UUID] {
			if local, ok := localTasks[*row.Parent]; ok {
				blueprint.ParentRef = &local
			}
		}
		for _, target := range row.Depends {
			if resolved.dependencyDrops[relationKey(row.UUID, "depends", target)] {
				continue
			}
			if local, ok := localTasks[target]; ok {
				blueprint.DependsRefs = append(blueprint.DependsRefs, local)
			}
		}
		snapshot.Tasks = append(snapshot.Tasks, blueprint)
	}
	for i, row := range source.Series {
		droppedContent := resolved.contentDrops["series\x00"+row.ID]
		description, _, err := rewriteCaptureDescription(row.Description, localTasks, droppedContent)
		if err != nil {
			return projecttemplate.Snapshot{}, nil, nil, mapProjectTemplateDomainError(err)
		}
		firstDue, until, err := captureSeriesSchedule(row, anchorDate, s.clock.Location(), resolved.seriesSchedules[row.ID])
		if err != nil {
			return projecttemplate.Snapshot{}, nil, nil, err
		}
		snapshot.Series = append(snapshot.Series, projecttemplate.SeriesBlueprintV1{
			Ref: fmt.Sprintf("series-%d", i+1), Title: row.Title, Description: description, Priority: row.Priority,
			Tags: append([]string{}, row.Tags...), AssigneeIDs: append([]string{}, row.AssigneeIDs...),
			UDAs: captureSeriesUDAs(row.UDAs, source.UDADefs), RecurrenceRule: row.RecurrenceRule, FirstDue: firstDue, Until: until,
		})
	}
	for i, row := range source.Automations {
		blueprint, err := captureAutomationBlueprint(row, i+1)
		if err != nil {
			return projecttemplate.Snapshot{}, nil, nil, err
		}
		snapshot.Automations = append(snapshot.Automations, blueprint)
	}
	unresolved := make([]CaptureIssue, 0, len(baseIssues))
	for _, issue := range baseIssues {
		if !resolved.resolves(issue) {
			unresolved = append(unresolved, issue)
		}
	}
	return snapshot, unresolved, baseWarnings, nil
}

func (s *Service) encryptProjectTemplateSecret(value string) (string, error) {
	ciphertext, err := EncryptConfigSecret(s.tokenSecretKey, value)
	if err == nil {
		return ciphertext, nil
	}
	if errors.Is(err, ErrConfigSecretKeyMissing) || errors.Is(err, ErrConfigSecretKeyInvalid) {
		return "", RuntimeError{Code: "config_secret_key_missing", Message: "config secret key is required to copy template secrets"}
	}
	return "", RuntimeError{Code: "project_template_secret_copy_unavailable", Message: "template secret cannot be encrypted"}
}

func (s *Service) capturePreviewIssues(source captureSource, anchorDate string, localTasks map[string]string) ([]CaptureIssue, []CaptureIssue, error) {
	issues, warnings := []CaptureIssue{}, []CaptureIssue{}
	members := make(map[string]bool, len(source.Members))
	for _, member := range source.Members {
		members[member.Membership.UserID] = true
	}
	for _, row := range source.Tasks {
		for _, assignee := range row.Assignees {
			if !members[assignee.UserID] {
				issues = append(issues, unavailableCaptureMemberIssue("task", row.UUID, assignee.UserID, "task assignee is not a current workspace member"))
			}
		}
		for _, userID := range source.UserRefs["task\x00"+row.UUID] {
			if !members[userID] {
				issue := unavailableCaptureMemberIssue("task", row.UUID, userID, "task content user reference is not a current workspace member")
				issue.Relation = "content"
				issues = append(issues, issue)
			}
		}
		if row.Parent != nil {
			if _, ok := localTasks[*row.Parent]; !ok {
				issues = append(issues, CaptureIssue{Code: "project_template_dependency_missing", SourceKind: "task", SourceRef: row.UUID, TargetRef: *row.Parent, Relation: "parent", Message: "task parent is not selected"})
			}
		}
		for _, target := range row.Depends {
			if _, ok := localTasks[target]; !ok {
				issues = append(issues, CaptureIssue{Code: "project_template_dependency_missing", SourceKind: "task", SourceRef: row.UUID, TargetRef: target, Relation: "depends", Message: "task dependency is not selected"})
			}
		}
		_, refs, err := rewriteCaptureDescription(row.Description, localTasks, nil)
		if err != nil {
			return nil, nil, mapProjectTemplateDomainError(err)
		}
		for _, ref := range refs {
			issues = append(issues, captureReferenceIssue("task", row.UUID, ref))
		}
		for field, value := range taskDateFields(row) {
			if value == nil {
				continue
			}
			relative, err := projecttemplate.ToRelativeLocalTime(*value, anchorDate, s.clock.Location())
			if err != nil {
				return nil, nil, mapProjectTemplateDomainError(err)
			}
			code := "project_template_date_preview"
			if relative.DayOffset < 0 {
				code = "project_template_date_before_anchor"
			}
			warnings = append(warnings, CaptureIssue{Code: code, SourceKind: "task", SourceRef: row.UUID, Field: field, Message: "task date converted relative to anchor date"})
		}
	}
	for _, row := range source.Series {
		for _, userID := range row.AssigneeIDs {
			if !members[userID] {
				issues = append(issues, unavailableCaptureMemberIssue("series", row.ID, userID, "series assignee is not a current workspace member"))
			}
		}
		for _, userID := range source.UserRefs["series\x00"+row.ID] {
			if !members[userID] {
				issue := unavailableCaptureMemberIssue("series", row.ID, userID, "series content user reference is not a current workspace member")
				issue.Relation = "content"
				issues = append(issues, issue)
			}
		}
		_, refs, err := rewriteCaptureDescription(row.Description, localTasks, nil)
		if err != nil {
			return nil, nil, mapProjectTemplateDomainError(err)
		}
		for _, ref := range refs {
			issues = append(issues, captureReferenceIssue("series", row.ID, ref))
		}
		_, _, needsConfirmation, err := defaultCaptureSeriesSchedule(row, anchorDate, s.clock.Location())
		if err != nil {
			return nil, nil, err
		}
		if needsConfirmation {
			issues = append(issues, CaptureIssue{Code: "project_template_series_schedule_confirmation_required", SourceKind: "series", SourceRef: row.ID, Field: "schedule", Message: "ended or stopped series schedule must be confirmed"})
		} else {
			warnings = append(warnings, CaptureIssue{Code: "project_template_series_schedule_preview", SourceKind: "series", SourceRef: row.ID, Field: "schedule", Message: "series schedule converted relative to anchor date"})
			firstDue, _, _, _ := defaultCaptureSeriesSchedule(row, anchorDate, s.clock.Location())
			firstUnix, err := projecttemplate.FromRelativeLocalTime(firstDue, anchorDate, s.clock.Location())
			if err != nil {
				return nil, nil, mapProjectTemplateDomainError(err)
			}
			if row.Until != nil && *row.Until < firstUnix {
				warnings = append(warnings, CaptureIssue{Code: "project_template_series_until_cleared", SourceKind: "series", SourceRef: row.ID, Field: "until", Message: "series until is before the new first due and will be cleared"})
			}
		}
	}
	return uniqueCaptureIssues(issues), uniqueCaptureIssues(warnings), nil
}

type validatedCaptureResolution struct {
	parentDrops     map[string]bool
	dependencyDrops map[string]bool
	contentDrops    map[string]map[string]bool
	taskDates       map[string]map[string]*projecttemplate.RelativeLocalTimeV1
	seriesSchedules map[string]*SeriesScheduleOverride
}

func validateCaptureResolution(input CaptureResolution, issues, warnings []CaptureIssue) (validatedCaptureResolution, error) {
	out := validatedCaptureResolution{parentDrops: map[string]bool{}, dependencyDrops: map[string]bool{}, contentDrops: map[string]map[string]bool{}, taskDates: map[string]map[string]*projecttemplate.RelativeLocalTimeV1{}, seriesSchedules: map[string]*SeriesScheduleOverride{}}
	matching := append(append([]CaptureIssue{}, issues...), warnings...)
	has := func(predicate func(CaptureIssue) bool) bool {
		for _, issue := range matching {
			if predicate(issue) {
				return true
			}
		}
		return false
	}
	for _, source := range input.DropParentTaskRefs {
		source = strings.TrimSpace(source)
		if out.parentDrops[source] || !has(func(issue CaptureIssue) bool {
			return issue.SourceKind == "task" && issue.SourceRef == source && issue.Relation == "parent"
		}) {
			return validatedCaptureResolution{}, invalidCaptureResolution("drop parent does not match a preview issue")
		}
		out.parentDrops[source] = true
	}
	for _, item := range input.DropDepends {
		key := relationKey(item.SourceTaskRef, item.Relation, item.TargetTaskRef)
		if item.Relation != "depends" || out.dependencyDrops[key] || !has(func(issue CaptureIssue) bool {
			return issue.SourceKind == "task" && issue.SourceRef == item.SourceTaskRef && issue.Relation == item.Relation && issue.TargetRef == item.TargetTaskRef
		}) {
			return validatedCaptureResolution{}, invalidCaptureResolution("drop dependency does not match a preview issue")
		}
		out.dependencyDrops[key] = true
	}
	for _, item := range input.DropContentTaskRefs {
		if item.SourceKind != "task" && item.SourceKind != "series" {
			return validatedCaptureResolution{}, invalidCaptureResolution("content source_kind must be task or series")
		}
		key := item.SourceKind + "\x00" + item.SourceRef
		if out.contentDrops[key] == nil {
			out.contentDrops[key] = map[string]bool{}
		}
		if out.contentDrops[key][item.TargetTaskRef] || !has(func(issue CaptureIssue) bool {
			return issue.SourceKind == item.SourceKind && issue.SourceRef == item.SourceRef && issue.Relation == "content" && issue.TargetRef == item.TargetTaskRef && issue.Code == "project_template_dependency_missing"
		}) {
			return validatedCaptureResolution{}, invalidCaptureResolution("drop content reference does not match a preview issue")
		}
		out.contentDrops[key][item.TargetTaskRef] = true
	}
	for _, item := range input.TaskDateOverrides {
		if !validCaptureTaskDateField(item.Field) || !has(func(issue CaptureIssue) bool {
			return issue.SourceKind == "task" && issue.SourceRef == item.SourceTaskRef && issue.Field == item.Field
		}) {
			return validatedCaptureResolution{}, invalidCaptureResolution("task date override does not match a preview issue")
		}
		if out.taskDates[item.SourceTaskRef] == nil {
			out.taskDates[item.SourceTaskRef] = map[string]*projecttemplate.RelativeLocalTimeV1{}
		}
		if _, duplicate := out.taskDates[item.SourceTaskRef][item.Field]; duplicate {
			return validatedCaptureResolution{}, invalidCaptureResolution("duplicate task date override")
		}
		out.taskDates[item.SourceTaskRef][item.Field] = item.Value
	}
	for i := range input.SeriesScheduleOverrides {
		item := input.SeriesScheduleOverrides[i]
		if item.ClearUntil && item.Until != nil {
			return validatedCaptureResolution{}, invalidCaptureResolution("series override cannot set and clear until")
		}
		if _, duplicate := out.seriesSchedules[item.SourceSeriesRef]; duplicate || !has(func(issue CaptureIssue) bool {
			return issue.SourceKind == "series" && issue.SourceRef == item.SourceSeriesRef && issue.Field == "schedule"
		}) {
			return validatedCaptureResolution{}, invalidCaptureResolution("series schedule override does not match a preview issue")
		}
		copyItem := item
		out.seriesSchedules[item.SourceSeriesRef] = &copyItem
	}
	return out, nil
}

func (r validatedCaptureResolution) resolves(issue CaptureIssue) bool {
	switch issue.Relation {
	case "parent":
		return r.parentDrops[issue.SourceRef]
	case "depends":
		return r.dependencyDrops[relationKey(issue.SourceRef, issue.Relation, issue.TargetRef)]
	case "content":
		return r.contentDrops[issue.SourceKind+"\x00"+issue.SourceRef][issue.TargetRef]
	}
	if issue.Code == "project_template_series_schedule_confirmation_required" {
		return r.seriesSchedules[issue.SourceRef] != nil
	}
	return false
}

func captureTaskDates(row task.Task, anchor string, loc *time.Location, overrides map[string]*projecttemplate.RelativeLocalTimeV1) (projecttemplate.TaskDatesV1, error) {
	values := map[string]**projecttemplate.RelativeLocalTimeV1{}
	out := projecttemplate.TaskDatesV1{}
	values["due"], values["wait"], values["scheduled"], values["until"] = &out.Due, &out.Wait, &out.Scheduled, &out.Until
	for field, source := range taskDateFields(row) {
		if override, ok := overrides[field]; ok {
			if override != nil {
				value := *override
				*values[field] = &value
			}
			continue
		}
		if source == nil {
			continue
		}
		relative, err := projecttemplate.ToRelativeLocalTime(*source, anchor, loc)
		if err != nil {
			return projecttemplate.TaskDatesV1{}, mapProjectTemplateDomainError(err)
		}
		*values[field] = &relative
	}
	return out, nil
}

func captureSeriesSchedule(row taskseries.Series, anchor string, loc *time.Location, override *SeriesScheduleOverride) (projecttemplate.RelativeLocalTimeV1, *projecttemplate.RelativeLocalTimeV1, error) {
	if override != nil {
		firstUnix, err := projecttemplate.FromRelativeLocalTime(override.FirstDue, anchor, loc)
		if err != nil {
			return projecttemplate.RelativeLocalTimeV1{}, nil, mapProjectTemplateDomainError(err)
		}
		var until *projecttemplate.RelativeLocalTimeV1
		if !override.ClearUntil && override.Until != nil {
			untilUnix, err := projecttemplate.FromRelativeLocalTime(*override.Until, anchor, loc)
			if err != nil {
				return projecttemplate.RelativeLocalTimeV1{}, nil, mapProjectTemplateDomainError(err)
			}
			if untilUnix < firstUnix {
				return projecttemplate.RelativeLocalTimeV1{}, nil, captureError("project_template_snapshot_invalid", "series until must not precede first_due")
			}
			value := *override.Until
			until = &value
		}
		return override.FirstDue, until, nil
	}
	first, until, _, err := defaultCaptureSeriesSchedule(row, anchor, loc)
	return first, until, err
}

func defaultCaptureSeriesSchedule(row taskseries.Series, anchor string, loc *time.Location) (projecttemplate.RelativeLocalTimeV1, *projecttemplate.RelativeLocalTimeV1, bool, error) {
	anchorStart, err := time.ParseInLocation("2006-01-02", anchor, loc)
	if err != nil {
		return projecttemplate.RelativeLocalTimeV1{}, nil, false, captureError("project_template_date_out_of_range", "invalid anchor date")
	}
	currentAnchor := row.FirstDue
	for _, version := range row.RuleVersions {
		if version.EffectiveFrom > currentAnchor {
			currentAnchor = version.EffectiveFrom
		}
	}
	var sourceUntil *int64
	if row.Status != taskseries.StatusActive {
		sourceUntil = row.Until
	}
	slot, err := taskseries.FirstSlotAfter([]taskseries.RuleVersion{{EffectiveFrom: currentAnchor, RecurrenceRule: row.RecurrenceRule}}, sourceUntil, anchorStart.Unix()-1, loc)
	if err != nil {
		return projecttemplate.RelativeLocalTimeV1{}, nil, false, captureError("project_template_snapshot_invalid", err.Error())
	}
	if slot == nil || (row.Status != taskseries.StatusActive && !slotInRangeForSeriesStatus(row, slot.RecurrenceAt)) {
		return projecttemplate.RelativeLocalTimeV1{DayOffset: 0, LocalTime: "23:59:59"}, nil, true, nil
	}
	first, err := projecttemplate.ToRelativeLocalTime(slot.RecurrenceAt, anchor, loc)
	if err != nil {
		return projecttemplate.RelativeLocalTimeV1{}, nil, false, mapProjectTemplateDomainError(err)
	}
	var until *projecttemplate.RelativeLocalTimeV1
	if row.Until != nil && *row.Until >= slot.RecurrenceAt {
		value, err := projecttemplate.ToRelativeLocalTime(*row.Until, anchor, loc)
		if err != nil {
			return projecttemplate.RelativeLocalTimeV1{}, nil, false, mapProjectTemplateDomainError(err)
		}
		until = &value
	}
	return first, until, false, nil
}

func captureAutomationBlueprint(row storage.ProjectAutomationRule, index int) (projecttemplate.AutomationBlueprintV1, error) {
	if row.ActionType != ProjectAutomationActionOpenAI {
		return projecttemplate.AutomationBlueprintV1{}, captureError("project_template_automation_invalid", "unsupported automation action type")
	}
	var trigger ProjectAutomationTriggerConfig
	var condition ProjectAutomationCondition
	var action ProjectAutomationActionConfig
	var context ProjectAutomationContextConfig
	for _, item := range []struct {
		raw    string
		target any
	}{{row.TriggerConfigJSON, &trigger}, {row.ConditionJSON, &condition}, {row.ActionConfigJSON, &action}, {row.ContextConfigJSON, &context}} {
		if err := decodeStrictCaptureJSON(item.raw, item.target); err != nil {
			return projecttemplate.AutomationBlueprintV1{}, captureError("project_template_automation_invalid", err.Error())
		}
	}
	normalized, err := normalizeProjectAutomationAddInput(ProjectAutomationRuleAddInput{Name: row.Name, Description: row.Description, TriggerType: row.TriggerType, TriggerConfig: trigger, Condition: condition, Action: action, Context: context, InstructionTemplate: row.InstructionTemplate, SystemPrompt: row.SystemPrompt})
	if err != nil {
		return projecttemplate.AutomationBlueprintV1{}, captureError("project_template_automation_invalid", err.Error())
	}
	return projecttemplate.AutomationBlueprintV1{
		Ref: fmt.Sprintf("automation-%d", index), Name: normalized.Name, Description: normalized.Description, TriggerType: normalized.TriggerType,
		TriggerConfig: projecttemplate.AutomationTriggerV1(normalized.TriggerConfig), Condition: projecttemplate.AutomationConditionV1(normalized.Condition),
		Action: projecttemplate.AutomationActionV1(normalized.Action), Context: projecttemplate.AutomationContextV1(normalized.Context),
		InstructionTemplate: normalized.InstructionTemplate, SystemPrompt: normalized.SystemPrompt,
	}, nil
}

func decodeStrictCaptureJSON(raw string, target any) error {
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON documents")
		}
		return err
	}
	return nil
}

func captureTaskUDAs(values map[string]task.UDAValue, defs map[string]uda.Definition) map[string]projecttemplate.UDABlueprintV1 {
	out := make(map[string]projecttemplate.UDABlueprintV1, len(values))
	for name, value := range values {
		normalized, _ := uda.NormalizeValue(defs[name], value.Raw)
		out[name] = projecttemplate.UDABlueprintV1{Raw: normalized, Type: string(defs[name].Type)}
	}
	return out
}

func captureSeriesUDAs(values map[string]string, defs map[string]uda.Definition) map[string]projecttemplate.UDABlueprintV1 {
	out := make(map[string]projecttemplate.UDABlueprintV1, len(values))
	for name, value := range values {
		normalized, _ := uda.NormalizeValue(defs[name], value)
		out[name] = projecttemplate.UDABlueprintV1{Raw: normalized, Type: string(defs[name].Type)}
	}
	return out
}

func normalizeCaptureSelection(input CaptureSelection) CaptureSelection {
	return CaptureSelection{ConfigKeys: sortedUniqueCapture(input.ConfigKeys), TaskRefs: sortedUniqueCapture(input.TaskRefs), SeriesRefs: sortedUniqueCapture(input.SeriesRefs), AutomationRuleIDs: sortedUniqueCapture(input.AutomationRuleIDs)}
}

func normalizeCaptureTaskSource(row *task.Task) {
	row.Tags = sortedUniqueCapture(row.Tags)
	row.Depends = sortedUniqueCapture(row.Depends)
	row.RecurrenceOverrides = task.NormalizeRecurrenceOverrides(row.RecurrenceOverrides)
	sort.Slice(row.Annotations, func(i, j int) bool {
		if row.Annotations[i].Entry != row.Annotations[j].Entry {
			return row.Annotations[i].Entry < row.Annotations[j].Entry
		}
		return row.Annotations[i].ID < row.Annotations[j].ID
	})
	sort.Slice(row.Assignees, func(i, j int) bool { return row.Assignees[i].UserID < row.Assignees[j].UserID })
	for i := range row.Assignees {
		sort.Slice(row.Assignees[i].ExternalIDs, func(left, right int) bool {
			a, b := row.Assignees[i].ExternalIDs[left], row.Assignees[i].ExternalIDs[right]
			if a.Provider != b.Provider {
				return a.Provider < b.Provider
			}
			if a.UserType != b.UserType {
				return a.UserType < b.UserType
			}
			return a.ExternalID < b.ExternalID
		})
	}
	sort.Slice(row.Links, func(i, j int) bool {
		if row.Links[i].CreatedAt != row.Links[j].CreatedAt {
			return row.Links[i].CreatedAt < row.Links[j].CreatedAt
		}
		return row.Links[i].ID < row.Links[j].ID
	})
}

func sortedUniqueCapture(values []string) []string {
	seen, out := map[string]bool{}, []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func lessSourceSeq(left *int64, leftID string, right *int64, rightID string) bool {
	if left == nil {
		return right == nil && leftID < rightID
	}
	if right == nil {
		return true
	}
	if *left != *right {
		return *left < *right
	}
	return leftID < rightID
}

func taskAssigneeIDs(values []task.AssigneeInfo) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value.UserID)
	}
	return out
}

func captureAssigneeIDs(source captureSource) []string {
	ids := []string{}
	for _, row := range source.Tasks {
		ids = append(ids, taskAssigneeIDs(row.Assignees)...)
	}
	for _, row := range source.Series {
		ids = append(ids, row.AssigneeIDs...)
	}
	return sortedUniqueCapture(ids)
}

func captureTaskLinks(values []task.TaskLinkInfo) []projecttemplate.TaskLinkBlueprintV1 {
	out := make([]projecttemplate.TaskLinkBlueprintV1, 0, len(values))
	for _, value := range values {
		out = append(out, projecttemplate.TaskLinkBlueprintV1{Type: value.Type, URL: value.URL, Title: value.Title})
	}
	return out
}

func taskDateFields(row task.Task) map[string]*int64 {
	return map[string]*int64{"due": row.Due, "wait": row.Wait, "scheduled": row.Scheduled, "until": row.Until}
}

func validCaptureTaskDateField(field string) bool {
	switch field {
	case "due", "wait", "scheduled", "until":
		return true
	}
	return false
}

func relationKey(source, relation, target string) string {
	return source + "\x00" + relation + "\x00" + target
}

func sortCaptureIssues(items []CaptureIssue) {
	sort.Slice(items, func(i, j int) bool {
		return captureIssueIdentity(items[i]) < captureIssueIdentity(items[j])
	})
}

func uniqueCaptureIssues(items []CaptureIssue) []CaptureIssue {
	if len(items) == 0 {
		return []CaptureIssue{}
	}
	sortCaptureIssues(items)
	out := items[:0]
	previous := ""
	for _, item := range items {
		key := captureIssueIdentity(item)
		if len(out) > 0 && key == previous {
			continue
		}
		out = append(out, item)
		previous = key
	}
	return out
}

func captureIssueIdentity(item CaptureIssue) string {
	raw, _ := json.Marshal(item)
	return string(raw) + "\x00" + item.userID
}

func nonNilCaptureIssues(items []CaptureIssue) []CaptureIssue {
	if items == nil {
		return []CaptureIssue{}
	}
	return items
}

func unavailableCaptureMemberIssue(sourceKind, sourceRef, userID, message string) CaptureIssue {
	return CaptureIssue{Code: "project_template_member_unavailable", SourceKind: sourceKind, SourceRef: sourceRef, Message: message, userID: userID}
}

func resolveCaptureIssueUsers(items []CaptureIssue, users map[string]task.UserInfo) {
	for i := range items {
		if items[i].userID == "" {
			continue
		}
		user := users[items[i].userID]
		if user.ID == "" {
			user = task.UserInfo{ID: items[i].userID, Name: items[i].userID}
		}
		items[i].User = &user
	}
}

func invalidCaptureResolution(message string) error {
	return captureError("project_template_snapshot_invalid", message)
}

func captureError(code, message string) RuntimeError {
	return RuntimeError{Code: code, Message: message}
}

func mapProjectTemplateDomainError(err error) error {
	if code := projecttemplate.ErrorCode(err); code != "" {
		return captureError(code, err.Error())
	}
	return err
}

func captureSnapshotRow(id, templateID string, plan capturePlan, actor actorColumns, now int64) storage.ProjectTemplateSnapshot {
	return storage.ProjectTemplateSnapshot{ID: id, TemplateID: templateID, SourceProjectID: plan.SourceProject.ID, SnapshotJSON: string(plan.SnapshotJSON), SnapshotHash: plan.SnapshotHash, CreatedByActorType: actor.Type, CreatedByUserID: actor.UserID, CreatedByTokenID: actor.TokenID, CreatedByTokenName: actor.TokenName, CreatedByTokenPrefix: actor.TokenPrefix, CreatedAt: now}
}

func captureAuditPayload(templateID string, snapshot storage.ProjectTemplateSnapshot, plan capturePlan) map[string]any {
	return map[string]any{"template_id": templateID, "snapshot_id": snapshot.ID, "source_project_id": plan.SourceProject.ID, "version": snapshot.Version, "hash": plan.SnapshotHash, "counts": componentCounts(plan.Snapshot)}
}
