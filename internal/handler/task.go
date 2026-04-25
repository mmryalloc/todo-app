package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/mmryalloc/tody/internal/domain"
	"github.com/mmryalloc/tody/internal/pagination"
	"github.com/mmryalloc/tody/internal/service"
)

type createTaskRequest struct {
	ProjectID   *int64             `json:"project_id" validate:"omitempty"`
	Title       string             `json:"title" validate:"required,notblank,max=255"`
	Description string             `json:"description" validate:"max=1000"`
	Status      *domain.TaskStatus `json:"status" validate:"omitempty"`
	DueAt       *time.Time         `json:"due_at" validate:"omitempty"`
	DueTimezone *string            `json:"due_timezone" validate:"omitempty,max=64"`
	Position    *int               `json:"position" validate:"omitempty"`
}

type updateTaskRequest struct {
	ProjectID   *int64             `json:"project_id" validate:"omitempty"`
	Title       *string            `json:"title" validate:"omitempty,notblank,max=255"`
	Description *string            `json:"description" validate:"omitempty,max=1000"`
	Status      *domain.TaskStatus `json:"status" validate:"omitempty"`
	Completed   *bool              `json:"completed" validate:"omitempty"`
	DueAt       *time.Time         `json:"due_at" validate:"omitempty"`
	DueTimezone *string            `json:"due_timezone" validate:"omitempty,max=64"`
	Position    *int               `json:"position" validate:"omitempty"`
}

type updateTaskStatusRequest struct {
	Status domain.TaskStatus `json:"status" validate:"required"`
}

type moveTaskRequest struct {
	ProjectID *int64 `json:"project_id" validate:"omitempty"`
	Position  *int   `json:"position" validate:"omitempty"`
}

type taskResponse struct {
	ID          int64             `json:"id"`
	ProjectID   int64             `json:"project_id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Status      domain.TaskStatus `json:"status"`
	DueAt       *time.Time        `json:"due_at,omitempty"`
	DueTimezone *string           `json:"due_timezone,omitempty"`
	Position    int               `json:"position"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type TaskService interface {
	CreateTask(ctx context.Context, userID int64, t service.CreateTaskInput) (domain.Task, error)
	ListTasks(ctx context.Context, userID int64, f domain.TaskListFilter) ([]domain.Task, int, error)
	GetTask(ctx context.Context, userID, id int64) (domain.Task, error)
	UpdateTask(ctx context.Context, userID, id int64, in service.UpdateTaskInput) (domain.Task, error)
	UpdateTaskStatus(ctx context.Context, userID, id int64, status domain.TaskStatus) (domain.Task, error)
	MoveTask(ctx context.Context, userID, id int64, in service.MoveTaskInput) (domain.Task, error)
	DeleteTask(ctx context.Context, userID, id int64) error
}

type TaskHandler struct {
	svc TaskService
}

func NewTaskHandler(svc TaskService) *TaskHandler {
	return &TaskHandler{
		svc: svc,
	}
}

func (h *TaskHandler) CreateTask(w http.ResponseWriter, r *http.Request) {
	h.createTask(w, r, nil)
}

func (h *TaskHandler) CreateProjectTask(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parsePathInt(w, r, "id", "invalid project id")
	if !ok {
		return
	}
	h.createTask(w, r, &projectID)
}

func (h *TaskHandler) createTask(w http.ResponseWriter, r *http.Request, pathProjectID *int64) {
	userID, hasUser := UserIDFromContext(r.Context())
	if !hasUser {
		unauthorized(w, "authentication required")
		return
	}

	var req createTaskRequest
	if !bind(w, r, &req) {
		return
	}
	if pathProjectID != nil {
		if req.ProjectID != nil && *req.ProjectID != *pathProjectID {
			badRequest(w, errorCodeBadRequest, "project_id does not match path project id", nil)
			return
		}
		req.ProjectID = pathProjectID
	}

	t, err := h.svc.CreateTask(r.Context(), userID, service.CreateTaskInput{
		ProjectID:   req.ProjectID,
		Title:       req.Title,
		Description: req.Description,
		Status:      req.Status,
		DueAt:       req.DueAt,
		DueTimezone: req.DueTimezone,
		Position:    req.Position,
	})
	if err != nil {
		if errors.Is(err, domain.ErrProjectNotFound) {
			notFound(w, "project not found")
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			forbidden(w, "insufficient project permissions")
			return
		}
		if isTaskValidationError(err) {
			unprocessableEntity(w, []errorDetail{{Message: err.Error()}})
			return
		}
		slog.Error("handler task create", "error", err)
		internalError(w, "failed to create task")
		return
	}

	created(w, newTaskResponse(t))
}

func (h *TaskHandler) ListTasks(w http.ResponseWriter, r *http.Request) {
	userID, hasUser := UserIDFromContext(r.Context())
	if !hasUser {
		unauthorized(w, "authentication required")
		return
	}

	pg := pagination.FromRequest(r)
	filter := domain.TaskListFilter{
		Limit:  pg.Limit,
		Offset: pg.Offset,
	}
	if v := r.URL.Query().Get("project_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			badRequest(w, errorCodeBadRequest, "invalid project id", nil)
			return
		}
		filter.ProjectID = &id
	}
	if v := r.URL.Query().Get("status"); v != "" {
		status := domain.TaskStatus(v)
		filter.Status = &status
	}
	if v := r.URL.Query().Get("q"); v != "" {
		filter.Search = v
	}
	if v := r.URL.Query().Get("sort"); v != "" {
		filter.Sort = domain.TaskSort(v)
	}

	tasks, total, err := h.svc.ListTasks(r.Context(), userID, filter)
	if err != nil {
		if errors.Is(err, domain.ErrProjectNotFound) {
			notFound(w, "project not found")
			return
		}
		if errors.Is(err, service.ErrInvalidTaskStatus) {
			badRequest(w, errorCodeBadRequest, "invalid task status", nil)
			return
		}
		if errors.Is(err, service.ErrInvalidTaskSort) {
			badRequest(w, errorCodeBadRequest, "invalid task sort", nil)
			return
		}
		slog.Error("handler list tasks", "error", err)
		internalError(w, "failed to list tasks")
		return
	}

	res := make([]taskResponse, len(tasks))
	for i, t := range tasks {
		res[i] = newTaskResponse(t)
	}

	okPaginated(w, res, pg.Page, pg.Limit, total)
}

