package app

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/google/uuid"

	"github.com/dajee/taskg/internal/query"
	"github.com/dajee/taskg/internal/report"
	"github.com/dajee/taskg/internal/storage/sqlite"
	"github.com/dajee/taskg/internal/task"
	"github.com/dajee/taskg/internal/urgency"
)

type Service struct {
	store       *sqlite.Store
	repo        *sqlite.TaskRepository
	workspaceID string
	clock       Clock
	reports     report.Registry
}

type ServiceOptions struct {
	Store *sqlite.Store
	Clock Clock
}

type AddInput struct {
	Description string
	Project     *string
	Priority    *string
	Due         *int64
	Tags        []string
}

type ListInput struct {
	Target     *string
	Status     string
	Sort       string
	Query      query.Expr
	ReportMode bool
}

type ModifyInput struct {
	Description *string
	Project     *string
	Priority    *string
	Due         *int64
	AddTags     []string
	RemoveTags  []string
}

type ReportInput struct {
	Name  string
	Query query.Expr
}

type ReportResult struct {
	Tasks []task.Task
}

func NewService(opts ServiceOptions) (*Service, error) {
	if opts.Clock == nil {
		opts.Clock = realClock{}
	}
	ws, err := opts.Store.LocalWorkspace()
	if err != nil {
		return nil, err
	}
	return &Service{
		store:       opts.Store,
		repo:        sqlite.NewTaskRepository(opts.Store.DB()),
		workspaceID: ws.ID,
		clock:       opts.Clock,
		reports:     report.DefaultRegistry(),
	}, nil
}

func (s *Service) Clock() Clock {
	return s.clock
}

func (s *Service) Projects() ([]string, error) {
	return s.repo.Projects(s.workspaceID)
}

func (s *Service) Tags() ([]string, error) {
	return s.repo.Tags(s.workspaceID)
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
	workingSet, err := s.List(ListInput{})
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

func (s *Service) Add(input AddInput) (task.Task, error) {
	now := s.clock.Unix()
	tsk := task.Task{
		UUID: uuid.NewString(), WorkspaceID: s.workspaceID, Description: input.Description,
		Status: task.StatusPending, Entry: now, Modified: now, Due: input.Due,
		Project: input.Project, Priority: input.Priority, Tags: input.Tags,
	}
	return s.repo.Create(tsk)
}

func (s *Service) List(input ListInput) ([]task.Task, error) {
	if input.Target != nil {
		tsk, err := s.ResolveTarget(*input.Target)
		if err != nil {
			return nil, err
		}
		return []task.Task{tsk}, nil
	}
	status := input.Status
	if status == "" && input.Query == nil && !input.ReportMode {
		status = task.StatusPending
	}
	return s.repo.List(s.workspaceID, sqlite.ListOptions{
		Status:  status,
		Sort:    input.Sort,
		Query:   input.Query,
		NowUnix: s.clock.Unix(),
	})
}

func (s *Service) ListReport(name string, input ListInput) ([]task.Task, error) {
	if input.Target != nil {
		return s.List(input)
	}
	result, err := s.RunReport(ReportInput{Name: name, Query: input.Query})
	if err != nil {
		return nil, err
	}
	return result.Tasks, nil
}

func (s *Service) Info(target string) (task.Task, error) {
	return s.repo.GetByUUID(s.workspaceID, target)
}

func (s *Service) ResolveTarget(target string) (task.Task, error) {
	if n, err := strconv.Atoi(target); err == nil && n >= 1 {
		tasks, err := s.List(ListInput{})
		if err != nil {
			return task.Task{}, err
		}
		if n > len(tasks) {
			return task.Task{}, sqlite.ErrNotFound
		}
		return tasks[n-1], nil
	}
	return s.Info(target)
}

func (s *Service) Modify(target string, input ModifyInput) error {
	tsk, err := s.ResolveTarget(target)
	if err != nil {
		return err
	}
	if input.Description != nil {
		tsk.Description = *input.Description
	}
	if input.Project != nil {
		tsk.Project = input.Project
	}
	if input.Priority != nil {
		tsk.Priority = input.Priority
	}
	if input.Due != nil {
		tsk.Due = input.Due
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
	tsk.Modified = s.clock.Unix()
	return s.repo.Update(tsk)
}

func (s *Service) Done(target string) error {
	tsk, err := s.ResolveTarget(target)
	if err != nil {
		return err
	}
	tsk.Complete(s.clock.Unix())
	return s.repo.Update(tsk)
}

func (s *Service) Delete(target string) error {
	tsk, err := s.ResolveTarget(target)
	if err != nil {
		return err
	}
	tsk.Delete(s.clock.Unix())
	return s.repo.Update(tsk)
}

func (s *Service) Export() ([]task.Task, error) {
	return s.repo.List(s.workspaceID, sqlite.ListOptions{})
}

func (s *Service) Import(tasks []task.JSONTask) (int, error) {
	count := 0
	for _, dto := range tasks {
		tsk := task.FromJSON(dto)
		tsk.WorkspaceID = s.workspaceID
		if tsk.UUID == "" {
			tsk.UUID = uuid.NewString()
		}
		// Try to get existing task
		existing, err := s.repo.GetByUUID(s.workspaceID, tsk.UUID)
		if err == sqlite.ErrNotFound {
			// Create new
			if tsk.Status == "" {
				tsk.Status = task.StatusPending
			}
			if tsk.Entry == 0 {
				tsk.Entry = s.clock.Unix()
			}
			if tsk.Modified == 0 {
				tsk.Modified = s.clock.Unix()
			}
			if _, err := s.repo.Create(tsk); err != nil {
				return count, err
			}
		} else if err != nil {
			return count, err
		} else {
			// Update existing
			if tsk.Description != "" {
				existing.Description = tsk.Description
			}
			if tsk.Status != "" {
				existing.Status = tsk.Status
			}
			existing.Modified = s.clock.Unix()
			if tsk.Project != nil {
				existing.Project = tsk.Project
			}
			if tsk.Priority != nil {
				existing.Priority = tsk.Priority
			}
			if tsk.Due != nil {
				existing.Due = tsk.Due
			}
			if len(tsk.Tags) > 0 {
				existing.Tags = tsk.Tags
			}
			if err := s.repo.Update(existing); err != nil {
				return count, err
			}
		}
		count++
	}
	return count, nil
}

func (s *Service) RunReport(input ReportInput) (ReportResult, error) {
	def, ok := s.reports.Get(input.Name)
	if !ok {
		return ReportResult{}, fmt.Errorf("unknown report %q", input.Name)
	}
	merged := query.And(def.Filter, input.Query)
	tasks, err := s.List(ListInput{Query: merged, Sort: def.Sort, ReportMode: true})
	if err != nil {
		return ReportResult{}, err
	}
	if def.Sort == "urgency" {
		type taskWithUrgency struct {
			Task  task.Task
			Total float64
		}
		withUrgency := make([]taskWithUrgency, len(tasks))
		for i, tsk := range tasks {
			explain := urgency.Explain(tsk, urgency.Options{NowUnix: s.clock.Unix()})
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
	tsk, err := s.ResolveTarget(target)
	if err != nil {
		return urgency.ExplainResult{}, err
	}
	return urgency.Explain(tsk, urgency.Options{NowUnix: s.clock.Unix()}), nil
}
