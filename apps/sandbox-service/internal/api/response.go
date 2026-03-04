package api

import (
	"encoding/json"
	"net/http"
)

// errorBody is the structured error payload returned in envelope.Error.
type errorBody struct {
	// Code is a stable machine-readable error code.
	Code string `json:"code"`
	// Message is the human-readable error message.
	Message string `json:"message"`
}

// envelope is the top-level response shape for all JSON endpoints.
type envelope struct {
	// OK reports whether the request succeeded.
	OK bool `json:"ok"`
	// Data carries the successful response payload.
	Data interface{} `json:"data,omitempty"`
	// Error carries error details when OK is false.
	Error *errorBody `json:"error,omitempty"`
}

// writeJSON writes a successful JSON response envelope.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{
		OK:   true,
		Data: data,
	})
}

// writeError writes a failed JSON response envelope.
func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{
		OK: false,
		Error: &errorBody{
			Code:    code,
			Message: message,
		},
	})
}
