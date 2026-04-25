package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mmryalloc/tody/internal/auth"
	"github.com/mmryalloc/tody/internal/domain"
	"github.com/mmryalloc/tody/internal/password"
)

var (
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidSession     = errors.New("invalid or expired session")
)

type UserRepository interface {
	Create(ctx context.Context, u *domain.User) error
	GetByEmail(ctx context.Context, email string) (domain.User, error)
	GetByID(ctx context.Context, id int64) (domain.User, error)
	UpdateProfile(ctx context.Context, u *domain.User) error
	UpdatePasswordHash(ctx context.Context, id int64, hash string) error
	SoftDelete(ctx context.Context, id int64) error
}

type SessionRepository interface {
	Save(ctx context.Context, userID int64, tokenHash string, s domain.Session, ttl time.Duration) error
	Exists(ctx context.Context, userID int64, tokenHash string) (bool, error)
	LookupUserID(ctx context.Context, tokenHash string) (int64, error)
	Delete(ctx context.Context, userID int64, tokenHash string) error
	DeleteAllForUser(ctx context.Context, userID int64) error
	DeleteAllForUserExcept(ctx context.Context, userID int64, keepTokenHash string) error
}

type TokenIssuer interface {
	Generate(userID int64) (string, time.Time, error)
}

type SessionContext struct {
	UserAgent string
	IPAddress string
}

type TokenPair struct {
	UserID       int64
	AccessToken  string
	RefreshToken string
}

type UpdateUserInput struct {
	Email string
	Name  string
}

type authService struct {
	users           UserRepository
	sessions        SessionRepository
	tokens          TokenIssuer
	refreshTokenTTL time.Duration
}

func NewAuthService(users UserRepository, sessions SessionRepository, tokens TokenIssuer, refreshTokenTTL time.Duration) *authService {
	return &authService{
		users:           users,
		sessions:        sessions,
		tokens:          tokens,
		refreshTokenTTL: refreshTokenTTL,
	}
}

func (s *authService) Register(ctx context.Context, email, pwd string) (domain.User, error) {
	hash, err := password.Hash(pwd)
	if err != nil {
		return domain.User{}, fmt.Errorf("service auth register: %w", err)
	}

	u := domain.User{
		Email:        strings.ToLower(strings.TrimSpace(email)),
		PasswordHash: hash,
	}

	if err := s.users.Create(ctx, &u); err != nil {
		if errors.Is(err, domain.ErrUserExists) {
			return domain.User{}, ErrEmailTaken
		}
		return domain.User{}, fmt.Errorf("service auth register: %w", err)
	}

	return u, nil
}

func (s *authService) GetMe(ctx context.Context, userID int64) (domain.User, error) {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return domain.User{}, domain.ErrUserNotFound
		}
		return domain.User{}, fmt.Errorf("service auth get me: %w", err)
	}
	return u, nil
}

func (s *authService) UpdateMe(ctx context.Context, userID int64, in UpdateUserInput) (domain.User, error) {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return domain.User{}, domain.ErrUserNotFound
		}
		return domain.User{}, fmt.Errorf("service auth update me lookup: %w", err)
	}

	u.Email = strings.ToLower(strings.TrimSpace(in.Email))
	u.Name = strings.TrimSpace(in.Name)

	if err := s.users.UpdateProfile(ctx, &u); err != nil {
		if errors.Is(err, domain.ErrUserExists) {
			return domain.User{}, ErrEmailTaken
		}
		if errors.Is(err, domain.ErrUserNotFound) {
			return domain.User{}, domain.ErrUserNotFound
		}
		return domain.User{}, fmt.Errorf("service auth update me: %w", err)
	}
	return u, nil
}

