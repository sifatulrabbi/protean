package api

import (
	"net/http"
	"strings"
)

// authorize validates the bearer token against the configured token map.
func (s *Server) authorize(r *http.Request) bool {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(header, "Bearer ") {
		return false
	}

	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	_, ok := s.serviceTokens[token]
	return ok
}
