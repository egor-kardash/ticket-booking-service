package httpserver

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// quietContext подменяет logger, чтобы ожидаемые ошибки не засоряли вывод теста.
func quietContext() context.Context {
	return context.WithValue(context.Background(),
		loggerKey{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()

	WriteJSON(quietContext(), rec, http.StatusCreated, struct {
		ID int64 `json:"id"`
	}{ID: 7})

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", got)
	}

	var body struct {
		ID int64 `json:"id"`
	}
	if err := decodeJSONBody(t, rec.Body.String(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.ID != 7 {
		t.Errorf("id = %d, want 7", body.ID)
	}
}

func TestWriteJSONMarshalError(t *testing.T) {
	rec := httptest.NewRecorder()

	WriteJSON(quietContext(), rec, http.StatusOK, make(chan int))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	var envelope errorEnvelope
	if err := decodeJSONBody(t, rec.Body.String(), &envelope); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if envelope.Error.Code != "internal_error" || envelope.Error.Message != "internal server error" {
		t.Errorf("envelope = %+v, want generic internal_error", envelope.Error)
	}
}

func TestWriteNoContent(t *testing.T) {
	rec := httptest.NewRecorder()
	// Если Content-Type уже выставлен, 204 обязан его снять.
	rec.Header().Set("Content-Type", "application/json")

	WriteNoContent(rec)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("Content-Type"); got != "" {
		t.Errorf("Content-Type = %q, want empty for 204", got)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("body = %q, want empty for 204", rec.Body.String())
	}
}

func TestWriteError(t *testing.T) {
	t.Run("without fields", func(t *testing.T) {
		rec := httptest.NewRecorder()

		WriteError(quietContext(), rec, http.StatusConflict, "email_taken", "email is already taken")

		var envelope errorEnvelope
		if err := decodeJSONBody(t, rec.Body.String(), &envelope); err != nil {
			t.Fatalf("decode error envelope: %v", err)
		}
		if envelope.Error.Code != "email_taken" || envelope.Error.Message != "email is already taken" {
			t.Errorf("error = %+v, want email_taken envelope", envelope.Error)
		}
		if strings.Contains(rec.Body.String(), `"fields"`) {
			t.Errorf("response contains fields key: %q", rec.Body.String())
		}
	})

	t.Run("with fields", func(t *testing.T) {
		rec := httptest.NewRecorder()

		WriteErrorFields(quietContext(), rec, http.StatusBadRequest,
			"validation_failed", "request validation failed",
			map[string]string{"email": "invalid email address"})

		var envelope errorEnvelope
		if err := decodeJSONBody(t, rec.Body.String(), &envelope); err != nil {
			t.Fatalf("decode error envelope: %v", err)
		}
		if got := envelope.Error.Fields["email"]; got != "invalid email address" {
			t.Errorf("fields[email] = %q, want %q", got, "invalid email address")
		}
	})
}
