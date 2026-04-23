package handler

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

type TokenParser interface {
	Parse(token string) (int64, error)
}

func requireAuth(parser TokenParser) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(cookieAccessToken)
			if err != nil || cookie.Value == "" {
				unauthorized(w, "authentication required")
				return
			}

			userID, err := parser.Parse(cookie.Value)
			if err != nil {
				unauthorized(w, "authentication required")
				return
			}

			ctx := WithUserID(r.Context(), userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if rec == http.ErrAbortHandler {
				panic(rec)
			}
			slog.Error("panic recovered",
				"error", rec,
				"method", r.Method,
				"path", r.URL.Path,
				"stack", string(debug.Stack()),
			)
			internalError(w, "internal server error")
		}()
		next.ServeHTTP(w, r)
	})
}
