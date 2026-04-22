package service

import (
	"context"

	"github.com/mmryalloc/tody/internal/entity"
)

type CreateTaskInput struct {
	ProjectID   *int64
	Title       string
	Description string
}

type UpdateTaskInput struct {
	ProjectID   *int64
	Title       *string
	Description *string
	Completed   *bool
}

type TaskRepository interface {
	Create(ctx context.Context, t *entity.Task) error
	List(ctx context.Context, userID int64, projectID *int64, limit, offset int) ([]entity.Task, int, error)
	GetByID(ctx context.Context, userID, id int64) (entity.Task, error)
	Update(ctx context.Context, t *entity.Task) error
	Delete(ctx context.Context, userID, id int64) error
}

type TaskProjectRepository interface {
	GetDefault(ctx context.Context, userID int64) (entity.Project, error)
	Exists(ctx context.Context, userID, id int64) (bool, error)
}

type taskService struct {
	repo     TaskRepository
	projects TaskProjectRepository
}

func NewTaskService(repo TaskRepository, projects TaskProjectRepository) *taskService {
	return &taskService{
		repo:     repo,
		projects: projects,
	}
}

func (s *taskService) CreateTask(ctx context.Context, userID int64, t CreateTaskInput) (entity.Task, error) {
	projectID, err := s.resolveProjectID(ctx, userID, t.ProjectID)
	if err != nil {
		return entity.Task{}, err
	}

	task := entity.Task{
		UserID:      userID,
		ProjectID:   projectID,
		Title:       t.Title,
		Description: t.Description,
	}
	if err := s.repo.Create(ctx, &task); err != nil {
		return entity.Task{}, err
	}
	return task, nil
}

func (s *taskService) ListTasks(ctx context.Context, userID int64, projectID *int64, page, limit int) ([]entity.Task, int, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	offset := (page - 1) * limit

	if projectID != nil {
		if err := s.ensureProject(ctx, userID, *projectID); err != nil {
			return nil, 0, err
		}
	}

	return s.repo.List(ctx, userID, projectID, limit, offset)
}

func (s *taskService) GetTask(ctx context.Context, userID, id int64) (entity.Task, error) {
	return s.repo.GetByID(ctx, userID, id)
}

func (s *taskService) UpdateTask(ctx context.Context, userID, id int64, in UpdateTaskInput) (entity.Task, error) {
	task, err := s.repo.GetByID(ctx, userID, id)
	if err != nil {
		return entity.Task{}, err
	}

	if in.ProjectID != nil {
		if err := s.ensureProject(ctx, userID, *in.ProjectID); err != nil {
			return entity.Task{}, err
		}
		task.ProjectID = *in.ProjectID
	}
	if in.Title != nil {
		task.Title = *in.Title
	}
	if in.Description != nil {
		task.Description = *in.Description
	}
	if in.Completed != nil {
		task.Completed = *in.Completed
	}

	if err := s.repo.Update(ctx, &task); err != nil {
		return entity.Task{}, err
	}

	return task, nil
}

func (s *taskService) DeleteTask(ctx context.Context, userID, id int64) error {
	return s.repo.Delete(ctx, userID, id)
}

func (s *taskService) resolveProjectID(ctx context.Context, userID int64, projectID *int64) (int64, error) {
	if projectID != nil {
		if err := s.ensureProject(ctx, userID, *projectID); err != nil {
			return 0, err
		}
		return *projectID, nil
	}

	p, err := s.projects.GetDefault(ctx, userID)
	if err != nil {
		return 0, err
	}
	return p.ID, nil
}

func (s *taskService) ensureProject(ctx context.Context, userID, projectID int64) error {
	exists, err := s.projects.Exists(ctx, userID, projectID)
	if err != nil {
		return err
	}
	if !exists {
		return entity.ErrProjectNotFound
	}
	return nil
}
