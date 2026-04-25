package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/mmryalloc/tody/internal/domain"
)

var ErrInvalidTaskStatus = errors.New("invalid task status")
var ErrInvalidTaskSort = errors.New("invalid task sort")
var ErrInvalidTaskPosition = errors.New("invalid task position")
var ErrInvalidTaskDeadline = errors.New("invalid task deadline")

type CreateTaskInput struct {
	ProjectID   *int64
	Title       string
	Description string
	Status      *domain.TaskStatus
	DueAt       *time.Time
	DueTimezone *string
	Position    *int
}

type UpdateTaskInput struct {
	ProjectID   *int64
	Title       *string
	Description *string
	Status      *domain.TaskStatus
	Completed   *bool
	DueAt       *time.Time
	DueTimezone *string
	Position    *int
}

type MoveTaskInput struct {
	ProjectID *int64
	Position  *int
}

type TaskRepository interface {
	Create(ctx context.Context, t *domain.Task) error
	List(ctx context.Context, userID int64, f domain.TaskListFilter) ([]domain.Task, int, error)
	GetByID(ctx context.Context, userID, id int64) (domain.Task, error)
	Update(ctx context.Context, t *domain.Task) error
	Move(ctx context.Context, t *domain.Task, targetProjectID int64, position *int) error
	Delete(ctx context.Context, userID, id int64) error
}

type TaskProjectRepository interface {
	GetDefault(ctx context.Context, userID int64) (domain.Project, error)
	Exists(ctx context.Context, userID, id int64) (bool, error)
	GetRole(ctx context.Context, projectID, userID int64) (domain.ProjectRole, error)
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

func (s *taskService) CreateTask(ctx context.Context, userID int64, t CreateTaskInput) (domain.Task, error) {
	projectID, err := s.resolveProjectID(ctx, userID, t.ProjectID)
	if err != nil {
		return domain.Task{}, err
	}
	if err := s.ensureProjectWrite(ctx, userID, projectID); err != nil {
		return domain.Task{}, err
	}

	status, err := normalizeTaskStatus(t.Status)
	if err != nil {
		return domain.Task{}, err
	}
	dueAt, dueTimezone, err := normalizeNewDeadline(t.DueAt, t.DueTimezone)
	if err != nil {
		return domain.Task{}, err
	}
	position, err := normalizeTaskPosition(t.Position)
	if err != nil {
		return domain.Task{}, err
	}

	task := domain.Task{
		UserID:      userID,
		ProjectID:   projectID,
		Title:       t.Title,
		Description: t.Description,
		Status:      status,
		Completed:   status == domain.TaskStatusDone,
		DueAt:       dueAt,
		DueTimezone: dueTimezone,
		Position:    position,
	}
	if err := s.repo.Create(ctx, &task); err != nil {
		return domain.Task{}, err
	}
	return task, nil
}

func (s *taskService) ListTasks(ctx context.Context, userID int64, f domain.TaskListFilter) ([]domain.Task, int, error) {
	if !f.Sort.IsValid() {
		return nil, 0, ErrInvalidTaskSort
	}
	if f.Sort == "" {
		f.Sort = domain.TaskSortCreatedAt
	}
	f.Search = strings.TrimSpace(f.Search)
	if f.Status != nil && !f.Status.IsValid() {
		return nil, 0, ErrInvalidTaskStatus
	}
	if f.ProjectID != nil {
		if err := s.ensureProject(ctx, userID, *f.ProjectID); err != nil {
			return nil, 0, err
		}
	}

	return s.repo.List(ctx, userID, f)
}

func (s *taskService) GetTask(ctx context.Context, userID, id int64) (domain.Task, error) {
	return s.repo.GetByID(ctx, userID, id)
}

func (s *taskService) UpdateTask(ctx context.Context, userID, id int64, in UpdateTaskInput) (domain.Task, error) {
	task, err := s.repo.GetByID(ctx, userID, id)
	if err != nil {
		return domain.Task{}, err
	}
	normalizeExistingTaskStatus(&task)
	if err := s.ensureProjectWrite(ctx, userID, task.ProjectID); err != nil {
		return domain.Task{}, err
	}

	if in.ProjectID != nil {
		if err := s.ensureProjectWrite(ctx, userID, *in.ProjectID); err != nil {
			return domain.Task{}, err
		}
		task.ProjectID = *in.ProjectID
	}
	if in.Title != nil {
		task.Title = *in.Title
	}
	if in.Description != nil {
		task.Description = *in.Description
	}
	if in.Status != nil {
		status, err := normalizeTaskStatus(in.Status)
		if err != nil {
			return domain.Task{}, err
		}
		task.Status = status
		task.Completed = status == domain.TaskStatusDone
	}
	if in.Completed != nil {
		task.Completed = *in.Completed
		if *in.Completed {
			task.Status = domain.TaskStatusDone
		} else if task.Status == domain.TaskStatusDone {
			task.Status = domain.TaskStatusOpen
		}
	}
	if in.DueAt != nil || in.DueTimezone != nil {
		dueAt, dueTimezone, err := normalizeUpdatedDeadline(task.DueAt, task.DueTimezone, in.DueAt, in.DueTimezone)
		if err != nil {
			return domain.Task{}, err
		}
		task.DueAt = dueAt
		task.DueTimezone = dueTimezone
	}
	if in.Position != nil {
		if *in.Position < 0 {
			return domain.Task{}, ErrInvalidTaskPosition
		}
		task.Position = *in.Position
	}

	if err := s.repo.Update(ctx, &task); err != nil {
		return domain.Task{}, err
	}

	return task, nil
}

func (s *taskService) UpdateTaskStatus(ctx context.Context, userID, id int64, status domain.TaskStatus) (domain.Task, error) {
	return s.UpdateTask(ctx, userID, id, UpdateTaskInput{Status: &status})
}

func (s *taskService) MoveTask(ctx context.Context, userID, id int64, in MoveTaskInput) (domain.Task, error) {
	if in.Position != nil && *in.Position < 0 {
		return domain.Task{}, ErrInvalidTaskPosition
	}

	task, err := s.repo.GetByID(ctx, userID, id)
	if err != nil {
		return domain.Task{}, err
	}
	normalizeExistingTaskStatus(&task)
	if err := s.ensureProjectWrite(ctx, userID, task.ProjectID); err != nil {
		return domain.Task{}, err
	}

	targetProjectID := task.ProjectID
	if in.ProjectID != nil {
		targetProjectID = *in.ProjectID
		if err := s.ensureProjectWrite(ctx, userID, targetProjectID); err != nil {
			return domain.Task{}, err
		}
	}

	if err := s.repo.Move(ctx, &task, targetProjectID, in.Position); err != nil {
		return domain.Task{}, err
	}
	return task, nil
}

func (s *taskService) DeleteTask(ctx context.Context, userID, id int64) error {
	task, err := s.repo.GetByID(ctx, userID, id)
	if err != nil {
		return err
	}
	if err := s.ensureProjectWrite(ctx, userID, task.ProjectID); err != nil {
		return err
	}
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
		return domain.ErrProjectNotFound
	}
	return nil
}

