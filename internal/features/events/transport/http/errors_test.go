package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/egor-kardash/ticket-booking-service/internal/features/events/domain"
	"github.com/egor-kardash/ticket-booking-service/internal/features/events/service"
	"github.com/egor-kardash/ticket-booking-service/internal/share/httpserver"
)

type errorEnvelope struct {
	Error struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Fields  map[string]string `json:"fields"`
	} `json:"error"`
}

// serveWrittenError вызывает writeError в контексте с discard-логгером,
// чтобы ожидаемые 500/503 не засоряли вывод тестов.
func serveWrittenError(t *testing.T, err error) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	handler := httpserver.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeError(r.Context(), w, err)
		}),
	)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/events", nil))

	return rec
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) errorEnvelope {
	t.Helper()

	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", got)
	}

	var envelope errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error envelope: %v (body %q)", err, rec.Body.String())
	}
	if envelope.Error.Message == "" {
		t.Error("error message is empty")
	}

	return envelope
}

func TestWriteError(t *testing.T) {
	validation := &domain.ValidationError{Fields: map[string]string{"id": "must be positive"}}

	tests := []struct {
		name           string
		err            error
		wantStatus     int
		wantCode       string
		wantFields     bool
		forbiddenLeaks []string
	}{
		{
			name:       "event not found",
			err:        fmt.Errorf("get event: %w", domain.ErrEventNotFound),
			wantStatus: http.StatusNotFound,
			wantCode:   "not_found",
		},
		{
			name:       "invalid limit",
			err:        fmt.Errorf("list events: %w", service.ErrInvalidLimit),
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_limit",
		},
		{
			name:       "invalid cursor",
			err:        fmt.Errorf("parse cursor: %w", ErrInvalidCursor),
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_cursor",
		},
		{
			name:       "validation error",
			err:        validation,
			wantStatus: http.StatusBadRequest,
			wantCode:   "validation_failed",
			wantFields: true,
		},
		{
			name:           "deadline exceeded",
			err:            fmt.Errorf("list events: %w", context.DeadlineExceeded),
			wantStatus:     http.StatusServiceUnavailable,
			wantCode:       "service_unavailable",
			forbiddenLeaks: []string{"deadline", "context"},
		},
		{
			name:           "internal error",
			err:            errors.New(`pq: relation "booking.events" does not exist`),
			wantStatus:     http.StatusInternalServerError,
			wantCode:       "internal_error",
			forbiddenLeaks: []string{"pq:", "booking.events", "relation"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serveWrittenError(t, tt.err)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, tt.wantStatus, rec.Body.String())
			}

			envelope := decodeError(t, rec)
			if envelope.Error.Code != tt.wantCode {
				t.Errorf("error code = %q, want %q", envelope.Error.Code, tt.wantCode)
			}

			if tt.wantFields {
				if got := envelope.Error.Fields["id"]; got != "must be positive" {
					t.Errorf("fields[id] = %q, want %q", got, "must be positive")
				}
			} else {
				if len(envelope.Error.Fields) != 0 {
					t.Errorf("fields = %v, want omitted", envelope.Error.Fields)
				}
				if strings.Contains(rec.Body.String(), `"fields"`) {
					t.Errorf("response contains fields key: %q", rec.Body.String())
				}
			}

			body := strings.ToLower(rec.Body.String())
			for _, leak := range tt.forbiddenLeaks {
				if strings.Contains(body, strings.ToLower(leak)) {
					t.Errorf("response leaks %q: %s", leak, rec.Body.String())
				}
			}
		})
	}
}
