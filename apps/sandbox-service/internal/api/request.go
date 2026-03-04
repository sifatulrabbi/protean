package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

const (
	maxJSONBodyBytes      int64 = 1 << 20
	maxMultipartBodyBytes int64 = 64 << 20
	maxTextReadBytes      int64 = 1 << 20
	maxBinaryReadBytes    int64 = 8 << 20
)

// requireMethod enforces a single HTTP method for an endpoint.
func (s *Server) requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method != method {
		s.writeMethodNotAllowed(w, r, method)
		return false
	}
	return true
}

// decodeJSON reads a bounded JSON body, disallows unknown fields, and rejects trailing data.
func (s *Server) decodeJSON(
	w http.ResponseWriter,
	r *http.Request,
	dst interface{},
	maxBytes int64,
) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		s.writeBadRequest(w, err)
		return err
	}

	// Decode once more to ensure the body contains exactly one JSON document.
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			err = errors.New("trailing data")
		}
		s.writeBadRequest(w, err)
		return err
	}

	return nil
}

// decodeMultipart reads and parses a bounded multipart form request.
func (s *Server) decodeMultipart(
	w http.ResponseWriter,
	r *http.Request,
	maxBytes int64,
) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		s.writeBadRequest(w, err)
		return err
	}
	return nil
}
