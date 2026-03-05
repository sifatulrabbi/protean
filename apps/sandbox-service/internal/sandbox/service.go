package sandbox

import (
	"crypto/sha1"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

// MetadataFilename is the name of the session metadata file written to each workspace.
const MetadataFilename = ".sandbox-session.json"

// SessionMetadata is the persisted metadata stored inside each workspace.
type SessionMetadata struct {
	// SessionID is the stable identifier assigned to the sandbox session.
	SessionID string `json:"sessionId"`
	// CreatedAt stores the creation timestamp in RFC3339Nano format.
	CreatedAt string `json:"createdAt"`
	// WorkspaceFullPath is the host path for the session workspace.
	WorkspaceFullPath string `json:"workspaceFullPath"`
	// WorkspaceMountPath is the path where the workspace is mounted inside the container.
	WorkspaceMountPath string `json:"workspaceMountPath"`
	// ContainerName is the stable Docker container name for the session.
	ContainerName string `json:"containerName"`
	// Image is the Docker image expected for the container.
	Image string `json:"image"`
}

// SandboxSession is returned when a session is created successfully.
type SandboxSession struct {
	// SessionID is the stable identifier assigned to the sandbox session.
	SessionID string `json:"sessionId"`
	// WorkspaceMountPath is the in-container mount path clients should treat as the workspace root.
	WorkspaceMountPath string `json:"workspaceMountPath"`
	// WorkspaceFullPath is the host path to the workspace on disk.
	WorkspaceFullPath string `json:"workspaceFullPath"`
	// ContainerName is the Docker name used for the session container.
	ContainerName string `json:"containerName"`
	// ContainerID is the Docker-generated container ID.
	ContainerID string `json:"containerId"`
	// Image is the Docker image backing the session.
	Image string `json:"image"`
	// State is Docker's current container state string.
	State string `json:"state"`
	// CreatedAt stores when the session was created.
	CreatedAt string `json:"createdAt"`
}

// SandboxSessionStatus reports the current state of an existing session.
type SandboxSessionStatus struct {
	// SessionID is the stable identifier assigned to the sandbox session.
	SessionID string `json:"sessionId"`
	// WorkspaceMountPath is the in-container mount path clients should use.
	WorkspaceMountPath string `json:"workspaceMountPath"`
	// WorkspaceFullPath is the host path to the workspace on disk.
	WorkspaceFullPath string `json:"workspaceFullPath"`
	// ContainerName is the Docker name used for the session container.
	ContainerName string `json:"containerName"`
	// ContainerID is set when Docker still has a container for the session.
	ContainerID string `json:"containerId,omitempty"`
	// Image is the Docker image expected for the session.
	Image string `json:"image"`
	// State is Docker's current state string, or "missing" when no container exists.
	State string `json:"state"`
	// CreatedAt stores when the session was created.
	CreatedAt string `json:"createdAt"`
	// Exists reports that valid metadata for the session still exists.
	Exists bool `json:"exists"`
	// WorkspaceReady reports that the workspace directory is still available.
	WorkspaceReady bool `json:"workspaceReady"`
	// ContainerPresent reports whether Docker still knows about the container.
	ContainerPresent bool `json:"containerPresent"`
	// ContainerRunning reports whether the container is currently running.
	ContainerRunning bool `json:"containerRunning"`
}

// ExecRequest describes a command the caller wants to run inside a sandbox.
type ExecRequest struct {
	// Command is executed with `bash -lc` inside the sandbox container.
	Command string `json:"command"`
	// Cwd is resolved relative to the workspace mount path when provided.
	Cwd string `json:"cwd,omitempty"`
	// TimeoutMs optionally overrides the default exec timeout.
	TimeoutMs int `json:"timeoutMs,omitempty"`
}

// ServiceConfig holds dependencies and limits used by Service.
type ServiceConfig struct {
	// WorkspaceBase is the host directory that stores per-session workspaces.
	WorkspaceBase string
	// ContainerPrefix is the prefix for generated Docker container names.
	ContainerPrefix string
	// WorkspaceMountPath is where the workspace is mounted inside the container.
	WorkspaceMountPath string
	// DefaultImage is the image used for newly created sessions.
	DefaultImage string
	// Logger is the service logger; a default logger is used when nil.
	Logger *slog.Logger
	// ExecDefaultTimeout is used when callers omit a timeout.
	ExecDefaultTimeout time.Duration
	// ExecMaxTimeout caps caller-provided timeouts.
	ExecMaxTimeout time.Duration
	// ExecMaxOutputBytes caps stdout and stderr captured from exec.
	ExecMaxOutputBytes int
}

// Service manages session metadata and delegates runtime operations to Runtime.
type Service struct {
	// config stores static service configuration.
	config ServiceConfig
	// runtime provisions and executes the backing containers.
	runtime Runtime
	// logger records operational events and validation failures.
	logger *slog.Logger
}

const (
	maxSessionIDLength       = 128
	maxContainerSessionSlice = 24
)

func validateSessionID(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", ErrInvalidSessionID
	}
	if trimmed == "." || trimmed == ".." {
		return "", ErrInvalidSessionID
	}
	if len(trimmed) > maxSessionIDLength {
		return "", ErrInvalidSessionID
	}

	for _, r := range trimmed {
		if r == '/' || r == '\\' || r < 0x20 || r == 0x7f {
			return "", ErrInvalidSessionID
		}
	}

	return trimmed, nil
}

