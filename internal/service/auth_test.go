package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mmryalloc/tody/internal/auth"
	"github.com/mmryalloc/tody/internal/domain"
	"github.com/mmryalloc/tody/internal/password"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockUserRepository struct {
	CreateFunc             func(ctx context.Context, u *domain.User) error
	GetByEmailFunc         func(ctx context.Context, email string) (domain.User, error)
	GetByIDFunc            func(ctx context.Context, id int64) (domain.User, error)
	UpdateProfileFunc      func(ctx context.Context, u *domain.User) error
	UpdatePasswordHashFunc func(ctx context.Context, id int64, hash string) error
	SoftDeleteFunc         func(ctx context.Context, id int64) error
}

func (m *mockUserRepository) Create(ctx context.Context, u *domain.User) error {
	return m.CreateFunc(ctx, u)
}
func (m *mockUserRepository) GetByEmail(ctx context.Context, email string) (domain.User, error) {
	return m.GetByEmailFunc(ctx, email)
}
func (m *mockUserRepository) GetByID(ctx context.Context, id int64) (domain.User, error) {
	return m.GetByIDFunc(ctx, id)
}
func (m *mockUserRepository) UpdateProfile(ctx context.Context, u *domain.User) error {
	return m.UpdateProfileFunc(ctx, u)
}
func (m *mockUserRepository) UpdatePasswordHash(ctx context.Context, id int64, hash string) error {
	return m.UpdatePasswordHashFunc(ctx, id, hash)
}
func (m *mockUserRepository) SoftDelete(ctx context.Context, id int64) error {
	return m.SoftDeleteFunc(ctx, id)
}

type mockSessionRepository struct {
	SaveFunc                   func(ctx context.Context, userID int64, hash string, s domain.Session, ttl time.Duration) error
	ExistsFunc                 func(ctx context.Context, userID int64, hash string) (bool, error)
	LookupUserIDFunc           func(ctx context.Context, hash string) (int64, error)
	DeleteFunc                 func(ctx context.Context, userID int64, hash string) error
	DeleteAllForUserFunc       func(ctx context.Context, userID int64) error
	DeleteAllForUserExceptFunc func(ctx context.Context, userID int64, keepHash string) error
}

func (m *mockSessionRepository) Save(ctx context.Context, userID int64, hash string, s domain.Session, ttl time.Duration) error {
	return m.SaveFunc(ctx, userID, hash, s, ttl)
}
func (m *mockSessionRepository) Exists(ctx context.Context, userID int64, hash string) (bool, error) {
	return m.ExistsFunc(ctx, userID, hash)
}
func (m *mockSessionRepository) LookupUserID(ctx context.Context, hash string) (int64, error) {
	return m.LookupUserIDFunc(ctx, hash)
}
func (m *mockSessionRepository) Delete(ctx context.Context, userID int64, hash string) error {
	return m.DeleteFunc(ctx, userID, hash)
}
func (m *mockSessionRepository) DeleteAllForUser(ctx context.Context, userID int64) error {
	return m.DeleteAllForUserFunc(ctx, userID)
}
func (m *mockSessionRepository) DeleteAllForUserExcept(ctx context.Context, userID int64, keepHash string) error {
	return m.DeleteAllForUserExceptFunc(ctx, userID, keepHash)
}

type stubTokenIssuer struct {
	token string
	err   error
}

func (s *stubTokenIssuer) Generate(userID int64) (string, time.Time, error) {
	if s.err != nil {
		return "", time.Time{}, s.err
	}
	return s.token, time.Now().Add(15 * time.Minute), nil
}

const testRefreshTTL = 30 * 24 * time.Hour

