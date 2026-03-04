package api

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/protean/sandbox-service/internal/sandbox"
	"github.com/protean/sandbox-service/internal/security"
)

// sessionPathOptions controls which reserved workspace paths a handler may touch.
type sessionPathOptions struct {
	// forbidRoot rejects operations against the workspace root itself.
	forbidRoot bool
	// forbidAsDestination rejects using the workspace root as the rename destination.
	forbidAsDestination bool
}

// resolveSessionMetadata loads validated metadata and writes an API error on failure.
func (s *Server) resolveSessionMetadata(
	w http.ResponseWriter,
	sessionID string,
) (sandbox.SessionMetadata, bool) {
	metadata, err := s.service.LoadMetadata(sessionID)
	if err != nil {
		s.writeSandboxError(w, err)
		return sandbox.SessionMetadata{}, false
	}
	return metadata, true
}

// resolveSessionPath validates a workspace-relative path and resolves it on disk.
func (s *Server) resolveSessionPath(
	w http.ResponseWriter,
	metadata sandbox.SessionMetadata,
	rawPath string,
	opts sessionPathOptions,
) (string, bool) {
	if err := validateSessionFilePath(rawPath, opts.forbidRoot, opts.forbidAsDestination); err != nil {
		s.logger.Warn("sandbox file access denied", "code", "RESERVED_PATH", "path", rawPath, "error", err)
		writeError(w, http.StatusForbidden, "RESERVED_PATH", err.Error())
		return "", false
	}

	resolved, err := security.ResolveSafePath(metadata.WorkspaceFullPath, rawPath)
	if err != nil {
		s.writePathError(w, err)
		return "", false
	}

	return resolved, true
}

// handleMissingPath logs and returns the shared not-found payload for file endpoints.
func (s *Server) handleMissingPath(w http.ResponseWriter, r *http.Request, kind string, requestPath string) {
	s.logger.Warn(
		"sandbox path not found",
		"method", r.Method,
		"path", r.URL.Path,
		"kind", kind,
		"requestPath", requestPath,
	)
	writeError(w, http.StatusNotFound, "NOT_FOUND", kind+" not found")
}

// validateSessionFilePath rejects reserved workspace paths such as metadata and, optionally, root.
func validateSessionFilePath(rawPath string, forbidRoot bool, forbidAsDestination bool) error {
	cleaned := path.Clean("/" + strings.TrimSpace(rawPath))
	if forbidRoot && cleaned == "/" {
		if forbidAsDestination {
			return errors.New("workspace root cannot be a destination")
		}
		return errors.New("workspace root cannot be modified")
	}

	if cleaned == "/"+sandbox.MetadataFilename {
		return errors.New("sandbox session metadata is reserved")
	}

	return nil
}

// readFileLimited reads at most limit bytes and reports oversize files explicitly.
func (s *Server) readFileLimited(
	resolved string,
	limit int64,
) ([]byte, error) {
	file, err := os.Open(resolved)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := io.LimitReader(file, limit+1)
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errFileTooLarge
	}
	return data, nil
}
