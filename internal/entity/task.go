package entity

import (
	"errors"
	"time"
)

var ErrTaskNotFound = errors.New("task not found")

type Task struct {
	ID          int64
	UserID      int64
	ProjectID   int64
	Title       string
	Description string
	Completed   bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
