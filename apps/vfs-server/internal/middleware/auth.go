package middleware

import (
	"net/http"
	"strings"

	"github.com/protean/vfs-server/internal/fsops"
)

// ServiceAuth validates the Bearer token in the Authorization header against
// the configured service tokens map.
func ServiceAuth(tokens map[string]string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				fsops.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid authorization header")
				return
			}

			token := strings.TrimPrefix(auth, "Bearer ")
			if _, ok := tokens[token]; !ok {
				fsops.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid service token")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