func (h *TaskHandler) GetTask(w http.ResponseWriter, r *http.Request) {
	userID, hasUser := UserIDFromContext(r.Context())
	if !hasUser {
		unauthorized(w, "authentication required")
		return
	}

	id, ok := parseTaskID(w, r)
	if !ok {
		return
	}

	t, err := h.svc.GetTask(r.Context(), userID, id)
	if err != nil {
		if errors.Is(err, domain.ErrTaskNotFound) {
			notFound(w, "task not found")
			return
		}
		slog.Error("handler get task", "error", err)
		internalError(w, "failed to get task")
		return
	}

	writeTask(w, t)
}

func (h *TaskHandler) UpdateTask(w http.ResponseWriter, r *http.Request) {
	userID, hasUser := UserIDFromContext(r.Context())
	if !hasUser {
		unauthorized(w, "authentication required")
		return
	}

	id, ok := parseTaskID(w, r)
	if !ok {
		return
	}

	var req updateTaskRequest
	if !bind(w, r, &req) {
		return
	}

	t, err := h.svc.UpdateTask(r.Context(), userID, id, service.UpdateTaskInput{
		ProjectID:   req.ProjectID,
		Title:       req.Title,
		Description: req.Description,
		Status:      req.Status,
		Completed:   req.Completed,
		DueAt:       req.DueAt,
		DueTimezone: req.DueTimezone,
		Position:    req.Position,
	})
	if err != nil {
		if errors.Is(err, domain.ErrProjectNotFound) {
			notFound(w, "project not found")
			return
		}
		if errors.Is(err, domain.ErrTaskNotFound) {
			notFound(w, "task not found")
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			forbidden(w, "insufficient project permissions")
			return
		}
		if isTaskValidationError(err) {
			unprocessableEntity(w, []errorDetail{{Message: err.Error()}})
			return
		}
		slog.Error("handler update task", "error", err)
		internalError(w, "failed to update task")
		return
	}

	writeTask(w, t)
}

