package http_test

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
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/egor-kardash/ticket-booking-service/internal/features/events/domain"
	eventshttp "github.com/egor-kardash/ticket-booking-service/internal/features/events/transport/http"
	"github.com/egor-kardash/ticket-booking-service/internal/share/httpserver"
)

type fakeEventService struct {
	listItems  []domain.EventListItem
	listNext   *domain.EventCursor
	listErr    error
	listLimit  int
	listCursor *domain.EventCursor
	listCalls  int

	getResult *domain.EventDetails
	getErr    error
	getID     int64
	getCalls  int

	seats      []domain.EventSeatItem
	seatsErr   error
	seatsID    int64
	seatsCalls int
}

func (f *fakeEventService) ListEvents(_ context.Context, limit int, cursor *domain.EventCursor) ([]domain.EventListItem, *domain.EventCursor, error) {
	f.listCalls++
	f.listLimit = limit
	f.listCursor = cursor

	return f.listItems, f.listNext, f.listErr
}

func (f *fakeEventService) GetEvent(_ context.Context, id int64) (*domain.EventDetails, error) {
	f.getCalls++
	f.getID = id

	return f.getResult, f.getErr
}

func (f *fakeEventService) ListSeats(_ context.Context, eventID int64) ([]domain.EventSeatItem, error) {
	f.seatsCalls++
	f.seatsID = eventID

	return f.seats, f.seatsErr
}

func newTestAPI(svc eventshttp.EventService) http.Handler {
	handler := eventshttp.New(svc)
	router := chi.NewRouter()
	router.Use(httpserver.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))

	router.Route("/api/v1", func(r chi.Router) {
		handler.Routes(r)
	})

	return router
}

func serve(t *testing.T, handler http.Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	return rec
}

func getRequest(t *testing.T, path, bearer string) *http.Request {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	return req
}

type errorEnvelope struct {
	Error struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Fields  map[string]string `json:"fields"`
	} `json:"error"`
}

func requireError(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode string) errorEnvelope {
	t.Helper()

	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d (body %q)", rec.Code, wantStatus, rec.Body.String())
	}

	var envelope errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error envelope: %v (body %q)", err, rec.Body.String())
	}
	if envelope.Error.Code != wantCode {
		t.Errorf("error code = %q, want %q", envelope.Error.Code, wantCode)
	}

	return envelope
}

type listEventsBody struct {
	Items []struct {
		ID       int64  `json:"id"`
		Title    string `json:"title"`
		Venue    string `json:"venue"`
		City     string `json:"city"`
		StartsAt string `json:"starts_at"`
	} `json:"items"`
	NextCursor string `json:"next_cursor"`
}

func TestListEvents(t *testing.T) {
	items := []domain.EventListItem{{
		ID: 1, Title: "Concert", VenueName: "Hall", City: "Moscow",
		StartsAt: time.Date(2026, time.November, 12, 15, 0, 0, 0, time.UTC),
	}}
	next := &domain.EventCursor{
		StartsAt: time.Date(2026, time.November, 12, 15, 0, 0, 0, time.UTC),
		ID:       1,
	}

	t.Run("returns page", func(t *testing.T) {
		svc := &fakeEventService{listItems: items, listNext: next}
		rec := serve(t, newTestAPI(svc), getRequest(t, "/api/v1/events?limit=2", ""))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %q)", rec.Code, http.StatusOK, rec.Body.String())
		}

		var body listEventsBody
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if len(body.Items) != 1 {
			t.Fatalf("items = %d, want 1", len(body.Items))
		}
		if body.Items[0].ID != 1 || body.Items[0].Title != "Concert" ||
			body.Items[0].Venue != "Hall" || body.Items[0].City != "Moscow" ||
			body.Items[0].StartsAt != "2026-11-12T15:00:00Z" {
			t.Errorf("item = %+v, want fields from service", body.Items[0])
		}
		if body.NextCursor == "" {
			t.Error("next_cursor is empty, want cursor from service")
		}
		if svc.listLimit != 2 {
			t.Errorf("ListEvents() limit = %d, want 2", svc.listLimit)
		}
		if svc.listCursor != nil {
			t.Errorf("ListEvents() cursor = %+v, want nil on first page", svc.listCursor)
		}
	})

	t.Run("rejects non-numeric limit", func(t *testing.T) {
		svc := &fakeEventService{}
		rec := serve(t, newTestAPI(svc), getRequest(t, "/api/v1/events?limit=abc", ""))

		requireError(t, rec, http.StatusBadRequest, "invalid_limit")
		if svc.listCalls != 0 {
			t.Errorf("ListEvents() calls = %d, want 0 on parse error", svc.listCalls)
		}
	})

	t.Run("rejects invalid cursor", func(t *testing.T) {
		svc := &fakeEventService{}
		rec := serve(t, newTestAPI(svc), getRequest(t, "/api/v1/events?cursor=AAAA", ""))

		requireError(t, rec, http.StatusBadRequest, "invalid_cursor")
		if svc.listCalls != 0 {
			t.Errorf("ListEvents() calls = %d, want 0 on parse error", svc.listCalls)
		}
	})

	t.Run("hides service error", func(t *testing.T) {
		svc := &fakeEventService{listErr: errors.New("pq: relation booking.events does not exist")}
		rec := serve(t, newTestAPI(svc), getRequest(t, "/api/v1/events", ""))

		requireError(t, rec, http.StatusInternalServerError, "internal_error")
		if strings.Contains(rec.Body.String(), "pq:") || strings.Contains(rec.Body.String(), "booking.events") {
			t.Errorf("response leaks service error: %q", rec.Body.String())
		}
	})
}

