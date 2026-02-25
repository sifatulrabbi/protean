package fsops

import (
	"encoding/json"
	"net/http"
)

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Envelope struct {
	OK    bool        `json:"ok"`
	Data  interface{} `json:"data,omitempty"`
	Error *ErrorBody  `json:"error,omitempty"`
}

func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(Envelope{OK: true, Data: data})
}

func WriteError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(Envelope{
		OK:    false,
		Error: &ErrorBody{Code: code, Message: message},
	})
}