func TestRegister(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		pass    string
		mock    func(t *testing.T) UserRepository
		wantErr error
	}{
		{
			name:  "success normalises email",
			email: "  Foo@Example.COM  ",
			pass:  "supersecret",
			mock: func(t *testing.T) UserRepository {
				return &mockUserRepository{
					CreateFunc: func(ctx context.Context, u *domain.User) error {
						assert.Equal(t, "foo@example.com", u.Email)
						assert.NotEqual(t, "supersecret", u.PasswordHash, "password must be hashed")
						require.NoError(t, password.Verify(u.PasswordHash, "supersecret"))
						u.ID = 42
						return nil
					},
				}
			},
		},
		{
			name:  "email already taken",
			email: "dup@example.com",
			pass:  "supersecret",
			mock: func(t *testing.T) UserRepository {
				return &mockUserRepository{
					CreateFunc: func(ctx context.Context, u *domain.User) error {
						return domain.ErrUserExists
					},
				}
			},
			wantErr: ErrEmailTaken,
		},
		{
			name:  "repository error",
			email: "boom@example.com",
			pass:  "supersecret",
			mock: func(t *testing.T) UserRepository {
				return &mockUserRepository{
					CreateFunc: func(ctx context.Context, u *domain.User) error {
						return errors.New("db down")
					},
				}
			},
			wantErr: errors.New("db down"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewAuthService(tt.mock(t), &mockSessionRepository{}, &stubTokenIssuer{token: "tok"}, testRefreshTTL)
			u, err := s.Register(context.Background(), tt.email, tt.pass)
			if tt.wantErr != nil {
				require.Error(t, err)
				if errors.Is(tt.wantErr, ErrEmailTaken) {
					assert.ErrorIs(t, err, ErrEmailTaken)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, int64(42), u.ID)
			assert.Equal(t, "foo@example.com", u.Email)
		})
	}
}

func TestLogin(t *testing.T) {
	const pwd = "supersecret"
	hash, err := password.Hash(pwd)
	require.NoError(t, err)

	stored := domain.User{ID: 7, Email: "user@example.com", PasswordHash: hash}

	tests := []struct {
		name     string
		email    string
		password string
		users    func(t *testing.T) UserRepository
		sessions func(t *testing.T) SessionRepository
		wantErr  error
	}{
		{
			name:     "success issues tokens and persists session",
			email:    "USER@example.com",
			password: pwd,
			users: func(t *testing.T) UserRepository {
				return &mockUserRepository{
					GetByEmailFunc: func(ctx context.Context, email string) (domain.User, error) {
						assert.Equal(t, "user@example.com", email)
						return stored, nil
					},
				}
			},
			sessions: func(t *testing.T) SessionRepository {
				return &mockSessionRepository{
					SaveFunc: func(ctx context.Context, userID int64, hash string, s domain.Session, ttl time.Duration) error {
						assert.Equal(t, int64(7), userID)
						assert.Len(t, hash, 64, "sha256 hex")
						assert.Equal(t, testRefreshTTL, ttl)
						assert.Equal(t, "ua", s.UserAgent)
						assert.Equal(t, "ip", s.IPAddress)
						assert.False(t, s.CreatedAt.IsZero())
						return nil
					},
				}
			},
		},
		{
			name:     "user not found",
			email:    "missing@example.com",
			password: pwd,
			users: func(t *testing.T) UserRepository {
				return &mockUserRepository{
					GetByEmailFunc: func(ctx context.Context, email string) (domain.User, error) {
						return domain.User{}, domain.ErrUserNotFound
					},
				}
			},
			sessions: func(t *testing.T) SessionRepository { return &mockSessionRepository{} },
			wantErr:  ErrInvalidCredentials,
		},
		{
			name:     "wrong password",
			email:    "user@example.com",
			password: "wrong-password",
			users: func(t *testing.T) UserRepository {
				return &mockUserRepository{
					GetByEmailFunc: func(ctx context.Context, email string) (domain.User, error) {
						return stored, nil
					},
				}
			},
			sessions: func(t *testing.T) SessionRepository { return &mockSessionRepository{} },
			wantErr:  ErrInvalidCredentials,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewAuthService(tt.users(t), tt.sessions(t), &stubTokenIssuer{token: "access-jwt"}, testRefreshTTL)
			pair, err := s.Login(context.Background(), tt.email, tt.password, SessionContext{UserAgent: "ua", IPAddress: "ip"})
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, int64(7), pair.UserID)
			assert.Equal(t, "access-jwt", pair.AccessToken)
			assert.NotEmpty(t, pair.RefreshToken)
		})
	}
}

