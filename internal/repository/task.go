package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/mmryalloc/tody/internal/domain"
)

type taskRepository struct {
	db *sql.DB
}

func NewTaskRepository(db *sql.DB) *taskRepository {
	return &taskRepository{db: db}
}

func (r *taskRepository) Create(ctx context.Context, t *domain.Task) error {
	query := `
		INSERT INTO tasks (
			user_id, project_id, title, description, status, completed, due_at, due_timezone, position
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8,
			CASE
				WHEN $9::integer >= 0 THEN $9
				ELSE COALESCE((SELECT MAX(position) + 1 FROM tasks WHERE project_id = $2), 0)
			END
		)
		RETURNING id, position, created_at, updated_at
	`
	err := r.db.QueryRowContext(
		ctx,
		query,
		t.UserID,
		t.ProjectID,
		t.Title,
		t.Description,
		t.Status,
		t.Completed,
		t.DueAt,
		t.DueTimezone,
		t.Position,
	).Scan(&t.ID, &t.Position, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return fmt.Errorf("repository task create: %w", err)
	}

	t.Completed = t.Status == domain.TaskStatusDone
	return nil
}

func (r *taskRepository) List(ctx context.Context, userID int64, f domain.TaskListFilter) ([]domain.Task, int, error) {
	where, args := taskListWhere(userID, f)
	countArgs := append([]any(nil), args...)
	query := fmt.Sprintf(`
		SELECT id, user_id, project_id, title, description, status, completed,
		       due_at, due_timezone, position, created_at, updated_at,
		       COUNT(*) OVER () AS total
		FROM tasks
		WHERE %s
		ORDER BY %s
		LIMIT $%d OFFSET $%d
	`, strings.Join(where, " AND "), taskOrderBy(f.Sort), len(args)+1, len(args)+2)
	args = append(args, f.Limit, f.Offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("repository task list: %w", err)
	}
	defer rows.Close()

	var (
		tasks = []domain.Task{}
		total int
	)
	for rows.Next() {
		var t domain.Task
		if err := scanTaskWithTotal(rows, &t, &total); err != nil {
			return nil, 0, fmt.Errorf("repository task list scan: %w", err)
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("repository task list iteration: %w", err)
	}

	if len(tasks) == 0 {
		countQuery := fmt.Sprintf(`
			SELECT COUNT(*)
			FROM tasks
			WHERE %s
		`, strings.Join(where, " AND "))
		if err := r.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("repository task list count: %w", err)
		}
	}

	return tasks, total, nil
}

func (r *taskRepository) GetByID(ctx context.Context, userID, id int64) (domain.Task, error) {
	query := `
		SELECT id, user_id, project_id, title, description, status, completed,
		       due_at, due_timezone, position, created_at, updated_at
		FROM tasks
		WHERE id = $1
		  AND EXISTS (
			SELECT 1
			FROM project_members pm
			WHERE pm.project_id = tasks.project_id
			  AND pm.user_id = $2
		  )
	`
	var t domain.Task
	err := scanTask(r.db.QueryRowContext(ctx, query, id, userID), &t)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Task{}, domain.ErrTaskNotFound
		}
		return domain.Task{}, fmt.Errorf("repository task get: %w", err)
	}

	return t, nil
}

func (r *taskRepository) Update(ctx context.Context, t *domain.Task) error {
	query := `
		UPDATE tasks
		SET project_id = $1, title = $2, description = $3, status = $4, completed = $5,
		    due_at = $6, due_timezone = $7, position = $8, updated_at = NOW()
		WHERE id = $9
		  AND EXISTS (
			SELECT 1
			FROM project_members pm
			WHERE pm.project_id = tasks.project_id
			  AND pm.user_id = $10
		  )
		RETURNING updated_at
	`
	err := r.db.QueryRowContext(
		ctx,
		query,
		t.ProjectID,
		t.Title,
		t.Description,
		t.Status,
		t.Completed,
		t.DueAt,
		t.DueTimezone,
		t.Position,
		t.ID,
		t.UserID,
	).Scan(&t.UpdatedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrTaskNotFound
		}
		return fmt.Errorf("repository task update: %w", err)
	}

	return nil
}

