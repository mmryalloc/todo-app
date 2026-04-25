package domain

import (
	"errors"
	"time"
)

var ErrTaskNotFound = errors.New("task not found")

type TaskStatus string

const (
	TaskStatusOpen       TaskStatus = "open"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusDone       TaskStatus = "done"
)

func (s TaskStatus) IsValid() bool {
	switch s {
	case TaskStatusOpen, TaskStatusInProgress, TaskStatusDone:
		return true
	default:
		return false
	}
}

type TaskSort string

const (
	TaskSortCreatedAt TaskSort = "created_at"
	TaskSortDueDate   TaskSort = "due_date"
	TaskSortPosition  TaskSort = "position"
	TaskSortStatus    TaskSort = "status"
)

func (s TaskSort) IsValid() bool {
	switch s {
	case "", TaskSortCreatedAt, TaskSortDueDate, TaskSortPosition, TaskSortStatus:
		return true
	default:
		return false
	}
}

type TaskListFilter struct {
	ProjectID *int64
	Status    *TaskStatus
	Search    string
	Sort      TaskSort
	Limit     int
	Offset    int
}

type Task struct {
	ID          int64
	UserID      int64
	ProjectID   int64
	Title       string
	Description string
	Status      TaskStatus
	Completed   bool
	DueAt       *time.Time
	DueTimezone *string
	Position    int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
