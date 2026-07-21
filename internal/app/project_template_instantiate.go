package app

import (
	"bytes"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

	"git.dajee.net/dajee/xuanchu/internal/projecttemplate"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
	"git.dajee.net/dajee/xuanchu/internal/taskseries"
	"git.dajee.net/dajee/xuanchu/internal/uda"
)

// ProjectTemplateIssue 是 Capture/Instantiate 协议共用的稳定问题形状。
type ProjectTemplateIssue struct {
	Code      string `json:"code"`
	Severity  string `json:"severity"`
	Component string `json:"component,omitempty"`
	SourceRef string `json:"source_ref,omitempty"`
	TargetRef string `json:"target_ref,omitempty"`
	Relation  string `json:"relation,omitempty"`
	Field     string `json:"field,omitempty"`
	Message   string `json:"message"`
}

// ProjectTemplateValidationError 让最终实例化保留全部 blocking issue。
type ProjectTemplateValidationError struct {
	Issues []ProjectTemplateIssue `json:"issues"`
}

func (e ProjectTemplateValidationError) Error() string {
	if len(e.Issues) == 0 {
		return "project template validation failed"
	}
	return fmt.Sprintf("%s: %s", e.PrimaryCode(), e.Issues[0].Message)
}

func (e ProjectTemplateValidationError) PrimaryCode() string {
	for _, issue := range e.Issues {
		if issue.Severity == "blocking" || issue.Severity == "" {
			return issue.Code
		}
	}
	return "project_template_snapshot_invalid"
}

type InstantiateInput struct {
	SnapshotID           string             `json:"snapshot_id,omitempty"`
	ExpectedHash         string             `json:"expected_snapshot_hash"`
	ProjectSlug          string             `json:"project_slug"`
	ProjectName          string             `json:"project_name"`
	Description          *string            `json:"description,omitempty"`
	StartDate            string             `json:"start_date"`
	SecretInputs         map[string]string  `json:"secret_inputs,omitempty"`
	AssigneeReplacements map[string]*string `json:"assignee_replacements,omitempty"`
}

type CurrentSnapshotInstantiateInput = InstantiateInput

type SecretResolutionView struct {
	Key          string `json:"key"`
	ResolvedFrom string `json:"resolved_from"`
}

type AssigneeIssueView struct {
	User         task.UserInfo `json:"-"`
	AffectedRefs []string      `json:"affected_refs"`
	Resolution   string        `json:"resolution"`
}

func (view AssigneeIssueView) MarshalJSON() ([]byte, error) {
	type wire struct {
		User         task.JSONUserInfo `json:"user"`
		AffectedRefs []string          `json:"affected_refs"`
		Resolution   string            `json:"resolution"`
	}
	refs := append([]string{}, view.AffectedRefs...)
	return json.Marshal(wire{User: task.UserInfoToJSON(view.User), AffectedRefs: refs, Resolution: view.Resolution})
}

type ProjectTemplateProjectPreview struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	StartDate   string `json:"start_date"`
}

type InstantiatePreview struct {
	Template          ProjectTemplateSummaryView         `json:"template"`
	Snapshot          ProjectTemplateSnapshotSummaryView `json:"snapshot"`
	Project           ProjectTemplateProjectPreview      `json:"project"`
	Counts            ComponentCounts                    `json:"counts"`
	SecretResolutions []SecretResolutionView             `json:"secret_resolutions"`
	AssigneeIssues    []AssigneeIssueView                `json:"assignee_issues"`
	Issues            []ProjectTemplateIssue             `json:"issues"`
	Warnings          []ProjectTemplateIssue             `json:"warnings"`
}

type instantiateSnapshotSummaryWire struct {
	ID                 string             `json:"id"`
	Version            int64              `json:"version"`
	Hash               string             `json:"hash"`
	SourceProjectID    string             `json:"source_project_id"`
	Counts             ComponentCounts    `json:"counts"`
	RequiredSecretKeys []string           `json:"required_secret_keys"`
	CreatedBy          task.JSONActorInfo `json:"created_by"`
	CreatedAt          int64              `json:"created_at"`
}

type instantiateTemplateSummaryWire struct {
	ID              string                          `json:"id"`
	Key             string                          `json:"key"`
	Name            string                          `json:"name"`
	Description     string                          `json:"description"`
	Status          string                          `json:"status"`
	CurrentSnapshot *instantiateSnapshotSummaryWire `json:"current_snapshot,omitempty"`
	CreatedBy       task.JSONActorInfo              `json:"created_by"`
	CreatedAt       int64                           `json:"created_at"`
	ModifiedAt      int64                           `json:"modified_at"`
	ArchivedAt      *int64                          `json:"archived_at,omitempty"`
}

func (preview InstantiatePreview) MarshalJSON() ([]byte, error) {
	type wire struct {
		Template          instantiateTemplateSummaryWire `json:"template"`
		Snapshot          instantiateSnapshotSummaryWire `json:"snapshot"`
		Project           ProjectTemplateProjectPreview  `json:"project"`
		Counts            ComponentCounts                `json:"counts"`
		SecretResolutions []SecretResolutionView         `json:"secret_resolutions"`
		AssigneeIssues    []AssigneeIssueView            `json:"assignee_issues"`
		Issues            []ProjectTemplateIssue         `json:"issues"`
		Warnings          []ProjectTemplateIssue         `json:"warnings"`
	}
	return json.Marshal(wire{
		Template: instantiateTemplateSummaryToWire(preview.Template), Snapshot: instantiateSnapshotSummaryToWire(preview.Snapshot),
		Project: preview.Project, Counts: preview.Counts, SecretResolutions: preview.SecretResolutions,
		AssigneeIssues: preview.AssigneeIssues, Issues: preview.Issues, Warnings: preview.Warnings,
	})
}

func instantiateTemplateSummaryToWire(view ProjectTemplateSummaryView) instantiateTemplateSummaryWire {
	out := instantiateTemplateSummaryWire{
		ID: view.ID, Key: view.Key, Name: view.Name, Description: view.Description, Status: view.Status,
		CreatedBy: task.ActorInfoToJSON(view.CreatedBy), CreatedAt: view.CreatedAt, ModifiedAt: view.ModifiedAt, ArchivedAt: view.ArchivedAt,
	}
	if view.CurrentSnapshot != nil {
		current := instantiateSnapshotSummaryToWire(*view.CurrentSnapshot)
		out.CurrentSnapshot = &current
	}
	return out
}

func instantiateSnapshotSummaryToWire(view ProjectTemplateSnapshotSummaryView) instantiateSnapshotSummaryWire {
	return instantiateSnapshotSummaryWire{
		ID: view.ID, Version: view.Version, Hash: view.Hash, SourceProjectID: view.SourceProjectID,
		Counts: view.Counts, RequiredSecretKeys: append([]string{}, view.RequiredSecretKeys...),
		CreatedBy: task.ActorInfoToJSON(view.CreatedBy), CreatedAt: view.CreatedAt,
	}
}