type eventBody struct {
	ID             int64  `json:"id"`
	Title          string `json:"title"`
	Venue          string `json:"venue"`
	City           string `json:"city"`
	StartsAt       string `json:"starts_at"`
	SeatsTotal     int64  `json:"seats_total"`
	SeatsAvailable int64  `json:"seats_available"`
}

func TestGetEvent(t *testing.T) {
	details := &domain.EventDetails{
		ID: 7, Title: "Concert", VenueName: "Hall", City: "Moscow",
		StartsAt:   time.Date(2026, time.November, 12, 15, 0, 0, 0, time.UTC),
		SeatsTotal: 100, SeatsAvailable: 5,
	}

	t.Run("returns event", func(t *testing.T) {
		svc := &fakeEventService{getResult: details}
		rec := serve(t, newTestAPI(svc), getRequest(t, "/api/v1/events/7", ""))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %q)", rec.Code, http.StatusOK, rec.Body.String())
		}

		var body eventBody
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.ID != 7 || body.Title != "Concert" || body.Venue != "Hall" || body.City != "Moscow" ||
			body.StartsAt != "2026-11-12T15:00:00Z" || body.SeatsTotal != 100 || body.SeatsAvailable != 5 {
			t.Errorf("body = %+v, want fields from service", body)
		}
		if svc.getID != 7 {
			t.Errorf("GetEvent() id = %d, want 7", svc.getID)
		}
	})

	t.Run("maps not found", func(t *testing.T) {
		svc := &fakeEventService{getErr: fmt.Errorf("get event: %w", domain.ErrEventNotFound)}
		rec := serve(t, newTestAPI(svc), getRequest(t, "/api/v1/events/7", ""))

		requireError(t, rec, http.StatusNotFound, "not_found")
	})

	t.Run("rejects invalid id", func(t *testing.T) {
		tests := []struct {
			name        string
			path        string
			wantMessage string
		}{
			{name: "not a number", path: "/api/v1/events/abc", wantMessage: "must be integer"},
			{name: "not positive", path: "/api/v1/events/0", wantMessage: "must be positive"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				svc := &fakeEventService{}
				rec := serve(t, newTestAPI(svc), getRequest(t, tt.path, ""))

				envelope := requireError(t, rec, http.StatusBadRequest, "validation_failed")
				if got := envelope.Error.Fields["id"]; got != tt.wantMessage {
					t.Errorf("fields[id] = %q, want %q", got, tt.wantMessage)
				}
				if svc.getCalls != 0 {
					t.Errorf("GetEvent() calls = %d, want 0 on parse error", svc.getCalls)
				}
			})
		}
	})

	t.Run("hides service error", func(t *testing.T) {
		svc := &fakeEventService{getErr: errors.New(`pq: relation "booking.events" does not exist`)}
		rec := serve(t, newTestAPI(svc), getRequest(t, "/api/v1/events/7", ""))

		requireError(t, rec, http.StatusInternalServerError, "internal_error")
		if strings.Contains(rec.Body.String(), "pq:") || strings.Contains(rec.Body.String(), "booking.events") {
			t.Errorf("response leaks service error: %q", rec.Body.String())
		}
	})
}

