package httpserver

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/sifatulrabbi/protean/backend/internal/ports"
)

type handlers struct {
	clock ports.Clock
}

type healthResponse struct {
	Status string `json:"status"`
	Time   string `json:"time"`
}

func (h *handlers) healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		Status: "ok",
		Time:   h.clock.Now().UTC().Format(time.RFC3339),
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
