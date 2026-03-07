package api

import (
	"errors"
	"net/http"

	"github.com/protean/sandbox-service/internal/sandbox"
	"github.com/protean/sandbox-service/internal/security"
)

var errFileTooLarge = errors.New("file exceeds maximum readable size")

// writeBadRequest standardizes request parsing errors into API responses.
func (s *Server) writeBadRequest(w http.ResponseWriter, err error) {
	message := "invalid request body"
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		message = "request body exceeds maximum size"
	}
	writeError(w, http.StatusBadRequest, "BAD_REQUEST", message)
}

// writePathError translates path validation failures into security-focused responses.
func (s *Server) writePathError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, security.ErrSymlinkDetected):
		s.logger.Warn("sandbox file access denied", "code", "SYMLINK_NOT_ALLOWED", "error", err)
		writeError(w, http.StatusForbidden, "SYMLINK_NOT_ALLOWED", "symlink access is not allowed")
	default:
		s.logger.Warn("sandbox file access denied", "code", "PATH_TRAVERSAL", "error", err)
		writeError(w, http.StatusForbidden, "PATH_TRAVERSAL", "path escapes workspace root")
	}
}

// writeSandboxError maps service-layer errors to their HTTP representation.
func (s *Server) writeSandboxError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sandbox.ErrInvalidSessionID):
		writeError(w, http.StatusBadRequest, "INVALID_SESSION_ID", "invalid sandbox session id")
	case errors.Is(err, sandbox.ErrSessionNotFound),
		errors.Is(err, sandbox.ErrInvalidSessionMetadata):
		writeError(w, http.StatusNotFound, "NOT_FOUND", "sandbox session not found")
	case errors.Is(err, sandbox.ErrExecInvalidRequest):
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid exec request")
	case errors.Is(err, sandbox.ErrExecCwdEscapesWorkspace):
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "exec cwd escapes workspace root")
	case errors.Is(err, errFileTooLarge):
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", errFileTooLarge.Error())
	default:
		s.logger.Error("sandbox request failed", "error", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "internal server error")
	}
}
