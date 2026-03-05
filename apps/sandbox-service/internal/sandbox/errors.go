package sandbox

import "errors"

var (
	// ErrInvalidSessionID is returned when the caller supplies a malformed session ID.
	ErrInvalidSessionID = errors.New("invalid sandbox session id")
	// ErrSessionNotFound is returned when no metadata exists for the requested session.
	ErrSessionNotFound = errors.New("sandbox session not found")
	// ErrInvalidSessionMetadata is returned when persisted metadata fails validation.
	ErrInvalidSessionMetadata = errors.New("invalid sandbox session metadata")
	// ErrExecInvalidRequest is returned when an exec request is missing required fields.
	ErrExecInvalidRequest = errors.New("invalid sandbox exec request")
	// ErrExecCwdEscapesWorkspace is returned when an exec cwd points outside the workspace.
	ErrExecCwdEscapesWorkspace = errors.New(
		"sandbox exec cwd escapes workspace root",
	)
)
