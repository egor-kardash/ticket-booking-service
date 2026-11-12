package http

import (
	"context"
	"encoding/base64"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egor-kardash/ticket-booking-service/internal/features/events/domain"
)

func rawCursor(micros int64, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(micros, 10) + ":" + id))
}

func TestEventCursorRoundTrip(t *testing.T) {
	// Не-UTC зона нужна, чтобы проверить приведение к UTC после decode.
	startsAt := time.Date(2026, time.November, 12, 18, 30, 45, 123456000, time.FixedZone("MSK", 3*60*60))
	original := &domain.EventCursor{StartsAt: startsAt, ID: 42}

	encoded := encodeEventCursor(original)
	if encoded == "" {
		t.Fatal("encodeEventCursor() = empty, want base64 string")
	}

	decoded, err := decodeEventCursor(encoded)
	if err != nil {
		t.Fatalf("decodeEventCursor() error = %v, want nil", err)
	}
	if !decoded.StartsAt.Equal(startsAt.Truncate(time.Microsecond)) {
		t.Errorf("StartsAt = %v, want %v", decoded.StartsAt, startsAt.Truncate(time.Microsecond))
	}
	if decoded.StartsAt.Location() != time.UTC {
		t.Errorf("StartsAt location = %v, want UTC", decoded.StartsAt.Location())
	}
	if decoded.ID != original.ID {
		t.Errorf("ID = %d, want %d", decoded.ID, original.ID)
	}
}

func TestEventCursorBoundaries(t *testing.T) {
	tests := []struct {
		name string
		at   time.Time
	}{
		{name: "1970-01-01", at: time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC)},
		{name: "2100-01-01", at: time.Date(2100, time.January, 1, 0, 0, 0, 0, time.UTC)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := decodeEventCursor(rawCursor(tt.at.UnixMicro(), "1")); err != nil {
				t.Fatalf("decodeEventCursor() error = %v, want nil on boundary", err)
			}
		})
	}
}

func TestDecodeEventCursorInvalid(t *testing.T) {
	beforeEpoch := time.Date(1969, time.December, 31, 23, 59, 59, 0, time.UTC).UnixMicro()
	afterMax := time.Date(2101, time.January, 1, 0, 0, 0, 0, time.UTC).UnixMicro()

	tests := []struct {
		name   string
		cursor string
	}{
		{name: "invalid base64", cursor: "!!!not-base64!!!"},
		{name: "no colon", cursor: rawCursor(1_700_000_000_000_000, "")},
		{name: "extra colon", cursor: rawCursor(1_700_000_000_000_000, "42:7")},
		{name: "non-numeric micros", cursor: base64.RawURLEncoding.EncodeToString([]byte("abc:42"))},
		{name: "non-numeric id", cursor: base64.RawURLEncoding.EncodeToString([]byte("1700000000000000:abc"))},
		{name: "zero id", cursor: rawCursor(1_700_000_000_000_000, "0")},
		{name: "negative id", cursor: rawCursor(1_700_000_000_000_000, "-1")},
		{name: "before 1970", cursor: rawCursor(beforeEpoch, "42")},
		{name: "after 2100", cursor: rawCursor(afterMax, "42")},
		{name: "min int64 micros", cursor: rawCursor(math.MinInt64, "42")},
		{name: "max int64 micros", cursor: rawCursor(math.MaxInt64, "42")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cursor, err := decodeEventCursor(tt.cursor)

			if !errors.Is(err, ErrInvalidCursor) {
				t.Fatalf("decodeEventCursor() error = %v, want ErrInvalidCursor", err)
			}
			if cursor != nil {
				t.Errorf("decodeEventCursor() = %+v, want nil on error", cursor)
			}
		})
	}
}

func TestEventCursorEmptyMeansFirstPage(t *testing.T) {
	if got := encodeEventCursor(nil); got != "" {
		t.Errorf("encodeEventCursor(nil) = %q, want empty string", got)
	}

	tests := []struct {
		name string
		url  string
	}{
		{name: "no cursor param", url: "/events"},
		{name: "empty cursor param", url: "/events?cursor="},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, cursor, err := parseListEventsParams(httptest.NewRequest(http.MethodGet, tt.url, nil))
			if err != nil {
				t.Fatalf("parseListEventsParams() error = %v, want nil", err)
			}
			if cursor != nil {
				t.Errorf("cursor = %+v, want nil for first page", cursor)
			}
		})
	}

	// На уровне codec пустая строка невалидна; params не вызывает decode для неё.
	if _, err := decodeEventCursor(""); !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("decodeEventCursor(\"\") error = %v, want ErrInvalidCursor", err)
	}
}

func TestInvalidCursorMapsToBadRequest(t *testing.T) {
	rec := httptest.NewRecorder()

	writeError(context.Background(), rec, ErrInvalidCursor)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), `"invalid_cursor"`) {
		t.Errorf("body = %q, want invalid_cursor code", rec.Body.String())
	}
}
