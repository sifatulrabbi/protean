package httpserver

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeClock struct{ now time.Time }

func (f fakeClock) Now() time.Time { return f.now }

func TestHealthz(t *testing.T) {
	fixed := time.Date(2026, 8, 6, 12, 30, 0, 0, time.UTC)
	srv := New(Deps{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Clock:  fakeClock{now: fixed},
	})

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{"ok", http.MethodGet, "/healthz", http.StatusOK},
		{"wrong method", http.MethodPost, "/healthz", http.StatusMethodNotAllowed},
		{"unknown path", http.MethodGet, "/nope", http.StatusNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if tc.wantStatus != http.StatusOK {
				return
			}

			var body healthResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body.Status != "ok" {
				t.Errorf("status = %q, want ok", body.Status)
			}
			if want := fixed.Format(time.RFC3339); body.Time != want {
				t.Errorf("time = %q, want %q", body.Time, want)
			}
		})
	}
}
