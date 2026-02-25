package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/protean/vfs-server/internal/fsops"
	"github.com/protean/vfs-server/internal/middleware"
)

// NewRouter creates the chi router with all VFS routes.
func NewRouter(workspaceBase string, tokens map[string]string) chi.Router {
	r := chi.NewRouter()
	locker := fsops.NewPathLocker()

	// Health check — outside auth group
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Authenticated API routes
	r.Group(func(r chi.Router) {
		r.Use(middleware.ServiceAuth(tokens))
		r.Use(middleware.UserContext(workspaceBase))

		r.Get("/api/v1/files/stat", Stat(workspaceBase, locker))
		r.Get("/api/v1/files/readdir", ListDir(workspaceBase, locker))
		r.Get("/api/v1/files/read", ReadFile(workspaceBase, locker))
		r.Get("/api/v1/files/read-binary", ReadFileBinary(workspaceBase, locker))

		r.Post("/api/v1/files/write", WriteFile(workspaceBase, locker))
		r.Post("/api/v1/files/write-binary", WriteFileBinary(workspaceBase, locker))
		r.Post("/api/v1/files/mkdir", MkDir(workspaceBase, locker))
		r.Delete("/api/v1/files/remove", Remove(workspaceBase, locker))
		r.Patch("/api/v1/files/rename", Rename(workspaceBase, locker))
	})

	return r
}
