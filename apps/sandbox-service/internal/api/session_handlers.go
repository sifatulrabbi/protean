package api

import (
	"net/http"
	"strings"

	"github.com/protean/sandbox-service/internal/sandbox"
)

type createSessionRequest struct {
	SessionID *string `json:"sessionId,omitempty"`
}

// handleSessions handles collection-level session routes.
func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if !s.requireMethod(w, r, http.MethodPost) {
		return
	}

	var payload createSessionRequest
	if r.Body != nil && r.Body != http.NoBody {
		if err := s.decodeJSON(w, r, &payload, maxJSONBodyBytes); err != nil {
			return
		}
	}

	session, err := s.service.CreateSession(r.Context(), payload.SessionID)
	if err != nil {
		s.writeSandboxError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, session)
}

// handleSessionRoute dispatches routes nested under a specific session ID.
func (s *Server) handleSessionRoute(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/v1/sandbox/sessions/")
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 || parts[0] == "" {
		s.logger.Warn("sandbox route not found", "method", r.Method, "path", r.URL.Path)
		http.NotFound(w, r)
		return
	}

	sessionID := parts[0]
	// Route shape is fixed, so the remaining path segments are enough to identify the handler.
	switch {
	case len(parts) == 1:
		s.handleSessionRoot(w, r, sessionID)
	case len(parts) == 2 && parts[1] == "exec":
		s.handleExec(w, r, sessionID)
	case len(parts) == 3 && parts[1] == "files":
		s.handleFiles(w, r, sessionID, parts[2])
	default:
		s.logger.Warn("sandbox route not found", "method", r.Method, "path", r.URL.Path, "sessionId", sessionID)
		http.NotFound(w, r)
	}
}

// handleSessionRoot serves GET/DELETE operations on a single session resource.
func (s *Server) handleSessionRoot(w http.ResponseWriter, r *http.Request, sessionID string) {
	switch r.Method {
	case http.MethodGet:
		status, err := s.service.GetSessionStatus(r.Context(), sessionID)
		if err != nil {
			s.writeSandboxError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, status)
	case http.MethodDelete:
		if err := s.service.DeleteSession(r.Context(), sessionID); err != nil {
			s.writeSandboxError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
	default:
		s.writeMethodNotAllowed(w, r, http.MethodGet+"/"+http.MethodDelete)
	}
}

// handleExec executes a command inside an existing sandbox session.
func (s *Server) handleExec(w http.ResponseWriter, r *http.Request, sessionID string) {
	if !s.requireMethod(w, r, http.MethodPost) {
		return
	}

	var req sandbox.ExecRequest
	if err := s.decodeJSON(w, r, &req, maxJSONBodyBytes); err != nil {
		return
	}

	result, err := s.service.Exec(r.Context(), sessionID, req)
	if err != nil {
		s.writeSandboxError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}
