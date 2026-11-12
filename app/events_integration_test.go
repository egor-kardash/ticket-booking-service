//go:build integration

package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/egor-kardash/ticket-booking-service/app"
	"github.com/egor-kardash/ticket-booking-service/internal/features/events/domain"
	"github.com/egor-kardash/ticket-booking-service/internal/testdb"
)

type eventsEnv struct {
	server *httptest.Server
	client *http.Client
	pool   *pgxpool.Pool
}

func newEventsEnv(t *testing.T) *eventsEnv {
	t.Helper()

	ctx := context.Background()

	dbCfg, err := testdb.ConfigFromEnv()
	if err != nil {
		t.Fatalf("test database config: %v", err)
	}

	pool, err := pgxpool.New(ctx, dbCfg.DSN(dbCfg.Database))
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	// События делят booking_test с repo-тестами: сериализуем пакеты.
	release, err := testdb.AcquireLock(ctx, pool)
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	t.Cleanup(release)

	router, err := app.NewRouter(testAppConfig(t), pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("app.NewRouter() error = %v", err)
	}

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	client := server.Client()
	client.Timeout = 10 * time.Second

	return &eventsEnv{server: server, client: client, pool: pool}
}

func resetEventTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	truncate := func() {
		if _, err := pool.Exec(context.Background(),
			"TRUNCATE booking.event_seats, booking.seats, booking.events, booking.venues RESTART IDENTITY CASCADE"); err != nil {
			t.Errorf("truncate events: %v", err)
		}
	}

	truncate()
	t.Cleanup(truncate)
}

func insertTestVenue(t *testing.T, pool *pgxpool.Pool, name string) int64 {
	t.Helper()

	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO booking.venues (name, city) VALUES ($1, 'Moscow') RETURNING id`, name).Scan(&id); err != nil {
		t.Fatalf("insert venue %q: %v", name, err)
	}

	return id
}

func insertTestEvent(t *testing.T, pool *pgxpool.Pool, venueID int64, title string, startsAt time.Time) int64 {
	t.Helper()

	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO booking.events (venue_id, title, starts_at) VALUES ($1, $2, $3) RETURNING id`,
		venueID, title, startsAt).Scan(&id); err != nil {
		t.Fatalf("insert event %q: %v", title, err)
	}

	return id
}

