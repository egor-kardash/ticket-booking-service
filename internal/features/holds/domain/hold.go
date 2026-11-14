package domain

import "time"

type HoldDetails struct {
	ID          int64
	EventSeatID int64
	ExpiresAt   time.Time
	PriceCents  int64
}

func ValidateNewHold(userID, eventSeatID int64) error {
	fields := make(map[string]string, 2)

	if userID <= 0 {
		fields["user_id"] = "must be positive"
	}
	if eventSeatID <= 0 {
		fields["event_seat_id"] = "must be positive"
	}

	if len(fields) > 0 {
		return &ValidationError{Fields: fields}
	}

	return nil
}
