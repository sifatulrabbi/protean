package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/protean/vfs-server/internal/fsops"
	"github.com/protean/vfs-server/internal/middleware"
)

type renameRequest struct {
	Path    string `json:"path"`
	NewName string `json:"newName"`
	NewPath string `json:"newPath"`
}

func Rename(workspaceBase string, locker *fsops.PathLocker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := middleware.GetUserID(r.Context())
		root := filepath.Join(workspaceBase, userID)

		var req renameRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			fsops.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
			return
		}

		resolved, err := fsops.ResolveWithinRoot(root, req.Path)
		if err != nil {
			fsops.WriteError(w, http.StatusForbidden, "PATH_TRAVERSAL", err.Error())
			return
		}

		destinationResolved := ""
		newPathInput := strings.TrimSpace(req.NewPath)
		if newPathInput != "" {
			destinationResolved, err = fsops.ResolveWithinRoot(root, newPathInput)
			if err != nil {
				fsops.WriteError(w, http.StatusForbidden, "PATH_TRAVERSAL", err.Error())
				return
			}
		} else {
			newName := strings.TrimSpace(req.NewName)
			if newName == "" || strings.Contains(newName, "/") || strings.Contains(newName, "\\") {
				fsops.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid new name")
				return
			}

			newPath := filepath.Join(filepath.Dir(resolved), newName)
			relNewPath, err := filepath.Rel(root, newPath)
			if err != nil {
				fsops.WriteError(w, http.StatusForbidden, "PATH_TRAVERSAL", err.Error())
				return
			}

			destinationResolved, err = fsops.ResolveWithinRoot(root, relNewPath)
			if err != nil {
				fsops.WriteError(w, http.StatusForbidden, "PATH_TRAVERSAL", err.Error())
				return
			}
		}

		unlock := locker.LockSubtree(resolved, destinationResolved)
		defer unlock()

		if err := os.MkdirAll(filepath.Dir(destinationResolved), 0o755); err != nil {
			fsops.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}

		if err := os.Rename(resolved, destinationResolved); err != nil {
			if os.IsNotExist(err) {
				fsops.WriteError(w, http.StatusNotFound, "NOT_FOUND", "file or directory not found")
				return
			}
			fsops.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}

		fsops.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"renamed": true,
		})
	}
}
