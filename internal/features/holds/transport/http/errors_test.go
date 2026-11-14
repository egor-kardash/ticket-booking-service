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

	"github.com/egor-kardash/ticket-booking-service/internal/features/holds/domain"
	"github.com/egor-kardash/ticket-booking-service/internal/share/httpserver"
)

type errorEnvelope struct {
	Error struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Fields  map[string]string `json:"fields"`
	} `json:"error"`
}

type writeFunc func(context.Context, http.ResponseWriter, error)

func serveError(t *testing.T, write writeFunc, err error) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	handler := httpserver.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			write(r.Context(), w, err)
		}),
	)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/holds/1", nil))

	return rec
}

func decodeErrorEnvelope(t *testing.T, rec *httptest.ResponseRecorder) errorEnvelope {
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

func TestWriteErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantFields bool
		noResponse bool
	}{
		{
			name:       "validation error",
			err:        fmt.Errorf("create hold: %w", &domain.ValidationError{Fields: map[string]string{"event_seat_id": "must be positive"}}),
			wantStatus: http.StatusBadRequest,
			wantCode:   "validation_failed",
			wantFields: true,
		},
		{
			name:       "seat taken",
			err:        fmt.Errorf("create hold: %w", domain.ErrSeatTaken),
			wantStatus: http.StatusConflict,
			wantCode:   "seat_taken",
		},
		{
			name:       "hold not found",
			err:        fmt.Errorf("cancel hold: %w", domain.ErrHoldNotFound),
			wantStatus: http.StatusNotFound,
			wantCode:   "not_found",
		},
		{
			name:       "event seat not found",
			err:        fmt.Errorf("create hold: %w", domain.ErrEventSeatNotFound),
			wantStatus: http.StatusNotFound,
			wantCode:   "not_found",
		},
		{
			// Токен валиден, но пользователь удалён: repository отдаёт ErrUnauthorized.
			name:       "unauthorized",
			err:        fmt.Errorf("create hold: %w", domain.ErrUnauthorized),
			wantStatus: http.StatusUnauthorized,
			wantCode:   "unauthorized",
		},
		{
			// Клиент отменил запрос: писать ответ некуда.
			name:       "context canceled writes nothing",
			err:        fmt.Errorf("cancel hold: %w", context.Canceled),
			noResponse: true,
		},
		{
			name:       "deadline exceeded",
			err:        fmt.Errorf("expire holds: %w", context.DeadlineExceeded),
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "service_unavailable",
		},
		{
			name:       "unknown error",
			err:        errors.New(`pq: relation "booking.holds" does not exist`),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serveError(t, writeError, tt.err)

			if tt.noResponse {
				if rec.Body.Len() != 0 {
					t.Errorf("body = %q, want empty", rec.Body.String())
				}
				if got := rec.Header().Get("Content-Type"); got != "" {
					t.Errorf("Content-Type = %q, want empty", got)
				}
				return
			}

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, tt.wantStatus, rec.Body.String())
			}

			envelope := decodeErrorEnvelope(t, rec)
			if envelope.Error.Code != tt.wantCode {
				t.Errorf("error code = %q, want %q", envelope.Error.Code, tt.wantCode)
			}

			if tt.wantFields {
				if got := envelope.Error.Fields["event_seat_id"]; got != "must be positive" {
					t.Errorf("fields[event_seat_id] = %q, want %q", got, "must be positive")
				}
			} else if strings.Contains(rec.Body.String(), `"fields"`) {
				t.Errorf("response contains fields key: %q", rec.Body.String())
			}

			var leaks []string
			switch tt.wantStatus {
			case http.StatusInternalServerError:
				leaks = []string{"pq:", "booking.holds", "relation"}
			case http.StatusServiceUnavailable:
				leaks = []string{"deadline", "context"}
			}
			for _, leak := range leaks {
				if strings.Contains(rec.Body.String(), leak) {
					t.Errorf("response leaks %q: %s", leak, rec.Body.String())
				}
			}
		})
	}
}

func TestWriteDecodeErrorMapping(t *testing.T) {
	var target any
	syntaxErr := json.Unmarshal([]byte("{"), &target)
	if syntaxErr == nil {
		t.Fatal("expected JSON syntax error")
	}

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "body too large", err: &http.MaxBytesError{Limit: 1}, wantStatus: http.StatusRequestEntityTooLarge, wantCode: "request_too_large"},
		{name: "unsupported media type", err: httpserver.ErrUnsupportedMediaType, wantStatus: http.StatusUnsupportedMediaType, wantCode: "unsupported_media_type"},
		{name: "not a json object", err: httpserver.ErrNotJSONObject, wantStatus: http.StatusBadRequest, wantCode: "invalid_json"},
		{name: "empty body", err: io.EOF, wantStatus: http.StatusBadRequest, wantCode: "invalid_json"},
		{name: "syntax error", err: syntaxErr, wantStatus: http.StatusBadRequest, wantCode: "invalid_json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serveError(t, writeDecodeError, tt.err)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, tt.wantStatus, rec.Body.String())
			}

			envelope := decodeErrorEnvelope(t, rec)
			if envelope.Error.Code != tt.wantCode {
				t.Errorf("error code = %q, want %q", envelope.Error.Code, tt.wantCode)
			}
			if strings.Contains(rec.Body.String(), `"fields"`) {
				t.Errorf("response contains fields key: %q", rec.Body.String())
			}
		})
	}
}