func TestGetMe(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		users := &mockUserRepository{
			GetByIDFunc: func(ctx context.Context, id int64) (domain.User, error) {
				assert.Equal(t, int64(7), id)
				return domain.User{ID: 7, Email: "user@example.com", Name: "User"}, nil
			},
		}
		s := NewAuthService(users, &mockSessionRepository{}, &stubTokenIssuer{token: "x"}, testRefreshTTL)

		u, err := s.GetMe(context.Background(), 7)
		require.NoError(t, err)
		assert.Equal(t, "User", u.Name)
	})

	t.Run("not found", func(t *testing.T) {
		users := &mockUserRepository{
			GetByIDFunc: func(ctx context.Context, id int64) (domain.User, error) {
				return domain.User{}, domain.ErrUserNotFound
			},
		}
		s := NewAuthService(users, &mockSessionRepository{}, &stubTokenIssuer{token: "x"}, testRefreshTTL)

		_, err := s.GetMe(context.Background(), 7)
		require.ErrorIs(t, err, domain.ErrUserNotFound)
	})
}

func TestUpdateMe(t *testing.T) {
	t.Run("success normalises email and trims name", func(t *testing.T) {
		users := &mockUserRepository{
			GetByIDFunc: func(ctx context.Context, id int64) (domain.User, error) {
				return domain.User{ID: id, Email: "old@example.com", Name: "Old"}, nil
			},
			UpdateProfileFunc: func(ctx context.Context, u *domain.User) error {
				assert.Equal(t, int64(7), u.ID)
				assert.Equal(t, "new@example.com", u.Email)
				assert.Equal(t, "New Name", u.Name)
				return nil
			},
		}
		s := NewAuthService(users, &mockSessionRepository{}, &stubTokenIssuer{token: "x"}, testRefreshTTL)

		u, err := s.UpdateMe(context.Background(), 7, UpdateUserInput{
			Email: "  New@Example.COM  ",
			Name:  "  New Name  ",
		})
		require.NoError(t, err)
		assert.Equal(t, "new@example.com", u.Email)
		assert.Equal(t, "New Name", u.Name)
	})

	t.Run("duplicate email", func(t *testing.T) {
		users := &mockUserRepository{
			GetByIDFunc: func(ctx context.Context, id int64) (domain.User, error) {
				return domain.User{ID: id}, nil
			},
			UpdateProfileFunc: func(ctx context.Context, u *domain.User) error {
				return domain.ErrUserExists
			},
		}
		s := NewAuthService(users, &mockSessionRepository{}, &stubTokenIssuer{token: "x"}, testRefreshTTL)

		_, err := s.UpdateMe(context.Background(), 7, UpdateUserInput{Email: "taken@example.com", Name: "User"})
		require.ErrorIs(t, err, ErrEmailTaken)
	})
}

func TestChangePassword(t *testing.T) {
	const oldPassword = "old-password"
	hash, err := password.Hash(oldPassword)
	require.NoError(t, err)

	t.Run("success updates password and revokes other sessions", func(t *testing.T) {
		updated := false
		revoked := false
		users := &mockUserRepository{
			GetByIDFunc: func(ctx context.Context, id int64) (domain.User, error) {
				return domain.User{ID: id, PasswordHash: hash}, nil
			},
			UpdatePasswordHashFunc: func(ctx context.Context, id int64, newHash string) error {
				updated = true
				assert.Equal(t, int64(7), id)
				assert.NoError(t, password.Verify(newHash, "new-password"))
				return nil
			},
		}
		sessions := &mockSessionRepository{
			ExistsFunc: func(ctx context.Context, userID int64, hash string) (bool, error) {
				assert.Equal(t, int64(7), userID)
				assert.Equal(t, auth.HashRefreshToken("current-refresh"), hash)
				return true, nil
			},
			DeleteAllForUserExceptFunc: func(ctx context.Context, userID int64, keepHash string) error {
				revoked = true
				assert.Equal(t, int64(7), userID)
				assert.Equal(t, auth.HashRefreshToken("current-refresh"), keepHash)
				return nil
			},
		}
		s := NewAuthService(users, sessions, &stubTokenIssuer{token: "x"}, testRefreshTTL)

		err := s.ChangePassword(context.Background(), 7, oldPassword, "new-password", "current-refresh")
		require.NoError(t, err)
		assert.True(t, updated)
		assert.True(t, revoked)
	})

	t.Run("invalid current session", func(t *testing.T) {
		users := &mockUserRepository{
			GetByIDFunc: func(ctx context.Context, id int64) (domain.User, error) {
				return domain.User{ID: id, PasswordHash: hash}, nil
			},
			UpdatePasswordHashFunc: func(ctx context.Context, id int64, newHash string) error {
				t.Fatal("password must not be updated")
				return nil
			},
		}
		sessions := &mockSessionRepository{
			ExistsFunc: func(ctx context.Context, userID int64, hash string) (bool, error) {
				return false, nil
			},
		}
		s := NewAuthService(users, sessions, &stubTokenIssuer{token: "x"}, testRefreshTTL)

		err := s.ChangePassword(context.Background(), 7, oldPassword, "new-password", "stale-refresh")
		require.ErrorIs(t, err, ErrInvalidSession)
	})

	t.Run("wrong current password", func(t *testing.T) {
		users := &mockUserRepository{
			GetByIDFunc: func(ctx context.Context, id int64) (domain.User, error) {
				return domain.User{ID: id, PasswordHash: hash}, nil
			},
			UpdatePasswordHashFunc: func(ctx context.Context, id int64, newHash string) error {
				t.Fatal("password must not be updated")
				return nil
			},
		}
		s := NewAuthService(users, &mockSessionRepository{}, &stubTokenIssuer{token: "x"}, testRefreshTTL)

		err := s.ChangePassword(context.Background(), 7, "wrong", "new-password", "current-refresh")
		require.ErrorIs(t, err, ErrInvalidCredentials)
	})
}

