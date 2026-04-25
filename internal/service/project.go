package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/mmryalloc/tody/internal/domain"
	"github.com/mmryalloc/tody/internal/pagination"
)

var ErrDefaultProjectDelete = errors.New("default project cannot be deleted")
var ErrForbidden = errors.New("forbidden")
var ErrInvalidProjectRole = errors.New("invalid project role")
var ErrLastProjectOwner = errors.New("project must have at least one owner")

type CreateProjectInput struct {
	Name  string
	Color string
}

type UpdateProjectInput struct {
	Name  string
	Color string
}

type InviteProjectMemberInput struct {
	Email string
	Role  domain.ProjectRole
}

type UpdateProjectMemberInput struct {
	Role domain.ProjectRole
}

type ProjectRepository interface {
	Create(ctx context.Context, p *domain.Project) error
	List(ctx context.Context, userID int64, limit, offset int) ([]domain.Project, int, error)
	GetByID(ctx context.Context, userID, id int64) (domain.Project, error)
	GetDetails(ctx context.Context, userID, id int64) (domain.ProjectDetails, error)
	Update(ctx context.Context, p *domain.Project) error
	Delete(ctx context.Context, userID, id int64) error
	GetRole(ctx context.Context, projectID, userID int64) (domain.ProjectRole, error)
	AddMemberByEmail(ctx context.Context, projectID int64, email string, role domain.ProjectRole) (domain.ProjectMember, error)
	ListMembers(ctx context.Context, projectID int64) ([]domain.ProjectMember, error)
	GetMember(ctx context.Context, projectID, userID int64) (domain.ProjectMember, error)
	UpdateMemberRole(ctx context.Context, projectID, userID int64, role domain.ProjectRole) (domain.ProjectMember, error)
	DeleteMember(ctx context.Context, projectID, userID int64) error
	CountOwners(ctx context.Context, projectID int64) (int, error)
}

type projectService struct {
	repo ProjectRepository
}

func NewProjectService(repo ProjectRepository) *projectService {
	return &projectService{repo: repo}
}

func (s *projectService) CreateProject(ctx context.Context, userID int64, in CreateProjectInput) (domain.Project, error) {
	p := domain.Project{
		UserID: userID,
		Name:   in.Name,
		Color:  in.Color,
	}
	if err := s.repo.Create(ctx, &p); err != nil {
		return domain.Project{}, fmt.Errorf("service project create: %w", err)
	}
	return p, nil
}

func (s *projectService) ListProjects(ctx context.Context, userID int64, p pagination.Params) ([]domain.Project, int, error) {
	return s.repo.List(ctx, userID, p.Limit, p.Offset)
}

func (s *projectService) GetProject(ctx context.Context, userID, id int64) (domain.ProjectDetails, error) {
	return s.repo.GetDetails(ctx, userID, id)
}

func (s *projectService) UpdateProject(ctx context.Context, userID, id int64, in UpdateProjectInput) (domain.Project, error) {
	if err := s.ensureRole(ctx, id, userID, domain.ProjectRoleOwner, domain.ProjectRoleEditor); err != nil {
		return domain.Project{}, err
	}

	p, err := s.repo.GetByID(ctx, userID, id)
	if err != nil {
		return domain.Project{}, err
	}

	p.Name = in.Name
	p.Color = in.Color

	if err := s.repo.Update(ctx, &p); err != nil {
		return domain.Project{}, err
	}
	return p, nil
}

func (s *projectService) DeleteProject(ctx context.Context, userID, id int64) error {
	if err := s.ensureRole(ctx, id, userID, domain.ProjectRoleOwner); err != nil {
		return err
	}

	p, err := s.repo.GetByID(ctx, userID, id)
	if err != nil {
		return err
	}
	if p.IsDefault {
		return ErrDefaultProjectDelete
	}
	return s.repo.Delete(ctx, userID, id)
}

func (s *projectService) InviteMember(ctx context.Context, actorID, projectID int64, in InviteProjectMemberInput) (domain.ProjectMember, error) {
	if err := validateProjectRole(in.Role); err != nil {
		return domain.ProjectMember{}, err
	}
	if err := s.ensureRole(ctx, projectID, actorID, domain.ProjectRoleOwner); err != nil {
		return domain.ProjectMember{}, err
	}
	return s.repo.AddMemberByEmail(ctx, projectID, in.Email, in.Role)
}

func (s *projectService) ListMembers(ctx context.Context, actorID, projectID int64) ([]domain.ProjectMember, error) {
	if err := s.ensureMember(ctx, projectID, actorID); err != nil {
		return nil, err
	}
	return s.repo.ListMembers(ctx, projectID)
}

func (s *projectService) UpdateMemberRole(ctx context.Context, actorID, projectID, memberID int64, in UpdateProjectMemberInput) (domain.ProjectMember, error) {
	if err := validateProjectRole(in.Role); err != nil {
		return domain.ProjectMember{}, err
	}
	if err := s.ensureRole(ctx, projectID, actorID, domain.ProjectRoleOwner); err != nil {
		return domain.ProjectMember{}, err
	}

	member, err := s.repo.GetMember(ctx, projectID, memberID)
	if err != nil {
		return domain.ProjectMember{}, err
	}
	if member.Role == domain.ProjectRoleOwner && in.Role != domain.ProjectRoleOwner {
		if err := s.ensureAnotherOwner(ctx, projectID); err != nil {
			return domain.ProjectMember{}, err
		}
	}

	return s.repo.UpdateMemberRole(ctx, projectID, memberID, in.Role)
}

func (s *projectService) RemoveMember(ctx context.Context, actorID, projectID, memberID int64) error {
	if actorID != memberID {
		if err := s.ensureRole(ctx, projectID, actorID, domain.ProjectRoleOwner); err != nil {
			return err
		}
	} else if err := s.ensureMember(ctx, projectID, actorID); err != nil {
		return err
	}

	member, err := s.repo.GetMember(ctx, projectID, memberID)
	if err != nil {
		return err
	}
	if member.Role == domain.ProjectRoleOwner {
		if err := s.ensureAnotherOwner(ctx, projectID); err != nil {
			return err
		}
	}

	return s.repo.DeleteMember(ctx, projectID, memberID)
}

func (s *projectService) ensureMember(ctx context.Context, projectID, userID int64) error {
	_, err := s.repo.GetRole(ctx, projectID, userID)
	return err
}

func (s *projectService) ensureRole(ctx context.Context, projectID, userID int64, allowed ...domain.ProjectRole) error {
	role, err := s.repo.GetRole(ctx, projectID, userID)
	if err != nil {
		return err
	}
	for _, candidate := range allowed {
		if role == candidate {
			return nil
		}
	}
	return ErrForbidden
}

func (s *projectService) ensureAnotherOwner(ctx context.Context, projectID int64) error {
	count, err := s.repo.CountOwners(ctx, projectID)
	if err != nil {
		return err
	}
	if count <= 1 {
		return ErrLastProjectOwner
	}
	return nil
}

func validateProjectRole(role domain.ProjectRole) error {
	switch role {
	case domain.ProjectRoleOwner, domain.ProjectRoleEditor, domain.ProjectRoleViewer:
		return nil
	default:
		return ErrInvalidProjectRole
	}
}
