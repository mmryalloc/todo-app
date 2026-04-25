package service

import (
	"context"
	"errors"
	"testing"

	"github.com/mmryalloc/tody/internal/domain"
	"github.com/mmryalloc/tody/internal/pagination"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockProjectRepository struct {
	CreateFunc           func(ctx context.Context, p *domain.Project) error
	ListFunc             func(ctx context.Context, userID int64, limit, offset int) ([]domain.Project, int, error)
	GetByIDFunc          func(ctx context.Context, userID, id int64) (domain.Project, error)
	GetDetailsFunc       func(ctx context.Context, userID, id int64) (domain.ProjectDetails, error)
	UpdateFunc           func(ctx context.Context, p *domain.Project) error
	DeleteFunc           func(ctx context.Context, userID, id int64) error
	GetRoleFunc          func(ctx context.Context, projectID, userID int64) (domain.ProjectRole, error)
	AddMemberByEmailFunc func(ctx context.Context, projectID int64, email string, role domain.ProjectRole) (domain.ProjectMember, error)
	ListMembersFunc      func(ctx context.Context, projectID int64) ([]domain.ProjectMember, error)
	GetMemberFunc        func(ctx context.Context, projectID, userID int64) (domain.ProjectMember, error)
	UpdateMemberRoleFunc func(ctx context.Context, projectID, userID int64, role domain.ProjectRole) (domain.ProjectMember, error)
	DeleteMemberFunc     func(ctx context.Context, projectID, userID int64) error
	CountOwnersFunc      func(ctx context.Context, projectID int64) (int, error)
}

func (m *mockProjectRepository) Create(ctx context.Context, p *domain.Project) error {
	return m.CreateFunc(ctx, p)
}

func (m *mockProjectRepository) List(ctx context.Context, userID int64, limit, offset int) ([]domain.Project, int, error) {
	return m.ListFunc(ctx, userID, limit, offset)
}

func (m *mockProjectRepository) GetByID(ctx context.Context, userID, id int64) (domain.Project, error) {
	return m.GetByIDFunc(ctx, userID, id)
}

func (m *mockProjectRepository) GetDetails(ctx context.Context, userID, id int64) (domain.ProjectDetails, error) {
	return m.GetDetailsFunc(ctx, userID, id)
}

func (m *mockProjectRepository) Update(ctx context.Context, p *domain.Project) error {
	return m.UpdateFunc(ctx, p)
}

func (m *mockProjectRepository) Delete(ctx context.Context, userID, id int64) error {
	return m.DeleteFunc(ctx, userID, id)
}

func (m *mockProjectRepository) GetRole(ctx context.Context, projectID, userID int64) (domain.ProjectRole, error) {
	return m.GetRoleFunc(ctx, projectID, userID)
}

func (m *mockProjectRepository) AddMemberByEmail(ctx context.Context, projectID int64, email string, role domain.ProjectRole) (domain.ProjectMember, error) {
	return m.AddMemberByEmailFunc(ctx, projectID, email, role)
}

func (m *mockProjectRepository) ListMembers(ctx context.Context, projectID int64) ([]domain.ProjectMember, error) {
	return m.ListMembersFunc(ctx, projectID)
}

func (m *mockProjectRepository) GetMember(ctx context.Context, projectID, userID int64) (domain.ProjectMember, error) {
	return m.GetMemberFunc(ctx, projectID, userID)
}

func (m *mockProjectRepository) UpdateMemberRole(ctx context.Context, projectID, userID int64, role domain.ProjectRole) (domain.ProjectMember, error) {
	return m.UpdateMemberRoleFunc(ctx, projectID, userID, role)
}

func (m *mockProjectRepository) DeleteMember(ctx context.Context, projectID, userID int64) error {
	return m.DeleteMemberFunc(ctx, projectID, userID)
}

func (m *mockProjectRepository) CountOwners(ctx context.Context, projectID int64) (int, error) {
	return m.CountOwnersFunc(ctx, projectID)
}

func TestCreateProject(t *testing.T) {
	repo := &mockProjectRepository{
		CreateFunc: func(ctx context.Context, p *domain.Project) error {
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
	projects := []domain.Project{{ID: 1, UserID: testUserID, Name: "Inbox"}}
	repo := &mockProjectRepository{
		ListFunc: func(ctx context.Context, userID int64, limit, offset int) ([]domain.Project, int, error) {
			assert.Equal(t, testUserID, userID)
			assert.Equal(t, 20, limit)
			assert.Equal(t, 40, offset)
			return projects, 1, nil
		},
	}
	s := NewProjectService(repo)

	got, total, err := s.ListProjects(context.Background(), testUserID, pagination.Params{Page: 3, Limit: 20, Offset: 40})
	require.NoError(t, err)
	assert.Equal(t, projects, got)
	assert.Equal(t, 1, total)
}

func TestGetProject(t *testing.T) {
	want := domain.ProjectDetails{
		Project: domain.Project{ID: 1, UserID: testUserID, Name: "Work"},
		Stats:   domain.ProjectStats{TotalTasks: 3, CompletedTasks: 1, ActiveTasks: 2},
	}
	repo := &mockProjectRepository{
		GetDetailsFunc: func(ctx context.Context, userID, id int64) (domain.ProjectDetails, error) {
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
		GetRoleFunc: func(ctx context.Context, projectID, userID int64) (domain.ProjectRole, error) {
			return domain.ProjectRoleEditor, nil
		},
		GetByIDFunc: func(ctx context.Context, userID, id int64) (domain.Project, error) {
			return domain.Project{ID: id, UserID: userID, Name: "Old", Color: "#64748B"}, nil
		},
		UpdateFunc: func(ctx context.Context, p *domain.Project) error {
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
			GetRoleFunc: func(ctx context.Context, projectID, userID int64) (domain.ProjectRole, error) {
				return domain.ProjectRoleOwner, nil
			},
			GetByIDFunc: func(ctx context.Context, userID, id int64) (domain.Project, error) {
				return domain.Project{ID: id, UserID: userID}, nil
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
			GetRoleFunc: func(ctx context.Context, projectID, userID int64) (domain.ProjectRole, error) {
				return domain.ProjectRoleOwner, nil
			},
			GetByIDFunc: func(ctx context.Context, userID, id int64) (domain.Project, error) {
				return domain.Project{ID: id, UserID: userID, IsDefault: true}, nil
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
			GetRoleFunc: func(ctx context.Context, projectID, userID int64) (domain.ProjectRole, error) {
				return "", domain.ErrProjectNotFound
			},
			GetByIDFunc: func(ctx context.Context, userID, id int64) (domain.Project, error) {
				return domain.Project{}, domain.ErrProjectNotFound
			},
			DeleteFunc: func(ctx context.Context, userID, id int64) error {
				return errors.New("must not be called")
			},
		}
		s := NewProjectService(repo)

		err := s.DeleteProject(context.Background(), testUserID, 1)
		require.ErrorIs(t, err, domain.ErrProjectNotFound)
	})
}
