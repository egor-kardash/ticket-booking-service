package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxEventTitleChars = 200
)

type Event struct {
	ID       int64
	VenueID  int64
	Title    string
	StartsAt time.Time
}

func NewEvent(venueID int64, title string, startsAt time.Time) (*Event, error) {
	title = strings.TrimSpace(title)

	fields := make(map[string]string, 3)

	if venueID <= 0 {
		fields["venue_id"] = "must be positive"
	}

	switch {
	case title == "":
		fields["title"] = "must not be blank"
	case utf8.RuneCountInString(title) > MaxEventTitleChars:
		fields["title"] = fmt.Sprintf("must be at most %d characters", MaxEventTitleChars)
	}

	if startsAt.IsZero() {
		fields["starts_at"] = "must not be zero"
	}

	if len(fields) > 0 {
		return nil, &ValidationError{Fields: fields}
	}

	return &Event{
		VenueID:  venueID,
		Title:    title,
		StartsAt: startsAt,
	}, nil
}