type InstantiateResult struct {
	Project ProjectView     `json:"project"`
	Counts  ComponentCounts `json:"counts"`
}

type plannedTask struct {
	ID, Ref, Title string
	Description    *string
	Priority       *string
	Tags           []string
	AssigneeIDs    []string
	UDAs           map[string]string
	Due            *int64
	Wait           *int64
	Scheduled      *int64
	Until          *int64
	ParentID       *string
	DependsIDs     []string
	Links          []projecttemplate.TaskLinkBlueprintV1
}

type plannedSeries struct {
	ID, Ref, Title, RecurrenceRule string
	Description                    *string
	Priority                       *string
	Tags                           []string
	AssigneeIDs                    []string
	UDAs                           map[string]string
	FirstDue                       int64
	Until                          *int64
}

type plannedAutomation struct {
	ID, Ref string
	Input   ProjectAutomationRuleAddInput
}

// instantiatePlan 已完成所有会随当前 workspace 状态漂移的校验。
// Task 7 只能执行该结构，不得重新从 Snapshot 临时解释字段。
type instantiatePlan struct {
	Template                 storage.ProjectTemplate
	Snapshot                 storage.ProjectTemplateSnapshot
	ProjectSlug, ProjectName string
	Description              string
	ConfigValues             map[string]string
	TaskIDs, SeriesIDs       map[string]string
	Tasks                    []plannedTask
	Series                   []plannedSeries
	Automations              []plannedAutomation
	Preview                  InstantiatePreview
	effectiveConfigValues    map[string]string
	memberReplacements       map[string]*string
}

func (plan instantiatePlan) validationError() error {
	if len(plan.Preview.Issues) == 0 {
		return nil
	}
	return ProjectTemplateValidationError{Issues: append([]ProjectTemplateIssue{}, plan.Preview.Issues...)}
}

func (s *Service) PreviewProjectTemplateInstantiation(templateRef string, input InstantiateInput) (InstantiatePreview, error) {
	plan, err := s.buildInstantiatePlan(templateRef, input, false)
	if err != nil {
		return InstantiatePreview{}, err
	}
	return plan.Preview, nil
}

func (s *Service) InstantiateProjectTemplate(templateRef string, input InstantiateInput) (InstantiateResult, error) {
	return s.instantiateProjectTemplate(templateRef, input, false)
}

func (s *Service) InstantiateCurrentProjectTemplate(templateRef string, input CurrentSnapshotInstantiateInput) (InstantiateResult, error) {
	return s.instantiateProjectTemplate(templateRef, input, true)
}

func (s *Service) instantiateProjectTemplate(templateRef string, input InstantiateInput, currentOnly bool) (InstantiateResult, error) {
	var result InstantiateResult
	err := s.withAuditEntriesAndEvents(func(tx *Service) ([]AuditEntry, []HookEvent, error) {
		// 必须在最终写事务内锁住 Template/current Snapshot 后重建完整 plan；
		// Preview 结果不作为写入依据，active/current pointer 也必须在锁内重查。
		plan, err := tx.buildInstantiatePlanLocked(templateRef, input, currentOnly)
		if err != nil {
			return nil, nil, err
		}
		if err := plan.validationError(); err != nil {
			return nil, nil, err
		}

		project, err := tx.addProjectLocked(plan.ProjectSlug, plan.ProjectName, plan.Description)
		if err != nil {
			return nil, nil, err
		}
		if err := tx.failProjectTemplateInstantiate("project-create"); err != nil {
			return nil, nil, err
		}
		sourcePayload := instantiateSourcePayload(plan)
		entries := []AuditEntry{{
			Action: "project.add", WorkspaceID: &project.WorkspaceID, ProjectID: &project.ID,
			TargetType: "project", TargetID: project.ID, Payload: cloneAnyMap(sourcePayload),
		}}

		for _, key := range sortedMapKeys(plan.ConfigValues) {
			value := plan.ConfigValues[key]
			def, err := tx.scopedConfigDefinition(key)
			if err != nil {
				return nil, nil, err
			}
			normalized, err := tx.validateScopedConfigValue(def, storage.ConfigScopeProject, value)
			if err != nil {
				return nil, nil, err
			}
			if err := tx.configRepo.Set(storage.ConfigKey{WorkspaceID: tx.workspaceID, Scope: storage.ConfigScopeProject, ScopeID: project.ID, Key: key}, normalized); err != nil {
				return nil, nil, err
			}
			payload := cloneAnyMap(sourcePayload)
			payload["key"] = key
			if def.Secret {
				payload["secret"], payload["changed"] = true, true
			} else {
				payload["value"] = normalized
			}
			entries = append(entries, AuditEntry{Action: "project.config.set", WorkspaceID: &project.WorkspaceID, ProjectID: &project.ID, TargetType: "project", TargetID: project.ID, Payload: payload})
		}
		if err := tx.failProjectTemplateInstantiate("config-create"); err != nil {
			return nil, nil, err
		}

		for _, item := range plan.Series {
			seriesInput := AddTaskSeriesInput{
				Title: item.Title, Description: nil, ProjectID: project.ID, RecurrenceRule: item.RecurrenceRule,
				FirstDue: item.FirstDue, Until: item.Until, Priority: item.Priority,
				Assignees: append([]string{}, item.AssigneeIDs...), Tags: append([]string{}, item.Tags...), UDAs: cloneStringMap(item.UDAs), presetID: item.ID,
			}
			if _, err := tx.addTaskSeriesLocked(seriesInput, false); err != nil {
				return nil, nil, err
			}
		}
		if err := tx.failProjectTemplateInstantiate("series-create"); err != nil {
			return nil, nil, err
		}

		changes := make(map[string]projectChange, len(plan.Tasks))
		for _, item := range plan.Tasks {
			projectSlug := project.Slug
			created, change, err := tx.addLockedWithUUID(AddInput{
				Title: item.Title, Project: &projectSlug, Priority: item.Priority, Due: item.Due,
				Assignees: append([]string{}, item.AssigneeIDs...), Wait: item.Wait, Scheduled: item.Scheduled, Until: item.Until,
				Tags: append([]string{}, item.Tags...), UDAs: cloneStringMap(item.UDAs),
			}, item.ID)
			if err != nil {
				return nil, nil, err
			}
			changes[created.UUID] = change
		}
		if err := tx.failProjectTemplateInstantiate("task-shell-create"); err != nil {
			return nil, nil, err
		}

		// Series 可能含指向普通 Task 的前向 ref；Task shell 全部存在后再回填正文。
		for _, item := range plan.Series {
			if item.Description == nil {
				continue
			}
			series, err := tx.taskSeriesRepo.Get(tx.workspaceID, item.ID)
			if err != nil {
				return nil, nil, err
			}
			if _, err := tx.validateDescriptionReferences(nil, item.Description, false); err != nil {
				return nil, nil, err
			}
			series.Description = normalizeOptionalText(item.Description)
			series.ModifiedAt = tx.clock.Unix()
			if err := tx.taskSeriesRepo.Update(series); err != nil {
				return nil, nil, err
			}
		}

		for _, item := range plan.Tasks {
			if _, err := tx.finalizeInstantiatedTask(project, item); err != nil {
				return nil, nil, err
			}
			entry := taskAuditEntry("task.add", item.ID, changes[item.ID])
			entry.Payload = mergeAnyMaps(entry.Payload, sourcePayload)
			entries = append(entries, entry)
		}
		if err := tx.failProjectTemplateInstantiate("task-finalize"); err != nil {
			return nil, nil, err
		}

		for _, item := range plan.Tasks {
			for _, link := range item.Links {
				created, _, err := tx.addLinkLocked(item.ID, link.Type, link.URL, link.Title)
				if err != nil {
					return nil, nil, err
				}
				payload := mergeAnyMaps(map[string]any{"link_id": created.ID, "type": created.Type, "url": created.URL}, sourcePayload)
				entries = append(entries, AuditEntry{Action: "task.link.add", ProjectID: &project.ID, TargetType: "task", TargetID: item.ID, Payload: payload})
			}
		}
		if err := tx.failProjectTemplateInstantiate("link-create"); err != nil {
			return nil, nil, err
		}

		for _, item := range plan.Automations {
			automationInput := item.Input
			automationInput.Enabled = false
			automationInput.presetID = item.ID
			if _, err := tx.addProjectAutomationRuleLocked(project, automationInput); err != nil {
				return nil, nil, err
			}
		}
		if err := tx.failProjectTemplateInstantiate("automation-create"); err != nil {
			return nil, nil, err
		}

		projectView, err := tx.projectViewForRow(project)
		if err != nil {
			return nil, nil, err
		}
		result = InstantiateResult{Project: projectView, Counts: plan.Preview.Counts}
		aggregate := mergeAnyMaps(sourcePayload, map[string]any{
			"template_id": plan.Template.ID, "snapshot_id": plan.Snapshot.ID,
			"snapshot_hash": plan.Snapshot.SnapshotHash, "counts": plan.Preview.Counts,
		})
		entries = append(entries, AuditEntry{Action: "project_template.instantiate", WorkspaceID: &project.WorkspaceID, ProjectID: &project.ID, TargetType: "project", TargetID: project.ID, Payload: aggregate})

		events := make([]HookEvent, 0, len(plan.Tasks))
		for _, item := range plan.Tasks {
			created, err := tx.repo.GetByUUID(tx.workspaceID, item.ID)
			if err != nil {
				return nil, nil, err
			}
			createdEvent := buildTaskHookEvent("task.created", created, tx.runtime, tx.clock.Unix())
			addProjectTemplateSourceMetadata(&createdEvent, plan.Template.ID, plan.Snapshot.ID, plan.Snapshot.SnapshotHash)
			events = append(events, createdEvent)
			events = append(events, tx.detectBlockedEventsAfterAdd(created)...)
			if mentionEvent, ok := tx.buildUserMentionedEventIfNeeded(task.Task{}, created, tx.clock.Unix()); ok {
				events = append(events, mentionEvent)
			}
		}
		if err := tx.failProjectTemplateInstantiate("audit-write"); err != nil {
			return nil, nil, err
		}
		return entries, events, nil
	})
	if err != nil {
		return InstantiateResult{}, err
	}
	return result, nil
}

