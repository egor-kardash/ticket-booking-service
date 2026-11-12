//go:build integration

package postgres_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/egor-kardash/ticket-booking-service/internal/features/events/domain"
	eventspostgres "github.com/egor-kardash/ticket-booking-service/internal/features/events/repository/postgres"
)

func resetEvents(t *testing.T) {
	t.Helper()

	truncate := func() {
		if _, err := testPool.Exec(context.Background(),
			"TRUNCATE booking.event_seats, booking.seats, booking.events, booking.venues RESTART IDENTITY CASCADE"); err != nil {
			t.Errorf("truncate events: %v", err)
		}
	}

	truncate()
	t.Cleanup(truncate)
}

func insertVenue(t *testing.T, name string) int64 {
	t.Helper()

	var id int64
	if err := testPool.QueryRow(context.Background(),
		`INSERT INTO booking.venues (name, city) VALUES ($1, 'Moscow') RETURNING id`, name).Scan(&id); err != nil {
		t.Fatalf("insert venue %q: %v", name, err)
	}

	return id
}

func insertEvent(t *testing.T, venueID int64, title string, startsAt time.Time) int64 {
	t.Helper()

	var id int64
	if err := testPool.QueryRow(context.Background(),
		`INSERT INTO booking.events (venue_id, title, starts_at) VALUES ($1, $2, $3) RETURNING id`,
		venueID, title, startsAt).Scan(&id); err != nil {
		t.Fatalf("insert event %q: %v", title, err)
	}

	return id
}

func eventIDs(items []domain.EventListItem) []int64 {
	ids := make([]int64, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}

	return ids
}

// collectPages проходит все страницы и проверяет, что next_cursor указывает
// ровно на последний элемент страницы, дублей и пропусков нет.
func collectPages(t *testing.T, repo *eventspostgres.EventRepository, limit int, wantIDs []int64) {
	t.Helper()

	var got []int64
	var cursor *domain.EventCursor

	for page := 1; ; page++ {
		if page > 100 {
			t.Fatal("pagination did not terminate")
		}

		items, next, err := repo.List(t.Context(), limit, cursor)
		if err != nil {
			t.Fatalf("page %d: List() error = %v", page, err)
		}
		if len(items) == 0 && next != nil {
			t.Fatalf("page %d: next cursor without items", page)
		}
		if len(items) > limit {
			t.Fatalf("page %d: items = %d, want at most %d", page, len(items), limit)
		}

		for _, item := range items {
			got = append(got, item.ID)
		}

		if next == nil {
			break
		}

		last := items[len(items)-1]
		if next.ID != last.ID || !next.StartsAt.Equal(last.StartsAt) {
			t.Fatalf("page %d: next = {%v, %d}, want last item {%v, %d}",
				page, next.StartsAt, next.ID, last.StartsAt, last.ID)
		}

		cursor = next
	}

	if !reflect.DeepEqual(got, wantIDs) {
		t.Errorf("paged ids = %v, want %v", got, wantIDs)
	}
}

func TestListEventsEmpty(t *testing.T) {
	resetEvents(t)

	repo := eventspostgres.New(testPool)

	items, next, err := repo.List(t.Context(), 10, nil)
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(items) != 0 {
		t.Errorf("items = %+v, want empty list", items)
	}
	if next != nil {
		t.Errorf("next = %+v, want nil for empty table", next)
	}
}

func TestListEventsOrdering(t *testing.T) {
	resetEvents(t)

	repo := eventspostgres.New(testPool)
	venueID := insertVenue(t, "Main Hall")
	base := time.Date(2026, time.November, 12, 18, 0, 0, 0, time.UTC)

	first := insertEvent(t, venueID, "first", base.Add(time.Hour))
	tieB := insertEvent(t, venueID, "B", base.Add(2*time.Hour))
	tieA := insertEvent(t, venueID, "A", base.Add(2*time.Hour))
	last := insertEvent(t, venueID, "last", base.Add(3*time.Hour))

	items, next, err := repo.List(t.Context(), 100, nil)
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if next != nil {
		t.Errorf("next = %+v, want nil when all items fit", next)
	}

	// starts_at ASC, при равенстве — id ASC, а не порядок вставки по title.
	want := []int64{first, tieB, tieA, last}
	if got := eventIDs(items); !reflect.DeepEqual(got, want) {
		t.Errorf("ids = %v, want %v", got, want)
	}
	if items[0].VenueName != "Main Hall" || items[0].City != "Moscow" {
		t.Errorf("venue fields = %q/%q, want Main Hall/Moscow", items[0].VenueName, items[0].City)
	}
}

