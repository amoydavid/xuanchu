package app

import (
	"strconv"

	"github.com/google/uuid"

	"github.com/dajee/taskg/internal/storage/sqlite"
	"github.com/dajee/taskg/internal/task"
)

type Service struct {
	store       *sqlite.Store
	repo        *sqlite.TaskRepository
	workspaceID string
	clock       Clock
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
	Status   string
	Project  *string
	Priority *string
	Tags     []string
	Text     *string
}

type ModifyInput struct {
	Description *string
	Project     *string
	Priority    *string
	Due         *int64
	AddTags     []string
	RemoveTags  []string
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
	}, nil
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
	status := input.Status
	if status == "" {
		status = task.StatusPending
	}
	return s.repo.List(s.workspaceID, sqlite.ListOptions{
		Status: status, Project: input.Project, Priority: input.Priority,
		Tags: input.Tags, Text: input.Text,
	})
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
