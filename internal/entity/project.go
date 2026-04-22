package entity

import (
	"errors"
	"time"
)

var ErrProjectNotFound = errors.New("project not found")

type Project struct {
	ID        int64
	UserID    int64
	Name      string
	Color     string
	IsDefault bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

type ProjectStats struct {
	TotalTasks     int
	CompletedTasks int
	ActiveTasks    int
}

type ProjectDetails struct {
	Project
	Stats ProjectStats
}