func sanitizeSessionIDForContainerName(sessionID string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(sessionID) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('-')
	}

	sanitized := strings.Trim(b.String(), "-")
	if sanitized == "" {
		sanitized = "session"
	}
	if len(sanitized) > maxContainerSessionSlice {
		sanitized = sanitized[:maxContainerSessionSlice]
	}
	return sanitized
}

func containerNameForSession(
	containerPrefix string,
	sessionID string,
) string {
	sum := sha1.Sum([]byte(sessionID))
	hash := hex.EncodeToString(sum[:])[:10]
	sessionPart := sanitizeSessionIDForContainerName(sessionID)
	return fmt.Sprintf("%s-%s-%s", containerPrefix, sessionPart, hash)
}

// NewService constructs a Service using the caller-provided logger.
func NewService(cfg ServiceConfig, r Runtime) *Service {
	if cfg.Logger == nil {
		panic("sandbox.NewService requires ServiceConfig.Logger")
	}

	return &Service{
		config:  cfg,
		runtime: r,
		logger:  cfg.Logger,
	}
}

// CreateSession allocates a workspace, writes metadata, and ensures its container exists.
func (s *Service) CreateSession(
	ctx context.Context,
	requestedSessionID *string,
) (SandboxSession, error) {
	sessionID := ulid.Make().String()
	if requestedSessionID != nil {
		validated, err := validateSessionID(*requestedSessionID)
		if err != nil {
			return SandboxSession{}, err
		}
		sessionID = validated

		metadata, loadErr := s.LoadMetadata(sessionID)
		if loadErr == nil {
			container, ensureErr := s.runtime.EnsureContainer(ctx, metadata)
			if ensureErr != nil {
				return SandboxSession{}, ensureErr
			}
			return s.toSandboxSession(metadata, container), nil
		}
		if !errors.Is(loadErr, ErrSessionNotFound) {
			return SandboxSession{}, loadErr
		}
	}

	workspacePath := filepath.Join(s.config.WorkspaceBase, sessionID)
	if err := os.MkdirAll(workspacePath, 0o755); err != nil {
		s.logger.Error("sandbox session create failed", "step", "mkdir", "error", err)
		return SandboxSession{}, fmt.Errorf("create workspace: %w", err)
	}

	// Persist metadata before starting Docker so later requests can recover the session consistently.
	metadata := SessionMetadata{
		SessionID:          sessionID,
		CreatedAt:          time.Now().UTC().Format(time.RFC3339Nano),
		WorkspaceFullPath:  workspacePath,
		WorkspaceMountPath: s.config.WorkspaceMountPath,
		ContainerName:      containerNameForSession(s.config.ContainerPrefix, sessionID),
		Image:              s.config.DefaultImage,
	}

	if err := s.writeMetadata(metadata); err != nil {
		s.logger.Error("sandbox session create failed", "step", "write_metadata", "sessionId", sessionID, "error", err)
		return SandboxSession{}, err
	}

	container, err := s.runtime.EnsureContainer(ctx, metadata)
	if err != nil {
		_ = os.RemoveAll(workspacePath)
		s.logger.Error("sandbox session create failed", "step", "ensure_container", "sessionId", sessionID, "error", err)
		return SandboxSession{}, err
	}

	s.logger.Info("sandbox session created", "sessionId", sessionID, "containerName", metadata.ContainerName)
	return s.toSandboxSession(metadata, container), nil
}

