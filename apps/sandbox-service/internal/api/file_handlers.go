package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/protean/sandbox-service/internal/sandbox"
)

const timeLayout = "2006-01-02T15:04:05.000Z07:00"

// writeFileRequest is the JSON payload for writing a text file.
type writeFileRequest struct {
	// Path is the workspace-relative destination path.
	Path string `json:"path"`
	// Content is written verbatim to the destination file.
	Content string `json:"content"`
}

// mkdirRequest is the JSON payload for creating a directory tree.
type mkdirRequest struct {
	// Path is the workspace-relative directory path to create.
	Path string `json:"path"`
}

// renameRequest is the JSON payload for moving a file or directory.
type renameRequest struct {
	// Path is the current workspace-relative path.
	Path string `json:"path"`
	// NewPath is the new workspace-relative destination path.
	NewPath string `json:"newPath"`
}

// handleFiles dispatches file-management routes within a session.
func (s *Server) handleFiles(
	w http.ResponseWriter,
	r *http.Request,
	sessionID string,
	endpoint string,
) {
	metadata, ok := s.resolveSessionMetadata(w, sessionID)
	if !ok {
		return
	}

	switch endpoint {
	case "stat":
		s.handleFileStat(w, r, metadata)
	case "readdir":
		s.handleFileReadDir(w, r, metadata)
	case "read":
		s.handleFileRead(w, r, metadata)
	case "read-binary":
		s.handleFileReadBinary(w, r, metadata)
	case "write":
		s.handleFileWrite(w, r, metadata)
	case "write-binary":
		s.handleFileWriteBinary(w, r, metadata)
	case "mkdir":
		s.handleFileMkdir(w, r, metadata)
	case "remove":
		s.handleFileRemove(w, r, metadata)
	case "rename":
		s.handleFileRename(w, r, metadata)
	default:
		s.logger.Warn(
			"sandbox file route not found",
			"method", r.Method,
			"path", r.URL.Path,
			"sessionId", sessionID,
			"endpoint", endpoint,
		)
		http.NotFound(w, r)
	}
}

// handleFileStat returns filesystem metadata for a single path.
func (s *Server) handleFileStat(
	w http.ResponseWriter,
	r *http.Request,
	metadata sandbox.SessionMetadata,
) {
	if !s.requireMethod(w, r, http.MethodGet) {
		return
	}

	resolved, ok := s.resolveSessionPath(
		w,
		metadata,
		r.URL.Query().Get("path"),
		sessionPathOptions{},
	)
	if !ok {
		return
	}

	info, err := os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			s.handleMissingPath(w, r, "file", r.URL.Query().Get("path"))
			return
		}
		s.writeSandboxError(w, err)
		return
	}

	modified := info.ModTime().UTC().Format(timeLayout)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"size":        info.Size(),
		"isDirectory": info.IsDir(),
		"modified":    modified,
		"created":     modified,
	})
}

// handleFileReadDir lists the immediate children of a directory.
func (s *Server) handleFileReadDir(
	w http.ResponseWriter,
	r *http.Request,
	metadata sandbox.SessionMetadata,
) {
	if !s.requireMethod(w, r, http.MethodGet) {
		return
	}

	resolved, ok := s.resolveSessionPath(
		w,
		metadata,
		r.URL.Query().Get("path"),
		sessionPathOptions{},
	)
	if !ok {
		return
	}

	entries, err := os.ReadDir(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			s.handleMissingPath(w, r, "directory", r.URL.Query().Get("path"))
			return
		}
		s.writeSandboxError(w, err)
		return
	}

	result := make([]map[string]interface{}, 0, len(entries))
	for _, entry := range entries {
		result = append(result, map[string]interface{}{
			"name":        entry.Name(),
			"isDirectory": entry.IsDir(),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"entries": result,
	})
}

// handleFileRead returns a text file, enforcing the text-size limit.
func (s *Server) handleFileRead(
	w http.ResponseWriter,
	r *http.Request,
	metadata sandbox.SessionMetadata,
) {
	if !s.requireMethod(w, r, http.MethodGet) {
		return
	}

	resolved, ok := s.resolveSessionPath(
		w,
		metadata,
		r.URL.Query().Get("path"),
		sessionPathOptions{},
	)
	if !ok {
		return
	}

	data, err := s.readFileLimited(resolved, maxTextReadBytes)
	if err != nil {
		switch {
		case os.IsNotExist(err):
			s.handleMissingPath(w, r, "file", r.URL.Query().Get("path"))
		default:
			s.writeSandboxError(w, err)
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"content": string(data),
	})
}

// handleFileReadBinary streams raw bytes from a file, enforcing the binary-size limit.
func (s *Server) handleFileReadBinary(
	w http.ResponseWriter,
	r *http.Request,
	metadata sandbox.SessionMetadata,
) {
	if !s.requireMethod(w, r, http.MethodGet) {
		return
	}

	resolved, ok := s.resolveSessionPath(
		w,
		metadata,
		r.URL.Query().Get("path"),
		sessionPathOptions{},
	)
	if !ok {
		return
	}

	data, err := s.readFileLimited(resolved, maxBinaryReadBytes)
	if err != nil {
		switch {
		case os.IsNotExist(err):
			s.handleMissingPath(w, r, "file", r.URL.Query().Get("path"))
		default:
			s.writeSandboxError(w, err)
		}
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// handleFileWrite writes UTF-8 text content to a workspace file.
func (s *Server) handleFileWrite(
	w http.ResponseWriter,
	r *http.Request,
	metadata sandbox.SessionMetadata,
) {
	if !s.requireMethod(w, r, http.MethodPost) {
		return
	}

	var req writeFileRequest
	if err := s.decodeJSON(w, r, &req, maxJSONBodyBytes); err != nil {
		return
	}

	resolved, ok := s.resolveSessionPath(
		w,
		metadata,
		req.Path,
		sessionPathOptions{},
	)
	if !ok {
		return
	}

	unlock := s.locker.LockExact(resolved)
	defer unlock()

	// Ensure parent directories exist so callers can create nested files in one request.
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		s.writeSandboxError(w, err)
		return
	}

	data := []byte(req.Content)
	if err := os.WriteFile(resolved, data, 0o644); err != nil {
		s.writeSandboxError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]int{"bytesWritten": len(data)})
}

