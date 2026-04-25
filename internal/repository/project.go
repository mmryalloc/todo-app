package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/mmryalloc/tody/internal/domain"
)

const (
	defaultUserProjectName  = "Inbox"
	defaultUserProjectColor = "#64748B"
)

type projectRepository struct {
	db *sql.DB
}

func NewProjectRepository(db *sql.DB) *projectRepository {
	return &projectRepository{db: db}
}

func (r *projectRepository) Create(ctx context.Context, p *domain.Project) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("repository project create begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	query := `
		INSERT INTO projects (user_id, name, color, is_default)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at
	`
	err = tx.QueryRowContext(ctx, query, p.UserID, p.Name, p.Color, p.IsDefault).
		Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("repository project create: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO project_members (project_id, user_id, role) VALUES ($1, $2, $3)`,
		p.ID, p.UserID, domain.ProjectRoleOwner,
	); err != nil {
		return fmt.Errorf("repository project create owner member: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repository project create commit: %w", err)
	}
	committed = true

	return nil
}

func (r *projectRepository) List(ctx context.Context, userID int64, limit, offset int) ([]domain.Project, int, error) {
	query := `
		SELECT p.id, p.user_id, p.name, p.color, p.is_default, p.created_at, p.updated_at,
		       COUNT(*) OVER () AS total
		FROM projects p
		INNER JOIN project_members pm ON pm.project_id = p.id
		WHERE pm.user_id = $1
		ORDER BY p.is_default DESC, p.created_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.db.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("repository project list: %w", err)
	}
	defer rows.Close()

	var (
		projects = []domain.Project{}
		total    int
	)
	for rows.Next() {
		var p domain.Project
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
			`SELECT COUNT(*) FROM project_members WHERE user_id = $1`, userID,
		).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("repository project list count: %w", err)
		}
	}

	return projects, total, nil
}

func (r *projectRepository) GetByID(ctx context.Context, userID, id int64) (domain.Project, error) {
	query := `
		SELECT p.id, p.user_id, p.name, p.color, p.is_default, p.created_at, p.updated_at
		FROM projects p
		INNER JOIN project_members pm ON pm.project_id = p.id
		WHERE p.id = $1 AND pm.user_id = $2
	`
	var p domain.Project
	err := r.db.QueryRowContext(ctx, query, id, userID).Scan(
		&p.ID, &p.UserID, &p.Name, &p.Color, &p.IsDefault, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Project{}, domain.ErrProjectNotFound
		}
		return domain.Project{}, fmt.Errorf("repository project get: %w", err)
	}
	return p, nil
}

func (r *projectRepository) GetDetails(ctx context.Context, userID, id int64) (domain.ProjectDetails, error) {
	query := `
		SELECT p.id, p.user_id, p.name, p.color, p.is_default, p.created_at, p.updated_at,
		       COUNT(t.id) AS total_tasks,
		       COUNT(t.id) FILTER (WHERE t.completed) AS completed_tasks,
		       COUNT(t.id) FILTER (WHERE NOT t.completed) AS active_tasks
		FROM projects p
		INNER JOIN project_members pm ON pm.project_id = p.id
		LEFT JOIN tasks t ON t.project_id = p.id
		WHERE p.id = $1 AND pm.user_id = $2
		GROUP BY p.id
	`
	var d domain.ProjectDetails
	err := r.db.QueryRowContext(ctx, query, id, userID).Scan(
		&d.ID, &d.UserID, &d.Name, &d.Color, &d.IsDefault, &d.CreatedAt, &d.UpdatedAt,
		&d.Stats.TotalTasks, &d.Stats.CompletedTasks, &d.Stats.ActiveTasks,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ProjectDetails{}, domain.ErrProjectNotFound
		}
		return domain.ProjectDetails{}, fmt.Errorf("repository project details: %w", err)
	}
	return d, nil
}

func (r *projectRepository) GetDefault(ctx context.Context, userID int64) (domain.Project, error) {
	query := `
		SELECT id, user_id, name, color, is_default, created_at, updated_at
		FROM projects
		WHERE user_id = $1 AND is_default = TRUE
	`
	var p domain.Project
	err := r.db.QueryRowContext(ctx, query, userID).Scan(
		&p.ID, &p.UserID, &p.Name, &p.Color, &p.IsDefault, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Project{}, domain.ErrProjectNotFound
		}
		return domain.Project{}, fmt.Errorf("repository project get default: %w", err)
	}
	return p, nil
}

func (r *projectRepository) Exists(ctx context.Context, userID, id int64) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS (
			SELECT 1
			FROM project_members
			WHERE project_id = $1 AND user_id = $2
		)`,
		id, userID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("repository project exists: %w", err)
	}
	return exists, nil
}

func (r *projectRepository) Update(ctx context.Context, p *domain.Project) error {
	query := `
		UPDATE projects
		SET name = $1, color = $2, updated_at = NOW()
		WHERE id = $3
		RETURNING updated_at
	`
	err := r.db.QueryRowContext(ctx, query, p.Name, p.Color, p.ID).Scan(&p.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrProjectNotFound
		}
		return fmt.Errorf("repository project update: %w", err)
	}
	return nil
}