func insertTestSeat(t *testing.T, pool *pgxpool.Pool, venueID int64, section, rowLabel, seatNumber string) int64 {
	t.Helper()

	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO booking.seats (venue_id, section, row_label, seat_number) VALUES ($1, $2, $3, $4) RETURNING id`,
		venueID, section, rowLabel, seatNumber).Scan(&id); err != nil {
		t.Fatalf("insert seat %s/%s/%s: %v", section, rowLabel, seatNumber, err)
	}

	return id
}

func insertTestEventSeat(t *testing.T, pool *pgxpool.Pool, eventID, seatID, venueID, priceCents int64, status domain.EventSeatStatus) {
	t.Helper()

	if _, err := pool.Exec(context.Background(),
		`INSERT INTO booking.event_seats (event_id, seat_id, venue_id, tier, price_cents, status)
		 VALUES ($1, $2, $3, 'standard', $4, $5)`,
		eventID, seatID, venueID, priceCents, string(status)); err != nil {
		t.Fatalf("insert event seat: %v", err)
	}
}

type httpResult struct {
	status int
	header http.Header
	body   []byte
}

func (e *eventsEnv) get(t *testing.T, path string) httpResult {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, e.server.URL+path, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	return httpResult{status: resp.StatusCode, header: resp.Header.Clone(), body: body}
}

type eventsListBody struct {
	Items []struct {
		ID       int64  `json:"id"`
		Title    string `json:"title"`
		Venue    string `json:"venue"`
		City     string `json:"city"`
		StartsAt string `json:"starts_at"`
	} `json:"items"`
	NextCursor string `json:"next_cursor"`
}

type eventDetailBody struct {
	ID             int64  `json:"id"`
	Title          string `json:"title"`
	Venue          string `json:"venue"`
	City           string `json:"city"`
	StartsAt       string `json:"starts_at"`
	SeatsTotal     int64  `json:"seats_total"`
	SeatsAvailable int64  `json:"seats_available"`
}

type eventSeatsBody struct {
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

type apiError struct {
	Error struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Fields  map[string]string `json:"fields"`
	} `json:"error"`
}

func decodeBody(t *testing.T, body []byte, dst any) {
	t.Helper()

	if err := json.Unmarshal(body, dst); err != nil {
		t.Fatalf("decode response: %v (body %q)", err, body)
	}
}

func requireAPIError(t *testing.T, result httpResult, wantStatus int, wantCode string) apiError {
	t.Helper()

	if result.status != wantStatus {
		t.Fatalf("status = %d, want %d (body %q)", result.status, wantStatus, result.body)
	}

	var payload apiError
	decodeBody(t, result.body, &payload)
	if payload.Error.Code != wantCode {
		t.Errorf("error code = %q, want %q", payload.Error.Code, wantCode)
	}

	return payload
}

func TestEventsListPagination(t *testing.T) {
	env := newEventsEnv(t)
	resetEventTables(t, env.pool)

	venueID := insertTestVenue(t, env.pool, "Main Hall")
	base := time.Date(2026, time.November, 12, 18, 0, 0, 0, time.UTC)

	wantIDs := make([]int64, 0, 5)
	for i := range 5 {
		wantIDs = append(wantIDs, insertTestEvent(t, env.pool, venueID,
			fmt.Sprintf("event-%d", i), base.Add(time.Duration(i)*time.Hour)))
	}

	var gotIDs []int64
	seen := make(map[int64]bool)
	cursor := ""
	pages := 0

	for {
		pages++
		if pages > 10 {
			t.Fatal("pagination did not terminate")
		}

		path := "/api/v1/events?limit=2"
		if cursor != "" {
			path += "&cursor=" + url.QueryEscape(cursor)
		}

		result := env.get(t, path)
		if result.status != http.StatusOK {
			t.Fatalf("page %d status = %d, want 200 (body %q)", pages, result.status, result.body)
		}

		var body eventsListBody
		decodeBody(t, result.body, &body)
		if len(body.Items) > 2 {
			t.Fatalf("page %d items = %d, want at most 2", pages, len(body.Items))
		}

		for _, item := range body.Items {
			if seen[item.ID] {
				t.Errorf("page %d duplicates event %d", pages, item.ID)
			}
			seen[item.ID] = true
			gotIDs = append(gotIDs, item.ID)
		}

		if body.NextCursor == "" {
			break
		}
		cursor = body.NextCursor
	}

	if pages != 3 {
		t.Errorf("pages = %d, want 3", pages)
	}
	if len(gotIDs) != len(wantIDs) {
		t.Fatalf("collected ids = %v, want %v", gotIDs, wantIDs)
	}
	for i, id := range wantIDs {
		if gotIDs[i] != id {
			t.Errorf("ids[%d] = %d, want %d", i, gotIDs[i], id)
		}
	}
}

func TestEventsListEmpty(t *testing.T) {
	env := newEventsEnv(t)
	resetEventTables(t, env.pool)

	result := env.get(t, "/api/v1/events")
	if result.status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", result.status, result.body)
	}
	if !strings.Contains(string(result.body), `"items":[]`) {
		t.Errorf("body = %q, want empty items array", result.body)
	}
	if strings.Contains(string(result.body), "next_cursor") {
		t.Errorf("body = %q, want no next_cursor on the only page", result.body)
	}
}

func TestEventsListErrors(t *testing.T) {
	env := newEventsEnv(t)
	resetEventTables(t, env.pool)

	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{name: "zero limit", path: "/api/v1/events?limit=0", wantErr: "invalid_limit"},
		{name: "limit above max", path: "/api/v1/events?limit=101", wantErr: "invalid_limit"},
		{name: "non-numeric limit", path: "/api/v1/events?limit=abc", wantErr: "invalid_limit"},
		// "AAAA" — валидный base64 без ':'.
		{name: "broken cursor", path: "/api/v1/events?cursor=AAAA", wantErr: "invalid_cursor"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := env.get(t, tt.path)

			requireAPIError(t, result, http.StatusBadRequest, tt.wantErr)
		})
	}
}

func TestEventsDetailAndSeats(t *testing.T) {
	env := newEventsEnv(t)
	resetEventTables(t, env.pool)

	venueID := insertTestVenue(t, env.pool, "Main Hall")
	startsAt := time.Date(2026, time.November, 12, 18, 0, 0, 0, time.UTC)
	eventID := insertTestEvent(t, env.pool, venueID, "Concert", startsAt)

	statuses := []domain.EventSeatStatus{
		domain.EventSeatAvailable,
		domain.EventSeatHeld,
		domain.EventSeatSold,
	}
	for i, status := range statuses {
		seatID := insertTestSeat(t, env.pool, venueID, "A", "1", strconv.Itoa(i+1))
		insertTestEventSeat(t, env.pool, eventID, seatID, venueID, int64(1000+i), status)
	}

	t.Run("detail", func(t *testing.T) {
		result := env.get(t, fmt.Sprintf("/api/v1/events/%d", eventID))
		if result.status != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %q)", result.status, result.body)
		}

		var body eventDetailBody
		decodeBody(t, result.body, &body)
		if body.ID != eventID || body.Title != "Concert" || body.Venue != "Main Hall" || body.City != "Moscow" {
			t.Errorf("detail = %+v, want event fields", body)
		}
		if body.SeatsTotal != 3 || body.SeatsAvailable != 1 {
			t.Errorf("seats = %d/%d, want available/total 1/3", body.SeatsAvailable, body.SeatsTotal)
		}
		if body.StartsAt != "2026-11-12T18:00:00Z" {
			t.Errorf("starts_at = %q, want UTC RFC3339", body.StartsAt)
		}
	})

	t.Run("seats", func(t *testing.T) {
		result := env.get(t, fmt.Sprintf("/api/v1/events/%d/seats", eventID))
		if result.status != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %q)", result.status, result.body)
		}

		var body eventSeatsBody
		decodeBody(t, result.body, &body)
		if len(body.Items) != 3 {
			t.Fatalf("items = %d, want 3", len(body.Items))
		}
		for i, want := range []string{"available", "held", "sold"} {
			if body.Items[i].Status != want {
				t.Errorf("item %d status = %q, want %q", i, body.Items[i].Status, want)
			}
			if body.Items[i].Section != "A" || body.Items[i].Tier != "standard" || body.Items[i].PriceCents != int64(1000+i) {
				t.Errorf("item %d = %+v, want fields from database", i, body.Items[i])
			}
		}
	})

	t.Run("missing event", func(t *testing.T) {
		for _, path := range []string{"/api/v1/events/999999", "/api/v1/events/999999/seats"} {
			requireAPIError(t, env.get(t, path), http.StatusNotFound, "not_found")
		}
	})

	t.Run("invalid id", func(t *testing.T) {
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
				payload := requireAPIError(t, env.get(t, tt.path), http.StatusBadRequest, "validation_failed")
				if got := payload.Error.Fields["id"]; got != tt.wantMessage {
					t.Errorf("fields[id] = %q, want %q", got, tt.wantMessage)
				}
			})
		}
	})
}

func TestEventsMethodNotAllowed(t *testing.T) {
	env := newEventsEnv(t)
	resetEventTables(t, env.pool)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, env.server.URL+"/api/v1/events", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
	if allow := resp.Header.Get("Allow"); !strings.Contains(allow, http.MethodGet) {
		t.Errorf("Allow = %q, want to contain GET", allow)
	}

	var payload apiError
	decodeBody(t, body, &payload)
	if payload.Error.Code != "method_not_allowed" {
		t.Errorf("error code = %q, want method_not_allowed", payload.Error.Code)
	}
}

// Закрытый pool даёт реальную ошибку pgx без DDL и без риска оставить схему
// изменённой при аварийном завершении теста.
func TestEventsInternalErrorHasNoInternalDetails(t *testing.T) {
	dbCfg, err := testdb.ConfigFromEnv()
	if err != nil {
		t.Fatalf("test database config: %v", err)
	}

	pool, err := pgxpool.New(context.Background(), dbCfg.DSN(dbCfg.Database))
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	pool.Close()

	router, err := app.NewRouter(testAppConfig(t), pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("app.NewRouter() error = %v", err)
	}

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	client := server.Client()
	client.Timeout = 10 * time.Second

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/api/v1/events", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (body %q)", resp.StatusCode, body)
	}

	var payload apiError
	decodeBody(t, body, &payload)
	if payload.Error.Code != "internal_error" {
		t.Errorf("error code = %q, want internal_error", payload.Error.Code)
	}
	if payload.Error.Message != "internal server error" {
		t.Errorf("message = %q, want generic message", payload.Error.Message)
	}

	lower := strings.ToLower(string(body))
	for _, leak := range []string{"closed", "pool", "pgx", "sqlstate"} {
		if strings.Contains(lower, leak) {
			t.Errorf("response leaks %q: %s", leak, body)
		}
	}
}

func TestEventsStartsAtIsUTC(t *testing.T) {
	env := newEventsEnv(t)
	resetEventTables(t, env.pool)

	venueID := insertTestVenue(t, env.pool, "Main Hall")
	// Вход в не-UTC зоне: если забыть .UTC(), ответ будет с +03:00.
	startsAt := time.Date(2026, time.November, 12, 18, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	eventID := insertTestEvent(t, env.pool, venueID, "Concert", startsAt)

	list := env.get(t, "/api/v1/events")
	if list.status != http.StatusOK {
		t.Fatalf("list status = %d, want 200 (body %q)", list.status, list.body)
	}
	var listBody eventsListBody
	decodeBody(t, list.body, &listBody)
	if len(listBody.Items) != 1 {
		t.Fatalf("list items = %d, want 1", len(listBody.Items))
	}

	detail := env.get(t, fmt.Sprintf("/api/v1/events/%d", eventID))
	if detail.status != http.StatusOK {
		t.Fatalf("detail status = %d, want 200 (body %q)", detail.status, detail.body)
	}
	var detailBody eventDetailBody
	decodeBody(t, detail.body, &detailBody)

	if listBody.Items[0].StartsAt != detailBody.StartsAt {
		t.Errorf("list starts_at = %q, detail = %q, want equal",
			listBody.Items[0].StartsAt, detailBody.StartsAt)
	}
	if !strings.HasSuffix(listBody.Items[0].StartsAt, "Z") {
		t.Errorf("starts_at = %q, want UTC with Z", listBody.Items[0].StartsAt)
	}

	parsed, err := time.Parse(time.RFC3339, listBody.Items[0].StartsAt)
	if err != nil {
		t.Fatalf("parse starts_at: %v", err)
	}
	if !parsed.Equal(startsAt) {
		t.Errorf("starts_at = %v, want instant %v", parsed, startsAt)
	}
}