func (s *Service) finalizeInstantiatedTask(project storage.Project, item plannedTask) (task.Task, error) {
	row, err := s.repo.GetByUUID(s.workspaceID, item.ID)
	if err != nil {
		return task.Task{}, err
	}
	before := row
	if _, err := s.validateDescriptionReferences(nil, item.Description, true); err != nil {
		return task.Task{}, err
	}
	row.Description = normalizeOptionalText(item.Description)
	if item.ParentID != nil {
		projectSlug := project.Slug
		parent, _, err := s.resolveParentForAdd(item.ParentID, &projectSlug)
		if err != nil {
			return task.Task{}, err
		}
		row.Parent = parent
	}
	row.Depends, err = s.resolveDependencyTargets(item.DependsIDs)
	if err != nil {
		return task.Task{}, err
	}
	if err := s.validateDependencyCycles(row.UUID, row.Depends); err != nil {
		return task.Task{}, err
	}
	row.Modified = s.clock.Unix()
	if err := s.repo.Update(row); err != nil {
		return task.Task{}, err
	}
	if err := s.validateAndBindDescriptionAttachments(before, row); err != nil {
		return task.Task{}, err
	}
	return s.repo.GetByUUID(s.workspaceID, row.UUID)
}

func (s *Service) failProjectTemplateInstantiate(stage string) error {
	if s.projectTemplateInstantiateFailure == nil {
		return nil
	}
	return s.projectTemplateInstantiateFailure(stage)
}

func instantiateSourcePayload(plan instantiatePlan) map[string]any {
	return map[string]any{
		"source_template_id":            plan.Template.ID,
		"source_template_snapshot_id":   plan.Snapshot.ID,
		"source_template_snapshot_hash": plan.Snapshot.SnapshotHash,
	}
}

func cloneAnyMap(input map[string]any) map[string]any {
	return mergeAnyMaps(input)
}

func mergeAnyMaps(inputs ...map[string]any) map[string]any {
	out := map[string]any{}
	for _, input := range inputs {
		for key, value := range input {
			out[key] = value
		}
	}
	return out
}

func (s *Service) buildInstantiatePlan(templateRef string, input InstantiateInput, currentOnly bool) (instantiatePlan, error) {
	if err := s.rejectProjectTemplateProjectScope(); err != nil {
		return instantiatePlan{}, err
	}
	if err := s.Require(PermissionProjectManage); err != nil {
		return instantiatePlan{}, err
	}
	template, err := s.resolveProjectTemplate(templateRef)
	if err != nil {
		return instantiatePlan{}, err
	}
	return s.buildInstantiatePlanForTemplate(template, input, currentOnly, false)
}

func (s *Service) buildInstantiatePlanLocked(templateRef string, input InstantiateInput, currentOnly bool) (instantiatePlan, error) {
	if err := s.rejectProjectTemplateProjectScope(); err != nil {
		return instantiatePlan{}, err
	}
	if err := s.Require(PermissionProjectManage); err != nil {
		return instantiatePlan{}, err
	}
	templateRef = strings.TrimSpace(templateRef)
	if templateRef == "" {
		return instantiatePlan{}, RuntimeError{Code: "project_template_not_found", Message: "project template reference is required"}
	}
	template, err := s.projectTemplateRepo.LockByRef(s.workspaceID, templateRef)
	if err != nil {
		return instantiatePlan{}, s.projectTemplateStorageError(templateRef, err)
	}
	return s.buildInstantiatePlanForTemplate(template, input, currentOnly, true)
}

