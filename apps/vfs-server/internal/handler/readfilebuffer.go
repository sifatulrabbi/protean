package handler

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/protean/vfs-server/internal/fsops"
	"github.com/protean/vfs-server/internal/middleware"
)

func ReadFileBinary(workspaceBase string, _ *fsops.PathLocker) http.HandlerFunc {
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

		mimeType := fsops.GetMimeType(resolved)
		fileName := filepath.Base(resolved)

		w.Header().Set("Content-Type", mimeType)
		w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, fileName))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	}
}
