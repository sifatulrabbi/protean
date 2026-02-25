package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/protean/vfs-server/internal/fsops"
	"github.com/protean/vfs-server/internal/middleware"
)

type writeFileRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func WriteFile(workspaceBase string, locker *fsops.PathLocker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := middleware.GetUserID(r.Context())
		root := filepath.Join(workspaceBase, userID)

		var req writeFileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			fsops.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
			return
		}

		resolved, err := fsops.ResolveWithinRoot(root, req.Path)
		if err != nil {
			fsops.WriteError(w, http.StatusForbidden, "PATH_TRAVERSAL", err.Error())
			return
		}

		unlock := locker.LockExact(resolved)
		defer unlock()

		// Auto-create parent directories
		if err := os.MkdirAll(filepath.Dir(resolved), 0755); err != nil {
			fsops.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}

		data := []byte(req.Content)
		if err := os.WriteFile(resolved, data, 0644); err != nil {
			fsops.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}

		fsops.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"bytesWritten": len(data),
		})
	}
}