func (s *Service) buildInstantiatePlanForTemplate(template storage.ProjectTemplate, input InstantiateInput, currentOnly, lockSnapshots bool) (instantiatePlan, error) {
	if template.Status != "active" {
		return instantiatePlan{}, RuntimeError{Code: "project_template_archived", Message: "archived project template cannot be instantiated"}
	}
	snapshot, err := s.instantiateSnapshotRow(template, input.SnapshotID, currentOnly, lockSnapshots)
	if err != nil {
		return instantiatePlan{}, err
	}
	if err := verifyExpectedSnapshotHash(input.ExpectedHash, snapshot.SnapshotHash); err != nil {
		return instantiatePlan{}, err
	}
	decoded, snapshotIssues, err := decodeInstantiateSnapshot([]byte(snapshot.SnapshotJSON))
	if err != nil {
		return instantiatePlan{}, err
	}
	if err := s.requireProjectTemplateInstantiatePermissions(decoded); err != nil {
		return instantiatePlan{}, err
	}

	plan := instantiatePlan{
		Template: template, Snapshot: snapshot,
		ConfigValues: map[string]string{}, effectiveConfigValues: map[string]string{},
		TaskIDs: map[string]string{}, SeriesIDs: map[string]string{}, memberReplacements: map[string]*string{},
	}
	for _, item := range decoded.Tasks {
		plan.TaskIDs[item.Ref] = uuid.NewString()
	}
	for _, item := range decoded.Series {
		plan.SeriesIDs[item.Ref] = uuid.NewString()
	}

	issues := append([]ProjectTemplateIssue{}, snapshotIssues...)
	slug, name, description, projectIssues := s.validateInstantiateProject(decoded, input)
	plan.ProjectSlug, plan.ProjectName, plan.Description = slug, name, description
	issues = append(issues, projectIssues...)

	memberState, memberIssues, assigneeViews, err := s.validateInstantiateMembers(decoded, input.AssigneeReplacements)
	if err != nil {
		return instantiatePlan{}, err
	}
	plan.memberReplacements = memberState.replacements
	issues = append(issues, memberIssues...)

	secretViews, configIssues, appliedConfigs, err := s.planInstantiateConfigs(decoded.Configs, input.SecretInputs, plan.ConfigValues, plan.effectiveConfigValues)
	if err != nil {
		return instantiatePlan{}, err
	}
	issues = append(issues, configIssues...)

	plan.Tasks, plan.Series, err = s.planInstantiateTaskContent(decoded, input.StartDate, memberState, plan.TaskIDs, plan.SeriesIDs, &issues)
	if err != nil {
		return instantiatePlan{}, err
	}
	plan.Automations = s.planInstantiateAutomations(decoded.Automations, plan.effectiveConfigValues, &issues)

	sortProjectTemplateIssues(issues)
	currentSnapshot := snapshot
	currentDecoded := decoded
	if template.CurrentSnapshotID != nil && *template.CurrentSnapshotID != snapshot.ID {
		currentSnapshot, err = s.getInstantiateSnapshotRow(template.WorkspaceID, template.ID, *template.CurrentSnapshotID, lockSnapshots)
		if err != nil {
			return instantiatePlan{}, err
		}
		currentDecoded, _, err = decodeInstantiateSnapshot([]byte(currentSnapshot.SnapshotJSON))
		if err != nil {
			// current 仅用于 Template 元数据摘要；指定合法历史 Snapshot 时，
			// current 的任何 codec/schema 损坏都不能阻断目标版本。安全降级只
			// 保留 row 元数据，组件数和 secret keys 置空，不解释无效 payload。
			currentDecoded = projecttemplate.Snapshot{}
		}
	}
	users, err := s.resolveUserInfos([]string{valueOrEmpty(template.CreatedByUserID), valueOrEmpty(snapshot.CreatedByUserID), valueOrEmpty(currentSnapshot.CreatedByUserID)})
	if err != nil {
		return instantiatePlan{}, err
	}
	templateSummary := ProjectTemplateSummaryView{
		ID: template.ID, Key: template.Key, Name: template.Name, Description: template.Description, Status: template.Status,
		CurrentSnapshot: projectTemplateSnapshotSummaryView(currentSnapshot, currentDecoded, users),
		CreatedBy:       actorInfoFromColumns(projectTemplateActorColumns(template), valueOrEmpty(template.CreatedByUserID), users),
		CreatedAt:       template.CreatedAt, ModifiedAt: template.ModifiedAt, ArchivedAt: template.ArchivedAt,
	}
	previewSnapshot := projectTemplateSnapshotSummaryView(snapshot, decoded, users)
	counts := ComponentCounts{Configs: appliedConfigs, Tasks: len(plan.Tasks), Series: len(plan.Series), Automations: len(plan.Automations)}
	plan.Preview = InstantiatePreview{
		Template: templateSummary, Snapshot: *previewSnapshot,
		Project: ProjectTemplateProjectPreview{Slug: slug, Name: name, Description: description, StartDate: strings.TrimSpace(input.StartDate)},
		Counts:  counts, SecretResolutions: nonNilSecretResolutions(secretViews), AssigneeIssues: nonNilAssigneeIssues(assigneeViews),
		Issues: nonNilProjectTemplateIssues(issues), Warnings: []ProjectTemplateIssue{},
	}
	return plan, nil
}

func decodeInstantiateSnapshot(raw []byte) (projecttemplate.Snapshot, []ProjectTemplateIssue, error) {
	decoded, err := projecttemplate.Decode(raw, projecttemplate.DefaultLimits)
	if err == nil {
		return decoded, nil, nil
	}
	code := projecttemplate.ErrorCode(err)
	if code != "project_template_ref_cycle" && code != "project_template_dependency_missing" {
		return projecttemplate.Snapshot{}, nil, mapProjectTemplateDomainError(err)
	}
	// 这两类错误表示 JSON/schema/字段都已通过 strict decode，只是 local ref
	// 图不合法。为让 Web Preview 返回可修复的 typed issue，重新取得同一强类型
	// payload；未知字段、trailing document 等其它错误仍在上面的 Decode 直接拒绝。
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decodeErr := decoder.Decode(&decoded); decodeErr != nil {
		return projecttemplate.Snapshot{}, nil, mapProjectTemplateDomainError(err)
	}
	if decodeErr := decoder.Decode(&struct{}{}); decodeErr != io.EOF {
		return projecttemplate.Snapshot{}, nil, mapProjectTemplateDomainError(err)
	}
	issue := blockingTemplateIssue(code, "snapshot", "", "references", err.Error())
	return decoded, []ProjectTemplateIssue{issue}, nil
}

func (s *Service) instantiateSnapshotRow(template storage.ProjectTemplate, requested string, currentOnly, lockSnapshot bool) (storage.ProjectTemplateSnapshot, error) {
	requested = strings.TrimSpace(requested)
	if currentOnly {
		if requested == "" || template.CurrentSnapshotID == nil || subtle.ConstantTimeCompare([]byte(requested), []byte(*template.CurrentSnapshotID)) != 1 {
			return storage.ProjectTemplateSnapshot{}, snapshotHashMismatch("snapshot is not the template current snapshot")
		}
	}
	if requested == "" && template.CurrentSnapshotID != nil {
		requested = *template.CurrentSnapshotID
	}
	if requested == "" {
		return storage.ProjectTemplateSnapshot{}, RuntimeError{Code: "project_template_snapshot_not_found", Message: "project template has no current snapshot"}
	}
	return s.getInstantiateSnapshotRow(template.WorkspaceID, template.ID, requested, lockSnapshot)
}

