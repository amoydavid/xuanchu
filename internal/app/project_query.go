package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/dajee/taskg/internal/query"
	"github.com/dajee/taskg/internal/storage/sqlite"
	"github.com/dajee/taskg/internal/task"
)

type projectBinding struct {
	ID       *string
	Slug     *string
	Archived bool
}

type projectChange struct {
	Before projectBinding
	After  projectBinding
}

func (s *Service) resolveActiveProjectBinding(ref *string) (projectBinding, error) {
	if ref == nil {
		return projectBinding{}, nil
	}
	trimmed := strings.TrimSpace(*ref)
	if trimmed == "" {
		return projectBinding{}, nil
	}
	project, err := s.ResolveProject(trimmed)
	if err != nil {
		normalized, normalizeErr := normalizeProjectSlug(trimmed)
		if normalizeErr != nil || normalized == trimmed {
			return projectBinding{}, err
		}
		project, err = s.ResolveProject(normalized)
		if err != nil {
			return projectBinding{}, err
		}
	}
	if err := s.ensureProjectScope(&project.ID); err != nil {
		return projectBinding{}, err
	}
	if project.Status == string(sqlite.ProjectStatusArchived) || project.ArchivedAt != nil {
		return projectBinding{}, RuntimeError{
			Code:    "project_archived",
			Message: fmt.Sprintf("project %q is archived", project.Slug),
		}
	}
	return projectBindingFromProject(project), nil
}

func clearProjectBinding(tsk *task.Task) {
	tsk.Project = nil
	tsk.ProjectID = nil
}

func (s *Service) applyProjectBinding(tsk *task.Task, slug *string) (projectChange, error) {
	before := projectBindingFromTask(*tsk)
	return s.applyProjectBindingFrom(tsk, slug, before)
}

func (s *Service) applyProjectBindingFrom(tsk *task.Task, slug *string, before projectBinding) (projectChange, error) {
	if slug == nil {
		if s.hasProjectScope() && !s.allowsProjectID(tsk.ProjectID) {
			return projectChange{}, RuntimeError{Code: "project_scope_denied", Message: "token cannot access project"}
		}
		return projectChange{Before: before, After: before}, nil
	}
	if strings.TrimSpace(*slug) == "" {
		if s.hasProjectScope() {
			return projectChange{}, RuntimeError{Code: "project_scope_denied", Message: "token cannot access project"}
		}
		clearProjectBinding(tsk)
		return projectChange{Before: before, After: projectBinding{}}, nil
	}
	after, err := s.resolveActiveProjectBinding(slug)
	if err != nil {
		return projectChange{}, err
	}
	tsk.Project = cloneStringPtr(after.Slug)
	tsk.ProjectID = cloneStringPtr(after.ID)
	return projectChange{Before: before, After: after}, nil
}

func (s *Service) validateTaskProjectInvariant(tsk task.Task) error {
	if tsk.Project == nil && tsk.ProjectID == nil {
		return nil
	}
	if tsk.Project == nil || tsk.ProjectID == nil {
		return RuntimeError{Code: "project_invariant_violation", Message: "project invariant violation"}
	}
	project, err := s.projectRepo.GetByID(*tsk.ProjectID)
	if err != nil {
		return RuntimeError{Code: "project_invariant_violation", Message: "project invariant violation"}
	}
	if project.WorkspaceID != tsk.WorkspaceID || project.Slug != *tsk.Project {
		return RuntimeError{Code: "project_invariant_violation", Message: "project invariant violation"}
	}
	return nil
}

func projectBindingFromTask(tsk task.Task) projectBinding {
	return projectBinding{
		ID:   cloneStringPtr(tsk.ProjectID),
		Slug: cloneStringPtr(tsk.Project),
	}
}

func projectBindingFromProject(project sqlite.Project) projectBinding {
	return projectBinding{
		ID:       cloneStringPtr(&project.ID),
		Slug:     cloneStringPtr(&project.Slug),
		Archived: project.Status == string(sqlite.ProjectStatusArchived) || project.ArchivedAt != nil,
	}
}

