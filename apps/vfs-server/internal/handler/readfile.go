package handler

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/protean/vfs-server/internal/fsops"
	"github.com/protean/vfs-server/internal/middleware"
)

func ReadFile(workspaceBase string, _ *fsops.PathLocker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := middleware.GetUserID(r.Context())
		root := filepath.Join(workspaceBase, userID)

		filePath := r.URL.Query().Get("path")
		resolved, err := fsops.ResolveWithinRoot(root, filePath)
		if err != nil {
			fsops.WriteError(w, http.StatusForbidden, "PATH_TRAVERSAL", err.Error())
			return
		}

		data, err := os.ReadFile(resolved)
		if err != nil {
			if os.IsNotExist(err) {
				fsops.WriteError(w, http.StatusNotFound, "NOT_FOUND", "file not found")
				return
			}
			fsops.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}

		fsops.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"content": string(data),
		})
	}
}