func (s *Service) getInstantiateSnapshotRow(workspaceID, templateID, snapshotID string, locked bool) (storage.ProjectTemplateSnapshot, error) {
	var (
		row storage.ProjectTemplateSnapshot
		err error
	)
	if locked {
		row, err = s.projectTemplateRepo.GetSnapshotLocked(workspaceID, templateID, snapshotID)
	} else {
		row, err = s.projectTemplateRepo.GetSnapshot(workspaceID, templateID, snapshotID)
	}
	if errors.Is(err, storage.ErrNotFound) {
		return storage.ProjectTemplateSnapshot{}, RuntimeError{Code: "project_template_snapshot_not_found", Message: fmt.Sprintf("project template snapshot %q not found", snapshotID)}
	}
	return row, err
}

func verifyExpectedSnapshotHash(expected, actual string) error {
	expected = strings.TrimSpace(expected)
	if len(expected) != 64 || strings.ToLower(expected) != expected {
		return snapshotHashMismatch("expected snapshot hash must be 64 lowercase hexadecimal characters")
	}
	if _, err := hex.DecodeString(expected); err != nil {
		return snapshotHashMismatch("expected snapshot hash must be 64 lowercase hexadecimal characters")
	}
	if len(actual) != 64 || subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) != 1 {
		return snapshotHashMismatch("project template snapshot hash mismatch")
	}
	return nil
}

func snapshotHashMismatch(message string) RuntimeError {
	return RuntimeError{Code: "project_template_snapshot_hash_mismatch", Message: message}
}

