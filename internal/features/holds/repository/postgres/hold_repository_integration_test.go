//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egor-kardash/ticket-booking-service/internal/features/holds/domain"
	holdsrepo "github.com/egor-kardash/ticket-booking-service/internal/features/holds/repository/postgres"
)

func resetHolds(t *testing.T) {
	t.Helper()

	truncate := func() {
		// Ограниченный контекст: зависшая БД не должна вешать cleanup.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if _, err := testPool.Exec(ctx, `
			TRUNCATE booking.holds, booking.event_seats, booking.seats,
			         booking.events, booking.venues, booking.users
			RESTART IDENTITY CASCADE`); err != nil {
			t.Errorf("truncate holds: %v", err)
		}
	}

	truncate()
	t.Cleanup(truncate)
}

var userSeq int

func createUser(t *testing.T) int64 {
	t.Helper()

	userSeq++
	email := fmt.Sprintf("hold-%d@example.com", userSeq)

	var id int64
	if err := testPool.QueryRow(context.Background(),
		`INSERT INTO booking.users (email, password_hash) VALUES ($1, 'hash') RETURNING id`, email).
		Scan(&id); err != nil {
		t.Fatalf("create user %q: %v", email, err)
	}

	return id
}

// createEventSeat создаёт минимальную цепочку venue → event → seat → event_seat.
func createEventSeat(t *testing.T) int64 {
	t.Helper()

	ctx := context.Background()

	var venueID, eventID, seatID, eventSeatID int64
	if err := testPool.QueryRow(ctx,
		`INSERT INTO booking.venues (name, city) VALUES ('Hall', 'Moscow') RETURNING id`).
		Scan(&venueID); err != nil {
		t.Fatalf("create venue: %v", err)
	}
	if err := testPool.QueryRow(ctx,
		`INSERT INTO booking.events (venue_id, title, starts_at) VALUES ($1, 'Concert', now()) RETURNING id`,
		venueID).Scan(&eventID); err != nil {
		t.Fatalf("create event: %v", err)
	}
	if err := testPool.QueryRow(ctx,
		`INSERT INTO booking.seats (venue_id, section, row_label, seat_number) VALUES ($1, 'A', '1', '1') RETURNING id`,
		venueID).Scan(&seatID); err != nil {
		t.Fatalf("create seat: %v", err)
	}
	if err := testPool.QueryRow(ctx,
		`INSERT INTO booking.event_seats (event_id, seat_id, venue_id, tier, price_cents)
		 VALUES ($1, $2, $3, 'standard', 500000) RETURNING id`,
		eventID, seatID, venueID).Scan(&eventSeatID); err != nil {
		t.Fatalf("create event seat: %v", err)
	}

	return eventSeatID
}

func holdStatus(t *testing.T, holdID int64) string {
	t.Helper()

	var status string
	if err := testPool.QueryRow(context.Background(),
		"SELECT status FROM booking.holds WHERE id = $1", holdID).Scan(&status); err != nil {
		t.Fatalf("read hold %d status: %v", holdID, err)
	}

	return status
}

func holdUserID(t *testing.T, holdID int64) int64 {
	t.Helper()

	var userID int64
	if err := testPool.QueryRow(context.Background(),
		"SELECT user_id FROM booking.holds WHERE id = $1", holdID).Scan(&userID); err != nil {
		t.Fatalf("read hold %d user_id: %v", holdID, err)
	}

	return userID
}

func seatStatus(t *testing.T, eventSeatID int64) string {
	t.Helper()

	var status string
	if err := testPool.QueryRow(context.Background(),
		"SELECT status FROM booking.event_seats WHERE id = $1", eventSeatID).Scan(&status); err != nil {
		t.Fatalf("read event seat %d status: %v", eventSeatID, err)
	}

	return status
}

func countHolds(t *testing.T, eventSeatID int64, status string) int {
	t.Helper()

	var count int
	if err := testPool.QueryRow(context.Background(),
		"SELECT count(*) FROM booking.holds WHERE event_seat_id = $1 AND status = $2",
		eventSeatID, status).Scan(&count); err != nil {
		t.Fatalf("count holds: %v", err)
	}

	return count
}

// expireHold сдвигает hold в прошлое. created_at тоже нужно двигать:
// иначе CHECK (expires_at > created_at) не даст выставить expires_at в прошлое.
func expireHold(t *testing.T, holdID int64) {
	t.Helper()

	if _, err := testPool.Exec(context.Background(), `
		UPDATE booking.holds
		SET created_at = now() - interval '1 hour',
		    expires_at = now() - interval '1 second'
		WHERE id = $1`, holdID); err != nil {
		t.Fatalf("expire hold %d: %v", holdID, err)
	}
}

func requireActiveSeatIndex(t *testing.T) {
	t.Helper()

	var definition string
	if err := testPool.QueryRow(context.Background(), `
		SELECT indexdef
		FROM pg_indexes
		WHERE schemaname = 'booking' AND tablename = 'holds' AND indexname = 'holds_active_seat_idx'`).
		Scan(&definition); err != nil {
		t.Fatalf("read holds_active_seat_idx: %v", err)
	}
	if !strings.Contains(definition, "CREATE UNIQUE INDEX") || !strings.Contains(definition, "status = 'active'") {
		t.Errorf("index definition = %q, want unique partial index on active holds", definition)
	}
}

func setSeatStatus(t *testing.T, eventSeatID int64, status string) {
	t.Helper()

	if _, err := testPool.Exec(context.Background(),
		"UPDATE booking.event_seats SET status = $1 WHERE id = $2", status, eventSeatID); err != nil {
		t.Fatalf("set event seat %d status %q: %v", eventSeatID, status, err)
	}
}

func totalHolds(t *testing.T) int {
	t.Helper()

	var count int
	if err := testPool.QueryRow(context.Background(), "SELECT count(*) FROM booking.holds").Scan(&count); err != nil {
		t.Fatalf("count all holds: %v", err)
	}

	return count
}

func TestGetAndCancelHold(t *testing.T) {
	resetHolds(t)

	repo := holdsrepo.New(testPool)
	ownerID := createUser(t)
	foreignID := createUser(t)

	hold, err := repo.Create(t.Context(), ownerID, createEventSeat(t), 10*time.Minute)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	t.Run("foreign user cannot read hold", func(t *testing.T) {
		got, err := repo.GetByID(t.Context(), hold.ID, foreignID)

		if got != nil {
			t.Errorf("GetByID() = %+v, want nil", got)
		}
		if !errors.Is(err, domain.ErrHoldNotFound) {
			t.Fatalf("GetByID() error = %v, want ErrHoldNotFound", err)
		}
	})

	t.Run("foreign user cannot cancel hold", func(t *testing.T) {
		err := repo.Cancel(t.Context(), hold.ID, foreignID)
		if !errors.Is(err, domain.ErrHoldNotFound) {
			t.Fatalf("Cancel() error = %v, want ErrHoldNotFound", err)
		}

		// Чужой cancel не должен менять состояние.
		if got := holdStatus(t, hold.ID); got != "active" {
			t.Errorf("hold status = %q, want active after foreign cancel", got)
		}
	})

	t.Run("expired hold is not found", func(t *testing.T) {
		expired, err := repo.Create(t.Context(), ownerID, createEventSeat(t), 10*time.Minute)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		expireHold(t, expired.ID)

		if _, err := repo.GetByID(t.Context(), expired.ID, ownerID); !errors.Is(err, domain.ErrHoldNotFound) {
			t.Errorf("GetByID() error = %v, want ErrHoldNotFound", err)
		}
		if err := repo.Cancel(t.Context(), expired.ID, ownerID); !errors.Is(err, domain.ErrHoldNotFound) {
			t.Errorf("Cancel() error = %v, want ErrHoldNotFound", err)
		}
	})

	t.Run("owner cancels active hold once", func(t *testing.T) {
		if err := repo.Cancel(t.Context(), hold.ID, ownerID); err != nil {
			t.Fatalf("Cancel() error = %v, want nil", err)
		}
		if got := holdStatus(t, hold.ID); got != "canceled" {
			t.Errorf("hold status = %q, want canceled", got)
		}

		seatID := hold.EventSeatID
		if got := seatStatus(t, seatID); got != "available" {
			t.Errorf("seat status = %q, want available", got)
		}

		if err := repo.Cancel(t.Context(), hold.ID, ownerID); !errors.Is(err, domain.ErrHoldNotFound) {
			t.Errorf("second Cancel() error = %v, want ErrHoldNotFound", err)
		}
	})
}

func TestCreateHoldUnavailableSeat(t *testing.T) {
	resetHolds(t)

	repo := holdsrepo.New(testPool)
	userID := createUser(t)

	t.Run("sold seat", func(t *testing.T) {
		eventSeatID := createEventSeat(t)
		setSeatStatus(t, eventSeatID, "sold")

		hold, err := repo.Create(t.Context(), userID, eventSeatID, 10*time.Minute)

		if hold != nil {
			t.Errorf("Create() = %+v, want nil", hold)
		}
		if !errors.Is(err, domain.ErrSeatTaken) {
			t.Fatalf("Create() error = %v, want ErrSeatTaken", err)
		}
		if got := totalHolds(t); got != 0 {
			t.Errorf("holds count = %d, want 0", got)
		}
		if got := seatStatus(t, eventSeatID); got != "sold" {
			t.Errorf("seat status = %q, want sold", got)
		}
	})

	t.Run("nonexistent seat", func(t *testing.T) {
		hold, err := repo.Create(t.Context(), userID, 999999, 10*time.Minute)

		if hold != nil {
			t.Errorf("Create() = %+v, want nil", hold)
		}
		if !errors.Is(err, domain.ErrEventSeatNotFound) {
			t.Fatalf("Create() error = %v, want ErrEventSeatNotFound", err)
		}
	})

	t.Run("zero seat id", func(t *testing.T) {
		// Нулевой id не должен паниковать: это обычный not found.
		hold, err := repo.Create(t.Context(), userID, 0, 10*time.Minute)

		if hold != nil {
			t.Errorf("Create() = %+v, want nil", hold)
		}
		if !errors.Is(err, domain.ErrEventSeatNotFound) {
			t.Fatalf("Create() error = %v, want ErrEventSeatNotFound", err)
		}
	})
}

// TestSoldSeatStaysSold защищает guard status = 'held' в freeEventSeat:
// проданное место не должно возвращаться в available ни отменой, ни воркером.
func TestSoldSeatStaysSold(t *testing.T) {
	resetHolds(t)

	repo := holdsrepo.New(testPool)
	userID := createUser(t)

	t.Run("cancel", func(t *testing.T) {
		eventSeatID := createEventSeat(t)
		hold, err := repo.Create(t.Context(), userID, eventSeatID, 10*time.Minute)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		setSeatStatus(t, eventSeatID, "sold")

		if err := repo.Cancel(t.Context(), hold.ID, userID); err != nil {
			t.Fatalf("Cancel() error = %v, want nil", err)
		}
		if got := holdStatus(t, hold.ID); got != "canceled" {
			t.Errorf("hold status = %q, want canceled", got)
		}
		if got := seatStatus(t, eventSeatID); got != "sold" {
			t.Errorf("seat status = %q, want sold after cancel", got)
		}
	})

	t.Run("expire", func(t *testing.T) {
		eventSeatID := createEventSeat(t)
		hold, err := repo.Create(t.Context(), userID, eventSeatID, 10*time.Minute)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		setSeatStatus(t, eventSeatID, "sold")
		expireHold(t, hold.ID)

		expired, err := repo.ExpireBatch(t.Context(), 10)
		if err != nil {
			t.Fatalf("ExpireBatch() error = %v, want nil", err)
		}
		if expired != 1 {
			t.Errorf("expired = %d, want 1", expired)
		}
		if got := holdStatus(t, hold.ID); got != "expired" {
			t.Errorf("hold status = %q, want expired", got)
		}
		if got := seatStatus(t, eventSeatID); got != "sold" {
			t.Errorf("seat status = %q, want sold after expire", got)
		}
	})
}

func TestCreateHoldRace(t *testing.T) {
	resetHolds(t)
	requireActiveSeatIndex(t)

	repo := holdsrepo.New(testPool)
	userID := createUser(t)
	eventSeatID := createEventSeat(t)

	const workers = 100

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	results := make([]error, workers)
	start := make(chan struct{})

	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			<-start
			_, results[i] = repo.Create(ctx, userID, eventSeatID, 10*time.Minute)
		}(i)
	}

	close(start)
	wg.Wait()

	var created, taken int
	for i, err := range results {
		switch {
		case err == nil:
			created++
		case errors.Is(err, domain.ErrSeatTaken):
			taken++
		default:
			t.Errorf("worker %d: unexpected error %v", i, err)
		}
	}

	if created != 1 {
		t.Errorf("successful holds = %d, want exactly 1", created)
	}
	if taken != workers-1 {
		t.Errorf("ErrSeatTaken = %d, want %d", taken, workers-1)
	}
	if got := countHolds(t, eventSeatID, "active"); got != 1 {
		t.Errorf("active holds = %d, want 1", got)
	}
	if got := seatStatus(t, eventSeatID); got != "held" {
		t.Errorf("seat status = %q, want held", got)
	}
}

func TestExpireHolds(t *testing.T) {
	resetHolds(t)

	repo := holdsrepo.New(testPool)
	userID := createUser(t)
	eventSeatID := createEventSeat(t)

	hold, err := repo.Create(t.Context(), userID, eventSeatID, 10*time.Minute)
	if err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	if got := seatStatus(t, eventSeatID); got != "held" {
		t.Fatalf("seat status = %q, want held after create", got)
	}

	expireHold(t, hold.ID)

	expired, err := repo.ExpireBatch(t.Context(), 10)
	if err != nil {
		t.Fatalf("ExpireBatch() error = %v, want nil", err)
	}
	if expired != 1 {
		t.Errorf("expired = %d, want 1", expired)
	}
	if got := holdStatus(t, hold.ID); got != "expired" {
		t.Errorf("hold status = %q, want expired", got)
	}
	if got := seatStatus(t, eventSeatID); got != "available" {
		t.Errorf("seat status = %q, want available", got)
	}

	expired, err = repo.ExpireBatch(t.Context(), 10)
	if err != nil {
		t.Fatalf("second ExpireBatch() error = %v, want nil", err)
	}
	if expired != 0 {
		t.Errorf("second expired = %d, want 0", expired)
	}
	if got := holdStatus(t, hold.ID); got != "expired" {
		t.Errorf("hold status after second run = %q, want expired", got)
	}
	if got := seatStatus(t, eventSeatID); got != "available" {
		t.Errorf("seat status after second run = %q, want available", got)
	}
}

func TestCreateHoldLazyExpiry(t *testing.T) {
	resetHolds(t)

	repo := holdsrepo.New(testPool)
	oldOwnerID := createUser(t)
	newOwnerID := createUser(t)
	eventSeatID := createEventSeat(t)

	oldHold, err := repo.Create(t.Context(), oldOwnerID, eventSeatID, 10*time.Minute)
	if err != nil {
		t.Fatalf("first Create() error = %v, want nil", err)
	}
	expireHold(t, oldHold.ID)

	newHold, err := repo.Create(t.Context(), newOwnerID, eventSeatID, 10*time.Minute)
	if err != nil {
		t.Fatalf("second Create() error = %v, want nil", err)
	}
	if newHold.ID == oldHold.ID {
		t.Fatalf("second Create() returned the same hold %d", oldHold.ID)
	}

	if got := holdStatus(t, oldHold.ID); got != "expired" {
		t.Errorf("old hold status = %q, want expired", got)
	}
	if got := holdUserID(t, oldHold.ID); got != oldOwnerID {
		t.Errorf("old hold user = %d, want %d", got, oldOwnerID)
	}
	if got := holdUserID(t, newHold.ID); got != newOwnerID {
		t.Errorf("new hold user = %d, want %d", got, newOwnerID)
	}
	if got := holdStatus(t, newHold.ID); got != "active" {
		t.Errorf("new hold status = %q, want active", got)
	}
	if got := countHolds(t, eventSeatID, "active"); got != 1 {
		t.Errorf("active holds = %d, want 1", got)
	}
	if got := seatStatus(t, eventSeatID); got != "held" {
		t.Errorf("seat status = %q, want held", got)
	}
}
