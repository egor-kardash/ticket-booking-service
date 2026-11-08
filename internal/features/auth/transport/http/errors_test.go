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

	"github.com/egor-kardash/ticket-booking-service/internal/features/auth/domain"
	"github.com/egor-kardash/ticket-booking-service/internal/share/httpserver"
)

type errorEnvelope struct {
	Error struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Fields  map[string]string `json:"fields"`
	} `json:"error"`
}

// serveWrittenError вызывает функцию записи ошибки в контексте с discard-логгером,
// чтобы ожидаемые 500/503 не засоряли вывод тестов.
func serveWrittenError(t *testing.T, write func(context.Context, http.ResponseWriter, error), err error) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	handler := httpserver.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			write(r.Context(), w, err)
		}),
	)
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

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
	tests := []struct {
		name           string
		err            error
		wantStatus     int
		wantCode       string
		wantFields     bool
		forbiddenLeaks []string
	}{
		{
			name:       "validation error",
			err:        &domain.ValidationError{Fields: map[string]string{"email": "invalid email address"}},
			wantStatus: http.StatusBadRequest,
			wantCode:   "validation_failed",
			wantFields: true,
		},
		{
			name:       "email taken",
			err:        fmt.Errorf("register: %w", domain.ErrEmailTaken),
			wantStatus: http.StatusConflict,
			wantCode:   "email_taken",
		},
		{
			name:       "not found",
			err:        fmt.Errorf("get me: %w", domain.ErrNotFound),
			wantStatus: http.StatusNotFound,
			wantCode:   "not_found",
		},
		{
			name:       "invalid credentials",
			err:        domain.ErrInvalidCredentials,
			wantStatus: http.StatusUnauthorized,
			wantCode:   "invalid_credentials",
		},
		{
			name:           "deadline exceeded",
			err:            fmt.Errorf("register: %w", context.DeadlineExceeded),
			wantStatus:     http.StatusServiceUnavailable,
			wantCode:       "service_unavailable",
			forbiddenLeaks: []string{"deadline", "context"},
		},
		{
			name:           "internal error",
			err:            errors.New(`pq: password authentication failed for user "booking"`),
			wantStatus:     http.StatusInternalServerError,
			wantCode:       "internal_error",
			forbiddenLeaks: []string{"pq:", "password authentication", "booking"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serveWrittenError(t, writeError, tt.err)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, tt.wantStatus, rec.Body.String())
			}

			envelope := decodeError(t, rec)
			if envelope.Error.Code != tt.wantCode {
				t.Errorf("error code = %q, want %q", envelope.Error.Code, tt.wantCode)
			}

			if tt.wantFields {
				if got := envelope.Error.Fields["email"]; got != "invalid email address" {
					t.Errorf("fields[email] = %q, want %q", got, "invalid email address")
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

func TestWriteDecodeError(t *testing.T) {
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
		{
			name:       "unsupported media type",
			err:        httpserver.ErrUnsupportedMediaType,
			wantStatus: http.StatusUnsupportedMediaType,
			wantCode:   "unsupported_media_type",
		},
		{
			name:       "body too large",
			err:        &http.MaxBytesError{Limit: 1},
			wantStatus: http.StatusRequestEntityTooLarge,
			wantCode:   "request_too_large",
		},
		{
			name:       "not a json object",
			err:        httpserver.ErrNotJSONObject,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_json",
		},
		{
			name:       "empty body",
			err:        io.EOF,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_json",
		},
		{
			name:       "syntax error",
			err:        syntaxErr,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serveWrittenError(t, writeDecodeError, tt.err)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, tt.wantStatus, rec.Body.String())
			}

			envelope := decodeError(t, rec)
			if envelope.Error.Code != tt.wantCode {
				t.Errorf("error code = %q, want %q", envelope.Error.Code, tt.wantCode)
			}
		})
	}
}