func projectChangeForTask(tsk task.Task) projectChange {
	binding := projectBindingFromTask(tsk)
	return projectChange{Before: binding, After: binding}
}

func projectChangePayload(change projectChange) map[string]any {
	return map[string]any{
		"before_project_id":   stringOrNil(change.Before.ID),
		"before_project_slug": stringOrNil(change.Before.Slug),
		"after_project_id":    stringOrNil(change.After.ID),
		"after_project_slug":  stringOrNil(change.After.Slug),
	}
}

func taskAuditEntry(action, targetID string, change projectChange) AuditEntry {
	return AuditEntry{
		Action:     action,
		ProjectID:  auditProjectIDForChange(change),
		TargetType: "task",
		TargetID:   targetID,
		Payload:    projectChangePayload(change),
	}
}

func auditProjectIDForChange(change projectChange) *string {
	if projectBindingEqual(change.Before, change.After) {
		if change.After.ID != nil {
			return cloneStringPtr(change.After.ID)
		}
		return cloneStringPtr(change.Before.ID)
	}
	if change.Before.ID != nil {
		return cloneStringPtr(change.Before.ID)
	}
	return cloneStringPtr(change.After.ID)
}

func projectBindingEqual(a, b projectBinding) bool {
	return stringPtrEqual(a.ID, b.ID) && stringPtrEqual(a.Slug, b.Slug)
}

func stringPtrEqual(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func cloneStringPtr(v *string) *string {
	if v == nil {
		return nil
	}
	cloned := *v
	return &cloned
}

func stringOrNil(v *string) any {
	if v == nil {
		return nil
	}
	return *v
}

func (s *Service) resolveProjectPredicates(expr query.Expr) (query.Expr, error) {
	switch e := expr.(type) {
	case nil:
		return nil, nil
	case query.Predicate:
		return s.resolveProjectPredicate(e)
	case query.Binary:
		left, err := s.resolveProjectPredicates(e.Left)
		if err != nil {
			return nil, err
		}
		right, err := s.resolveProjectPredicates(e.Right)
		if err != nil {
			return nil, err
		}
		return query.Binary{Op: e.Op, Left: left, Right: right}, nil
	case query.Unary:
		resolved, err := s.resolveProjectPredicates(e.Expr)
		if err != nil {
			return nil, err
		}
		return query.Unary{Op: e.Op, Expr: resolved}, nil
	default:
		return nil, fmt.Errorf("unsupported query expr %T", expr)
	}
}

func (s *Service) resolveProjectPredicate(p query.Predicate) (query.Expr, error) {
	if p.Attribute != query.AttrProject {
		return p, nil
	}
	switch p.Operator {
	case query.OpIsNull:
		return p, nil
	case query.OpEqual:
		if strings.TrimSpace(p.Value.Text) == "" {
			return query.Predicate{Attribute: query.AttrProject, Operator: query.OpIsNull, Value: query.StringValue("")}, nil
		}
		project, err := s.ResolveProject(p.Value.Text)
		if err != nil {
			normalized, normalizeErr := normalizeProjectSlug(p.Value.Text)
			if normalizeErr != nil || normalized == p.Value.Text {
				return nil, err
			}
			project, err = s.ResolveProject(normalized)
			if err != nil {
				return nil, err
			}
		}
		return query.Predicate{
			Attribute: query.AttrProjectID,
			Operator:  query.OpEqual,
			Value:     query.StringValue(project.ID),
		}, nil
	default:
		return nil, RuntimeError{
			Code:    "project_invariant_violation",
			Message: fmt.Sprintf("unsupported project predicate operator %q", p.Operator),
		}
	}
}

func mapProjectQueryCompileError(err error) error {
	if errors.Is(err, query.ErrProjectPredicateUnresolved) {
		return RuntimeError{Code: "project_invariant_violation", Message: "project invariant violation"}
	}
	return err
}
