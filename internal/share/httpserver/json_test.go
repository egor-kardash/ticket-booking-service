package httpserver

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type jsonPayload struct {
	Email string `json:"email"`
}

func TestDecodeJSON(t *testing.T) {
	oversized := `{"email":"` + strings.Repeat("a", maxBodyBytes) + `"}`

	tests := []struct {
		name        string
		contentType string
		body        string
		wantErr     error // nil = успешный разбор
		wantAnyErr  bool  // ошибка любого вида (сообщение не контракт)
		wantMaxBody bool
		wantEmail   string
	}{
		{
			name:        "accepts valid object",
			contentType: "application/json",
			body:        `{"email":"user@example.com"}`,
			wantEmail:   "user@example.com",
		},
		{
			name:        "accepts content type with charset",
			contentType: "application/json; charset=utf-8",
			body:        `{"email":"user@example.com"}`,
			wantEmail:   "user@example.com",
		},
		{
			// Политика: application/*+json тоже допустим.
			name:        "accepts json suffix media type",
			contentType: "application/vnd.api+json",
			body:        `{"email":"user@example.com"}`,
			wantEmail:   "user@example.com",
		},
		{
			// Пустой объект синтаксически валиден; обязательность полей — забота домена.
			name:        "accepts empty object",
			contentType: "application/json",
			body:        `{}`,
		},
		{
			name:        "rejects unknown field",
			contentType: "application/json",
			body:        `{"email":"user@example.com","role":"admin"}`,
			wantAnyErr:  true,
		},
		{
			name:        "rejects second json value",
			contentType: "application/json",
			body:        `{"email":"user@example.com"}{"email":"other@example.com"}`,
			wantAnyErr:  true,
		},
		{
			name:        "rejects empty body",
			contentType: "application/json",
			body:        "",
			wantErr:     io.EOF,
		},
		{
			name:        "rejects whitespace-only body",
			contentType: "application/json",
			body:        " \n\t ",
			wantErr:     io.EOF,
		},
		{
			// RawMessage + проверка '{' отвергают null: фиксируем выбранное поведение.
			name:        "rejects null body",
			contentType: "application/json",
			body:        `null`,
			wantErr:     ErrNotJSONObject,
		},
		{
			name:        "rejects array",
			contentType: "application/json",
			body:        `[]`,
			wantErr:     ErrNotJSONObject,
		},
		{
			name:        "rejects scalar",
			contentType: "application/json",
			body:        `42`,
			wantErr:     ErrNotJSONObject,
		},
		{
			name:        "rejects body larger than limit",
			contentType: "application/json",
			body:        oversized,
			wantMaxBody: true,
		},
		{
			name:        "rejects unsupported content type",
			contentType: "text/plain",
			body:        `{"email":"user@example.com"}`,
			wantErr:     ErrUnsupportedMediaType,
		},
		{
			name:        "rejects missing content type",
			contentType: "",
			body:        `{"email":"user@example.com"}`,
			wantErr:     ErrUnsupportedMediaType,
		},
		{
			name:        "rejects malformed content type",
			contentType: "application/json; charset",
			body:        `{"email":"user@example.com"}`,
			wantErr:     ErrUnsupportedMediaType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			rec := httptest.NewRecorder()

			var dst jsonPayload
			err := DecodeJSON(rec, req, &dst)

			switch {
			case tt.wantMaxBody:
				var maxBytesErr *http.MaxBytesError
				if !errors.As(err, &maxBytesErr) {
					t.Fatalf("DecodeJSON() error = %T(%v), want *http.MaxBytesError", err, err)
				}
			case tt.wantAnyErr:
				if err == nil {
					t.Fatalf("DecodeJSON() = %+v, want error", dst)
				}
			default:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("DecodeJSON() error = %v, want %v", err, tt.wantErr)
				}
				if tt.wantErr == nil && dst.Email != tt.wantEmail {
					t.Errorf("decoded email = %q, want %q", dst.Email, tt.wantEmail)
				}
			}
		})
	}
}
