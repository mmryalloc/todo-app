package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/mmryalloc/tody/internal/auth"
	"github.com/mmryalloc/tody/internal/entity"
	"github.com/mmryalloc/tody/internal/service"
)

type createProjectRequest struct {
	Name  string `json:"name" validate:"required,notblank,max=255"`
	Color string `json:"color" validate:"required,hexrgb"`
}

type updateProjectRequest struct {
	Name  string `json:"name" validate:"required,notblank,max=255"`
	Color string `json:"color" validate:"required,hexrgb"`
}

type projectResponse struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	IsDefault bool   `json:"is_default"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type projectDetailsResponse struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	Color          string `json:"color"`
	IsDefault      bool   `json:"is_default"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	TotalTasks     int    `json:"total_tasks"`
	CompletedTasks int    `json:"completed_tasks"`
	ActiveTasks    int    `json:"active_tasks"`
}

type ProjectService interface {
	CreateProject(ctx context.Context, userID int64, in service.CreateProjectInput) (entity.Project, error)
	ListProjects(ctx context.Context, userID int64, page, limit int) ([]entity.Project, int, error)
	GetProject(ctx context.Context, userID, id int64) (entity.ProjectDetails, error)
	UpdateProject(ctx context.Context, userID, id int64, in service.UpdateProjectInput) (entity.Project, error)
	DeleteProject(ctx context.Context, userID, id int64) error
}

type ProjectHandler struct {
	svc ProjectService
}

func NewProjectHandler(svc ProjectService) *ProjectHandler {
	return &ProjectHandler{svc: svc}
}

func (h *ProjectHandler) CreateProject(w http.ResponseWriter, r *http.Request) {
	userID, hasUser := auth.UserIDFromContext(r.Context())
	if !hasUser {
		unauthorized(w, "authentication required")
		return
	}

	var req createProjectRequest
	if !bind(w, r, &req) {
		return
	}

	p, err := h.svc.CreateProject(r.Context(), userID, service.CreateProjectInput{
		Name:  req.Name,
		Color: req.Color,
	})
	if err != nil {
		slog.Error("handler project create", "error", err)
		internalError(w, "failed to create project")
		return
	}

	created(w, projectToResponse(p))
}

func (h *ProjectHandler) ListProjects(w http.ResponseWriter, r *http.Request) {
	userID, hasUser := auth.UserIDFromContext(r.Context())
	if !hasUser {
		unauthorized(w, "authentication required")
		return
	}

	page, limit := pageLimitFromRequest(r)

	projects, total, err := h.svc.ListProjects(r.Context(), userID, page, limit)
	if err != nil {
		slog.Error("handler list projects", "error", err)
		internalError(w, "failed to list projects")
		return
	}

	res := make([]projectResponse, len(projects))
	for i, p := range projects {
		res[i] = projectToResponse(p)
	}

	okPaginated(w, res, page, limit, total)
}

func (h *ProjectHandler) GetProject(w http.ResponseWriter, r *http.Request) {
	userID, hasUser := auth.UserIDFromContext(r.Context())
	if !hasUser {
		unauthorized(w, "authentication required")
		return
	}

	id, valid := parseProjectID(w, r)
	if !valid {
		return
	}

	p, err := h.svc.GetProject(r.Context(), userID, id)
	if err != nil {
		if errors.Is(err, entity.ErrProjectNotFound) {
			notFound(w, "project not found")
			return
		}
		slog.Error("handler get project", "error", err)
		internalError(w, "failed to get project")
		return
	}

	ok(w, projectDetailsToResponse(p))
}

func (h *ProjectHandler) UpdateProject(w http.ResponseWriter, r *http.Request) {
	userID, hasUser := auth.UserIDFromContext(r.Context())
	if !hasUser {
		unauthorized(w, "authentication required")
		return
	}

	id, valid := parseProjectID(w, r)
	if !valid {
		return
	}

	var req updateProjectRequest
	if !bind(w, r, &req) {
		return
	}

	p, err := h.svc.UpdateProject(r.Context(), userID, id, service.UpdateProjectInput{
		Name:  req.Name,
		Color: req.Color,
	})
	if err != nil {
		if errors.Is(err, entity.ErrProjectNotFound) {
			notFound(w, "project not found")
			return
		}
		slog.Error("handler update project", "error", err)
		internalError(w, "failed to update project")
		return
	}

	ok(w, projectToResponse(p))
}

func (h *ProjectHandler) DeleteProject(w http.ResponseWriter, r *http.Request) {
	userID, hasUser := auth.UserIDFromContext(r.Context())
	if !hasUser {
		unauthorized(w, "authentication required")
		return
	}

	id, valid := parseProjectID(w, r)
	if !valid {
		return
	}

	err := h.svc.DeleteProject(r.Context(), userID, id)
	if err != nil {
		if errors.Is(err, service.ErrDefaultProjectDelete) {
			conflict(w, "default project cannot be deleted")
			return
		}
		if errors.Is(err, entity.ErrProjectNotFound) {
			notFound(w, "project not found")
			return
		}
		slog.Error("handler delete project", "error", err)
		internalError(w, "failed to delete project")
		return
	}

	ok(w, struct{}{})
}

func parseProjectID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		badRequest(w, errorCodeBadRequest, "invalid project id", nil)
		return 0, false
	}
	return id, true
}

func projectToResponse(p entity.Project) projectResponse {
	return projectResponse{
		ID:        p.ID,
		Name:      p.Name,
		Color:     p.Color,
		IsDefault: p.IsDefault,
		CreatedAt: p.CreatedAt.Format(time.RFC3339),
		UpdatedAt: p.UpdatedAt.Format(time.RFC3339),
	}
}

func projectDetailsToResponse(p entity.ProjectDetails) projectDetailsResponse {
	return projectDetailsResponse{
		ID:             p.ID,
		Name:           p.Name,
		Color:          p.Color,
		IsDefault:      p.IsDefault,
		CreatedAt:      p.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      p.UpdatedAt.Format(time.RFC3339),
		TotalTasks:     p.Stats.TotalTasks,
		CompletedTasks: p.Stats.CompletedTasks,
		ActiveTasks:    p.Stats.ActiveTasks,
	}
}
