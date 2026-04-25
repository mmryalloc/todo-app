package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mmryalloc/tody/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testUserID int64 = 42

type mockTaskRepository struct {
	CreateFunc  func(ctx context.Context, t *domain.Task) error
	ListFunc    func(ctx context.Context, userID int64, f domain.TaskListFilter) ([]domain.Task, int, error)
	GetByIDFunc func(ctx context.Context, userID, id int64) (domain.Task, error)
	UpdateFunc  func(ctx context.Context, t *domain.Task) error
	MoveFunc    func(ctx context.Context, t *domain.Task, targetProjectID int64, position *int) error
	DeleteFunc  func(ctx context.Context, userID, id int64) error
}

func (m *mockTaskRepository) Create(ctx context.Context, t *domain.Task) error {
	return m.CreateFunc(ctx, t)
}

func (m *mockTaskRepository) List(ctx context.Context, userID int64, f domain.TaskListFilter) ([]domain.Task, int, error) {
	return m.ListFunc(ctx, userID, f)
}

func (m *mockTaskRepository) GetByID(ctx context.Context, userID, id int64) (domain.Task, error) {
	return m.GetByIDFunc(ctx, userID, id)
}

func (m *mockTaskRepository) Update(ctx context.Context, t *domain.Task) error {
	return m.UpdateFunc(ctx, t)
}

func (m *mockTaskRepository) Move(ctx context.Context, t *domain.Task, targetProjectID int64, position *int) error {
	return m.MoveFunc(ctx, t, targetProjectID, position)
}

func (m *mockTaskRepository) Delete(ctx context.Context, userID, id int64) error {
	return m.DeleteFunc(ctx, userID, id)
}

type mockTaskProjectRepository struct {
	GetDefaultFunc func(ctx context.Context, userID int64) (domain.Project, error)
	ExistsFunc     func(ctx context.Context, userID, id int64) (bool, error)
	GetRoleFunc    func(ctx context.Context, projectID, userID int64) (domain.ProjectRole, error)
}

func (m *mockTaskProjectRepository) GetDefault(ctx context.Context, userID int64) (domain.Project, error) {
	return m.GetDefaultFunc(ctx, userID)
}

func (m *mockTaskProjectRepository) Exists(ctx context.Context, userID, id int64) (bool, error) {
	return m.ExistsFunc(ctx, userID, id)
}

func (m *mockTaskProjectRepository) GetRole(ctx context.Context, projectID, userID int64) (domain.ProjectRole, error) {
	return m.GetRoleFunc(ctx, projectID, userID)
}

func defaultProjectMock(projectID int64) TaskProjectRepository {
	return &mockTaskProjectRepository{
		GetDefaultFunc: func(ctx context.Context, userID int64) (domain.Project, error) {
			return domain.Project{ID: projectID, UserID: userID, IsDefault: true}, nil
		},
		ExistsFunc: func(ctx context.Context, userID, id int64) (bool, error) {
			return id == projectID, nil
		},
		GetRoleFunc: func(ctx context.Context, id, userID int64) (domain.ProjectRole, error) {
			if id != projectID {
				return "", domain.ErrProjectNotFound
			}
			return domain.ProjectRoleOwner, nil
		},
	}
}

