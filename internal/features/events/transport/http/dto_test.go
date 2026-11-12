package http

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/egor-kardash/ticket-booking-service/internal/features/events/domain"
)

// msk — не-UTC зона: если formatDate забудет .UTC(), тест упадёт независимо
// от TZ процесса. Отдельная переменная окружения для этого не нужна.
var msk = time.FixedZone("MSK", 3*60*60)

func marshalJSON(t *testing.T, value any) string {
	t.Helper()

	body, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	return string(body)
}

func TestStartsAtIsUTCRFC3339(t *testing.T) {
	startsAt := time.Date(2026, time.November, 12, 18, 30, 0, 0, msk)
	const want = "2026-11-12T15:30:00Z"

	t.Run("list item", func(t *testing.T) {
		response := toListEventsResponse([]domain.EventListItem{{
			ID: 1, Title: "Concert", VenueName: "Hall", City: "Moscow", StartsAt: startsAt,
		}}, nil)

		if got := response.Items[0].StartsAt; got != want {
			t.Errorf("starts_at = %q, want %q", got, want)
		}
		if !strings.Contains(marshalJSON(t, response), `"starts_at":"`+want+`"`) {
			t.Errorf("JSON does not contain UTC starts_at: %s", marshalJSON(t, response))
		}
	})

	t.Run("event detail", func(t *testing.T) {
		response := toEventResponse(domain.EventDetails{
			ID: 1, Title: "Concert", VenueName: "Hall", City: "Moscow", StartsAt: startsAt,
		})

		if got := response.StartsAt; got != want {
			t.Errorf("starts_at = %q, want %q", got, want)
		}
		if !strings.Contains(marshalJSON(t, response), `"starts_at":"`+want+`"`) {
			t.Errorf("JSON does not contain UTC starts_at: %s", marshalJSON(t, response))
		}
	})

	parsed, err := time.Parse(time.RFC3339, want)
	if err != nil {
		t.Fatalf("parse reference time: %v", err)
	}
	if !parsed.Equal(startsAt) {
		t.Errorf("parsed starts_at = %v, want instant %v", parsed, startsAt)
	}
	if parsed.Location() != time.UTC {
		t.Errorf("parsed location = %v, want UTC", parsed.Location())
	}
}

func TestSeatCountsAreNotDistorted(t *testing.T) {
	response := toEventResponse(domain.EventDetails{
		ID: 1, SeatsTotal: 120, SeatsAvailable: 7,
	})

	if response.SeatsTotal != 120 {
		t.Errorf("seats_total = %d, want 120", response.SeatsTotal)
	}
	if response.SeatsAvailable != 7 {
		t.Errorf("seats_available = %d, want 7", response.SeatsAvailable)
	}

	body := marshalJSON(t, response)
	for _, want := range []string{`"seats_total":120`, `"seats_available":7`} {
		if !strings.Contains(body, want) {
			t.Errorf("JSON does not contain %s: %s", want, body)
		}
	}
}

func TestItemsAreNeverNull(t *testing.T) {
	tests := []struct {
		name     string
		response any
	}{
		{name: "events list", response: toListEventsResponse(nil, nil)},
		{name: "seats list", response: toListEventSeatsResponse(nil)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := marshalJSON(t, tt.response)
			if !strings.Contains(body, `"items":[]`) {
				t.Errorf("body = %s, want empty items array, not null", body)
			}
		})
	}
}

func TestNextCursorInJSON(t *testing.T) {
	items := []domain.EventListItem{{ID: 1, StartsAt: time.Date(2026, time.November, 12, 18, 0, 0, 0, time.UTC)}}

	t.Run("omitted on last page", func(t *testing.T) {
		body := marshalJSON(t, toListEventsResponse(items, nil))
		if strings.Contains(body, "next_cursor") {
			t.Errorf("body = %s, want no next_cursor on last page", body)
		}
	})

	t.Run("present with next page", func(t *testing.T) {
		next := &domain.EventCursor{
			StartsAt: time.Date(2026, time.November, 12, 18, 0, 0, 0, time.UTC),
			ID:       7,
		}

		var decoded struct {
			NextCursor string `json:"next_cursor"`
		}
		if err := json.Unmarshal([]byte(marshalJSON(t, toListEventsResponse(items, next))), &decoded); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}

		cursor, err := decodeEventCursor(decoded.NextCursor)
		if err != nil {
			t.Fatalf("decode next_cursor: %v", err)
		}
		if cursor.ID != next.ID || !cursor.StartsAt.Equal(next.StartsAt) {
			t.Errorf("next_cursor = %+v, want %+v", cursor, next)
		}
	})
}

func TestSeatStatusesAreSerialized(t *testing.T) {
	items := []domain.EventSeatItem{
		{ID: 1, Status: domain.EventSeatAvailable},
		{ID: 2, Status: domain.EventSeatHeld},
		{ID: 3, Status: domain.EventSeatSold},
	}

	response := toListEventSeatsResponse(items)

	wantStatuses := []domain.EventSeatStatus{
		domain.EventSeatAvailable,
		domain.EventSeatHeld,
		domain.EventSeatSold,
	}
	for i, want := range wantStatuses {
		if got := response.Items[i].Status; got != want {
			t.Errorf("item %d status = %q, want %q", i, got, want)
		}
	}

	body := marshalJSON(t, response)
	for _, want := range []string{`"status":"available"`, `"status":"held"`, `"status":"sold"`} {
		if !strings.Contains(body, want) {
			t.Errorf("JSON does not contain %s: %s", want, body)
		}
	}
}
