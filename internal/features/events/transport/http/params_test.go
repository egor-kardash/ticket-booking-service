package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/egor-kardash/ticket-booking-service/internal/features/events/domain"
	"github.com/egor-kardash/ticket-booking-service/internal/features/events/service"
)

func TestParseListEventsParams(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		wantLimit  int
		wantCursor bool
		wantErr    error
	}{
		{name: "default limit", url: "/events", wantLimit: defaultLimit},
		{name: "lower boundary", url: "/events?limit=1", wantLimit: 1},
		{name: "upper boundary", url: "/events?limit=100", wantLimit: 100},
		{name: "non-numeric limit", url: "/events?limit=abc", wantErr: service.ErrInvalidLimit},
		{name: "empty cursor means first page", url: "/events?cursor=", wantLimit: defaultLimit},
		// Валидный base64 без ':' — невалидный cursor.
		{name: "invalid cursor", url: "/events?cursor=AAAA", wantErr: ErrInvalidCursor},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit, cursor, err := parseListEventsParams(httptest.NewRequest(http.MethodGet, tt.url, nil))

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("parseListEventsParams() error = %v, want %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("parseListEventsParams() error = %v, want nil", err)
			}
			if limit != tt.wantLimit {
				t.Errorf("limit = %d, want %d", limit, tt.wantLimit)
			}
			if got := cursor != nil; got != tt.wantCursor {
				t.Errorf("cursor present = %v, want %v", got, tt.wantCursor)
			}
		})
	}
}

func requestWithEventID(id string) *http.Request {
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", id)

	req := httptest.NewRequest(http.MethodGet, "/events/"+id, nil)

	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
}

func TestParseEventID(t *testing.T) {
	id, err := parseEventID(requestWithEventID("1"))
	if err != nil {
		t.Fatalf("parseEventID(\"1\") error = %v, want nil", err)
	}
	if id != 1 {
		t.Errorf("parseEventID(\"1\") = %d, want 1", id)
	}

	tests := []struct {
		name        string
		id          string
		wantMessage string
	}{
		{name: "zero", id: "0", wantMessage: "must be positive"},
		{name: "negative", id: "-1", wantMessage: "must be positive"},
		{name: "not a number", id: "abc", wantMessage: "must be integer"},
		{name: "int64 overflow", id: "9223372036854775808", wantMessage: "must be integer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseEventID(requestWithEventID(tt.id))

			var validationErr *domain.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("parseEventID() error = %T(%v), want *domain.ValidationError", err, err)
			}
			// Ровно одно поле: "must be integer" не должно перезаписываться на "positive".
			if len(validationErr.Fields) != 1 {
				t.Fatalf("ValidationError.Fields = %v, want exactly one field", validationErr.Fields)
			}
			if got := validationErr.Fields["id"]; got != tt.wantMessage {
				t.Errorf("fields[id] = %q, want %q", got, tt.wantMessage)
			}
		})
	}
}