// GetSessionStatus returns the current metadata and container state for a session.
func (s *Service) GetSessionStatus(
	ctx context.Context,
	sessionID string,
) (SandboxSessionStatus, error) {
	metadata, err := s.LoadMetadata(sessionID)
	if err != nil {
		return SandboxSessionStatus{}, err
	}

	container, err := s.runtime.InspectContainer(ctx, metadata)
	if err != nil && !errors.Is(err, ErrContainerNotFound) {
		s.logger.Error("sandbox session status failed", "sessionId", sessionID, "error", err)
		return SandboxSessionStatus{}, err
	}

	status := SandboxSessionStatus{
		SessionID:          metadata.SessionID,
		WorkspaceMountPath: metadata.WorkspaceMountPath,
		WorkspaceFullPath:  metadata.WorkspaceFullPath,
		ContainerName:      metadata.ContainerName,
		Image:              metadata.Image,
		State:              "missing",
		CreatedAt:          metadata.CreatedAt,
		Exists:             true,
		WorkspaceReady:     true,
		ContainerPresent:   false,
		ContainerRunning:   false,
	}

	if err == nil {
		status.ContainerID = container.ID
		status.State = container.State
		status.ContainerPresent = container.Present
		status.ContainerRunning = container.Running
	}

	return status, nil
}

// DeleteSession removes the backing container but leaves metadata and workspace on disk.
func (s *Service) DeleteSession(ctx context.Context, sessionID string) error {
	metadata, err := s.LoadMetadata(sessionID)
	if err != nil {
		return err
	}

	if err := s.runtime.RemoveContainer(ctx, metadata); err != nil {
		s.logger.Error("sandbox session delete failed", "sessionId", sessionID, "error", err)
		return err
	}

	s.logger.Info("sandbox session container removed", "sessionId", sessionID)
	return nil
}

// Exec validates an exec request, ensures the container is running, and executes it.
func (s *Service) Exec(
	ctx context.Context,
	sessionID string,
	req ExecRequest,
) (ExecResult, error) {
	if req.Command == "" {
		return ExecResult{}, ErrExecInvalidRequest
	}

	metadata, err := s.LoadMetadata(sessionID)
	if err != nil {
		return ExecResult{}, err
	}

	if _, err := s.runtime.EnsureContainer(ctx, metadata); err != nil {
		s.logger.Error("sandbox exec ensure failed", "sessionId", sessionID, "error", err)
		return ExecResult{}, err
	}

	timeout := s.normalizeTimeout(req.TimeoutMs)
	containerCwd, err := ResolveContainerCwd(
		metadata.WorkspaceMountPath,
		req.Cwd,
	)
	if err != nil {
		s.logger.Warn("sandbox exec denied", "sessionId", sessionID, "cwd", req.Cwd, "error", err)
		return ExecResult{}, err
	}

	result, err := s.runtime.Exec(
		ctx,
		metadata,
		containerCwd,
		req.Command,
		timeout,
		s.config.ExecMaxOutputBytes,
	)
	if err != nil {
		s.logger.Error("sandbox exec failed", "sessionId", sessionID, "error", err)
		return ExecResult{}, err
	}
	if result.TimedOut {
		s.logger.Warn("sandbox exec timed out", "sessionId", sessionID)
	}
	return result, nil
}