func TestCreateTask(t *testing.T) {
	tests := []struct {
		name    string
		input   CreateTaskInput
		mock    func(t *testing.T) TaskRepository
		want    domain.Task
		wantErr bool
	}{
		{
			name: "success sets user_id from caller",
			input: CreateTaskInput{
				Title:       "Test Title",
				Description: "Test Description",
			},
			mock: func(t *testing.T) TaskRepository {
				return &mockTaskRepository{
					CreateFunc: func(ctx context.Context, task *domain.Task) error {
						assert.Equal(t, testUserID, task.UserID, "service must propagate userID into entity")
						assert.Equal(t, int64(100), task.ProjectID)
						assert.Equal(t, domain.TaskStatusOpen, task.Status)
						task.ID = 1
						task.Position = 0
						return nil
					},
				}
			},
			want: domain.Task{
				ID:          1,
				UserID:      testUserID,
				ProjectID:   100,
				Title:       "Test Title",
				Description: "Test Description",
				Status:      domain.TaskStatusOpen,
			},
			wantErr: false,
		},
		{
			name: "error",
			input: CreateTaskInput{
				Title:       "Test Title",
				Description: "Test Description",
			},
			mock: func(t *testing.T) TaskRepository {
				return &mockTaskRepository{
					CreateFunc: func(ctx context.Context, task *domain.Task) error {
						return errors.New("db error")
					},
				}
			},
			want:    domain.Task{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewTaskService(tt.mock(t), defaultProjectMock(100))
			got, err := s.CreateTask(context.Background(), testUserID, tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestListTasks(t *testing.T) {
	mockTasks := []domain.Task{
		{ID: 1, UserID: testUserID, ProjectID: 100, Title: "Task 1", Description: "Desc 1", Status: domain.TaskStatusOpen},
		{ID: 2, UserID: testUserID, ProjectID: 100, Title: "Task 2", Description: "Desc 2", Status: domain.TaskStatusDone, Completed: true},
	}

	tests := []struct {
		name      string
		filter    domain.TaskListFilter
		mock      func(t *testing.T) TaskRepository
		wantTasks []domain.Task
		wantTotal int
		wantErr   bool
	}{
		{
			name:   "success forwards filter",
			filter: domain.TaskListFilter{Limit: 10, Offset: 0},
			mock: func(t *testing.T) TaskRepository {
				return &mockTaskRepository{
					ListFunc: func(ctx context.Context, userID int64, f domain.TaskListFilter) ([]domain.Task, int, error) {
						assert.Equal(t, testUserID, userID)
						assert.Nil(t, f.ProjectID)
						assert.Equal(t, 10, f.Limit)
						assert.Equal(t, 0, f.Offset)
						assert.Equal(t, domain.TaskSortCreatedAt, f.Sort)
						return mockTasks, 2, nil
					},
				}
			},
			wantTasks: mockTasks,
			wantTotal: 2,
		},
		{
			name:   "error",
			filter: domain.TaskListFilter{Limit: 10, Offset: 0},
			mock: func(t *testing.T) TaskRepository {
				return &mockTaskRepository{
					ListFunc: func(ctx context.Context, userID int64, f domain.TaskListFilter) ([]domain.Task, int, error) {
						return nil, 0, errors.New("db error")
					},
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewTaskService(tt.mock(t), defaultProjectMock(100))
			gotTasks, gotTotal, err := s.ListTasks(context.Background(), testUserID, tt.filter)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantTasks, gotTasks)
			assert.Equal(t, tt.wantTotal, gotTotal)
		})
	}
}

func TestGetTask(t *testing.T) {
	mockTask := domain.Task{ID: 1, UserID: testUserID, ProjectID: 100, Title: "Task 1", Description: "Desc 1", Status: domain.TaskStatusOpen}

	tests := []struct {
		name    string
		id      int64
		mock    func(t *testing.T) TaskRepository
		want    domain.Task
		wantErr bool
	}{
		{
			name: "success",
			id:   1,
			mock: func(t *testing.T) TaskRepository {
				return &mockTaskRepository{
					GetByIDFunc: func(ctx context.Context, userID, id int64) (domain.Task, error) {
						assert.Equal(t, testUserID, userID)
						assert.Equal(t, int64(1), id)
						return mockTask, nil
					},
				}
			},
			want: mockTask,
		},
		{
			name: "not found",
			id:   1,
			mock: func(t *testing.T) TaskRepository {
				return &mockTaskRepository{
					GetByIDFunc: func(ctx context.Context, userID, id int64) (domain.Task, error) {
						return domain.Task{}, domain.ErrTaskNotFound
					},
				}
			},
			want:    domain.Task{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewTaskService(tt.mock(t), defaultProjectMock(100))
			got, err := s.GetTask(context.Background(), testUserID, tt.id)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestUpdateTask(t *testing.T) {
	mockTask := domain.Task{ID: 1, UserID: testUserID, ProjectID: 100, Title: "Task 1", Description: "Desc 1", Status: domain.TaskStatusOpen, Completed: false}

	newTitle := "New Title"
	newDescription := "New Desc"
	newCompleted := true

	tests := []struct {
		name    string
		id      int64
		input   UpdateTaskInput
		mock    func(t *testing.T) TaskRepository
		want    domain.Task
		wantErr bool
	}{
		{
			name: "success update all fields",
			id:   1,
			input: UpdateTaskInput{
				Title:       &newTitle,
				Description: &newDescription,
				Completed:   &newCompleted,
			},
			mock: func(t *testing.T) TaskRepository {
				return &mockTaskRepository{
					GetByIDFunc: func(ctx context.Context, userID, id int64) (domain.Task, error) {
						assert.Equal(t, testUserID, userID)
						return mockTask, nil
					},
					UpdateFunc: func(ctx context.Context, task *domain.Task) error {
						assert.Equal(t, testUserID, task.UserID, "ownership must be preserved on update")
						return nil
					},
				}
			},
			want: domain.Task{
				ID:          1,
				UserID:      testUserID,
				ProjectID:   100,
				Title:       newTitle,
				Description: newDescription,
				Status:      domain.TaskStatusDone,
				Completed:   newCompleted,
			},
		},
		{
			name: "success partial update",
			id:   1,
			input: UpdateTaskInput{
				Completed: &newCompleted,
			},
			mock: func(t *testing.T) TaskRepository {
				return &mockTaskRepository{
					GetByIDFunc: func(ctx context.Context, userID, id int64) (domain.Task, error) {
						return mockTask, nil
					},
					UpdateFunc: func(ctx context.Context, task *domain.Task) error {
						return nil
					},
				}
			},
			want: domain.Task{
				ID:          1,
				UserID:      testUserID,
				ProjectID:   100,
				Title:       mockTask.Title,
				Description: mockTask.Description,
				Status:      domain.TaskStatusDone,
				Completed:   newCompleted,
			},
		},
		{
			name: "error get by id",
			id:   1,
			input: UpdateTaskInput{
				Title: &newTitle,
			},
			mock: func(t *testing.T) TaskRepository {
				return &mockTaskRepository{
					GetByIDFunc: func(ctx context.Context, userID, id int64) (domain.Task, error) {
						return domain.Task{}, domain.ErrTaskNotFound
					},
				}
			},
			wantErr: true,
		},
		{
			name: "error update",
			id:   1,
			input: UpdateTaskInput{
				Title: &newTitle,
			},
			mock: func(t *testing.T) TaskRepository {
				return &mockTaskRepository{
					GetByIDFunc: func(ctx context.Context, userID, id int64) (domain.Task, error) {
						return mockTask, nil
					},
					UpdateFunc: func(ctx context.Context, task *domain.Task) error {
						return errors.New("update error")
					},
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewTaskService(tt.mock(t), defaultProjectMock(100))
			got, err := s.UpdateTask(context.Background(), testUserID, tt.id, tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestUpdateTaskStatus(t *testing.T) {
	mockTask := domain.Task{ID: 1, UserID: testUserID, ProjectID: 100, Title: "Task 1", Status: domain.TaskStatusOpen}
	status := domain.TaskStatusDone

	s := NewTaskService(&mockTaskRepository{
		GetByIDFunc: func(ctx context.Context, userID, id int64) (domain.Task, error) {
			assert.Equal(t, testUserID, userID)
			assert.Equal(t, int64(1), id)
			return mockTask, nil
		},
		UpdateFunc: func(ctx context.Context, task *domain.Task) error {
			assert.Equal(t, status, task.Status)
			assert.True(t, task.Completed)
			return nil
		},
	}, defaultProjectMock(100))

	got, err := s.UpdateTaskStatus(context.Background(), testUserID, 1, status)
	require.NoError(t, err)
	assert.Equal(t, status, got.Status)
	assert.True(t, got.Completed)
}

func TestMoveTask(t *testing.T) {
	targetProjectID := int64(200)
	position := 2
	mockTask := domain.Task{ID: 1, UserID: testUserID, ProjectID: 100, Title: "Task 1", Status: domain.TaskStatusOpen}

	projects := &mockTaskProjectRepository{
		GetDefaultFunc: func(ctx context.Context, userID int64) (domain.Project, error) {
			return domain.Project{ID: 100, UserID: userID, IsDefault: true}, nil
		},
		ExistsFunc: func(ctx context.Context, userID, id int64) (bool, error) {
			return id == 100 || id == targetProjectID, nil
		},
		GetRoleFunc: func(ctx context.Context, projectID, userID int64) (domain.ProjectRole, error) {
			if projectID != 100 && projectID != targetProjectID {
				return "", domain.ErrProjectNotFound
			}
			return domain.ProjectRoleEditor, nil
		},
	}

	s := NewTaskService(&mockTaskRepository{
		GetByIDFunc: func(ctx context.Context, userID, id int64) (domain.Task, error) {
			return mockTask, nil
		},
		MoveFunc: func(ctx context.Context, task *domain.Task, gotProjectID int64, gotPosition *int) error {
			assert.Equal(t, targetProjectID, gotProjectID)
			require.NotNil(t, gotPosition)
			assert.Equal(t, position, *gotPosition)
			task.ProjectID = gotProjectID
			task.Position = *gotPosition
			return nil
		},
	}, projects)

	got, err := s.MoveTask(context.Background(), testUserID, 1, MoveTaskInput{
		ProjectID: &targetProjectID,
		Position:  &position,
	})
	require.NoError(t, err)
	assert.Equal(t, targetProjectID, got.ProjectID)
	assert.Equal(t, position, got.Position)
}

func TestCreateTaskWithDeadline(t *testing.T) {
	dueAt := time.Date(2026, 5, 1, 12, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	timezone := "Europe/Moscow"

	s := NewTaskService(&mockTaskRepository{
		CreateFunc: func(ctx context.Context, task *domain.Task) error {
			require.NotNil(t, task.DueAt)
			require.NotNil(t, task.DueTimezone)
			assert.Equal(t, dueAt.UTC(), *task.DueAt)
			assert.Equal(t, timezone, *task.DueTimezone)
			task.ID = 1
			task.Position = 0
			return nil
		},
	}, defaultProjectMock(100))

	got, err := s.CreateTask(context.Background(), testUserID, CreateTaskInput{
		Title:       "Task with deadline",
		Description: "Desc",
		DueAt:       &dueAt,
		DueTimezone: &timezone,
	})
	require.NoError(t, err)
	require.NotNil(t, got.DueAt)
	require.NotNil(t, got.DueTimezone)
	assert.Equal(t, dueAt.UTC(), *got.DueAt)
	assert.Equal(t, timezone, *got.DueTimezone)
}

func TestDeleteTask(t *testing.T) {
	tests := []struct {
		name    string
		id      int64
		mock    func(t *testing.T) TaskRepository
		wantErr bool
	}{
		{
			name: "success",
			id:   1,
			mock: func(t *testing.T) TaskRepository {
				return &mockTaskRepository{
					GetByIDFunc: func(ctx context.Context, userID, id int64) (domain.Task, error) {
						return domain.Task{ID: id, UserID: userID, ProjectID: 100}, nil
					},
					DeleteFunc: func(ctx context.Context, userID, id int64) error {
						assert.Equal(t, testUserID, userID)
						assert.Equal(t, int64(1), id)
						return nil
					},
				}
			},
		},
		{
			name: "error",
			id:   1,
			mock: func(t *testing.T) TaskRepository {
				return &mockTaskRepository{
					GetByIDFunc: func(ctx context.Context, userID, id int64) (domain.Task, error) {
						return domain.Task{ID: id, UserID: userID, ProjectID: 100}, nil
					},
					DeleteFunc: func(ctx context.Context, userID, id int64) error {
						return errors.New("delete error")
					},
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewTaskService(tt.mock(t), defaultProjectMock(100))
			err := s.DeleteTask(context.Background(), testUserID, tt.id)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
