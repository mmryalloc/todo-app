package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/mmryalloc/tody/internal/auth"
	"github.com/mmryalloc/tody/internal/config"
	"github.com/mmryalloc/tody/internal/handler"
	"github.com/mmryalloc/tody/internal/repository"
	"github.com/mmryalloc/tody/internal/service"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := config.MustLoad()

	db, err := sql.Open("postgres", cfg.Database.DSN())
	if err != nil {
		slog.Error("invalid dsn", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	db.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	db.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.Database.ConnMaxLifetime)

	if err := db.Ping(); err != nil {
		slog.Error("failed to connect to db", "error", err)
		os.Exit(1)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr(),
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer rdb.Close()

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		slog.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}

	jwtManager := auth.NewJWTManager(cfg.JWT.AccessSecret, cfg.JWT.AccessTokenTTL, cfg.JWT.Issuer)

	taskRepo := repository.NewTaskRepository(db)
	projectRepo := repository.NewProjectRepository(db)
	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(rdb)

	taskSvc := service.NewTaskService(taskRepo, projectRepo)
	projectSvc := service.NewProjectService(projectRepo)
	authSvc := service.NewAuthService(userRepo, sessionRepo, jwtManager, cfg.JWT.RefreshTokenTTL)

	taskHandler := handler.NewTaskHandler(taskSvc)
	projectHandler := handler.NewProjectHandler(projectSvc)
	authHandler := handler.NewAuthHandler(
		authSvc,
		cfg.Cookie.Secure,
		cfg.Cookie.Domain,
		cfg.JWT.AccessTokenTTL,
		cfg.JWT.RefreshTokenTTL,
	)

	r := handler.NewRouter(taskHandler, authHandler, projectHandler, jwtManager)
	h := r.Setup()

	addr := ":" + cfg.App.Port
	srv := &http.Server{
		Addr:         addr,
		Handler:      h,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("listening on", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server error", "error", err)
	}

	slog.Info("server stopped")
}