func TestDeleteMe(t *testing.T) {
	t.Run("soft deletes user and revokes sessions", func(t *testing.T) {
		deleted := false
		revoked := false
		users := &mockUserRepository{
			SoftDeleteFunc: func(ctx context.Context, id int64) error {
				deleted = true
				assert.Equal(t, int64(7), id)
				return nil
			},
		}
		sessions := &mockSessionRepository{
			DeleteAllForUserFunc: func(ctx context.Context, userID int64) error {
				revoked = true
				assert.Equal(t, int64(7), userID)
				return nil
			},
		}
		s := NewAuthService(users, sessions, &stubTokenIssuer{token: "x"}, testRefreshTTL)

		require.NoError(t, s.DeleteMe(context.Background(), 7))
		assert.True(t, deleted)
		assert.True(t, revoked)
	})
}

func TestRefresh(t *testing.T) {
	t.Run("success rotates token", func(t *testing.T) {
		var savedHash string
		var deletedHash string

		sessions := &mockSessionRepository{
			LookupUserIDFunc: func(ctx context.Context, hash string) (int64, error) {
				return 9, nil
			},
			SaveFunc: func(ctx context.Context, userID int64, hash string, s domain.Session, ttl time.Duration) error {
				savedHash = hash
				return nil
			},
			DeleteFunc: func(ctx context.Context, userID int64, hash string) error {
				assert.Equal(t, int64(9), userID)
				deletedHash = hash
				return nil
			},
		}

		s := NewAuthService(&mockUserRepository{}, sessions, &stubTokenIssuer{token: "new-jwt"}, testRefreshTTL)

		pair, err := s.Refresh(context.Background(), "old-refresh", SessionContext{})
		require.NoError(t, err)
		assert.Equal(t, "new-jwt", pair.AccessToken)
		assert.NotEmpty(t, pair.RefreshToken)
		assert.NotEqual(t, "old-refresh", pair.RefreshToken)
		assert.Equal(t, auth.HashRefreshToken("old-refresh"), deletedHash, "old session should be revoked")
		assert.NotEqual(t, deletedHash, savedHash, "new and old session hashes must differ")
	})

	t.Run("unknown token returns ErrInvalidSession", func(t *testing.T) {
		sessions := &mockSessionRepository{
			LookupUserIDFunc: func(ctx context.Context, hash string) (int64, error) {
				return 0, domain.ErrSessionNotFound
			},
		}
		s := NewAuthService(&mockUserRepository{}, sessions, &stubTokenIssuer{token: "x"}, testRefreshTTL)

		_, err := s.Refresh(context.Background(), "bogus", SessionContext{})
		require.ErrorIs(t, err, ErrInvalidSession)
	})

	t.Run("save failure leaves old session intact", func(t *testing.T) {
		deleted := false
		sessions := &mockSessionRepository{
			LookupUserIDFunc: func(ctx context.Context, hash string) (int64, error) { return 1, nil },
			SaveFunc: func(ctx context.Context, userID int64, hash string, s domain.Session, ttl time.Duration) error {
				return errors.New("redis down")
			},
			DeleteFunc: func(ctx context.Context, userID int64, hash string) error {
				deleted = true
				return nil
			},
		}
		s := NewAuthService(&mockUserRepository{}, sessions, &stubTokenIssuer{token: "x"}, testRefreshTTL)

		_, err := s.Refresh(context.Background(), "any", SessionContext{})
		require.Error(t, err)
		assert.False(t, deleted, "old session must NOT be deleted when new save fails")
	})
}