func (s *Service) requireProjectTemplateInstantiatePermissions(snapshot projecttemplate.Snapshot) error {
	if len(snapshot.Tasks) > 0 || len(snapshot.Series) > 0 {
		if err := s.Require(PermissionTaskWrite); err != nil {
			return err
		}
	}
	if len(snapshot.Configs) > 0 {
		if err := s.Require(PermissionProjectConfigWrite); err != nil {
			return err
		}
	}
	if len(snapshot.Automations) > 0 {
		if err := s.Require(PermissionHookWrite); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) validateInstantiateProject(snapshot projecttemplate.Snapshot, input InstantiateInput) (slug, name, description string, issues []ProjectTemplateIssue) {
	description = snapshot.Project.Description
	if input.Description != nil {
		description = *input.Description
	}
	var err error
	slug, name, description, err = normalizeProjectCreateInput(AddProjectInput{Slug: input.ProjectSlug, Name: input.ProjectName, Description: description})
	if err != nil {
		return strings.TrimSpace(strings.ToLower(input.ProjectSlug)), strings.TrimSpace(input.ProjectName), strings.TrimSpace(description), []ProjectTemplateIssue{runtimeInstantiateIssue("project", "", "project", err)}
	}
	if _, err := s.projectRepo.GetBySlug(s.workspaceID, slug); err == nil {
		issues = append(issues, blockingTemplateIssue("project_already_exists", "project", slug, "project_slug", "project slug already exists"))
	} else if !errors.Is(err, storage.ErrNotFound) {
		issues = append(issues, runtimeInstantiateIssue("project", slug, "project_slug", err))
	}
	if _, err := projecttemplate.FromRelativeLocalTime(projecttemplate.RelativeLocalTimeV1{LocalTime: "00:00:00"}, strings.TrimSpace(input.StartDate), s.clock.Location()); err != nil {
		issues = append(issues, blockingTemplateIssue("project_template_date_out_of_range", "project", slug, "start_date", "project start date is invalid in the workspace timezone"))
	}
	return slug, name, description, issues
}

type instantiateMemberState struct {
	active       map[string]bool
	replacements map[string]*string
}

func (s *Service) validateInstantiateMembers(snapshot projecttemplate.Snapshot, replacements map[string]*string) (instantiateMemberState, []ProjectTemplateIssue, []AssigneeIssueView, error) {
	affected := map[string][]string{}
	add := func(userID, ref string) {
		if userID != "" {
			affected[userID] = append(affected[userID], ref)
		}
	}
	for _, item := range snapshot.Tasks {
		for _, id := range item.AssigneeIDs {
			add(id, item.Ref)
		}
		for _, id := range mentionedUserIDs(item.Description) {
			add(id, item.Ref)
		}
	}
	for _, item := range snapshot.Series {
		for _, id := range item.AssigneeIDs {
			add(id, item.Ref)
		}
		for _, id := range mentionedUserIDs(item.Description) {
			add(id, item.Ref)
		}
	}
	ids := sortedMapKeys(affected)
	for source, replacement := range replacements {
		ids = append(ids, strings.TrimSpace(source))
		if replacement != nil {
			ids = append(ids, strings.TrimSpace(*replacement))
		}
	}
	ids = sortedUniqueStrings(ids)
	members, err := s.memberRepo.ListMembersByUserIDs(s.workspaceID, ids)
	if err != nil {
		return instantiateMemberState{}, nil, nil, err
	}
	active := make(map[string]bool, len(members))
	for _, member := range members {
		active[member.User.ID] = true
	}
	users, err := s.resolveUserInfos(ids)
	if err != nil {
		return instantiateMemberState{}, nil, nil, err
	}
	state := instantiateMemberState{active: active, replacements: map[string]*string{}}
	issues := []ProjectTemplateIssue{}
	views := []AssigneeIssueView{}

	for _, source := range sortedMapKeys(replacements) {
		normalizedSource := strings.TrimSpace(source)
		replacement := replacements[source]
		if source != normalizedSource || normalizedSource == "" || active[normalizedSource] || len(affected[normalizedSource]) == 0 {
			issues = append(issues, blockingTemplateIssue("project_template_member_unavailable", "member", normalizedSource, "assignee_replacements", "assignee replacement does not match an unavailable template member"))
			continue
		}
		if replacement == nil {
			state.replacements[normalizedSource] = nil
			continue
		}
		target := strings.TrimSpace(*replacement)
		if target == "" || !active[target] {
			issues = append(issues, blockingTemplateIssue("project_template_member_unavailable", "member", normalizedSource, "assignee_replacements", "replacement user is not an active workspace member"))
			continue
		}
		copyTarget := target
		state.replacements[normalizedSource] = &copyTarget
	}
	for _, userID := range sortedMapKeys(affected) {
		if active[userID] {
			continue
		}
		refs := sortedUniqueStrings(affected[userID])
		resolution := "unresolved"
		if replacement, ok := state.replacements[userID]; ok {
			if replacement == nil {
				resolution = "removed"
			} else {
				resolution = "replaced"
			}
		} else {
			issues = append(issues, ProjectTemplateIssue{Code: "project_template_member_unavailable", Severity: "blocking", Component: "member", SourceRef: userID, Field: "assignees", Message: "template member is not an active workspace member"})
		}
		user := users[userID]
		if user.ID == "" {
			user = task.UserInfo{ID: userID, Name: userID}
		}
		views = append(views, AssigneeIssueView{User: user, AffectedRefs: refs, Resolution: resolution})
	}
	return state, issues, views, nil
}

func mentionedUserIDs(description *string) []string {
	if description == nil {
		return nil
	}
	ids, err := task.MentionedUserIDs(*description)
	if err != nil {
		return nil
	}
	return ids
}

func (s *Service) planInstantiateConfigs(blueprints []projecttemplate.ConfigBlueprintV1, secretInputs map[string]string, projectValues, effectiveValues map[string]string) ([]SecretResolutionView, []ProjectTemplateIssue, int, error) {
	secretViews := []SecretResolutionView{}
	issues := []ProjectTemplateIssue{}
	applied := 0
	allowedSecretInputs := map[string]bool{}
	for _, blueprint := range blueprints {
		def, err := s.scopedConfigDefinition(blueprint.Key)
		if err != nil {
			issues = append(issues, blockingTemplateIssue("project_template_config_invalid", "config", blueprint.Key, "definition", "config definition is unavailable"))
			continue
		}
		if !configDefinitionAllowsScope(def, storage.ConfigScopeProject) {
			issues = append(issues, blockingTemplateIssue("project_template_config_invalid", "config", blueprint.Key, "scope", "config no longer allows project scope"))
			continue
		}
		secret := blueprint.Mode == "secret_input" || def.Secret
		if secret {
			allowedSecretInputs[blueprint.Key] = true
			source := "missing"
			if raw, ok := secretInputs[blueprint.Key]; ok && strings.TrimSpace(raw) != "" {
				value, validateErr := s.validateProspectiveProjectConfigValue(def, raw)
				if validateErr != nil {
					issues = append(issues, blockingTemplateIssue("project_template_config_invalid", "config", blueprint.Key, "value", "secret input is incompatible with current config schema"))
				} else {
					projectValues[blueprint.Key], effectiveValues[blueprint.Key], source = value, value, "input"
					applied++
				}
			} else {
				value, inheritedSource, _, resolveErr := s.prospectiveProjectConfigValue(blueprint.Key, projectValues)
				if resolveErr != nil {
					issues = append(issues, blockingTemplateIssue("project_template_config_invalid", "config", blueprint.Key, "value", "inherited secret is incompatible with current config schema"))
				} else if (inheritedSource == "workspace" || inheritedSource == "default") && strings.TrimSpace(value) != "" {
					effectiveValues[blueprint.Key], source = value, inheritedSource
					applied++
				} else {
					issues = append(issues, blockingTemplateIssue("project_template_secret_required", "config", blueprint.Key, "value", "secret config requires input or an inherited value"))
				}
			}
			secretViews = append(secretViews, SecretResolutionView{Key: blueprint.Key, ResolvedFrom: source})
			continue
		}
		if blueprint.Mode != "literal" || blueprint.Value == nil {
			issues = append(issues, blockingTemplateIssue("project_template_config_invalid", "config", blueprint.Key, "mode", "config blueprint mode is incompatible with current schema"))
			continue
		}
		value, validateErr := s.validateProspectiveProjectConfigValue(def, *blueprint.Value)
		if validateErr != nil {
			issues = append(issues, blockingTemplateIssue("project_template_config_invalid", "config", blueprint.Key, "value", "config value is incompatible with current schema"))
			continue
		}
		projectValues[blueprint.Key], effectiveValues[blueprint.Key] = value, value
		applied++
	}
	for key := range secretInputs {
		if !allowedSecretInputs[key] {
			issues = append(issues, blockingTemplateIssue("project_template_config_invalid", "config", key, "secret_inputs", "secret input does not match a template secret requirement"))
		}
	}
	sort.Slice(secretViews, func(i, j int) bool { return secretViews[i].Key < secretViews[j].Key })
	return secretViews, issues, applied, nil
}

func (s *Service) planInstantiateTaskContent(snapshot projecttemplate.Snapshot, startDate string, members instantiateMemberState, taskIDs, seriesIDs map[string]string, issues *[]ProjectTemplateIssue) ([]plannedTask, []plannedSeries, error) {
	tasks := make([]plannedTask, 0, len(snapshot.Tasks))
	for _, blueprint := range snapshot.Tasks {
		description := blueprint.Description
		if blueprint.Description != nil {
			rewritten, rewriteErr := projecttemplate.RewriteInstantiateTaskReferences(*blueprint.Description, taskIDs)
			if rewriteErr != nil {
				*issues = append(*issues, templateDomainIssue("task", blueprint.Ref, "description", rewriteErr))
			} else {
				description = &rewritten
				description, rewriteErr = rewriteInstantiateUsers(description, members.replacements)
				if rewriteErr != nil {
					*issues = append(*issues, blockingTemplateIssue("project_template_snapshot_invalid", "task", blueprint.Ref, "description", "template user reference cannot be rewritten"))
				}
			}
		}
		udas := s.validateInstantiateUDAs("task", blueprint.Ref, blueprint.UDAs, issues)
		planned := plannedTask{ID: taskIDs[blueprint.Ref], Ref: blueprint.Ref, Title: blueprint.Title, Description: description, Priority: blueprint.Priority, Tags: append([]string{}, blueprint.Tags...), AssigneeIDs: replaceMemberIDs(blueprint.AssigneeIDs, members), UDAs: udas, Links: append([]projecttemplate.TaskLinkBlueprintV1{}, blueprint.Links...)}
		planned.Due = restoreInstantiateDate(startDate, blueprint.Dates.Due, "task", blueprint.Ref, "due", s.clock.Location(), issues)
		planned.Wait = restoreInstantiateDate(startDate, blueprint.Dates.Wait, "task", blueprint.Ref, "wait", s.clock.Location(), issues)
		planned.Scheduled = restoreInstantiateDate(startDate, blueprint.Dates.Scheduled, "task", blueprint.Ref, "scheduled", s.clock.Location(), issues)
		planned.Until = restoreInstantiateDate(startDate, blueprint.Dates.Until, "task", blueprint.Ref, "until", s.clock.Location(), issues)
		if blueprint.ParentRef != nil {
			parent := taskIDs[*blueprint.ParentRef]
			if parent == "" {
				*issues = append(*issues, blockingTemplateIssue("project_template_dependency_missing", "task", blueprint.Ref, "parent", "task parent reference is missing"))
			} else {
				planned.ParentID = &parent
			}
		}
		for _, ref := range blueprint.DependsRefs {
			if target := taskIDs[ref]; target != "" {
				planned.DependsIDs = append(planned.DependsIDs, target)
			} else {
				*issues = append(*issues, blockingTemplateIssue("project_template_dependency_missing", "task", blueprint.Ref, "depends", "task dependency reference is missing"))
			}
		}
		tasks = append(tasks, planned)
	}

	series := make([]plannedSeries, 0, len(snapshot.Series))
	for _, blueprint := range snapshot.Series {
		description := blueprint.Description
		if description != nil {
			rewritten, rewriteErr := projecttemplate.RewriteInstantiateTaskReferences(*description, taskIDs)
			if rewriteErr != nil {
				*issues = append(*issues, templateDomainIssue("series", blueprint.Ref, "description", rewriteErr))
			} else {
				description = &rewritten
				description, rewriteErr = rewriteInstantiateUsers(description, members.replacements)
				if rewriteErr != nil {
					*issues = append(*issues, blockingTemplateIssue("project_template_snapshot_invalid", "series", blueprint.Ref, "description", "template user reference cannot be rewritten"))
				}
			}
		}
		if err := taskseries.ValidateRule(blueprint.RecurrenceRule); err != nil {
			*issues = append(*issues, blockingTemplateIssue("project_template_snapshot_invalid", "series", blueprint.Ref, "recurrence_rule", "series recurrence rule is invalid"))
		}
		udas := s.validateInstantiateUDAs("series", blueprint.Ref, blueprint.UDAs, issues)
		firstDue := restoreInstantiateDate(startDate, &blueprint.FirstDue, "series", blueprint.Ref, "first_due", s.clock.Location(), issues)
		until := restoreInstantiateDate(startDate, blueprint.Until, "series", blueprint.Ref, "until", s.clock.Location(), issues)
		firstUnix := int64(0)
		if firstDue != nil {
			firstUnix = *firstDue
		}
		if until != nil && firstDue != nil && *until < *firstDue {
			*issues = append(*issues, blockingTemplateIssue("project_template_date_out_of_range", "series", blueprint.Ref, "until", "series until is before first due"))
		}
		series = append(series, plannedSeries{ID: seriesIDs[blueprint.Ref], Ref: blueprint.Ref, Title: blueprint.Title, RecurrenceRule: blueprint.RecurrenceRule, Description: description, Priority: blueprint.Priority, Tags: append([]string{}, blueprint.Tags...), AssigneeIDs: replaceMemberIDs(blueprint.AssigneeIDs, members), UDAs: udas, FirstDue: firstUnix, Until: until})
	}
	return tasks, series, nil
}

type instantiateMarkdownPatch struct {
	start, end  int
	replacement string
}

func rewriteInstantiateUsers(description *string, replacements map[string]*string) (*string, error) {
	if description == nil || len(replacements) == 0 {
		return description, nil
	}
	source := []byte(*description)
	doc := goldmark.New().Parser().Parse(text.NewReader(source))
	patches := []instantiateMarkdownPatch{}
	err := ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		var destination []byte
		var start, end int
		var spanErr error
		switch typed := node.(type) {
		case *ast.Link:
			if typed.Reference != nil {
				return ast.WalkContinue, nil
			}
			destination = typed.Destination
			start, end, spanErr = instantiateInlineDestinationSpan(source, typed.Pos(), destination)
		case *ast.Image:
			if typed.Reference != nil {
				return ast.WalkContinue, nil
			}
			destination = typed.Destination
			start, end, spanErr = instantiateInlineDestinationSpan(source, typed.Pos(), destination)
		case *ast.LinkReferenceDefinition:
			destination = typed.Destination
			start, end, spanErr = instantiateReferenceDestinationSpan(source, typed)
		default:
			return ast.WalkContinue, nil
		}
		if spanErr != nil {
			return ast.WalkStop, spanErr
		}
		userID := strings.TrimPrefix(string(destination), "ref://user/")
		if userID == string(destination) {
			return ast.WalkContinue, nil
		}
		replacement, ok := replacements[userID]
		if !ok {
			return ast.WalkContinue, nil
		}
		next := "#"
		if replacement != nil {
			next = "ref://user/" + *replacement
		}
		patches = append(patches, instantiateMarkdownPatch{start: start, end: end, replacement: next})
		return ast.WalkContinue, nil
	})
	if err != nil || len(patches) == 0 {
		return description, err
	}
	sort.Slice(patches, func(i, j int) bool { return patches[i].start < patches[j].start })
	var out strings.Builder
	previous := 0
	for _, patch := range patches {
		if patch.start < previous || patch.start > patch.end || patch.end > len(source) {
			return description, fmt.Errorf("overlapping Markdown reference patches")
		}
		out.Write(source[previous:patch.start])
		out.WriteString(patch.replacement)
		previous = patch.end
	}
	out.Write(source[previous:])
	rewritten := out.String()
	return &rewritten, nil
}