func (s *authService) ChangePassword(ctx context.Context, userID int64, currentPassword, newPassword, currentRefreshToken string) error {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return domain.ErrUserNotFound
		}
		return fmt.Errorf("service auth change password lookup: %w", err)
	}

	if err := password.Verify(u.PasswordHash, currentPassword); err != nil {
		if errors.Is(err, password.ErrInvalid) {
			return ErrInvalidCredentials
		}
		return fmt.Errorf("service auth change password verify: %w", err)
	}

	keepHash := auth.HashRefreshToken(currentRefreshToken)
	exists, err := s.sessions.Exists(ctx, userID, keepHash)
	if err != nil {
		return fmt.Errorf("service auth change password session lookup: %w", err)
	}
	if !exists {
		return ErrInvalidSession
	}

	hash, err := password.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("service auth change password hash: %w", err)
	}

	if err := s.users.UpdatePasswordHash(ctx, userID, hash); err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return domain.ErrUserNotFound
		}
		return fmt.Errorf("service auth change password update: %w", err)
	}

	if err := s.sessions.DeleteAllForUserExcept(ctx, userID, keepHash); err != nil {
		return fmt.Errorf("service auth change password revoke sessions: %w", err)
	}
	return nil
}

func (s *authService) DeleteMe(ctx context.Context, userID int64) error {
	if err := s.users.SoftDelete(ctx, userID); err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return domain.ErrUserNotFound
		}
		return fmt.Errorf("service auth delete me: %w", err)
	}
	if err := s.sessions.DeleteAllForUser(ctx, userID); err != nil {
		return fmt.Errorf("service auth delete me revoke sessions: %w", err)
	}
	return nil
}

func (s *authService) Login(ctx context.Context, email, pwd string, sc SessionContext) (TokenPair, error) {
	u, err := s.users.GetByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return TokenPair{}, ErrInvalidCredentials
		}
		return TokenPair{}, fmt.Errorf("service auth login lookup: %w", err)
	}

	if err := password.Verify(u.PasswordHash, pwd); err != nil {
		if errors.Is(err, password.ErrInvalid) {
			return TokenPair{}, ErrInvalidCredentials
		}
		return TokenPair{}, fmt.Errorf("service auth login verify: %w", err)
	}

	return s.issueTokenPair(ctx, u.ID, sc)
}

func (s *authService) Refresh(ctx context.Context, refreshToken string, sc SessionContext) (TokenPair, error) {
	oldHash := auth.HashRefreshToken(refreshToken)

	userID, err := s.sessions.LookupUserID(ctx, oldHash)
	if err != nil {
		if errors.Is(err, domain.ErrSessionNotFound) {
			return TokenPair{}, ErrInvalidSession
		}
		return TokenPair{}, fmt.Errorf("service auth refresh lookup: %w", err)
	}

	pair, err := s.issueTokenPair(ctx, userID, sc)
	if err != nil {
		return TokenPair{}, err
	}

	if err := s.sessions.Delete(ctx, userID, oldHash); err != nil {
		return TokenPair{}, fmt.Errorf("service auth refresh revoke old: %w", err)
	}

	return pair, nil
}

func (s *authService) Logout(ctx context.Context, refreshToken string) error {
	hash := auth.HashRefreshToken(refreshToken)

	userID, err := s.sessions.LookupUserID(ctx, hash)
	if err != nil {
		if errors.Is(err, domain.ErrSessionNotFound) {
			return nil
		}
		return fmt.Errorf("service auth logout lookup: %w", err)
	}

	if err := s.sessions.Delete(ctx, userID, hash); err != nil {
		return fmt.Errorf("service auth logout: %w", err)
	}
	return nil
}

func (s *authService) LogoutAll(ctx context.Context, userID int64) error {
	if err := s.sessions.DeleteAllForUser(ctx, userID); err != nil {
		return fmt.Errorf("service auth logout all: %w", err)
	}
	return nil
}

func (s *authService) issueTokenPair(ctx context.Context, userID int64, sc SessionContext) (TokenPair, error) {
	accessToken, _, err := s.tokens.Generate(userID)
	if err != nil {
		return TokenPair{}, fmt.Errorf("service auth issue access: %w", err)
	}

	refreshToken, err := auth.GenerateRefreshToken()
	if err != nil {
		return TokenPair{}, fmt.Errorf("service auth issue refresh: %w", err)
	}

	session := domain.Session{
		UserAgent: sc.UserAgent,
		IPAddress: sc.IPAddress,
		CreatedAt: time.Now().UTC(),
	}

	if err := s.sessions.Save(ctx, userID, auth.HashRefreshToken(refreshToken), session, s.refreshTokenTTL); err != nil {
		return TokenPair{}, fmt.Errorf("service auth issue persist: %w", err)
	}

	return TokenPair{
		UserID:       userID,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}
