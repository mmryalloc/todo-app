package entity

import (
	"errors"
	"time"
)

var (
	ErrProjectNotFound       = errors.New("project not found")
	ErrProjectMemberNotFound = errors.New("project member not found")
	ErrProjectMemberExists   = errors.New("project member already exists")
)

type ProjectRole string

const (
	ProjectRoleOwner  ProjectRole = "owner"
	ProjectRoleEditor ProjectRole = "editor"
	ProjectRoleViewer ProjectRole = "viewer"
)

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

type ProjectMember struct {
	ProjectID int64
	UserID    int64
	Email     string
	Name      string
	Role      ProjectRole
	CreatedAt time.Time
	UpdatedAt time.Time
}