func instantiateInlineDestinationSpan(source []byte, nodeStart int, destination []byte) (int, int, error) {
	if nodeStart < 0 || nodeStart >= len(source) {
		return 0, 0, fmt.Errorf("Markdown link has no source position")
	}
	open := nodeStart
	if source[open] == '!' {
		open++
	}
	if open >= len(source) || source[open] != '[' {
		return 0, 0, fmt.Errorf("Markdown link source span is invalid")
	}
	close, ok := instantiateMatchingBracket(source, open)
	if !ok || close+1 >= len(source) || source[close+1] != '(' {
		return 0, 0, fmt.Errorf("Markdown inline link source span is invalid")
	}
	return instantiateDestinationSpan(source, close+2, len(source), destination)
}

func instantiateReferenceDestinationSpan(source []byte, definition *ast.LinkReferenceDefinition) (int, int, error) {
	if definition.Pos() < 0 || definition.Lines().Len() == 0 {
		return 0, 0, fmt.Errorf("Markdown reference definition has no source span")
	}
	start, end := definition.Pos(), definition.Pos()
	for i := 0; i < definition.Lines().Len(); i++ {
		if stop := definition.Lines().At(i).Stop; stop > end {
			end = stop
		}
	}
	open := start
	for open < end && (source[open] == ' ' || source[open] == '\t') {
		open++
	}
	if open >= end || source[open] != '[' {
		return 0, 0, fmt.Errorf("Markdown reference definition source span is invalid")
	}
	close, ok := instantiateMatchingBracket(source[:end], open)
	if !ok || close+1 >= end || source[close+1] != ':' {
		return 0, 0, fmt.Errorf("Markdown reference definition source span is invalid")
	}
	return instantiateDestinationSpan(source, close+2, end, definition.Destination)
}

func instantiateDestinationSpan(source []byte, start, limit int, destination []byte) (int, int, error) {
	for start < limit && (source[start] == ' ' || source[start] == '\t' || source[start] == '\n' || source[start] == '\r') {
		start++
	}
	if start >= limit {
		return 0, 0, fmt.Errorf("Markdown destination source span is empty")
	}
	if source[start] == '<' {
		start++
	}
	end := start + len(destination)
	if end > limit || string(source[start:end]) != string(destination) {
		return 0, 0, fmt.Errorf("Markdown destination source span does not match AST")
	}
	return start, end, nil
}

