package handler

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/mmryalloc/tody/internal/domain"
	"github.com/mmryalloc/tody/internal/service"
)

type registerRequest struct {
	Email    string `json:"email" validate:"required,email,max=255"`
	Password string `json:"password" validate:"required,min=8,max=72"`
}

type loginRequest struct {
	Email    string `json:"email" validate:"required,email,max=255"`
	Password string `json:"password" validate:"required,min=1,max=72"`
}

type updateMeRequest struct {
	Email string `json:"email" validate:"required,email,max=255"`
	Name  string `json:"name" validate:"required,notblank,max=255"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required,min=1,max=72"`
	NewPassword     string `json:"new_password" validate:"required,min=8,max=72"`
}

type registerResponse struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
}

type userResponse struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type AuthService interface {
	Register(ctx context.Context, email, password string) (domain.User, error)
	GetMe(ctx context.Context, userID int64) (domain.User, error)
	UpdateMe(ctx context.Context, userID int64, in service.UpdateUserInput) (domain.User, error)
	ChangePassword(ctx context.Context, userID int64, currentPassword, newPassword, currentRefreshToken string) error
	DeleteMe(ctx context.Context, userID int64) error
	Login(ctx context.Context, email, password string, sc service.SessionContext) (service.TokenPair, error)
	Refresh(ctx context.Context, refreshToken string, sc service.SessionContext) (service.TokenPair, error)
	Logout(ctx context.Context, refreshToken string) error
	LogoutAll(ctx context.Context, userID int64) error
}

type AuthHandler struct {
	svc             AuthService
	cookies         *cookieIssuer
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

func NewAuthHandler(
	svc AuthService,
	secureCookies bool,
	cookieDomain string,
	accessTokenTTL, refreshTokenTTL time.Duration,
) *AuthHandler {
	return &AuthHandler{
		svc:             svc,
		cookies:         newCookieIssuer(secureCookies, cookieDomain),
		accessTokenTTL:  accessTokenTTL,
		refreshTokenTTL: refreshTokenTTL,
	}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if !bind(w, r, &req) {
		return
	}

	u, err := h.svc.Register(r.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, service.ErrEmailTaken) {
			conflict(w, "email already registered")
			return
		}
		slog.Error("handler auth register", "error", err)
		internalError(w, "failed to register user")
		return
	}

	created(w, registerResponse{ID: u.ID, Email: u.Email})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !bind(w, r, &req) {
		return
	}

	pair, err := h.svc.Login(r.Context(), req.Email, req.Password, sessionContextFromRequest(r))
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			unauthorized(w, "invalid email or password")
			return
		}
		slog.Error("handler auth login", "error", err)
		internalError(w, "failed to login")
		return
	}

	h.cookies.setAccess(w, pair.AccessToken, h.accessTokenTTL)
	h.cookies.setRefresh(w, pair.RefreshToken, h.refreshTokenTTL)
	ok(w, struct{}{})
}

func (h *AuthHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	userID, hasUser := UserIDFromContext(r.Context())
	if !hasUser {
		unauthorized(w, "authentication required")
		return
	}

	u, err := h.svc.GetMe(r.Context(), userID)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			notFound(w, "user not found")
			return
		}
		slog.Error("handler auth get me", "error", err)
		internalError(w, "failed to get user")
		return
	}

	writeUser(w, u)
}

func (h *AuthHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	userID, hasUser := UserIDFromContext(r.Context())
	if !hasUser {
		unauthorized(w, "authentication required")
		return
	}

	var req updateMeRequest
	if !bind(w, r, &req) {
		return
	}

	u, err := h.svc.UpdateMe(r.Context(), userID, service.UpdateUserInput{
		Email: req.Email,
		Name:  req.Name,
	})
	if err != nil {
		if errors.Is(err, service.ErrEmailTaken) {
			conflict(w, "email already registered")
			return
		}
		if errors.Is(err, domain.ErrUserNotFound) {
			notFound(w, "user not found")
			return
		}
		slog.Error("handler auth update me", "error", err)
		internalError(w, "failed to update user")
		return
	}

	writeUser(w, u)
}