// LoadMetadata reads and validates the metadata file for a session.
func (s *Service) LoadMetadata(sessionID string) (SessionMetadata, error) {
	validSessionID, err := validateSessionID(sessionID)
	if err != nil {
		return SessionMetadata{}, ErrInvalidSessionID
	}

	workspacePath := filepath.Join(s.config.WorkspaceBase, validSessionID)
	metadataPath := filepath.Join(workspacePath, MetadataFilename)
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return SessionMetadata{}, ErrSessionNotFound
		}
		return SessionMetadata{}, fmt.Errorf("read session metadata: %w", err)
	}

	var metadata SessionMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		s.logger.Warn("sandbox metadata invalid", "sessionId", sessionID, "error", err)
		return SessionMetadata{}, ErrInvalidSessionMetadata
	}

	// Each field is validated against current config so stale or tampered metadata is rejected.
	if metadata.SessionID != validSessionID {
		s.logger.Warn("sandbox metadata invalid", "sessionId", validSessionID, "field", "sessionId")
		return SessionMetadata{}, ErrInvalidSessionMetadata
	}
	expectedWorkspacePath := filepath.Join(s.config.WorkspaceBase, validSessionID)
	if filepath.Clean(metadata.WorkspaceFullPath) != filepath.Clean(expectedWorkspacePath) {
		s.logger.Warn("sandbox metadata invalid", "sessionId", validSessionID, "field", "workspaceFullPath")
		return SessionMetadata{}, ErrInvalidSessionMetadata
	}
	if metadata.WorkspaceMountPath != s.config.WorkspaceMountPath {
		s.logger.Warn("sandbox metadata invalid", "sessionId", validSessionID, "field", "workspaceMountPath")
		return SessionMetadata{}, ErrInvalidSessionMetadata
	}
	expectedContainerName := containerNameForSession(
		s.config.ContainerPrefix,
		validSessionID,
	)
	if metadata.ContainerName != expectedContainerName {
		s.logger.Warn("sandbox metadata invalid", "sessionId", validSessionID, "field", "containerName")
		return SessionMetadata{}, ErrInvalidSessionMetadata
	}
	if metadata.Image != s.config.DefaultImage {
		s.logger.Warn("sandbox metadata invalid", "sessionId", validSessionID, "field", "image")
		return SessionMetadata{}, ErrInvalidSessionMetadata
	}

	return metadata, nil
}

// MetadataPath returns the on-disk metadata path for a valid session.
func (s *Service) MetadataPath(sessionID string) (string, error) {
	metadata, err := s.LoadMetadata(sessionID)
	if err != nil {
		return "", err
	}

	return filepath.Join(metadata.WorkspaceFullPath, MetadataFilename), nil
}

// normalizeTimeout applies defaults and caps user-supplied timeout values.
func (s *Service) normalizeTimeout(timeoutMs int) time.Duration {
	if timeoutMs <= 0 {
		return s.config.ExecDefaultTimeout
	}

	timeout := time.Duration(timeoutMs) * time.Millisecond
	if timeout > s.config.ExecMaxTimeout {
		return s.config.ExecMaxTimeout
	}

	return timeout
}

// toSandboxSession builds the API response returned from session creation.
func (s *Service) toSandboxSession(
	metadata SessionMetadata,
	container ContainerState,
) SandboxSession {
	return SandboxSession{
		SessionID:          metadata.SessionID,
		WorkspaceMountPath: metadata.WorkspaceMountPath,
		WorkspaceFullPath:  metadata.WorkspaceFullPath,
		ContainerName:      metadata.ContainerName,
		ContainerID:        container.ID,
		Image:              metadata.Image,
		State:              container.State,
		CreatedAt:          metadata.CreatedAt,
	}
}

// writeMetadata atomically writes session metadata into the workspace directory.
func (s *Service) writeMetadata(metadata SessionMetadata) error {
	metadataPath := filepath.Join(metadata.WorkspaceFullPath, MetadataFilename)
	tempPath := metadataPath + ".tmp"

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session metadata: %w", err)
	}

	if err := os.WriteFile(tempPath, data, 0o644); err != nil {
		return fmt.Errorf("write session metadata: %w", err)
	}
	// Rename keeps readers from observing a partially written metadata file.
	if err := os.Rename(tempPath, metadataPath); err != nil {
		return fmt.Errorf("persist session metadata: %w", err)
	}
	return nil
}