func TestLogout(t *testing.T) {
	t.Run("deletes resolved session", func(t *testing.T) {
		called := false
		sessions := &mockSessionRepository{
			LookupUserIDFunc: func(ctx context.Context, hash string) (int64, error) { return 5, nil },
			DeleteFunc: func(ctx context.Context, userID int64, hash string) error {
				called = true
				assert.Equal(t, int64(5), userID)
				assert.Equal(t, auth.HashRefreshToken("rtok"), hash)
				return nil
			},
		}
		s := NewAuthService(&mockUserRepository{}, sessions, &stubTokenIssuer{token: "x"}, testRefreshTTL)

		require.NoError(t, s.Logout(context.Background(), "rtok"))
		assert.True(t, called)
	})

	t.Run("missing session is not an error", func(t *testing.T) {
		sessions := &mockSessionRepository{
			LookupUserIDFunc: func(ctx context.Context, hash string) (int64, error) {
				return 0, domain.ErrSessionNotFound
			},
		}
		s := NewAuthService(&mockUserRepository{}, sessions, &stubTokenIssuer{token: "x"}, testRefreshTTL)
		require.NoError(t, s.Logout(context.Background(), "stale"))
	})
}

func TestLogoutAll(t *testing.T) {
	called := false
	sessions := &mockSessionRepository{
		DeleteAllForUserFunc: func(ctx context.Context, userID int64) error {
			called = true
			assert.Equal(t, int64(11), userID)
			return nil
		},
	}
	s := NewAuthService(&mockUserRepository{}, sessions, &stubTokenIssuer{token: "x"}, testRefreshTTL)
	require.NoError(t, s.LogoutAll(context.Background(), 11))
	assert.True(t, called)
}

func TestJWTManagerRoundTrip(t *testing.T) {
	m := auth.NewJWTManager("test-secret", time.Minute, "tody")

	tok, exp, err := m.Generate(123)
	require.NoError(t, err)
	require.NotEmpty(t, tok)
	assert.WithinDuration(t, time.Now().Add(time.Minute), exp, 2*time.Second)

	got, err := m.Parse(tok)
	require.NoError(t, err)
	assert.Equal(t, int64(123), got)
}

func TestJWTManagerRejectsTamperedToken(t *testing.T) {
	m := auth.NewJWTManager("test-secret", time.Minute, "tody")
	tok, _, err := m.Generate(1)
	require.NoError(t, err)

	tampered := tok[:len(tok)-1] + "A"
	if tampered == tok {
		tampered = tok[:len(tok)-1] + "B"
	}

	_, err = m.Parse(tampered)
	require.ErrorIs(t, err, auth.ErrInvalidToken)
}

func TestJWTManagerRejectsExpired(t *testing.T) {
	m := auth.NewJWTManager("test-secret", -time.Minute, "tody")
	tok, _, err := m.Generate(1)
	require.NoError(t, err)

	_, err = m.Parse(tok)
	require.ErrorIs(t, err, auth.ErrInvalidToken)
}

func TestJWTManagerRejectsWrongIssuer(t *testing.T) {
	signer := auth.NewJWTManager("test-secret", time.Minute, "evil")
	tok, _, err := signer.Generate(1)
	require.NoError(t, err)

	verifier := auth.NewJWTManager("test-secret", time.Minute, "tody")
	_, err = verifier.Parse(tok)
	require.ErrorIs(t, err, auth.ErrInvalidToken)
}
