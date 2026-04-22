package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/mmryalloc/tody/internal/entity"
)

var ErrDefaultProjectDelete = errors.New("default project cannot be deleted")

type CreateProjectInput struct {
	Name  string
	Color string
}

type UpdateProjectInput struct {
	Name  string
	Color string
}

type ProjectRepository interface {
	Create(ctx context.Context, p *entity.Project) error
	List(ctx context.Context, userID int64, limit, offset int) ([]entity.Project, int, error)
	GetByID(ctx context.Context, userID, id int64) (entity.Project, error)
	GetDetails(ctx context.Context, userID, id int64) (entity.ProjectDetails, error)
	Update(ctx context.Context, p *entity.Project) error
	Delete(ctx context.Context, userID, id int64) error
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

func (s *projectService) ListProjects(ctx context.Context, userID int64, page, limit int) ([]entity.Project, int, error) {
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

	return s.repo.List(ctx, userID, limit, offset)
}

func (s *projectService) GetProject(ctx context.Context, userID, id int64) (entity.ProjectDetails, error) {
	return s.repo.GetDetails(ctx, userID, id)
}

func (s *projectService) UpdateProject(ctx context.Context, userID, id int64, in UpdateProjectInput) (entity.Project, error) {
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
	p, err := s.repo.GetByID(ctx, userID, id)
	if err != nil {
		return err
	}
	if p.IsDefault {
		return ErrDefaultProjectDelete
	}
	return s.repo.Delete(ctx, userID, id)
}
