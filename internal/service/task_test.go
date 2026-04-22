package service

import (
	"context"
	"errors"
	"testing"

	"github.com/mmryalloc/tody/internal/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testUserID int64 = 42

type mockTaskRepository struct {
	CreateFunc  func(ctx context.Context, t *entity.Task) error
	ListFunc    func(ctx context.Context, userID int64, projectID *int64, limit, offset int) ([]entity.Task, int, error)
	GetByIDFunc func(ctx context.Context, userID, id int64) (entity.Task, error)
	UpdateFunc  func(ctx context.Context, t *entity.Task) error
	DeleteFunc  func(ctx context.Context, userID, id int64) error
}

func (m *mockTaskRepository) Create(ctx context.Context, t *entity.Task) error {
	return m.CreateFunc(ctx, t)
}

func (m *mockTaskRepository) List(ctx context.Context, userID int64, projectID *int64, limit, offset int) ([]entity.Task, int, error) {
	return m.ListFunc(ctx, userID, projectID, limit, offset)
}

func (m *mockTaskRepository) GetByID(ctx context.Context, userID, id int64) (entity.Task, error) {
	return m.GetByIDFunc(ctx, userID, id)
}

func (m *mockTaskRepository) Update(ctx context.Context, t *entity.Task) error {
	return m.UpdateFunc(ctx, t)
}

func (m *mockTaskRepository) Delete(ctx context.Context, userID, id int64) error {
	return m.DeleteFunc(ctx, userID, id)
}

type mockTaskProjectRepository struct {
	GetDefaultFunc func(ctx context.Context, userID int64) (entity.Project, error)
	ExistsFunc     func(ctx context.Context, userID, id int64) (bool, error)
}

func (m *mockTaskProjectRepository) GetDefault(ctx context.Context, userID int64) (entity.Project, error) {
	return m.GetDefaultFunc(ctx, userID)
}

func (m *mockTaskProjectRepository) Exists(ctx context.Context, userID, id int64) (bool, error) {
	return m.ExistsFunc(ctx, userID, id)
}

func defaultProjectMock(projectID int64) TaskProjectRepository {
	return &mockTaskProjectRepository{
		GetDefaultFunc: func(ctx context.Context, userID int64) (entity.Project, error) {
			return entity.Project{ID: projectID, UserID: userID, IsDefault: true}, nil
		},
		ExistsFunc: func(ctx context.Context, userID, id int64) (bool, error) {
			return id == projectID, nil
		},
	}
}

func TestCreateTask(t *testing.T) {
	tests := []struct {
		name    string
		input   CreateTaskInput
		mock    func(t *testing.T) TaskRepository
		want    entity.Task
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
					CreateFunc: func(ctx context.Context, task *entity.Task) error {
						assert.Equal(t, testUserID, task.UserID, "service must propagate userID into entity")
						assert.Equal(t, int64(100), task.ProjectID)
						task.ID = 1
						return nil
					},
				}
			},
			want: entity.Task{
				ID:          1,
				UserID:      testUserID,
				ProjectID:   100,
				Title:       "Test Title",
				Description: "Test Description",
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
					CreateFunc: func(ctx context.Context, task *entity.Task) error {
						return errors.New("db error")
					},
				}
			},
			want:    entity.Task{},
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
	mockTasks := []entity.Task{
		{ID: 1, UserID: testUserID, ProjectID: 100, Title: "Task 1", Description: "Desc 1"},
		{ID: 2, UserID: testUserID, ProjectID: 100, Title: "Task 2", Description: "Desc 2"},
	}

	tests := []struct {
		name      string
		page      int
		limit     int
		mock      func(t *testing.T) TaskRepository
		wantTasks []entity.Task
		wantTotal int
		wantErr   bool
	}{
		{
			name:  "success",
			page:  1,
			limit: 10,
			mock: func(t *testing.T) TaskRepository {
				return &mockTaskRepository{
					ListFunc: func(ctx context.Context, userID int64, projectID *int64, limit, offset int) ([]entity.Task, int, error) {
						assert.Equal(t, testUserID, userID)
						assert.Nil(t, projectID)
						assert.Equal(t, 10, limit)
						assert.Equal(t, 0, offset)
						return mockTasks, 2, nil
					},
				}
			},
			wantTasks: mockTasks,
			wantTotal: 2,
		},
		{
			name:  "limit clamped to 100",
			page:  0,
			limit: 1000,
			mock: func(t *testing.T) TaskRepository {
				return &mockTaskRepository{
					ListFunc: func(ctx context.Context, userID int64, projectID *int64, limit, offset int) ([]entity.Task, int, error) {
						assert.Equal(t, 100, limit, "limit > 100 must be clamped, not silently replaced")
						assert.Equal(t, 0, offset, "page < 1 must be normalised to 1")
						return mockTasks, 2, nil
					},
				}
			},
			wantTasks: mockTasks,
			wantTotal: 2,
		},
		{
			name:  "error",
			page:  1,
			limit: 10,
			mock: func(t *testing.T) TaskRepository {
				return &mockTaskRepository{
					ListFunc: func(ctx context.Context, userID int64, projectID *int64, limit, offset int) ([]entity.Task, int, error) {
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
			gotTasks, gotTotal, err := s.ListTasks(context.Background(), testUserID, nil, tt.page, tt.limit)
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
	mockTask := entity.Task{ID: 1, UserID: testUserID, ProjectID: 100, Title: "Task 1", Description: "Desc 1"}

	tests := []struct {
		name    string
		id      int64
		mock    func(t *testing.T) TaskRepository
		want    entity.Task
		wantErr bool
	}{
		{
			name: "success",
			id:   1,
			mock: func(t *testing.T) TaskRepository {
				return &mockTaskRepository{
					GetByIDFunc: func(ctx context.Context, userID, id int64) (entity.Task, error) {
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
					GetByIDFunc: func(ctx context.Context, userID, id int64) (entity.Task, error) {
						return entity.Task{}, entity.ErrTaskNotFound
					},
				}
			},
			want:    entity.Task{},
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
	mockTask := entity.Task{ID: 1, UserID: testUserID, ProjectID: 100, Title: "Task 1", Description: "Desc 1", Completed: false}

	newTitle := "New Title"
	newDescription := "New Desc"
	newCompleted := true

	tests := []struct {
		name    string
		id      int64
		input   UpdateTaskInput
		mock    func(t *testing.T) TaskRepository
		want    entity.Task
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
					GetByIDFunc: func(ctx context.Context, userID, id int64) (entity.Task, error) {
						assert.Equal(t, testUserID, userID)
						return mockTask, nil
					},
					UpdateFunc: func(ctx context.Context, task *entity.Task) error {
						assert.Equal(t, testUserID, task.UserID, "ownership must be preserved on update")
						return nil
					},
				}
			},
			want: entity.Task{
				ID:          1,
				UserID:      testUserID,
				ProjectID:   100,
				Title:       newTitle,
				Description: newDescription,
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
					GetByIDFunc: func(ctx context.Context, userID, id int64) (entity.Task, error) {
						return mockTask, nil
					},
					UpdateFunc: func(ctx context.Context, task *entity.Task) error {
						return nil
					},
				}
			},
			want: entity.Task{
				ID:          1,
				UserID:      testUserID,
				ProjectID:   100,
				Title:       mockTask.Title,
				Description: mockTask.Description,
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
					GetByIDFunc: func(ctx context.Context, userID, id int64) (entity.Task, error) {
						return entity.Task{}, entity.ErrTaskNotFound
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
					GetByIDFunc: func(ctx context.Context, userID, id int64) (entity.Task, error) {
						return mockTask, nil
					},
					UpdateFunc: func(ctx context.Context, task *entity.Task) error {
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
