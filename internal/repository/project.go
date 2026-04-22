package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/mmryalloc/tody/internal/entity"
)

const defaultUserProjectName = "Inbox"
const defaultUserProjectColor = "#64748B"

type projectRepository struct {
	db *sql.DB
}

func NewProjectRepository(db *sql.DB) *projectRepository {
	return &projectRepository{db: db}
}

func (r *projectRepository) Create(ctx context.Context, p *entity.Project) error {
	query := `
		INSERT INTO projects (user_id, name, color, is_default)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at
	`
	err := r.db.QueryRowContext(ctx, query, p.UserID, p.Name, p.Color, p.IsDefault).
		Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("repository project create: %w", err)
	}
	return nil
}

func (r *projectRepository) List(ctx context.Context, userID int64, limit, offset int) ([]entity.Project, int, error) {
	query := `
		SELECT id, user_id, name, color, is_default, created_at, updated_at,
		       COUNT(*) OVER () AS total
		FROM projects
		WHERE user_id = $1
		ORDER BY is_default DESC, created_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.db.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("repository project list: %w", err)
	}
	defer rows.Close()

	var (
		projects = []entity.Project{}
		total    int
	)
	for rows.Next() {
		var p entity.Project
		if err := rows.Scan(
			&p.ID, &p.UserID, &p.Name, &p.Color, &p.IsDefault,
			&p.CreatedAt, &p.UpdatedAt, &total,
		); err != nil {
			return nil, 0, fmt.Errorf("repository project list scan: %w", err)
		}
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("repository project list iteration: %w", err)
	}

	if len(projects) == 0 {
		if err := r.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM projects WHERE user_id = $1`, userID,
		).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("repository project list count: %w", err)
		}
	}

	return projects, total, nil
}

func (r *projectRepository) GetByID(ctx context.Context, userID, id int64) (entity.Project, error) {
	query := `
		SELECT id, user_id, name, color, is_default, created_at, updated_at
		FROM projects
		WHERE id = $1 AND user_id = $2
	`
	var p entity.Project
	err := r.db.QueryRowContext(ctx, query, id, userID).Scan(
		&p.ID, &p.UserID, &p.Name, &p.Color, &p.IsDefault, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return entity.Project{}, entity.ErrProjectNotFound
		}
		return entity.Project{}, fmt.Errorf("repository project get: %w", err)
	}
	return p, nil
}

func (r *projectRepository) GetDetails(ctx context.Context, userID, id int64) (entity.ProjectDetails, error) {
	query := `
		SELECT p.id, p.user_id, p.name, p.color, p.is_default, p.created_at, p.updated_at,
		       COUNT(t.id) AS total_tasks,
		       COUNT(t.id) FILTER (WHERE t.completed) AS completed_tasks,
		       COUNT(t.id) FILTER (WHERE NOT t.completed) AS active_tasks
		FROM projects p
		LEFT JOIN tasks t ON t.project_id = p.id
		WHERE p.id = $1 AND p.user_id = $2
		GROUP BY p.id
	`
	var d entity.ProjectDetails
	err := r.db.QueryRowContext(ctx, query, id, userID).Scan(
		&d.ID, &d.UserID, &d.Name, &d.Color, &d.IsDefault, &d.CreatedAt, &d.UpdatedAt,
		&d.Stats.TotalTasks, &d.Stats.CompletedTasks, &d.Stats.ActiveTasks,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return entity.ProjectDetails{}, entity.ErrProjectNotFound
		}
		return entity.ProjectDetails{}, fmt.Errorf("repository project details: %w", err)
	}
	return d, nil
}

func (r *projectRepository) GetDefault(ctx context.Context, userID int64) (entity.Project, error) {
	query := `
		SELECT id, user_id, name, color, is_default, created_at, updated_at
		FROM projects
		WHERE user_id = $1 AND is_default = TRUE
	`
	var p entity.Project
	err := r.db.QueryRowContext(ctx, query, userID).Scan(
		&p.ID, &p.UserID, &p.Name, &p.Color, &p.IsDefault, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return entity.Project{}, entity.ErrProjectNotFound
		}
		return entity.Project{}, fmt.Errorf("repository project get default: %w", err)
	}
	return p, nil
}

func (r *projectRepository) Exists(ctx context.Context, userID, id int64) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM projects WHERE id = $1 AND user_id = $2)`,
		id, userID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("repository project exists: %w", err)
	}
	return exists, nil
}

func (r *projectRepository) Update(ctx context.Context, p *entity.Project) error {
	query := `
		UPDATE projects
		SET name = $1, color = $2, updated_at = NOW()
		WHERE id = $3 AND user_id = $4
		RETURNING updated_at
	`
	err := r.db.QueryRowContext(ctx, query, p.Name, p.Color, p.ID, p.UserID).Scan(&p.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return entity.ErrProjectNotFound
		}
		return fmt.Errorf("repository project update: %w", err)
	}
	return nil
}

func (r *projectRepository) Delete(ctx context.Context, userID, id int64) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM projects WHERE id = $1 AND user_id = $2`,
		id, userID,
	)
	if err != nil {
		return fmt.Errorf("repository project delete: %w", err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("repository project delete rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return entity.ErrProjectNotFound
	}
	return nil
}
