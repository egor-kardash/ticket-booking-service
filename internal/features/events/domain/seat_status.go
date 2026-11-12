package domain

import (
	"fmt"
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

func ParseEventSeatStatus(raw string) (EventSeatStatus, error) {
	status := EventSeatStatus(raw)
	if !status.Valid() {
		return "", fmt.Errorf("invalid event seat status %q", raw)
	}
	return status, nil
}
