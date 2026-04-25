package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/mmryalloc/tody/internal/domain"
)

type userRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *userRepository {
	return &userRepository{db: db}
}

func (r *userRepository) Create(ctx context.Context, u *domain.User) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("repository user create begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	query := `
		INSERT INTO users (email, name, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at
	`
	err = tx.QueryRowContext(
		ctx,
		query,
		strings.ToLower(u.Email),
		u.Name,
		u.PasswordHash,
	).Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)

	if err != nil {
		if isUniqueViolation(err) {
			return domain.ErrUserExists
		}
		return fmt.Errorf("repository user create: %w", err)
	}

	var projectID int64
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO projects (user_id, name, color, is_default) VALUES ($1, $2, $3, TRUE) RETURNING id`,
		u.ID, defaultUserProjectName, defaultUserProjectColor,
	).Scan(&projectID); err != nil {
		return fmt.Errorf("repository user create default project: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO project_members (project_id, user_id, role) VALUES ($1, $2, 'owner')`,
		projectID, u.ID,
	); err != nil {
		return fmt.Errorf("repository user create default project member: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repository user create commit: %w", err)
	}
	committed = true

	return nil
}

func (r *userRepository) GetByEmail(ctx context.Context, email string) (domain.User, error) {
	query := `
		SELECT id, email, name, password_hash, created_at, updated_at, deleted_at
		FROM users
		WHERE email = $1 AND deleted_at IS NULL
	`
	var u domain.User
	var deletedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, query, strings.ToLower(email)).Scan(
		&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt, &deletedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.User{}, domain.ErrUserNotFound
		}
		return domain.User{}, fmt.Errorf("repository user get by email: %w", err)
	}
	if deletedAt.Valid {
		u.DeletedAt = &deletedAt.Time
	}
	return u, nil
}

func (r *userRepository) GetByID(ctx context.Context, id int64) (domain.User, error) {
	query := `
		SELECT id, email, name, password_hash, created_at, updated_at, deleted_at
		FROM users
		WHERE id = $1 AND deleted_at IS NULL
	`
	var u domain.User
	var deletedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt, &deletedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.User{}, domain.ErrUserNotFound
		}
		return domain.User{}, fmt.Errorf("repository user get by id: %w", err)
	}
	if deletedAt.Valid {
		u.DeletedAt = &deletedAt.Time
	}
	return u, nil
}

func (r *userRepository) UpdateProfile(ctx context.Context, u *domain.User) error {
	query := `
		UPDATE users
		SET email = $1, name = $2, updated_at = NOW()
		WHERE id = $3 AND deleted_at IS NULL
		RETURNING updated_at
	`
	err := r.db.QueryRowContext(ctx, query, strings.ToLower(u.Email), u.Name, u.ID).Scan(&u.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrUserNotFound
		}
		if isUniqueViolation(err) {
			return domain.ErrUserExists
		}
		return fmt.Errorf("repository user update profile: %w", err)
	}
	return nil
}

func (r *userRepository) UpdatePasswordHash(ctx context.Context, id int64, hash string) error {
	query := `
		UPDATE users
		SET password_hash = $1, updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
	`
	res, err := r.db.ExecContext(ctx, query, hash, id)
	if err != nil {
		return fmt.Errorf("repository user update password: %w", err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("repository user update password rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return domain.ErrUserNotFound
	}
	return nil
}

func (r *userRepository) SoftDelete(ctx context.Context, id int64) error {
	query := `
		UPDATE users
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`
	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("repository user soft delete: %w", err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("repository user soft delete rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return domain.ErrUserNotFound
	}
	return nil
}
