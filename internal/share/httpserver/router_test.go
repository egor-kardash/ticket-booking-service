package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func decodeJSONBody(t *testing.T, body string, dst any) error {
	t.Helper()

	return json.Unmarshal([]byte(body), dst)
}

type stubPinger struct {
	err   error
	calls int
}

func (p *stubPinger) Ping(context.Context) error {
	p.calls++

	return p.err
}

func newTestRouter(p Pinger) http.Handler {
	return NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), p)
}

func TestHealthEndpoints(t *testing.T) {
	t.Run("healthz reports ok", func(t *testing.T) {
		rec := httptest.NewRecorder()

		newTestRouter(&stubPinger{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		var body struct {
			Status string `json:"status"`
		}
		if err := decodeJSONBody(t, rec.Body.String(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.Status != "ok" {
			t.Errorf("status = %q, want %q", body.Status, "ok")
		}
	})

	t.Run("readyz reports ready", func(t *testing.T) {
		pinger := &stubPinger{}
		rec := httptest.NewRecorder()

		newTestRouter(pinger).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %q)", rec.Code, http.StatusOK, rec.Body.String())
		}
		if pinger.calls != 1 {
			t.Errorf("Ping() calls = %d, want 1", pinger.calls)
		}

		var body struct {
			Status string `json:"status"`
		}
		if err := decodeJSONBody(t, rec.Body.String(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.Status != "ready" {
			t.Errorf("status = %q, want %q", body.Status, "ready")
		}
	})

	t.Run("readyz reports database unavailable", func(t *testing.T) {
		pinger := &stubPinger{err: errors.New(`dial tcp 127.0.0.1:5433: connection refused`)}
		rec := httptest.NewRecorder()

		newTestRouter(pinger).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
		}
		if pinger.calls != 1 {
			t.Errorf("Ping() calls = %d, want 1", pinger.calls)
		}

		var envelope errorEnvelope
		if err := decodeJSONBody(t, rec.Body.String(), &envelope); err != nil {
			t.Fatalf("decode error envelope: %v", err)
		}
		if envelope.Error.Code != "database_unavailable" {
			t.Errorf("error code = %q, want database_unavailable", envelope.Error.Code)
		}
	})
}

func TestRouterErrors(t *testing.T) {
	t.Run("unknown route returns json 404", func(t *testing.T) {
		rec := httptest.NewRecorder()

		newTestRouter(&stubPinger{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}

		var envelope errorEnvelope
		if err := decodeJSONBody(t, rec.Body.String(), &envelope); err != nil {
			t.Fatalf("decode error envelope: %v", err)
		}
		if envelope.Error.Code != "not_found" {
			t.Errorf("error code = %q, want not_found", envelope.Error.Code)
		}
	})

	t.Run("wrong method returns 405 with Allow", func(t *testing.T) {
		rec := httptest.NewRecorder()

		newTestRouter(&stubPinger{}).ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/healthz", nil))

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
		}

		allow := rec.Header().Get("Allow")
		if !strings.Contains(allow, http.MethodGet) {
			t.Errorf("Allow = %q, want to contain %q", allow, http.MethodGet)
		}

		var envelope errorEnvelope
		if err := decodeJSONBody(t, rec.Body.String(), &envelope); err != nil {
			t.Fatalf("decode error envelope: %v", err)
		}
		if envelope.Error.Code != "method_not_allowed" {
			t.Errorf("error code = %q, want method_not_allowed", envelope.Error.Code)
		}
	})
}
