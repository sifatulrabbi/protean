package handler

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/protean/vfs-server/internal/fsops"
	"github.com/protean/vfs-server/internal/middleware"
)

func Stat(workspaceBase string, _ *fsops.PathLocker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := middleware.GetUserID(r.Context())
		root := filepath.Join(workspaceBase, userID)

		filePath := r.URL.Query().Get("path")
		resolved, err := fsops.ResolveWithinRoot(root, filePath)
		if err != nil {
			fsops.WriteError(w, http.StatusForbidden, "PATH_TRAVERSAL", err.Error())
			return
		}

		info, err := os.Stat(resolved)
		if err != nil {
			if os.IsNotExist(err) {
				fsops.WriteError(w, http.StatusNotFound, "NOT_FOUND", "file or directory not found")
				return
			}
			fsops.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}

		fsops.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"size":        info.Size(),
			"isDirectory": info.IsDir(),
			"modified":    info.ModTime().UTC().Format("2006-01-02T15:04:05.000Z"),
			"created":     info.ModTime().UTC().Format("2006-01-02T15:04:05.000Z"),
		})
	}
}
