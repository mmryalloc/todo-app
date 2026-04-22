package service

import (
	"context"
	"errors"
	"testing"

	"github.com/mmryalloc/tody/internal/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockProjectRepository struct {
	CreateFunc     func(ctx context.Context, p *entity.Project) error
	ListFunc       func(ctx context.Context, userID int64, limit, offset int) ([]entity.Project, int, error)
	GetByIDFunc    func(ctx context.Context, userID, id int64) (entity.Project, error)
	GetDetailsFunc func(ctx context.Context, userID, id int64) (entity.ProjectDetails, error)
	UpdateFunc     func(ctx context.Context, p *entity.Project) error
	DeleteFunc     func(ctx context.Context, userID, id int64) error
}

func (m *mockProjectRepository) Create(ctx context.Context, p *entity.Project) error {
	return m.CreateFunc(ctx, p)
}

func (m *mockProjectRepository) List(ctx context.Context, userID int64, limit, offset int) ([]entity.Project, int, error) {
	return m.ListFunc(ctx, userID, limit, offset)
}

func (m *mockProjectRepository) GetByID(ctx context.Context, userID, id int64) (entity.Project, error) {
	return m.GetByIDFunc(ctx, userID, id)
}

func (m *mockProjectRepository) GetDetails(ctx context.Context, userID, id int64) (entity.ProjectDetails, error) {
	return m.GetDetailsFunc(ctx, userID, id)
}

func (m *mockProjectRepository) Update(ctx context.Context, p *entity.Project) error {
	return m.UpdateFunc(ctx, p)
}

func (m *mockProjectRepository) Delete(ctx context.Context, userID, id int64) error {
	return m.DeleteFunc(ctx, userID, id)
}

func TestCreateProject(t *testing.T) {
	repo := &mockProjectRepository{
		CreateFunc: func(ctx context.Context, p *entity.Project) error {
			assert.Equal(t, testUserID, p.UserID)
			assert.Equal(t, "Work", p.Name)
			assert.Equal(t, "#3B82F6", p.Color)
			assert.False(t, p.IsDefault)
			p.ID = 10
			return nil
		},
	}
	s := NewProjectService(repo)

	p, err := s.CreateProject(context.Background(), testUserID, CreateProjectInput{Name: "Work", Color: "#3B82F6"})
	require.NoError(t, err)
	assert.Equal(t, int64(10), p.ID)
}

func TestListProjects(t *testing.T) {
	projects := []entity.Project{{ID: 1, UserID: testUserID, Name: "Inbox"}}
	repo := &mockProjectRepository{
		ListFunc: func(ctx context.Context, userID int64, limit, offset int) ([]entity.Project, int, error) {
			assert.Equal(t, testUserID, userID)
			assert.Equal(t, 100, limit)
			assert.Equal(t, 0, offset)
			return projects, 1, nil
		},
	}
	s := NewProjectService(repo)

	got, total, err := s.ListProjects(context.Background(), testUserID, 0, 1000)
	require.NoError(t, err)
	assert.Equal(t, projects, got)
	assert.Equal(t, 1, total)
}

func TestGetProject(t *testing.T) {
	want := entity.ProjectDetails{
		Project: entity.Project{ID: 1, UserID: testUserID, Name: "Work"},
		Stats:   entity.ProjectStats{TotalTasks: 3, CompletedTasks: 1, ActiveTasks: 2},
	}
	repo := &mockProjectRepository{
		GetDetailsFunc: func(ctx context.Context, userID, id int64) (entity.ProjectDetails, error) {
			assert.Equal(t, testUserID, userID)
			assert.Equal(t, int64(1), id)
			return want, nil
		},
	}
	s := NewProjectService(repo)

	got, err := s.GetProject(context.Background(), testUserID, 1)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestUpdateProject(t *testing.T) {
	repo := &mockProjectRepository{
		GetByIDFunc: func(ctx context.Context, userID, id int64) (entity.Project, error) {
			return entity.Project{ID: id, UserID: userID, Name: "Old", Color: "#64748B"}, nil
		},
		UpdateFunc: func(ctx context.Context, p *entity.Project) error {
			assert.Equal(t, "New", p.Name)
			assert.Equal(t, "#22C55E", p.Color)
			return nil
		},
	}
	s := NewProjectService(repo)

	p, err := s.UpdateProject(context.Background(), testUserID, 1, UpdateProjectInput{Name: "New", Color: "#22C55E"})
	require.NoError(t, err)
	assert.Equal(t, "New", p.Name)
	assert.Equal(t, "#22C55E", p.Color)
}

func TestDeleteProject(t *testing.T) {
	t.Run("ordinary project", func(t *testing.T) {
		deleted := false
		repo := &mockProjectRepository{
			GetByIDFunc: func(ctx context.Context, userID, id int64) (entity.Project, error) {
				return entity.Project{ID: id, UserID: userID}, nil
			},
			DeleteFunc: func(ctx context.Context, userID, id int64) error {
				deleted = true
				assert.Equal(t, testUserID, userID)
				assert.Equal(t, int64(1), id)
				return nil
			},
		}
		s := NewProjectService(repo)

		require.NoError(t, s.DeleteProject(context.Background(), testUserID, 1))
		assert.True(t, deleted)
	})

	t.Run("default project", func(t *testing.T) {
		repo := &mockProjectRepository{
			GetByIDFunc: func(ctx context.Context, userID, id int64) (entity.Project, error) {
				return entity.Project{ID: id, UserID: userID, IsDefault: true}, nil
			},
			DeleteFunc: func(ctx context.Context, userID, id int64) error {
				t.Fatal("default project must not be deleted")
				return nil
			},
		}
		s := NewProjectService(repo)

		err := s.DeleteProject(context.Background(), testUserID, 1)
		require.ErrorIs(t, err, ErrDefaultProjectDelete)
	})

	t.Run("not found", func(t *testing.T) {
		repo := &mockProjectRepository{
			GetByIDFunc: func(ctx context.Context, userID, id int64) (entity.Project, error) {
				return entity.Project{}, entity.ErrProjectNotFound
			},
			DeleteFunc: func(ctx context.Context, userID, id int64) error {
				return errors.New("must not be called")
			},
		}
		s := NewProjectService(repo)

		err := s.DeleteProject(context.Background(), testUserID, 1)
		require.ErrorIs(t, err, entity.ErrProjectNotFound)
	})
}