func (h *TaskHandler) UpdateTaskStatus(w http.ResponseWriter, r *http.Request) {
	userID, hasUser := UserIDFromContext(r.Context())
	if !hasUser {
		unauthorized(w, "authentication required")
		return
	}

	id, ok := parseTaskID(w, r)
	if !ok {
		return
	}

	var req updateTaskStatusRequest
	if !bind(w, r, &req) {
		return
	}

	t, err := h.svc.UpdateTaskStatus(r.Context(), userID, id, req.Status)
	if err != nil {
		if errors.Is(err, domain.ErrTaskNotFound) {
			notFound(w, "task not found")
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			forbidden(w, "insufficient project permissions")
			return
		}
		if isTaskValidationError(err) {
			unprocessableEntity(w, []errorDetail{{Message: err.Error()}})
			return
		}
		slog.Error("handler update task status", "error", err)
		internalError(w, "failed to update task status")
		return
	}

	writeTask(w, t)
}

func (h *TaskHandler) MoveTask(w http.ResponseWriter, r *http.Request) {
	userID, hasUser := UserIDFromContext(r.Context())
	if !hasUser {
		unauthorized(w, "authentication required")
		return
	}

	id, ok := parseTaskID(w, r)
	if !ok {
		return
	}

	var req moveTaskRequest
	if !bind(w, r, &req) {
		return
	}

	t, err := h.svc.MoveTask(r.Context(), userID, id, service.MoveTaskInput{
		ProjectID: req.ProjectID,
		Position:  req.Position,
	})
	if err != nil {
		if errors.Is(err, domain.ErrProjectNotFound) {
			notFound(w, "project not found")
			return
		}
		if errors.Is(err, domain.ErrTaskNotFound) {
			notFound(w, "task not found")
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			forbidden(w, "insufficient project permissions")
			return
		}
		if isTaskValidationError(err) {
			unprocessableEntity(w, []errorDetail{{Message: err.Error()}})
			return
		}
		slog.Error("handler move task", "error", err)
		internalError(w, "failed to move task")
		return
	}

	writeTask(w, t)
}

func (h *TaskHandler) DeleteTask(w http.ResponseWriter, r *http.Request) {
	userID, hasUser := UserIDFromContext(r.Context())
	if !hasUser {
		unauthorized(w, "authentication required")
		return
	}

	id, valid := parseTaskID(w, r)
	if !valid {
		return
	}

	if err := h.svc.DeleteTask(r.Context(), userID, id); err != nil {
		if errors.Is(err, domain.ErrTaskNotFound) {
			notFound(w, "task not found")
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			forbidden(w, "insufficient project permissions")
			return
		}
		slog.Error("handler delete task", "error", err)
		internalError(w, "failed to delete task")
		return
	}

	ok(w, struct{}{})
}

func parseTaskID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	return parsePathInt(w, r, "id", "invalid task id")
}

func parsePathInt(w http.ResponseWriter, r *http.Request, name, message string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil {
		badRequest(w, errorCodeBadRequest, message, nil)
		return 0, false
	}
	return id, true
}

func writeTask(w http.ResponseWriter, t domain.Task) {
	ok(w, newTaskResponse(t))
}

func newTaskResponse(t domain.Task) taskResponse {
	if !t.Status.IsValid() {
		if t.Completed {
			t.Status = domain.TaskStatusDone
		} else {
			t.Status = domain.TaskStatusOpen
		}
	}
	return taskResponse{
		ID:          t.ID,
		ProjectID:   t.ProjectID,
		Title:       t.Title,
		Description: t.Description,
		Status:      t.Status,
		DueAt:       t.DueAt,
		DueTimezone: t.DueTimezone,
		Position:    t.Position,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
}

func isTaskValidationError(err error) bool {
	return errors.Is(err, service.ErrInvalidTaskStatus) ||
		errors.Is(err, service.ErrInvalidTaskPosition) ||
		errors.Is(err, service.ErrInvalidTaskDeadline)
}
