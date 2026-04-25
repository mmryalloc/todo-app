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

type createProjectRequest struct {
	Name  string `json:"name" validate:"required,notblank,max=255"`
	Color string `json:"color" validate:"required,hexrgb"`
}

type updateProjectRequest struct {
	Name  string `json:"name" validate:"required,notblank,max=255"`
	Color string `json:"color" validate:"required,hexrgb"`
}

type inviteProjectMemberRequest struct {
	Email string `json:"email" validate:"required,email,max=255"`
	Role  string `json:"role" validate:"required,oneof=owner editor viewer"`
}

type updateProjectMemberRequest struct {
	Role string `json:"role" validate:"required,oneof=owner editor viewer"`
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

type projectMemberResponse struct {
	UserID    int64  `json:"user_id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type ProjectService interface {
	CreateProject(ctx context.Context, userID int64, in service.CreateProjectInput) (domain.Project, error)
	ListProjects(ctx context.Context, userID int64, p pagination.Params) ([]domain.Project, int, error)
	GetProject(ctx context.Context, userID, id int64) (domain.ProjectDetails, error)
	UpdateProject(ctx context.Context, userID, id int64, in service.UpdateProjectInput) (domain.Project, error)
	DeleteProject(ctx context.Context, userID, id int64) error
	InviteMember(ctx context.Context, actorID, projectID int64, in service.InviteProjectMemberInput) (domain.ProjectMember, error)
	ListMembers(ctx context.Context, actorID, projectID int64) ([]domain.ProjectMember, error)
	UpdateMemberRole(ctx context.Context, actorID, projectID, memberID int64, in service.UpdateProjectMemberInput) (domain.ProjectMember, error)
	RemoveMember(ctx context.Context, actorID, projectID, memberID int64) error
}

type ProjectHandler struct {
	svc ProjectService
}

func NewProjectHandler(svc ProjectService) *ProjectHandler {
	return &ProjectHandler{svc: svc}
}

func (h *ProjectHandler) CreateProject(w http.ResponseWriter, r *http.Request) {
	userID, hasUser := UserIDFromContext(r.Context())
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
	userID, hasUser := UserIDFromContext(r.Context())
	if !hasUser {
		unauthorized(w, "authentication required")
		return
	}

	pg := pagination.FromRequest(r)

	projects, total, err := h.svc.ListProjects(r.Context(), userID, pg)
	if err != nil {
		slog.Error("handler list projects", "error", err)
		internalError(w, "failed to list projects")
		return
	}

	res := make([]projectResponse, len(projects))
	for i, p := range projects {
		res[i] = projectToResponse(p)
	}

	okPaginated(w, res, pg.Page, pg.Limit, total)
}

func (h *ProjectHandler) GetProject(w http.ResponseWriter, r *http.Request) {
	userID, hasUser := UserIDFromContext(r.Context())
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
		if errors.Is(err, domain.ErrProjectNotFound) {
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
	userID, hasUser := UserIDFromContext(r.Context())
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
		if handleProjectError(w, err) {
			return
		}
		slog.Error("handler update project", "error", err)
		internalError(w, "failed to update project")
		return
	}

	ok(w, projectToResponse(p))
}

func (h *ProjectHandler) DeleteProject(w http.ResponseWriter, r *http.Request) {
	userID, hasUser := UserIDFromContext(r.Context())
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
		if handleProjectError(w, err) {
			return
		}
		slog.Error("handler delete project", "error", err)
		internalError(w, "failed to delete project")
		return
	}

	ok(w, struct{}{})
}

func (h *ProjectHandler) InviteMember(w http.ResponseWriter, r *http.Request) {
	userID, hasUser := UserIDFromContext(r.Context())
	if !hasUser {
		unauthorized(w, "authentication required")
		return
	}

	projectID, valid := parseProjectID(w, r)
	if !valid {
		return
	}

	var req inviteProjectMemberRequest
	if !bind(w, r, &req) {
		return
	}

	member, err := h.svc.InviteMember(r.Context(), userID, projectID, service.InviteProjectMemberInput{
		Email: req.Email,
		Role:  domain.ProjectRole(req.Role),
	})
	if err != nil {
		if handleProjectError(w, err) {
			return
		}
		if errors.Is(err, domain.ErrUserNotFound) {
			notFound(w, "user not found")
			return
		}
		if errors.Is(err, domain.ErrProjectMemberExists) {
			conflict(w, "project member already exists")
			return
		}
		slog.Error("handler invite project member", "error", err)
		internalError(w, "failed to invite project member")
		return
	}

	created(w, projectMemberToResponse(member))
}

func (h *ProjectHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	userID, hasUser := UserIDFromContext(r.Context())
	if !hasUser {
		unauthorized(w, "authentication required")
		return
	}

	projectID, valid := parseProjectID(w, r)
	if !valid {
		return
	}

	members, err := h.svc.ListMembers(r.Context(), userID, projectID)
	if err != nil {
		if handleProjectError(w, err) {
			return
		}
		slog.Error("handler list project members", "error", err)
		internalError(w, "failed to list project members")
		return
	}

	res := make([]projectMemberResponse, len(members))
	for i, m := range members {
		res[i] = projectMemberToResponse(m)
	}
	ok(w, res)
}

func (h *ProjectHandler) UpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	userID, hasUser := UserIDFromContext(r.Context())
	if !hasUser {
		unauthorized(w, "authentication required")
		return
	}

	projectID, valid := parseProjectID(w, r)
	if !valid {
		return
	}
	memberID, valid := parseMemberUserID(w, r)
	if !valid {
		return
	}

	var req updateProjectMemberRequest
	if !bind(w, r, &req) {
		return
	}

	member, err := h.svc.UpdateMemberRole(r.Context(), userID, projectID, memberID, service.UpdateProjectMemberInput{
		Role: domain.ProjectRole(req.Role),
	})
	if err != nil {
		if handleProjectError(w, err) {
			return
		}
		slog.Error("handler update project member role", "error", err)
		internalError(w, "failed to update project member role")
		return
	}

	ok(w, projectMemberToResponse(member))
}

func (h *ProjectHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	userID, hasUser := UserIDFromContext(r.Context())
	if !hasUser {
		unauthorized(w, "authentication required")
		return
	}

	projectID, valid := parseProjectID(w, r)
	if !valid {
		return
	}
	memberID, valid := parseMemberUserID(w, r)
	if !valid {
		return
	}

	err := h.svc.RemoveMember(r.Context(), userID, projectID, memberID)
	if err != nil {
		if handleProjectError(w, err) {
			return
		}
		slog.Error("handler remove project member", "error", err)
		internalError(w, "failed to remove project member")
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

func parseMemberUserID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("user_id"), 10, 64)
	if err != nil {
		badRequest(w, errorCodeBadRequest, "invalid user id", nil)
		return 0, false
	}
	return id, true
}

func handleProjectError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, domain.ErrProjectNotFound):
		notFound(w, "project not found")
	case errors.Is(err, domain.ErrProjectMemberNotFound):
		notFound(w, "project member not found")
	case errors.Is(err, service.ErrDefaultProjectDelete):
		conflict(w, "default project cannot be deleted")
	case errors.Is(err, service.ErrForbidden):
		forbidden(w, "insufficient project permissions")
	case errors.Is(err, service.ErrInvalidProjectRole):
		badRequest(w, errorCodeBadRequest, "invalid project role", nil)
	case errors.Is(err, service.ErrLastProjectOwner):
		conflict(w, "project must have at least one owner")
	default:
		return false
	}
	return true
}

func projectToResponse(p domain.Project) projectResponse {
	return projectResponse{
		ID:        p.ID,
		Name:      p.Name,
		Color:     p.Color,
		IsDefault: p.IsDefault,
		CreatedAt: p.CreatedAt.Format(time.RFC3339),
		UpdatedAt: p.UpdatedAt.Format(time.RFC3339),
	}
}

func projectDetailsToResponse(p domain.ProjectDetails) projectDetailsResponse {
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

func projectMemberToResponse(m domain.ProjectMember) projectMemberResponse {
	return projectMemberResponse{
		UserID:    m.UserID,
		Email:     m.Email,
		Name:      m.Name,
		Role:      string(m.Role),
		CreatedAt: m.CreatedAt.Format(time.RFC3339),
		UpdatedAt: m.UpdatedAt.Format(time.RFC3339),
	}
}
