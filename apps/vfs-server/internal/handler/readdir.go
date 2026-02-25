package handler

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/protean/vfs-server/internal/fsops"
	"github.com/protean/vfs-server/internal/middleware"
)

func ListDir(workspaceBase string, _ *fsops.PathLocker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := middleware.GetUserID(r.Context())
		root := filepath.Join(workspaceBase, userID)

		dirPath := r.URL.Query().Get("path")
		resolved, err := fsops.ResolveWithinRoot(root, dirPath)
		if err != nil {
			fsops.WriteError(w, http.StatusForbidden, "PATH_TRAVERSAL", err.Error())
			return
		}

		entries, err := os.ReadDir(resolved)
		if err != nil {
			if os.IsNotExist(err) {
				fsops.WriteError(w, http.StatusNotFound, "NOT_FOUND", "directory not found")
				return
			}
			fsops.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}

		result := make([]map[string]interface{}, 0, len(entries))
		for _, e := range entries {
			result = append(result, map[string]interface{}{
				"name":        e.Name(),
				"isDirectory": e.IsDir(),
			})
		}

		fsops.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"entries": result,
		})
	}
}