func normalizeTaskStatus(status *domain.TaskStatus) (domain.TaskStatus, error) {
	if status == nil {
		return domain.TaskStatusOpen, nil
	}
	if !status.IsValid() {
		return "", ErrInvalidTaskStatus
	}
	return *status, nil
}

func normalizeTaskPosition(position *int) (int, error) {
	if position == nil {
		return -1, nil
	}
	if *position < 0 {
		return 0, ErrInvalidTaskPosition
	}
	return *position, nil
}

func normalizeExistingTaskStatus(task *domain.Task) {
	if task.Status.IsValid() {
		return
	}
	if task.Completed {
		task.Status = domain.TaskStatusDone
		return
	}
	task.Status = domain.TaskStatusOpen
}

func normalizeNewDeadline(dueAt *time.Time, dueTimezone *string) (*time.Time, *string, error) {
	if dueAt == nil {
		if dueTimezone != nil && *dueTimezone != "" {
			return nil, nil, ErrInvalidTaskDeadline
		}
		return nil, nil, nil
	}

	timezone := "UTC"
	if dueTimezone != nil && *dueTimezone != "" {
		timezone = *dueTimezone
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return nil, nil, ErrInvalidTaskDeadline
	}

	utc := dueAt.UTC()
	return &utc, &timezone, nil
}

func normalizeUpdatedDeadline(currentDueAt *time.Time, currentTimezone *string, dueAt *time.Time, dueTimezone *string) (*time.Time, *string, error) {
	if dueAt == nil {
		if currentDueAt == nil || dueTimezone == nil || *dueTimezone == "" {
			return nil, nil, ErrInvalidTaskDeadline
		}
		if _, err := time.LoadLocation(*dueTimezone); err != nil {
			return nil, nil, ErrInvalidTaskDeadline
		}
		return currentDueAt, dueTimezone, nil
	}

	timezone := "UTC"
	if currentTimezone != nil && *currentTimezone != "" {
		timezone = *currentTimezone
	}
	if dueTimezone != nil && *dueTimezone != "" {
		timezone = *dueTimezone
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return nil, nil, ErrInvalidTaskDeadline
	}

	utc := dueAt.UTC()
	return &utc, &timezone, nil
}

func (s *taskService) ensureProjectWrite(ctx context.Context, userID, projectID int64) error {
	role, err := s.projects.GetRole(ctx, projectID, userID)
	if err != nil {
		return err
	}
	if role != domain.ProjectRoleOwner && role != domain.ProjectRoleEditor {
		return ErrForbidden
	}
	return nil
}