func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, hasUser := UserIDFromContext(r.Context())
	if !hasUser {
		unauthorized(w, "authentication required")
		return
	}

	refreshCookie, err := r.Cookie(cookieRefreshToken)
	if err != nil || refreshCookie.Value == "" {
		unauthorized(w, "refresh token missing")
		return
	}

	var req changePasswordRequest
	if !bind(w, r, &req) {
		return
	}

	err = h.svc.ChangePassword(r.Context(), userID, req.CurrentPassword, req.NewPassword, refreshCookie.Value)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			unauthorized(w, "invalid current password")
			return
		}
		if errors.Is(err, service.ErrInvalidSession) {
			h.cookies.clearAccess(w)
			h.cookies.clearRefresh(w)
			unauthorized(w, "invalid or expired session")
			return
		}
		if errors.Is(err, domain.ErrUserNotFound) {
			notFound(w, "user not found")
			return
		}
		slog.Error("handler auth change password", "error", err)
		internalError(w, "failed to change password")
		return
	}

	ok(w, struct{}{})
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	refreshCookie, err := r.Cookie(cookieRefreshToken)
	if err != nil || refreshCookie.Value == "" {
		unauthorized(w, "refresh token missing")
		return
	}

	pair, err := h.svc.Refresh(r.Context(), refreshCookie.Value, sessionContextFromRequest(r))
	if err != nil {
		if errors.Is(err, service.ErrInvalidSession) {
			h.cookies.clearAccess(w)
			h.cookies.clearRefresh(w)
			unauthorized(w, "invalid or expired session")
			return
		}
		slog.Error("handler auth refresh", "error", err)
		internalError(w, "failed to refresh session")
		return
	}

	h.cookies.setAccess(w, pair.AccessToken, h.accessTokenTTL)
	h.cookies.setRefresh(w, pair.RefreshToken, h.refreshTokenTTL)
	ok(w, struct{}{})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	defer func() {
		h.cookies.clearAccess(w)
		h.cookies.clearRefresh(w)
	}()

	refreshCookie, err := r.Cookie(cookieRefreshToken)
	if err == nil && refreshCookie.Value != "" {
		if err := h.svc.Logout(r.Context(), refreshCookie.Value); err != nil {
			slog.Error("handler auth logout", "error", err)
		}
	}

	ok(w, struct{}{})
}

func (h *AuthHandler) LogoutAll(w http.ResponseWriter, r *http.Request) {
	userID, hasUser := UserIDFromContext(r.Context())
	if !hasUser {
		unauthorized(w, "authentication required")
		return
	}

	if err := h.svc.LogoutAll(r.Context(), userID); err != nil {
		slog.Error("handler auth logout all", "error", err)
		internalError(w, "failed to logout")
		return
	}

	h.cookies.clearAccess(w)
	h.cookies.clearRefresh(w)
	ok(w, struct{}{})
}

func (h *AuthHandler) DeleteMe(w http.ResponseWriter, r *http.Request) {
	userID, hasUser := UserIDFromContext(r.Context())
	if !hasUser {
		unauthorized(w, "authentication required")
		return
	}

	if err := h.svc.DeleteMe(r.Context(), userID); err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			notFound(w, "user not found")
			return
		}
		slog.Error("handler auth delete me", "error", err)
		internalError(w, "failed to delete user")
		return
	}

	h.cookies.clearAccess(w)
	h.cookies.clearRefresh(w)
	ok(w, struct{}{})
}

func sessionContextFromRequest(r *http.Request) service.SessionContext {
	return service.SessionContext{
		UserAgent: r.UserAgent(),
		IPAddress: clientIP(r),
	}
}

func writeUser(w http.ResponseWriter, u domain.User) {
	ok(w, userResponse{
		ID:    u.ID,
		Email: u.Email,
		Name:  u.Name,
	})
}

func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		for i := 0; i < len(fwd); i++ {
			if fwd[i] == ',' {
				return fwd[:i]
			}
		}
		return fwd
	}
	if real := r.Header.Get("X-Real-IP"); real != "" {
		return real
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
