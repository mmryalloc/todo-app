package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/mmryalloc/tody/internal/entity"
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
	Role  entity.ProjectRole
}

type UpdateProjectMemberInput struct {
	Role entity.ProjectRole
}

type ProjectRepository interface {
	Create(ctx context.Context, p *entity.Project) error
	List(ctx context.Context, userID int64, limit, offset int) ([]entity.Project, int, error)
	GetByID(ctx context.Context, userID, id int64) (entity.Project, error)
	GetDetails(ctx context.Context, userID, id int64) (entity.ProjectDetails, error)
	Update(ctx context.Context, p *entity.Project) error
	Delete(ctx context.Context, userID, id int64) error
	GetRole(ctx context.Context, projectID, userID int64) (entity.ProjectRole, error)
	AddMemberByEmail(ctx context.Context, projectID int64, email string, role entity.ProjectRole) (entity.ProjectMember, error)
	ListMembers(ctx context.Context, projectID int64) ([]entity.ProjectMember, error)
	GetMember(ctx context.Context, projectID, userID int64) (entity.ProjectMember, error)
	UpdateMemberRole(ctx context.Context, projectID, userID int64, role entity.ProjectRole) (entity.ProjectMember, error)
	DeleteMember(ctx context.Context, projectID, userID int64) error
	CountOwners(ctx context.Context, projectID int64) (int, error)
}

type projectService struct {
	repo ProjectRepository
}

func NewProjectService(repo ProjectRepository) *projectService {
	return &projectService{repo: repo}
}

func (s *projectService) CreateProject(ctx context.Context, userID int64, in CreateProjectInput) (entity.Project, error) {
	p := entity.Project{
		UserID: userID,
		Name:   in.Name,
		Color:  in.Color,
	}
	if err := s.repo.Create(ctx, &p); err != nil {
		return entity.Project{}, fmt.Errorf("service project create: %w", err)
	}
	return p, nil
}

func (s *projectService) ListProjects(ctx context.Context, userID int64, p pagination.Params) ([]entity.Project, int, error) {
	return s.repo.List(ctx, userID, p.Limit, p.Offset)
}

func (s *projectService) GetProject(ctx context.Context, userID, id int64) (entity.ProjectDetails, error) {
	return s.repo.GetDetails(ctx, userID, id)
}

func (s *projectService) UpdateProject(ctx context.Context, userID, id int64, in UpdateProjectInput) (entity.Project, error) {
	if err := s.ensureRole(ctx, id, userID, entity.ProjectRoleOwner, entity.ProjectRoleEditor); err != nil {
		return entity.Project{}, err
	}

	p, err := s.repo.GetByID(ctx, userID, id)
	if err != nil {
		return entity.Project{}, err
	}

	p.Name = in.Name
	p.Color = in.Color

	if err := s.repo.Update(ctx, &p); err != nil {
		return entity.Project{}, err
	}
	return p, nil
}

func (s *projectService) DeleteProject(ctx context.Context, userID, id int64) error {
	if err := s.ensureRole(ctx, id, userID, entity.ProjectRoleOwner); err != nil {
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

func (s *projectService) InviteMember(ctx context.Context, actorID, projectID int64, in InviteProjectMemberInput) (entity.ProjectMember, error) {
	if err := validateProjectRole(in.Role); err != nil {
		return entity.ProjectMember{}, err
	}
	if err := s.ensureRole(ctx, projectID, actorID, entity.ProjectRoleOwner); err != nil {
		return entity.ProjectMember{}, err
	}
	return s.repo.AddMemberByEmail(ctx, projectID, in.Email, in.Role)
}

func (s *projectService) ListMembers(ctx context.Context, actorID, projectID int64) ([]entity.ProjectMember, error) {
	if err := s.ensureMember(ctx, projectID, actorID); err != nil {
		return nil, err
	}
	return s.repo.ListMembers(ctx, projectID)
}

func (s *projectService) UpdateMemberRole(ctx context.Context, actorID, projectID, memberID int64, in UpdateProjectMemberInput) (entity.ProjectMember, error) {
	if err := validateProjectRole(in.Role); err != nil {
		return entity.ProjectMember{}, err
	}
	if err := s.ensureRole(ctx, projectID, actorID, entity.ProjectRoleOwner); err != nil {
		return entity.ProjectMember{}, err
	}

	member, err := s.repo.GetMember(ctx, projectID, memberID)
	if err != nil {
		return entity.ProjectMember{}, err
	}
	if member.Role == entity.ProjectRoleOwner && in.Role != entity.ProjectRoleOwner {
		if err := s.ensureAnotherOwner(ctx, projectID); err != nil {
			return entity.ProjectMember{}, err
		}
	}

	return s.repo.UpdateMemberRole(ctx, projectID, memberID, in.Role)
}

func (s *projectService) RemoveMember(ctx context.Context, actorID, projectID, memberID int64) error {
	if actorID != memberID {
		if err := s.ensureRole(ctx, projectID, actorID, entity.ProjectRoleOwner); err != nil {
			return err
		}
	} else if err := s.ensureMember(ctx, projectID, actorID); err != nil {
		return err
	}

	member, err := s.repo.GetMember(ctx, projectID, memberID)
	if err != nil {
		return err
	}
	if member.Role == entity.ProjectRoleOwner {
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

func (s *projectService) ensureRole(ctx context.Context, projectID, userID int64, allowed ...entity.ProjectRole) error {
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

func validateProjectRole(role entity.ProjectRole) error {
	switch role {
	case entity.ProjectRoleOwner, entity.ProjectRoleEditor, entity.ProjectRoleViewer:
		return nil
	default:
		return ErrInvalidProjectRole
	}
}