func (r *projectRepository) Delete(ctx context.Context, userID, id int64) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM projects WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("repository project delete: %w", err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("repository project delete rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return domain.ErrProjectNotFound
	}
	return nil
}

func (r *projectRepository) GetRole(ctx context.Context, projectID, userID int64) (domain.ProjectRole, error) {
	var role domain.ProjectRole
	err := r.db.QueryRowContext(ctx,
		`SELECT role FROM project_members WHERE project_id = $1 AND user_id = $2`,
		projectID, userID,
	).Scan(&role)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", domain.ErrProjectNotFound
		}
		return "", fmt.Errorf("repository project get role: %w", err)
	}
	return role, nil
}

func (r *projectRepository) AddMemberByEmail(ctx context.Context, projectID int64, email string, role domain.ProjectRole) (domain.ProjectMember, error) {
	query := `
		INSERT INTO project_members (project_id, user_id, role)
		SELECT $1, u.id, $3
		FROM users u
		WHERE u.email = $2 AND u.deleted_at IS NULL
		RETURNING project_id, user_id, role, created_at, updated_at
	`
	var m domain.ProjectMember
	err := r.db.QueryRowContext(ctx, query, projectID, strings.ToLower(email), role).Scan(
		&m.ProjectID, &m.UserID, &m.Role, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ProjectMember{}, domain.ErrUserNotFound
		}
		if isUniqueViolation(err) {
			return domain.ProjectMember{}, domain.ErrProjectMemberExists
		}
		if isForeignKeyViolation(err) {
			return domain.ProjectMember{}, domain.ErrProjectNotFound
		}
		return domain.ProjectMember{}, fmt.Errorf("repository project add member by email: %w", err)
	}
	return r.GetMember(ctx, projectID, m.UserID)
}

func (r *projectRepository) ListMembers(ctx context.Context, projectID int64) ([]domain.ProjectMember, error) {
	query := `
		SELECT pm.project_id, pm.user_id, u.email, u.name, pm.role, pm.created_at, pm.updated_at
		FROM project_members pm
		INNER JOIN users u ON u.id = pm.user_id
		WHERE pm.project_id = $1 AND u.deleted_at IS NULL
		ORDER BY
			CASE pm.role WHEN 'owner' THEN 1 WHEN 'editor' THEN 2 ELSE 3 END,
			u.email ASC
	`
	rows, err := r.db.QueryContext(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("repository project list members: %w", err)
	}
	defer rows.Close()

	members := []domain.ProjectMember{}
	for rows.Next() {
		var m domain.ProjectMember
		if err := rows.Scan(&m.ProjectID, &m.UserID, &m.Email, &m.Name, &m.Role, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("repository project list members scan: %w", err)
		}
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository project list members iteration: %w", err)
	}
	return members, nil
}

func (r *projectRepository) GetMember(ctx context.Context, projectID, userID int64) (domain.ProjectMember, error) {
	query := `
		SELECT pm.project_id, pm.user_id, u.email, u.name, pm.role, pm.created_at, pm.updated_at
		FROM project_members pm
		INNER JOIN users u ON u.id = pm.user_id
		WHERE pm.project_id = $1 AND pm.user_id = $2 AND u.deleted_at IS NULL
	`
	var m domain.ProjectMember
	err := r.db.QueryRowContext(ctx, query, projectID, userID).Scan(
		&m.ProjectID, &m.UserID, &m.Email, &m.Name, &m.Role, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ProjectMember{}, domain.ErrProjectMemberNotFound
		}
		return domain.ProjectMember{}, fmt.Errorf("repository project get member: %w", err)
	}
	return m, nil
}

func (r *projectRepository) UpdateMemberRole(ctx context.Context, projectID, userID int64, role domain.ProjectRole) (domain.ProjectMember, error) {
	query := `
		UPDATE project_members
		SET role = $1, updated_at = NOW()
		WHERE project_id = $2 AND user_id = $3
		RETURNING updated_at
	`
	var updatedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, query, role, projectID, userID).Scan(&updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ProjectMember{}, domain.ErrProjectMemberNotFound
		}
		return domain.ProjectMember{}, fmt.Errorf("repository project update member role: %w", err)
	}
	return r.GetMember(ctx, projectID, userID)
}

func (r *projectRepository) DeleteMember(ctx context.Context, projectID, userID int64) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM project_members WHERE project_id = $1 AND user_id = $2`,
		projectID, userID,
	)
	if err != nil {
		return fmt.Errorf("repository project delete member: %w", err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("repository project delete member rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return domain.ErrProjectMemberNotFound
	}
	return nil
}

func (r *projectRepository) CountOwners(ctx context.Context, projectID int64) (int, error) {
	var count int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM project_members WHERE project_id = $1 AND role = 'owner'`,
		projectID,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("repository project count owners: %w", err)
	}
	return count, nil
}
