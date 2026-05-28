package app

import (
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