type seatsBody struct {
	Items []struct {
		ID         int64  `json:"id"`
		Section    string `json:"section"`
		Row        string `json:"row"`
		Seat       string `json:"seat"`
		Tier       string `json:"tier"`
		PriceCents int64  `json:"price_cents"`
		Status     string `json:"status"`
	} `json:"items"`
}

func TestListSeats(t *testing.T) {
	seats := []domain.EventSeatItem{
		{ID: 1, Section: "A", RowLabel: "1", SeatNumber: "2", Tier: "vip", PriceCents: 1000, Status: domain.EventSeatAvailable},
		{ID: 2, Section: "B", RowLabel: "3", SeatNumber: "4", Tier: "standard", PriceCents: 500, Status: domain.EventSeatSold},
	}

	t.Run("returns items", func(t *testing.T) {
		svc := &fakeEventService{seats: seats}
		rec := serve(t, newTestAPI(svc), getRequest(t, "/api/v1/events/7/seats", ""))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %q)", rec.Code, http.StatusOK, rec.Body.String())
		}

		var body seatsBody
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if len(body.Items) != 2 {
			t.Fatalf("items = %d, want 2", len(body.Items))
		}
		if body.Items[0].ID != 1 || body.Items[0].Section != "A" || body.Items[0].Row != "1" ||
			body.Items[0].Seat != "2" || body.Items[0].Tier != "vip" ||
			body.Items[0].PriceCents != 1000 || body.Items[0].Status != "available" {
			t.Errorf("item = %+v, want fields from service", body.Items[0])
		}
		if body.Items[1].Status != "sold" {
			t.Errorf("second item status = %q, want sold", body.Items[1].Status)
		}
		if svc.seatsID != 7 {
			t.Errorf("ListSeats() id = %d, want 7", svc.seatsID)
		}
	})

	t.Run("returns empty array for event without seats", func(t *testing.T) {
		svc := &fakeEventService{seats: []domain.EventSeatItem{}}
		rec := serve(t, newTestAPI(svc), getRequest(t, "/api/v1/events/7/seats", ""))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %q)", rec.Code, http.StatusOK, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"items":[]`) {
			t.Errorf("body = %q, want empty items array", rec.Body.String())
		}
	})

	t.Run("maps missing event", func(t *testing.T) {
		svc := &fakeEventService{seatsErr: fmt.Errorf("list seats: %w", domain.ErrEventNotFound)}
		rec := serve(t, newTestAPI(svc), getRequest(t, "/api/v1/events/7/seats", ""))

		requireError(t, rec, http.StatusNotFound, "not_found")
	})

	t.Run("hides service error", func(t *testing.T) {
		svc := &fakeEventService{seatsErr: errors.New(`pq: relation "booking.event_seats" does not exist`)}
		rec := serve(t, newTestAPI(svc), getRequest(t, "/api/v1/events/7/seats", ""))

		requireError(t, rec, http.StatusInternalServerError, "internal_error")
		if strings.Contains(rec.Body.String(), "pq:") || strings.Contains(rec.Body.String(), "booking.event_seats") {
			t.Errorf("response leaks service error: %q", rec.Body.String())
		}
	})
}

// TestEventsRoutesArePublic фиксирует, что events не попадают под Auth middleware:
// ни отсутствие, ни битый Authorization не должны давать 401.
func TestEventsRoutesArePublic(t *testing.T) {
	svc := &fakeEventService{listItems: []domain.EventListItem{}}
	api := newTestAPI(svc)

	tests := []struct {
		name   string
		bearer string
	}{
		{name: "without authorization"},
		{name: "with broken token", bearer: "not-a-jwt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serve(t, api, getRequest(t, "/api/v1/events", tt.bearer))

			if rec.Code == http.StatusUnauthorized {
				t.Fatalf("status = 401, events route must be public (body %q)", rec.Body.String())
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, http.StatusOK, rec.Body.String())
			}
		})
	}
}