func (r *taskRepository) Move(ctx context.Context, t *domain.Task, targetProjectID int64, position *int) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("repository task move begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	targetPosition, err := taskTargetPosition(ctx, tx, t.ID, t.ProjectID, targetProjectID, position)
	if err != nil {
		return err
	}

	if t.ProjectID == targetProjectID {
		if targetPosition < t.Position {
			if _, err := tx.ExecContext(ctx, `
				UPDATE tasks
				SET position = position + 1
				WHERE project_id = $1 AND id <> $2 AND position >= $3 AND position < $4
			`, t.ProjectID, t.ID, targetPosition, t.Position); err != nil {
				return fmt.Errorf("repository task move shift same project up: %w", err)
			}
		} else if targetPosition > t.Position {
			if _, err := tx.ExecContext(ctx, `
				UPDATE tasks
				SET position = position - 1
				WHERE project_id = $1 AND id <> $2 AND position <= $3 AND position > $4
			`, t.ProjectID, t.ID, targetPosition, t.Position); err != nil {
				return fmt.Errorf("repository task move shift same project down: %w", err)
			}
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
			UPDATE tasks
			SET position = position - 1
			WHERE project_id = $1 AND position > $2
		`, t.ProjectID, t.Position); err != nil {
			return fmt.Errorf("repository task move shift source project: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE tasks
			SET position = position + 1
			WHERE project_id = $1 AND position >= $2
		`, targetProjectID, targetPosition); err != nil {
			return fmt.Errorf("repository task move shift target project: %w", err)
		}
	}

	err = tx.QueryRowContext(ctx, `
		UPDATE tasks
		SET project_id = $1, position = $2, updated_at = NOW()
		WHERE id = $3
		  AND EXISTS (
			SELECT 1
			FROM project_members pm
			WHERE pm.project_id = tasks.project_id
			  AND pm.user_id = $4
		  )
		RETURNING updated_at
	`, targetProjectID, targetPosition, t.ID, t.UserID).Scan(&t.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrTaskNotFound
		}
		return fmt.Errorf("repository task move update task: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repository task move commit: %w", err)
	}
	committed = true

	t.ProjectID = targetProjectID
	t.Position = targetPosition
	return nil
}

func (r *taskRepository) Delete(ctx context.Context, userID, id int64) error {
	query := `
		DELETE FROM tasks
		WHERE id = $1
		  AND EXISTS (
			SELECT 1
			FROM project_members pm
			WHERE pm.project_id = tasks.project_id
			  AND pm.user_id = $2
		  )
	`
	res, err := r.db.ExecContext(ctx, query, id, userID)
	if err != nil {
		return fmt.Errorf("repository task delete: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("repository task delete rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return domain.ErrTaskNotFound
	}

	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanTask(row rowScanner, t *domain.Task) error {
	var dueAt sql.NullTime
	var dueTimezone sql.NullString
	err := row.Scan(
		&t.ID, &t.UserID, &t.ProjectID, &t.Title, &t.Description, &t.Status, &t.Completed,
		&dueAt, &dueTimezone, &t.Position, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return err
	}
	applyTaskNulls(t, dueAt, dueTimezone)
	return nil
}

func scanTaskWithTotal(row rowScanner, t *domain.Task, total *int) error {
	var dueAt sql.NullTime
	var dueTimezone sql.NullString
	err := row.Scan(
		&t.ID, &t.UserID, &t.ProjectID, &t.Title, &t.Description, &t.Status, &t.Completed,
		&dueAt, &dueTimezone, &t.Position, &t.CreatedAt, &t.UpdatedAt, total,
	)
	if err != nil {
		return err
	}
	applyTaskNulls(t, dueAt, dueTimezone)
	return nil
}

func applyTaskNulls(t *domain.Task, dueAt sql.NullTime, dueTimezone sql.NullString) {
	if t.Status == "" {
		if t.Completed {
			t.Status = domain.TaskStatusDone
		} else {
			t.Status = domain.TaskStatusOpen
		}
	}
	if dueAt.Valid {
		v := dueAt.Time.UTC()
		t.DueAt = &v
	}
	if dueTimezone.Valid {
		v := dueTimezone.String
		t.DueTimezone = &v
	}
}

func taskListWhere(userID int64, f domain.TaskListFilter) ([]string, []any) {
	where := []string{`EXISTS (
		SELECT 1
		FROM project_members pm
		WHERE pm.project_id = tasks.project_id
		  AND pm.user_id = $1
	)`}
	args := []any{userID}
	if f.ProjectID != nil {
		args = append(args, *f.ProjectID)
		where = append(where, fmt.Sprintf("project_id = $%d", len(args)))
	}
	if f.Status != nil {
		args = append(args, *f.Status)
		where = append(where, fmt.Sprintf("status = $%d", len(args)))
	}
	if f.Search != "" {
		args = append(args, "%"+f.Search+"%")
		where = append(where, fmt.Sprintf("(title ILIKE $%d OR description ILIKE $%d)", len(args), len(args)))
	}
	return where, args
}

func taskOrderBy(sort domain.TaskSort) string {
	switch sort {
	case domain.TaskSortDueDate:
		return "due_at ASC NULLS LAST, created_at DESC"
	case domain.TaskSortPosition:
		return "project_id ASC, position ASC, created_at DESC"
	case domain.TaskSortStatus:
		return "status ASC, created_at DESC"
	default:
		return "created_at DESC"
	}
}

func taskTargetPosition(ctx context.Context, tx *sql.Tx, taskID, sourceProjectID, targetProjectID int64, position *int) (int, error) {
	query := `SELECT COUNT(*) FROM tasks WHERE project_id = $1`
	args := []any{targetProjectID}
	if sourceProjectID == targetProjectID {
		query += ` AND id <> $2`
		args = append(args, taskID)
	}

	var count int
	if err := tx.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("repository task move count target tasks: %w", err)
	}

	if position == nil {
		return count, nil
	}
	if *position > count {
		return count, nil
	}
	return *position, nil
}
