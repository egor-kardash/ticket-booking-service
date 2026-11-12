package domain

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	MaxEventSeatTierChars = 20
)

type EventSeatStatus string

const (
	EventSeatAvailable EventSeatStatus = "available"
	EventSeatHeld      EventSeatStatus = "held"
	EventSeatSold      EventSeatStatus = "sold"
)

func (s EventSeatStatus) Valid() bool {
	switch s {
	case EventSeatAvailable, EventSeatHeld, EventSeatSold:
		return true
	default:
		return false
	}
}

type EventSeat struct {
	ID         int64
	EventID    int64
	SeatID     int64
	VenueID    int64
	Tier       string
	PriceCents int64
	Status     EventSeatStatus
}

func NewEventSeat(eventID, seatID, venueID int64, tier string, priceCents int64) (*EventSeat, error) {
	tier = strings.TrimSpace(tier)

	fields := make(map[string]string, 5)

	if eventID <= 0 {
		fields["event_id"] = "must be positive"
	}
	if seatID <= 0 {
		fields["seat_id"] = "must be positive"
	}
	if venueID <= 0 {
		fields["venue_id"] = "must be positive"
	}

	switch {
	case tier == "":
		fields["tier"] = "must not be blank"
	case utf8.RuneCountInString(tier) > MaxEventSeatTierChars:
		fields["tier"] = fmt.Sprintf("must be at most %d characters", MaxEventSeatTierChars)
	}

	if priceCents <= 0 {
		fields["price_cents"] = "must be positive"
	}

	if len(fields) > 0 {
		return nil, &ValidationError{Fields: fields}
	}

	return &EventSeat{
		EventID:    eventID,
		SeatID:     seatID,
		VenueID:    venueID,
		Tier:       tier,
		PriceCents: priceCents,
		Status:     EventSeatAvailable,
	}, nil
}

func ParseEventSeatStatus(raw string) (EventSeatStatus, error) {
	status := EventSeatStatus(raw)
	if !status.Valid() {
		return "", fmt.Errorf("invalid event seat status %q", raw)
	}
	return status, nil
}