// handleFileWriteBinary stores a multipart-uploaded file inside the workspace.
func (s *Server) handleFileWriteBinary(
	w http.ResponseWriter,
	r *http.Request,
	metadata sandbox.SessionMetadata,
) {
	if !s.requireMethod(w, r, http.MethodPost) {
		return
	}

	if err := s.decodeMultipart(w, r, maxMultipartBodyBytes); err != nil {
		return
	}

	filePath := r.FormValue("path")
	if filePath == "" {
		s.logger.Warn("sandbox file upload missing path", "method", r.Method, "path", r.URL.Path, "sessionId", metadata.SessionID)
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "missing path field")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		s.logger.Warn("sandbox file upload missing file", "method", r.Method, "path", r.URL.Path, "sessionId", metadata.SessionID)
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "missing file field")
		return
	}
	defer file.Close()

	resolved, ok := s.resolveSessionPath(
		w,
		metadata,
		filePath,
		sessionPathOptions{},
	)
	if !ok {
		return
	}

	unlock := s.locker.LockExact(resolved)
	defer unlock()

	// Match the text write path by creating parent directories before opening the file.
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		s.writeSandboxError(w, err)
		return
	}

	destination, err := os.OpenFile(resolved, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		s.writeSandboxError(w, err)
		return
	}
	defer destination.Close()

	bytesWritten, err := copyFileWithLimit(destination, file, maxMultipartBodyBytes)
	if err != nil {
		s.writeSandboxError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]int64{"bytesWritten": bytesWritten})
}

// handleFileMkdir creates a directory and any missing parents.
func (s *Server) handleFileMkdir(
	w http.ResponseWriter,
	r *http.Request,
	metadata sandbox.SessionMetadata,
) {
	if !s.requireMethod(w, r, http.MethodPost) {
		return
	}

	var req mkdirRequest
	if err := s.decodeJSON(w, r, &req, maxJSONBodyBytes); err != nil {
		return
	}

	resolved, ok := s.resolveSessionPath(
		w,
		metadata,
		req.Path,
		sessionPathOptions{},
	)
	if !ok {
		return
	}

	unlock := s.locker.LockSubtree(resolved)
	defer unlock()

	if err := os.MkdirAll(resolved, 0o755); err != nil {
		s.writeSandboxError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"created": true})
}

// handleFileRemove removes a file or directory subtree.
func (s *Server) handleFileRemove(
	w http.ResponseWriter,
	r *http.Request,
	metadata sandbox.SessionMetadata,
) {
	if !s.requireMethod(w, r, http.MethodDelete) {
		return
	}

	resolved, ok := s.resolveSessionPath(
		w,
		metadata,
		r.URL.Query().Get("path"),
		sessionPathOptions{forbidRoot: true},
	)
	if !ok {
		return
	}

	unlock := s.locker.LockSubtree(resolved)
	defer unlock()

	if err := os.RemoveAll(resolved); err != nil {
		s.writeSandboxError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"removed": true})
}

// handleFileRename moves a file or directory to a new location inside the workspace.
func (s *Server) handleFileRename(
	w http.ResponseWriter,
	r *http.Request,
	metadata sandbox.SessionMetadata,
) {
	if !s.requireMethod(w, r, http.MethodPatch) {
		return
	}

	var req renameRequest
	if err := s.decodeJSON(w, r, &req, maxJSONBodyBytes); err != nil {
		return
	}

	source, ok := s.resolveSessionPath(
		w,
		metadata,
		req.Path,
		sessionPathOptions{forbidRoot: true},
	)
	if !ok {
		return
	}
	destination, ok := s.resolveSessionPath(
		w,
		metadata,
		req.NewPath,
		sessionPathOptions{forbidRoot: true, forbidAsDestination: true},
	)
	if !ok {
		return
	}

	unlock := s.locker.LockSubtree(source, destination)
	defer unlock()

	// Create the destination parent first so cross-directory renames succeed.
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		s.writeSandboxError(w, err)
		return
	}
	if err := os.Rename(source, destination); err != nil {
		if os.IsNotExist(err) {
			s.handleMissingPath(w, r, "file or directory", req.Path)
			return
		}
		s.writeSandboxError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"renamed": true})
}

// copyFileWithLimit copies src into dst and removes partial output when the upload is too large.
func copyFileWithLimit(dst *os.File, src io.Reader, limit int64) (int64, error) {
	limited := io.LimitReader(src, limit+1)
	written, err := io.Copy(dst, limited)
	if err != nil {
		return 0, err
	}
	if written > limit {
		// Reset the file so callers do not observe a truncated partial upload.
		if truncateErr := dst.Truncate(0); truncateErr != nil {
			return 0, fmt.Errorf("truncate oversized upload: %w", truncateErr)
		}
		if _, seekErr := dst.Seek(0, 0); seekErr != nil {
			return 0, fmt.Errorf("reset oversized upload: %w", seekErr)
		}
		return 0, errFileTooLarge
	}
	return written, nil
}
