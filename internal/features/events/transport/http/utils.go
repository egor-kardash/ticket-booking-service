package http

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/egor-kardash/ticket-booking-service/internal/features/events/domain"
	"github.com/egor-kardash/ticket-booking-service/internal/features/events/service"
)

const defaultLimit = 20

func encodeEventCursor(c *domain.EventCursor) string {
	if c == nil {
		return ""
	}

	raw := strconv.FormatInt(c.StartsAt.UnixMicro(), 10) + ":" +
		strconv.FormatInt(c.ID, 10)

	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeEventCursor(s string) (*domain.EventCursor, error) {
	if s == "" {
		return nil, ErrInvalidCursor
	}

	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, ErrInvalidCursor
	}

	tsPart, idPart, ok := strings.Cut(string(b), ":")
	if !ok {
		return nil, ErrInvalidCursor
	}

	micros, err := strconv.ParseInt(tsPart, 10, 64)
	if err != nil {
		return nil, ErrInvalidCursor
	}

	id, err := strconv.ParseInt(idPart, 10, 64)
	if err != nil || id <= 0 {
		return nil, ErrInvalidCursor
	}

	cursorMinStartsAt := time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC)
	cursorMaxStartsAt := time.Date(2100, time.January, 1, 0, 0, 0, 0, time.UTC)

	startsAt := time.UnixMicro(micros).UTC()
	if startsAt.Before(cursorMinStartsAt) || startsAt.After(cursorMaxStartsAt) {
		return nil, ErrInvalidCursor
	}

	return &domain.EventCursor{
		StartsAt: startsAt,
		ID:       id,
	}, nil
}

func parseEventID(r *http.Request) (int64, error) {
	fields := make(map[string]string, 1)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		fields["id"] = "must be integer"
	} else if id <= 0 {
		fields["id"] = "must be positive"
	}

	if len(fields) > 0 {
		return 0, &domain.ValidationError{Fields: fields}
	}

	return id, nil
}

func parseListEventsParams(r *http.Request) (int, *domain.EventCursor, error) {
	query := r.URL.Query()

	limit := defaultLimit
	if raw := query.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return 0, nil, fmt.Errorf("%w: %q", service.ErrInvalidLimit, raw)
		}
		limit = parsed
	}

	var cursor *domain.EventCursor
	if raw := query.Get("cursor"); raw != "" {
		c, err := decodeEventCursor(raw)
		if err != nil {
			return 0, nil, err
		}
		cursor = c
	}

	return limit, cursor, nil
}

func formatDate(date time.Time) string {
	return date.UTC().Format(time.RFC3339)
}
