//go:build integration

package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/egor-kardash/ticket-booking-service/internal/features/events/domain"
	"github.com/egor-kardash/ticket-booking-service/internal/share/token"
)

func resetHoldTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	truncate := func() {
		// context.Background: t.Context уже отменён к моменту cleanup.
		if _, err := pool.Exec(context.Background(), `
			TRUNCATE booking.holds, booking.event_seats, booking.seats,
			         booking.events, booking.venues, booking.users
			RESTART IDENTITY CASCADE`); err != nil {
			t.Errorf("truncate holds: %v", err)
		}
	}

	truncate()
	t.Cleanup(truncate)
}

func insertTestUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()

	var id int64
	if err := pool.QueryRow(t.Context(),
		`INSERT INTO booking.users (email, password_hash) VALUES ($1, 'hash') RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("insert user %q: %v", email, err)
	}

	return id
}

func issueTestToken(t *testing.T, userID int64) string {
	t.Helper()

	issuer := token.New(testJWTSecret, 15*time.Minute)
	tokenStr, _, err := issuer.IssueToken(userID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	return tokenStr
}

func (e *eventsEnv) request(t *testing.T, method, path, body, bearer string) httpResult {
	t.Helper()

	headers := make(map[string]string, 2)
	if body != "" {
		headers["Content-Type"] = "application/json"
	}
	if bearer != "" {
		headers["Authorization"] = "Bearer " + bearer
	}

	return e.requestWithHeaders(t, method, path, body, headers)
}

func (e *eventsEnv) requestWithHeaders(t *testing.T, method, path, body string, headers map[string]string) httpResult {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}

	req, err := http.NewRequestWithContext(t.Context(), method, e.server.URL+path, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	return httpResult{status: resp.StatusCode, header: resp.Header.Clone(), body: data}
}

type holdBody struct {
	ID          int64  `json:"id"`
	EventSeatID int64  `json:"event_seat_id"`
	ExpiresAt   string `json:"expires_at"`
	PriceCents  int64  `json:"price_cents"`
}

func TestHoldsRequireAuth(t *testing.T) {
	env := newEventsEnv(t)

	endpoints := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "create", method: http.MethodPost, path: "/api/v1/holds", body: `{"event_seat_id":1}`},
		{name: "get", method: http.MethodGet, path: "/api/v1/holds/1"},
		{name: "delete", method: http.MethodDelete, path: "/api/v1/holds/1"},
	}

	tokens := []struct {
		name          string
		bearer        string
		wantChallenge string
	}{
		{name: "without token", wantChallenge: "Bearer"},
		{name: "with broken token", bearer: "not-a-jwt", wantChallenge: `Bearer error="invalid_token"`},
	}

	for _, endpoint := range endpoints {
		for _, token := range tokens {
			t.Run(endpoint.name+"/"+token.name, func(t *testing.T) {
				result := env.request(t, endpoint.method, endpoint.path, endpoint.body, token.bearer)

				requireAPIError(t, result, http.StatusUnauthorized, "unauthorized")
				if got := result.header.Get("WWW-Authenticate"); got != token.wantChallenge {
					t.Errorf("WWW-Authenticate = %q, want %q", got, token.wantChallenge)
				}
			})
		}
	}
}

func TestCreateHoldHTTP(t *testing.T) {
	env := newEventsEnv(t)
	resetHoldTables(t, env.pool)

	venueID := insertTestVenue(t, env.pool, "Main Hall")
	eventID := insertTestEvent(t, env.pool, venueID, "Concert", time.Now().Add(time.Hour))
	seatID := insertTestSeat(t, env.pool, venueID, "A", "1", "1")
	eventSeatID := insertTestEventSeat(t, env.pool, eventID, seatID, venueID, 500000, domain.EventSeatAvailable)

	ownerID := insertTestUser(t, env.pool, "owner@example.com")
	foreignID := insertTestUser(t, env.pool, "foreign@example.com")
	ownerToken := issueTestToken(t, ownerID)
	foreignToken := issueTestToken(t, foreignID)

	result := env.request(t, http.MethodPost, "/api/v1/holds",
		fmt.Sprintf(`{"event_seat_id":%d}`, eventSeatID), ownerToken)

	if result.status != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body %q)", result.status, result.body)
	}

	var hold holdBody
	decodeBody(t, result.body, &hold)
	if hold.ID <= 0 || hold.EventSeatID != eventSeatID || hold.PriceCents != 500000 {
		t.Errorf("hold = %+v, want id, event_seat_id %d and price 500000", hold, eventSeatID)
	}
	if !strings.HasSuffix(hold.ExpiresAt, "Z") {
		t.Errorf("expires_at = %q, want UTC RFC3339", hold.ExpiresAt)
	}

	var fields map[string]json.RawMessage
	decodeBody(t, result.body, &fields)
	if len(fields) != 4 {
		t.Errorf("body fields = %v, want exactly 4", fields)
	}
	for _, field := range []string{"id", "event_seat_id", "expires_at", "price_cents"} {
		if _, ok := fields[field]; !ok {
			t.Errorf("body missing field %q", field)
		}
	}

	wantLocation := "/api/v1/holds/" + strconv.FormatInt(hold.ID, 10)
	if got := result.header.Get("Location"); got != wantLocation {
		t.Errorf("Location = %q, want %q", got, wantLocation)
	}

	t.Run("foreign user cannot read hold", func(t *testing.T) {
		path := "/api/v1/holds/" + strconv.FormatInt(hold.ID, 10)

		foreign := env.request(t, http.MethodGet, path, "", foreignToken)
		requireAPIError(t, foreign, http.StatusNotFound, "not_found")

		owner := env.request(t, http.MethodGet, path, "", ownerToken)
		if owner.status != http.StatusOK {
			t.Fatalf("owner GET status = %d, want 200 (body %q)", owner.status, owner.body)
		}
	})

	t.Run("foreign user cannot cancel hold", func(t *testing.T) {
		path := "/api/v1/holds/" + strconv.FormatInt(hold.ID, 10)

		foreign := env.request(t, http.MethodDelete, path, "", foreignToken)
		requireAPIError(t, foreign, http.StatusNotFound, "not_found")

		owner := env.request(t, http.MethodGet, path, "", ownerToken)
		if owner.status != http.StatusOK {
			t.Errorf("hold disappeared after foreign cancel: status %d (body %q)", owner.status, owner.body)
		}
	})
}

type holdFixture struct {
	eventSeatID  int64
	ownerToken   string
	foreignToken string
}

func newHoldFixture(t *testing.T, pool *pgxpool.Pool) holdFixture {
	t.Helper()

	venueID := insertTestVenue(t, pool, "Main Hall")
	eventID := insertTestEvent(t, pool, venueID, "Concert", time.Now().Add(time.Hour))
	seatID := insertTestSeat(t, pool, venueID, "A", "1", "1")
	eventSeatID := insertTestEventSeat(t, pool, eventID, seatID, venueID, 500000, domain.EventSeatAvailable)

	ownerID := insertTestUser(t, pool, "owner@example.com")
	foreignID := insertTestUser(t, pool, "foreign@example.com")

	return holdFixture{
		eventSeatID:  eventSeatID,
		ownerToken:   issueTestToken(t, ownerID),
		foreignToken: issueTestToken(t, foreignID),
	}
}

func (f holdFixture) create(t *testing.T, env *eventsEnv) holdBody {
	t.Helper()

	result := env.request(t, http.MethodPost, "/api/v1/holds",
		fmt.Sprintf(`{"event_seat_id":%d}`, f.eventSeatID), f.ownerToken)
	if result.status != http.StatusCreated {
		t.Fatalf("create hold status = %d, want 201 (body %q)", result.status, result.body)
	}

	if got := result.header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("create hold Cache-Control = %q, want no-store", got)
	}

	var hold holdBody
	decodeBody(t, result.body, &hold)

	return hold
}

func holdPath(id int64) string {
	return "/api/v1/holds/" + strconv.FormatInt(id, 10)
}

func requireAllowHeader(t *testing.T, header http.Header, methods ...string) {
	t.Helper()

	allow := header.Get("Allow")
	if allow == "" {
		t.Fatalf("Allow header is missing, want %v", methods)
	}
	for _, method := range methods {
		if !strings.Contains(allow, method) {
			t.Errorf("Allow = %q, missing %q", allow, method)
		}
	}
}

func TestHoldHTTPLifecycle(t *testing.T) {
	env := newEventsEnv(t)
	resetHoldTables(t, env.pool)

	fixture := newHoldFixture(t, env.pool)
	hold := fixture.create(t, env)
	path := holdPath(hold.ID)

	got := env.request(t, http.MethodGet, path, "", fixture.ownerToken)
	if got.status != http.StatusOK {
		t.Fatalf("GET status = %d, want 200 (body %q)", got.status, got.body)
	}
	if cc := got.header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("GET Cache-Control = %q, want no-store", cc)
	}
	if pragma := got.header.Get("Pragma"); pragma != "no-cache" {
		t.Errorf("GET Pragma = %q, want no-cache", pragma)
	}

	cancelled := env.request(t, http.MethodDelete, path, "", fixture.ownerToken)
	if cancelled.status != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want 204 (body %q)", cancelled.status, cancelled.body)
	}
	if len(cancelled.body) != 0 {
		t.Errorf("DELETE body = %q, want empty", cancelled.body)
	}
	if ct := cancelled.header.Get("Content-Type"); ct != "" {
		t.Errorf("DELETE Content-Type = %q, want empty", ct)
	}

	requireAPIError(t, env.request(t, http.MethodGet, path, "", fixture.ownerToken), http.StatusNotFound, "not_found")
	requireAPIError(t, env.request(t, http.MethodDelete, path, "", fixture.ownerToken), http.StatusNotFound, "not_found")

	var seatStatus string
	if err := env.pool.QueryRow(t.Context(),
		`SELECT status FROM booking.event_seats WHERE id = $1`, fixture.eventSeatID).Scan(&seatStatus); err != nil {
		t.Fatalf("select seat status: %v", err)
	}
	if seatStatus != "available" {
		t.Errorf("event seat status = %q, want available", seatStatus)
	}
}

func TestHoldExpiredHTTP(t *testing.T) {
	env := newEventsEnv(t)
	resetHoldTables(t, env.pool)

	fixture := newHoldFixture(t, env.pool)
	hold := fixture.create(t, env)
	path := holdPath(hold.ID)

	// CHECK expires_at > created_at, поэтому двигаем обе метки.
	if _, err := env.pool.Exec(t.Context(), `
		UPDATE booking.holds
		SET created_at = now() - interval '2 minutes',
		    expires_at = now() - interval '1 minute'
		WHERE id = $1`, hold.ID); err != nil {
		t.Fatalf("expire hold: %v", err)
	}

	requireAPIError(t, env.request(t, http.MethodGet, path, "", fixture.ownerToken), http.StatusNotFound, "not_found")
	requireAPIError(t, env.request(t, http.MethodDelete, path, "", fixture.ownerToken), http.StatusNotFound, "not_found")

	var status string
	if err := env.pool.QueryRow(t.Context(),
		`SELECT status FROM booking.holds WHERE id = $1`, hold.ID).Scan(&status); err != nil {
		t.Fatalf("select hold status: %v", err)
	}
	if status != "active" {
		t.Errorf("hold status = %q, want active (воркер в этом тесте не запускался)", status)
	}
}

func TestHoldProtocolErrors(t *testing.T) {
	env := newEventsEnv(t)
	resetHoldTables(t, env.pool)

	fixture := newHoldFixture(t, env.pool)

	t.Run("broken json", func(t *testing.T) {
		result := env.request(t, http.MethodPost, "/api/v1/holds", `{"event_seat_id":`, fixture.ownerToken)
		requireAPIError(t, result, http.StatusBadRequest, "invalid_json")
	})

	t.Run("unknown field", func(t *testing.T) {
		body := fmt.Sprintf(`{"event_seat_id":%d,"extra":true}`, fixture.eventSeatID)
		result := env.request(t, http.MethodPost, "/api/v1/holds", body, fixture.ownerToken)
		requireAPIError(t, result, http.StatusBadRequest, "invalid_json")
	})

	t.Run("missing event_seat_id", func(t *testing.T) {
		result := env.request(t, http.MethodPost, "/api/v1/holds", `{}`, fixture.ownerToken)
		payload := requireAPIError(t, result, http.StatusBadRequest, "validation_failed")
		if payload.Error.Fields["event_seat_id"] == "" {
			t.Errorf("fields = %v, want event_seat_id", payload.Error.Fields)
		}
	})

	t.Run("zero event_seat_id", func(t *testing.T) {
		result := env.request(t, http.MethodPost, "/api/v1/holds", `{"event_seat_id":0}`, fixture.ownerToken)
		payload := requireAPIError(t, result, http.StatusBadRequest, "validation_failed")
		if payload.Error.Fields["event_seat_id"] == "" {
			t.Errorf("fields = %v, want event_seat_id", payload.Error.Fields)
		}
	})

	t.Run("non numeric hold id", func(t *testing.T) {
		result := env.request(t, http.MethodGet, "/api/v1/holds/abc", "", fixture.ownerToken)
		payload := requireAPIError(t, result, http.StatusBadRequest, "validation_failed")
		if payload.Error.Fields["id"] == "" {
			t.Errorf("fields = %v, want id", payload.Error.Fields)
		}
	})

	t.Run("zero hold id", func(t *testing.T) {
		result := env.request(t, http.MethodGet, "/api/v1/holds/0", "", fixture.ownerToken)
		payload := requireAPIError(t, result, http.StatusBadRequest, "validation_failed")
		if payload.Error.Fields["id"] == "" {
			t.Errorf("fields = %v, want id", payload.Error.Fields)
		}
	})

	t.Run("non numeric hold id on delete", func(t *testing.T) {
		result := env.request(t, http.MethodDelete, "/api/v1/holds/abc", "", fixture.ownerToken)
		payload := requireAPIError(t, result, http.StatusBadRequest, "validation_failed")
		if payload.Error.Fields["id"] == "" {
			t.Errorf("fields = %v, want id", payload.Error.Fields)
		}
	})

	t.Run("zero hold id on delete", func(t *testing.T) {
		result := env.request(t, http.MethodDelete, "/api/v1/holds/0", "", fixture.ownerToken)
		payload := requireAPIError(t, result, http.StatusBadRequest, "validation_failed")
		if payload.Error.Fields["id"] == "" {
			t.Errorf("fields = %v, want id", payload.Error.Fields)
		}
	})

	t.Run("missing hold", func(t *testing.T) {
		result := env.request(t, http.MethodGet, "/api/v1/holds/999999", "", fixture.ownerToken)
		requireAPIError(t, result, http.StatusNotFound, "not_found")
	})

	t.Run("unsupported media type", func(t *testing.T) {
		result := env.requestWithHeaders(t, http.MethodPost, "/api/v1/holds", `{"event_seat_id":1}`,
			map[string]string{
				"Content-Type":  "text/plain",
				"Authorization": "Bearer " + fixture.ownerToken,
			})
		requireAPIError(t, result, http.StatusUnsupportedMediaType, "unsupported_media_type")
	})

	t.Run("body too large", func(t *testing.T) {
		// Валидный JSON, который заставляет decoder читать тело за лимитом 1 MiB.
		body := `{"event_seat_id":"` + strings.Repeat("a", 1<<20) + `"}`

		result := env.requestWithHeaders(t, http.MethodPost, "/api/v1/holds", body,
			map[string]string{
				"Content-Type":  "application/json",
				"Authorization": "Bearer " + fixture.ownerToken,
			})
		requireAPIError(t, result, http.StatusRequestEntityTooLarge, "request_too_large")
	})

	t.Run("seat already held", func(t *testing.T) {
		fixture.create(t, env)

		result := env.request(t, http.MethodPost, "/api/v1/holds",
			fmt.Sprintf(`{"event_seat_id":%d}`, fixture.eventSeatID), fixture.foreignToken)
		requireAPIError(t, result, http.StatusConflict, "seat_taken")
	})

	t.Run("method not allowed", func(t *testing.T) {
		result := env.request(t, http.MethodPut, holdPath(1), "", fixture.ownerToken)
		requireAPIError(t, result, http.StatusMethodNotAllowed, "method_not_allowed")
		requireAllowHeader(t, result.header, http.MethodGet, http.MethodDelete)

		result = env.request(t, http.MethodPut, "/api/v1/holds", "", fixture.ownerToken)
		requireAPIError(t, result, http.StatusMethodNotAllowed, "method_not_allowed")
		requireAllowHeader(t, result.header, http.MethodPost)
	})
}
