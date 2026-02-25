package middleware

import (
	"context"
	"net/http"
	"os"
	"path/filepath"

	"github.com/protean/vfs-server/internal/fsops"
)

type contextKey string

const userIDKey contextKey = "userId"

// UserContext extracts and validates the X-User-Id header, ensures the user's
// workspace directory exists, and injects the user ID into the request context.
func UserContext(workspaceBase string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := r.Header.Get("X-User-Id")
			if userID == "" || len(userID) < 8 {
				fsops.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "missing X-User-Id header")
				return
			}

			userRoot := filepath.Join(workspaceBase, userID)
			if err := os.MkdirAll(userRoot, 0755); err != nil {
				fsops.WriteError(w, http.StatusInternalServerError, "INTERNAL", "failed to prepare workspace")
				return
			}

			ctx := context.WithValue(r.Context(), userIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUserID retrieves the user ID from the request context.
func GetUserID(ctx context.Context) string {
	v, _ := ctx.Value(userIDKey).(string)
	return v
}
