package handler

import (
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/protean/vfs-server/internal/fsops"
	"github.com/protean/vfs-server/internal/middleware"
)

func WriteFileBinary(workspaceBase string, locker *fsops.PathLocker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := middleware.GetUserID(r.Context())
		root := filepath.Join(workspaceBase, userID)

		// Parse multipart form: 32MB max
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			fsops.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid multipart form")
			return
		}

		filePath := r.FormValue("path")
		if filePath == "" {
			fsops.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "missing path field")
			return
		}

		file, _, err := r.FormFile("file")
		if err != nil {
			fsops.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "missing file field")
			return
		}
		defer file.Close()

		resolved, err := fsops.ResolveWithinRoot(root, filePath)
		if err != nil {
			fsops.WriteError(w, http.StatusForbidden, "PATH_TRAVERSAL", err.Error())
			return
		}

		data, err := io.ReadAll(file)
		if err != nil {
			fsops.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}

		unlock := locker.LockExact(resolved)
		defer unlock()

		if err := os.MkdirAll(filepath.Dir(resolved), 0755); err != nil {
			fsops.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}

		if err := os.WriteFile(resolved, data, 0644); err != nil {
			fsops.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}

		fsops.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"bytesWritten": len(data),
		})
	}
}
