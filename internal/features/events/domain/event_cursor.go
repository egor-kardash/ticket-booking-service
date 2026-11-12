package domain

import "time"

type EventCursor struct {
	StartsAt time.Time
	ID       int64
}