func TestListEventsPagination(t *testing.T) {
	resetEvents(t)

	repo := eventspostgres.New(testPool)
	venueID := insertVenue(t, "Main Hall")
	base := time.Date(2026, time.November, 12, 18, 0, 0, 0, time.UTC)

	// Пять событий, две пары с одинаковым starts_at: tie-break по id.
	want := []int64{
		insertEvent(t, venueID, "first", base.Add(time.Hour)),
		insertEvent(t, venueID, "tie-1", base.Add(2*time.Hour)),
		insertEvent(t, venueID, "tie-2", base.Add(2*time.Hour)),
		insertEvent(t, venueID, "tie-3", base.Add(3*time.Hour)),
		insertEvent(t, venueID, "tie-4", base.Add(3*time.Hour)),
	}

	for _, limit := range []int{1, 2} {
		t.Run(fmt.Sprintf("limit=%d", limit), func(t *testing.T) {
			collectPages(t, repo, limit, want)
		})
	}
}

func TestListEventsLimit100(t *testing.T) {
	resetEvents(t)

	repo := eventspostgres.New(testPool)
	venueID := insertVenue(t, "Main Hall")
	base := time.Date(2026, time.November, 12, 18, 0, 0, 0, time.UTC)

	for i := range 3 {
		insertEvent(t, venueID, fmt.Sprintf("event-%d", i), base.Add(time.Duration(i)*time.Hour))
	}

	items, next, err := repo.List(t.Context(), 100, nil)
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(items) != 3 {
		t.Errorf("items = %d, want 3", len(items))
	}
	if next != nil {
		t.Errorf("next = %+v, want nil", next)
	}
}

// TestListEventsInsertBetweenPages проверяет keyset-пагинацию: новое событие
// после курсора появляется, событие до курсора — нет, старые не дублируются.
func TestListEventsInsertBetweenPages(t *testing.T) {
	resetEvents(t)

	repo := eventspostgres.New(testPool)
	venueID := insertVenue(t, "Main Hall")
	base := time.Date(2026, time.November, 12, 18, 0, 0, 0, time.UTC)

	eventA := insertEvent(t, venueID, "A", base.Add(time.Hour))
	eventC := insertEvent(t, venueID, "C", base.Add(3*time.Hour))

	page1, cursor, err := repo.List(t.Context(), 1, nil)
	if err != nil {
		t.Fatalf("page 1: List() error = %v", err)
	}
	if got := eventIDs(page1); !reflect.DeepEqual(got, []int64{eventA}) {
		t.Fatalf("page 1 ids = %v, want [%d]", got, eventA)
	}

	eventB := insertEvent(t, venueID, "B", base.Add(2*time.Hour))
	eventBefore := insertEvent(t, venueID, "before", base.Add(-time.Hour))

	page2, next, err := repo.List(t.Context(), 10, cursor)
	if err != nil {
		t.Fatalf("page 2: List() error = %v", err)
	}
	if next != nil {
		t.Errorf("next = %+v, want nil on last page", next)
	}

	want := []int64{eventB, eventC}
	if got := eventIDs(page2); !reflect.DeepEqual(got, want) {
		t.Errorf("page 2 ids = %v, want %v (no %d and no %d)", got, want, eventA, eventBefore)
	}
}

func TestListEventsInvalidLimit(t *testing.T) {
	resetEvents(t)

	repo := eventspostgres.New(testPool)

	for _, limit := range []int{0, -1} {
		t.Run(fmt.Sprintf("limit=%d", limit), func(t *testing.T) {
			items, next, err := repo.List(t.Context(), limit, nil)

			if err == nil {
				t.Fatalf("List() error = nil, want error for limit %d", limit)
			}
			if items != nil || next != nil {
				t.Errorf("List() = (%+v, %+v), want nils on error", items, next)
			}
		})
	}
}
