//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/egor-kardash/ticket-booking-service/internal/features/events/domain"
	eventspostgres "github.com/egor-kardash/ticket-booking-service/internal/features/events/repository/postgres"
	eventsservice "github.com/egor-kardash/ticket-booking-service/internal/features/events/service"
)

func insertSeatRow(t *testing.T, venueID int64, section, rowLabel, seatNumber string) int64 {
	t.Helper()

	var id int64
	err := testPool.QueryRow(context.Background(),
		`INSERT INTO booking.seats (venue_id, section, row_label, seat_number) VALUES ($1, $2, $3, $4) RETURNING id`,
		venueID, section, rowLabel, seatNumber).Scan(&id)
	if err != nil {
		t.Fatalf("insert seat %s/%s/%s: %v", section, rowLabel, seatNumber, err)
	}

	return id
}

func insertEventSeatRow(
	t *testing.T,
	eventID, seatID, venueID int64,
	tier string,
	priceCents int64,
	status domain.EventSeatStatus,
) int64 {
	t.Helper()

	var id int64
	err := testPool.QueryRow(context.Background(),
		`INSERT INTO booking.event_seats (event_id, seat_id, venue_id, tier, price_cents, status)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		eventID, seatID, venueID, tier, priceCents, string(status)).Scan(&id)
	if err != nil {
		t.Fatalf("insert event seat (event=%d seat=%d): %v", eventID, seatID, err)
	}

	return id
}

func TestGetByIDEmptyEvent(t *testing.T) {
	resetEvents(t)

	repo := eventspostgres.New(testPool)
	venueID := insertVenue(t, "Main Hall")
	eventID := insertEvent(t, venueID, "Empty", testStart(t))

	details, err := repo.GetByID(t.Context(), eventID)
	if err != nil {
		t.Fatalf("GetByID() error = %v, want nil", err)
	}
	if details.ID != eventID || details.Title != "Empty" || details.VenueName != "Main Hall" {
		t.Errorf("details = %+v, want fields from database", details)
	}
	if details.SeatsTotal != 0 || details.SeatsAvailable != 0 {
		t.Errorf("seats = %d/%d, want 0/0", details.SeatsAvailable, details.SeatsTotal)
	}
}

func TestGetByIDSeatCounts(t *testing.T) {
	resetEvents(t)

	repo := eventspostgres.New(testPool)
	venueID := insertVenue(t, "Main Hall")
	eventID := insertEvent(t, venueID, "Concert", testStart(t))

	statuses := make([]domain.EventSeatStatus, 0, 100)
	for range 60 {
		statuses = append(statuses, domain.EventSeatAvailable)
	}
	for range 25 {
		statuses = append(statuses, domain.EventSeatHeld)
	}
	for range 15 {
		statuses = append(statuses, domain.EventSeatSold)
	}

	for i, status := range statuses {
		seatID := insertSeatRow(t, venueID, "A", "1", strconv.Itoa(i+1))
		insertEventSeatRow(t, eventID, seatID, venueID, "standard", 1000, status)
	}

	details, err := repo.GetByID(t.Context(), eventID)
	if err != nil {
		t.Fatalf("GetByID() error = %v, want nil", err)
	}
	if details.SeatsTotal != 100 {
		t.Errorf("seats_total = %d, want 100", details.SeatsTotal)
	}
	if details.SeatsAvailable != 60 {
		t.Errorf("seats_available = %d, want 60", details.SeatsAvailable)
	}
}

func TestGetByIDNotFound(t *testing.T) {
	resetEvents(t)

	repo := eventspostgres.New(testPool)

	details, err := repo.GetByID(t.Context(), 999999)
	if details != nil {
		t.Errorf("GetByID() = %+v, want nil", details)
	}
	if !errors.Is(err, domain.ErrEventNotFound) {
		t.Fatalf("GetByID() error = %v, want errors.Is(ErrEventNotFound)", err)
	}
	// pgx.ErrNoRows не должен просачиваться наружу.
	if errors.Is(err, pgx.ErrNoRows) {
		t.Error("GetByID() leaks pgx.ErrNoRows")
	}
}

func TestListSeatsFieldsAndStatuses(t *testing.T) {
	resetEvents(t)

	repo := eventspostgres.New(testPool)
	venueID := insertVenue(t, "Main Hall")
	eventID := insertEvent(t, venueID, "Concert", testStart(t))

	wantStatuses := []domain.EventSeatStatus{
		domain.EventSeatAvailable,
		domain.EventSeatHeld,
		domain.EventSeatSold,
	}

	for i := range 100 {
		seatID := insertSeatRow(t, venueID, "A", "1", strconv.Itoa(i+1))
		insertEventSeatRow(t, eventID, seatID, venueID, "standard", int64(1000+i), wantStatuses[i%len(wantStatuses)])
	}

	seats, err := repo.ListSeats(t.Context(), eventID)
	if err != nil {
		t.Fatalf("ListSeats() error = %v, want nil", err)
	}
	if len(seats) != 100 {
		t.Fatalf("seats = %d, want 100", len(seats))
	}

	// Числовой порядок seat_number: 1..100, а не лексикографический.
	for i, seat := range seats {
		wantNumber := strconv.Itoa(i + 1)
		if seat.SeatNumber != wantNumber {
			t.Fatalf("seat[%d].number = %q, want %q", i, seat.SeatNumber, wantNumber)
		}
		if seat.Section != "A" || seat.RowLabel != "1" || seat.Tier != "standard" {
			t.Errorf("seat %q fields = %+v, want A/1/standard", wantNumber, seat)
		}
		if seat.PriceCents != int64(1000+i) {
			t.Errorf("seat %q price = %d, want %d", wantNumber, seat.PriceCents, 1000+i)
		}
		if seat.Status != wantStatuses[i%len(wantStatuses)] {
			t.Errorf("seat %q status = %q, want %q", wantNumber, seat.Status, wantStatuses[i%len(wantStatuses)])
		}
	}
}

func TestListSeatsOrdering(t *testing.T) {
	resetEvents(t)

	repo := eventspostgres.New(testPool)
	venueID := insertVenue(t, "Main Hall")
	eventID := insertEvent(t, venueID, "Concert", testStart(t))

	// section/row — лексикографически; seat_number — численно, нечисловые после.
	type seatFixture struct {
		section  string
		rowLabel string
		number   string
	}
	fixtures := []seatFixture{
		{section: "A", rowLabel: "1", number: "10"},
		{section: "A", rowLabel: "1", number: "2"},
		{section: "A", rowLabel: "1", number: "1"},
		{section: "A", rowLabel: "1", number: "A1"},
		{section: "A", rowLabel: "2", number: "1"},
		{section: "B", rowLabel: "1", number: "1"},
	}
	for _, fixture := range fixtures {
		seatID := insertSeatRow(t, venueID, fixture.section, fixture.rowLabel, fixture.number)
		insertEventSeatRow(t, eventID, seatID, venueID, "standard", 1000, domain.EventSeatAvailable)
	}

	seats, err := repo.ListSeats(t.Context(), eventID)
	if err != nil {
		t.Fatalf("ListSeats() error = %v, want nil", err)
	}

	want := []string{"A/1/1", "A/1/2", "A/1/10", "A/1/A1", "A/2/1", "B/1/1"}
	if len(seats) != len(want) {
		t.Fatalf("seats = %d, want %d", len(seats), len(want))
	}
	for i, seat := range seats {
		got := seat.Section + "/" + seat.RowLabel + "/" + seat.SeatNumber
		if got != want[i] {
			t.Errorf("seat[%d] = %q, want %q", i, got, want[i])
		}
	}
}

func TestListSeatsEmptyEvent(t *testing.T) {
	resetEvents(t)

	repo := eventspostgres.New(testPool)
	venueID := insertVenue(t, "Main Hall")
	eventID := insertEvent(t, venueID, "Empty", testStart(t))

	seats, err := repo.ListSeats(t.Context(), eventID)
	if err != nil {
		t.Fatalf("ListSeats() error = %v, want nil", err)
	}
	if len(seats) != 0 {
		t.Errorf("seats = %+v, want empty list", seats)
	}
}

// TestListSeatsMissingEvent фиксирует разделение ответственности:
// репозиторий возвращает пустой список, а 404 появляется в сервисе через Exists.
func TestListSeatsMissingEvent(t *testing.T) {
	resetEvents(t)

	repo := eventspostgres.New(testPool)

	seats, err := repo.ListSeats(t.Context(), 999999)
	if err != nil {
		t.Fatalf("repository ListSeats() error = %v, want nil", err)
	}
	if len(seats) != 0 {
		t.Errorf("repository seats = %+v, want empty list", seats)
	}

	svc := eventsservice.New(repo)

	_, err = svc.ListSeats(t.Context(), 999999)
	if !errors.Is(err, domain.ErrEventNotFound) {
		t.Fatalf("service ListSeats() error = %v, want errors.Is(ErrEventNotFound)", err)
	}
}

func testStart(t *testing.T) time.Time {
	t.Helper()

	return time.Date(2026, time.November, 12, 18, 0, 0, 0, time.UTC)
}
