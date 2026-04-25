package handler

import "net/http"

type Router struct {
	task    *TaskHandler
	auth    *AuthHandler
	project *ProjectHandler
	parser  TokenParser
}

func NewRouter(taskHandler *TaskHandler, authHandler *AuthHandler, projectHandler *ProjectHandler, parser TokenParser) *Router {
	return &Router{
		task:    taskHandler,
		auth:    authHandler,
		project: projectHandler,
		parser:  parser,
	}
}

func (r *Router) Setup() http.Handler {
	mux := http.NewServeMux()

	const v1 = "/api/v1"

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	})

	requireAuth := requireAuth(r.parser)

	mux.HandleFunc("POST "+v1+"/auth/register", r.auth.Register)
	mux.HandleFunc("POST "+v1+"/auth/login", r.auth.Login)
	mux.HandleFunc("POST "+v1+"/auth/refresh", r.auth.Refresh)
	mux.HandleFunc("POST "+v1+"/auth/logout", r.auth.Logout)
	mux.Handle("POST "+v1+"/auth/logout-all", requireAuth(http.HandlerFunc(r.auth.LogoutAll)))
	mux.Handle("PUT "+v1+"/auth/password", requireAuth(http.HandlerFunc(r.auth.ChangePassword)))

	mux.Handle("GET "+v1+"/users/me", requireAuth(http.HandlerFunc(r.auth.GetMe)))
	mux.Handle("PUT "+v1+"/users/me", requireAuth(http.HandlerFunc(r.auth.UpdateMe)))
	mux.Handle("DELETE "+v1+"/users/me", requireAuth(http.HandlerFunc(r.auth.DeleteMe)))

	mux.Handle("POST "+v1+"/projects", requireAuth(http.HandlerFunc(r.project.CreateProject)))
	mux.Handle("GET "+v1+"/projects", requireAuth(http.HandlerFunc(r.project.ListProjects)))
	mux.Handle("POST "+v1+"/projects/{id}/tasks", requireAuth(http.HandlerFunc(r.task.CreateProjectTask)))
	mux.Handle("POST "+v1+"/projects/{id}/invites", requireAuth(http.HandlerFunc(r.project.InviteMember)))
	mux.Handle("GET "+v1+"/projects/{id}/members", requireAuth(http.HandlerFunc(r.project.ListMembers)))
	mux.Handle("PUT "+v1+"/projects/{id}/members/{user_id}", requireAuth(http.HandlerFunc(r.project.UpdateMemberRole)))
	mux.Handle("DELETE "+v1+"/projects/{id}/members/{user_id}", requireAuth(http.HandlerFunc(r.project.RemoveMember)))
	mux.Handle("GET "+v1+"/projects/{id}", requireAuth(http.HandlerFunc(r.project.GetProject)))
	mux.Handle("PUT "+v1+"/projects/{id}", requireAuth(http.HandlerFunc(r.project.UpdateProject)))
	mux.Handle("DELETE "+v1+"/projects/{id}", requireAuth(http.HandlerFunc(r.project.DeleteProject)))

	mux.Handle("POST "+v1+"/tasks", requireAuth(http.HandlerFunc(r.task.CreateTask)))
	mux.Handle("GET "+v1+"/tasks", requireAuth(http.HandlerFunc(r.task.ListTasks)))
	mux.Handle("GET "+v1+"/tasks/{id}", requireAuth(http.HandlerFunc(r.task.GetTask)))
	mux.Handle("PUT "+v1+"/tasks/{id}", requireAuth(http.HandlerFunc(r.task.UpdateTask)))
	mux.Handle("PATCH "+v1+"/tasks/{id}/status", requireAuth(http.HandlerFunc(r.task.UpdateTaskStatus)))
	mux.Handle("PATCH "+v1+"/tasks/{id}/move", requireAuth(http.HandlerFunc(r.task.MoveTask)))
	mux.Handle("DELETE "+v1+"/tasks/{id}", requireAuth(http.HandlerFunc(r.task.DeleteTask)))

	return recoverMiddleware(mux)
}