func instantiateMatchingBracket(source []byte, open int) (int, bool) {
	depth := 0
	for i := open; i < len(source); i++ {
		if source[i] == '\\' {
			i++
			continue
		}
		switch source[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

func replaceMemberIDs(ids []string, state instantiateMemberState) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if state.active[id] {
			out = append(out, id)
			continue
		}
		if replacement, ok := state.replacements[id]; ok && replacement != nil {
			out = append(out, *replacement)
		}
	}
	return sortedUniqueStrings(out)
}

func (s *Service) validateInstantiateUDAs(component, ref string, blueprints map[string]projecttemplate.UDABlueprintV1, issues *[]ProjectTemplateIssue) map[string]string {
	out := map[string]string{}
	for _, name := range sortedMapKeys(blueprints) {
		blueprint := blueprints[name]
		def, err := s.udaDefinition(name)
		if err != nil {
			*issues = append(*issues, blockingTemplateIssue("project_template_uda_invalid", component, ref, "uda."+name, "UDA definition is unavailable"))
			continue
		}
		if blueprint.Type != "" && blueprint.Type != string(def.Type) {
			*issues = append(*issues, blockingTemplateIssue("project_template_uda_invalid", component, ref, "uda."+name, "UDA type is incompatible with current definition"))
			continue
		}
		value, err := uda.NormalizeValue(def, blueprint.Raw)
		if err != nil || value == "" {
			*issues = append(*issues, blockingTemplateIssue("project_template_uda_invalid", component, ref, "uda."+name, "UDA value is incompatible with current definition"))
			continue
		}
		out[name] = value
	}
	return out
}

func restoreInstantiateDate(startDate string, relative *projecttemplate.RelativeLocalTimeV1, component, ref, field string, loc *time.Location, issues *[]ProjectTemplateIssue) *int64 {
	if relative == nil {
		return nil
	}
	value, err := projecttemplate.FromRelativeLocalTime(*relative, strings.TrimSpace(startDate), loc)
	if err != nil {
		*issues = append(*issues, blockingTemplateIssue("project_template_date_out_of_range", component, ref, field, "template date cannot be restored in the workspace timezone"))
		return nil
	}
	return &value
}

func (s *Service) planInstantiateAutomations(blueprints []projecttemplate.AutomationBlueprintV1, effectiveValues map[string]string, issues *[]ProjectTemplateIssue) []plannedAutomation {
	out := make([]plannedAutomation, 0, len(blueprints))
	for _, blueprint := range blueprints {
		input := projectAutomationInputFromTemplate(blueprint)
		normalized, err := normalizeProjectAutomationAddInput(input)
		if err == nil {
			_, _, _, err = validateProjectAutomationProviderConfig(normalized.Action, func(key string) (string, bool, error) {
				if value, ok := effectiveValues[key]; ok {
					return value, strings.TrimSpace(value) != "", nil
				}
				value, source, _, resolveErr := s.prospectiveProjectConfigValue(key, effectiveValues)
				return value, source != "missing" && strings.TrimSpace(value) != "", resolveErr
			})
		}
		if err != nil {
			*issues = append(*issues, blockingTemplateIssue("project_template_automation_invalid", "automation", blueprint.Ref, "provider", "automation provider or config is incompatible with current workspace"))
		}
		normalized.Enabled = false
		out = append(out, plannedAutomation{ID: uuid.NewString(), Ref: blueprint.Ref, Input: normalized})
	}
	return out
}

func projectAutomationInputFromTemplate(in projecttemplate.AutomationBlueprintV1) ProjectAutomationRuleAddInput {
	return ProjectAutomationRuleAddInput{
		Name: in.Name, Description: in.Description, Enabled: false, TriggerType: in.TriggerType,
		TriggerConfig: ProjectAutomationTriggerConfig{ScheduleType: in.TriggerConfig.ScheduleType, ScheduleValue: in.TriggerConfig.ScheduleValue, Timezone: in.TriggerConfig.Timezone, EventType: in.TriggerConfig.EventType},
		Condition:     ProjectAutomationCondition{TaskFilter: in.Condition.TaskFilter, MaxTasks: in.Condition.MaxTasks, OnlyAddedAssignees: in.Condition.OnlyAddedAssignees},
		Action:        ProjectAutomationActionConfig{Protocol: in.Action.Protocol, BaseURLConfigKey: in.Action.BaseURLConfigKey, APIKeyConfigKey: in.Action.APIKeyConfigKey, ModelConfigKey: in.Action.ModelConfigKey, AllowedHostsConfigKey: in.Action.AllowedHostsConfigKey, ModelOverride: in.Action.ModelOverride, Temperature: in.Action.Temperature, MaxAttempts: in.Action.MaxAttempts, AttachMetadata: in.Action.AttachMetadata},
		Context:       ProjectAutomationContextConfig{Include: append([]string{}, in.Context.Include...)}, InstructionTemplate: in.InstructionTemplate, SystemPrompt: in.SystemPrompt,
	}
}

func blockingTemplateIssue(code, component, sourceRef, field, message string) ProjectTemplateIssue {
	return ProjectTemplateIssue{Code: code, Severity: "blocking", Component: component, SourceRef: sourceRef, Field: field, Message: message}
}

func runtimeInstantiateIssue(component, sourceRef, field string, err error) ProjectTemplateIssue {
	code, ok := IsRuntimeErrorCode(err)
	if !ok || code == "" {
		code = "project_template_snapshot_invalid"
	}
	return blockingTemplateIssue(code, component, sourceRef, field, err.Error())
}

func templateDomainIssue(component, sourceRef, field string, err error) ProjectTemplateIssue {
	code := projecttemplate.ErrorCode(err)
	if code == "" {
		code = "project_template_snapshot_invalid"
	}
	return blockingTemplateIssue(code, component, sourceRef, field, err.Error())
}

func sortProjectTemplateIssues(items []ProjectTemplateIssue) {
	sort.SliceStable(items, func(i, j int) bool {
		left := strings.Join([]string{items[i].Code, items[i].Component, items[i].SourceRef, items[i].TargetRef, items[i].Relation, items[i].Field, items[i].Message}, "\x00")
		right := strings.Join([]string{items[j].Code, items[j].Component, items[j].SourceRef, items[j].TargetRef, items[j].Relation, items[j].Field, items[j].Message}, "\x00")
		return left < right
	})
}

func nonNilProjectTemplateIssues(items []ProjectTemplateIssue) []ProjectTemplateIssue {
	if items == nil {
		return []ProjectTemplateIssue{}
	}
	return items
}

func nonNilSecretResolutions(items []SecretResolutionView) []SecretResolutionView {
	if items == nil {
		return []SecretResolutionView{}
	}
	return items
}

func nonNilAssigneeIssues(items []AssigneeIssueView) []AssigneeIssueView {
	if items == nil {
		return []AssigneeIssueView{}
	}
	return items
}

func sortedUniqueStrings(items []string) []string {
	seen := map[string]struct{}{}
	for _, item := range items {
		if item = strings.TrimSpace(item); item != "" {
			seen[item] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for item := range seen {
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func sortedMapKeys[V any](items map[string]V) []string {
	out := make([]string, 0, len(items))
	for key := range items {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
