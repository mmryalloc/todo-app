package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/mmryalloc/tody/internal/entity"
	"github.com/mmryalloc/tody/internal/pagination"
	"github.com/mmryalloc/tody/internal/service"
)

type createTaskRequest struct {
	ProjectID   *int64 `json:"project_id" validate:"omitempty"`
	Title       string `json:"title" validate:"required,notblank,max=255"`
	Description string `json:"description" validate:"max=1000"`
}

type updateTaskRequest struct {
	ProjectID   *int64  `json:"project_id" validate:"omitempty"`
	Title       *string `json:"title" validate:"omitempty,notblank,max=255"`
	Description *string `json:"description" validate:"omitempty,max=1000"`
	Completed   *bool   `json:"completed" validate:"omitempty"`
}

type taskResponse struct {
	ID          int64  `json:"id"`
	ProjectID   int64  `json:"project_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Completed   bool   `json:"completed"`
}

type TaskService interface {
	CreateTask(ctx context.Context, userID int64, t service.CreateTaskInput) (entity.Task, error)
	ListTasks(ctx context.Context, userID int64, projectID *int64, p pagination.Params) ([]entity.Task, int, error)
	GetTask(ctx context.Context, userID, id int64) (entity.Task, error)
	UpdateTask(ctx context.Context, userID, id int64, in service.UpdateTaskInput) (entity.Task, error)
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
	userID, hasUser := UserIDFromContext(r.Context())
	if !hasUser {
		unauthorized(w, "authentication required")
		return
	}

	var req createTaskRequest
	if !bind(w, r, &req) {
		return
	}

	t, err := h.svc.CreateTask(r.Context(), userID, service.CreateTaskInput{
		ProjectID:   req.ProjectID,
		Title:       req.Title,
		Description: req.Description,
	})
	if err != nil {
		if errors.Is(err, entity.ErrProjectNotFound) {
			notFound(w, "project not found")
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			forbidden(w, "insufficient project permissions")
			return
		}
		slog.Error("handler task create", "error", err)
		internalError(w, "failed to create task")
		return
	}

	created(w, taskResponse{
		ID:          t.ID,
		ProjectID:   t.ProjectID,
		Title:       t.Title,
		Description: t.Description,
		Completed:   t.Completed,
	})
}

func (h *TaskHandler) ListTasks(w http.ResponseWriter, r *http.Request) {
	userID, hasUser := UserIDFromContext(r.Context())
	if !hasUser {
		unauthorized(w, "authentication required")
		return
	}

	pg := pagination.FromRequest(r)
	var projectID *int64
	if v := r.URL.Query().Get("project_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			badRequest(w, errorCodeBadRequest, "invalid project id", nil)
			return
		}
		projectID = &id
	}

	tasks, total, err := h.svc.ListTasks(r.Context(), userID, projectID, pg)
	if err != nil {
		if errors.Is(err, entity.ErrProjectNotFound) {
			notFound(w, "project not found")
			return
		}
		slog.Error("handler list tasks", "error", err)
		internalError(w, "failed to list tasks")
		return
	}

	res := make([]taskResponse, len(tasks))
	for i, t := range tasks {
		res[i] = taskResponse{
			ID:          t.ID,
			ProjectID:   t.ProjectID,
			Title:       t.Title,
			Description: t.Description,
			Completed:   t.Completed,
		}
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
		if errors.Is(err, entity.ErrTaskNotFound) {
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
		Completed:   req.Completed,
	})
	if err != nil {
		if errors.Is(err, entity.ErrProjectNotFound) {
			notFound(w, "project not found")
			return
		}
		if errors.Is(err, entity.ErrTaskNotFound) {
			notFound(w, "task not found")
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			forbidden(w, "insufficient project permissions")
			return
		}
		slog.Error("handler update task", "error", err)
		internalError(w, "failed to update task")
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
		if errors.Is(err, entity.ErrTaskNotFound) {
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
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		badRequest(w, errorCodeBadRequest, "invalid task id", nil)
		return 0, false
	}
	return id, true
}

func writeTask(w http.ResponseWriter, t entity.Task) {
	ok(w, taskResponse{
		ID:          t.ID,
		ProjectID:   t.ProjectID,
		Title:       t.Title,
		Description: t.Description,
		Completed:   t.Completed,
	})
}
