package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/protean/vfs-server/internal/fsops"
	"github.com/protean/vfs-server/internal/middleware"
)

type mkdirRequest struct {
	Path string `json:"path"`
}

func MkDir(workspaceBase string, locker *fsops.PathLocker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := middleware.GetUserID(r.Context())
		root := filepath.Join(workspaceBase, userID)

		var req mkdirRequest
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

		if err := os.MkdirAll(resolved, 0o755); err != nil {
			fsops.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}

		fsops.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"created": true,
		})
	}
}
