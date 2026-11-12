package domain

import "time"

type EventListItem struct {
	ID        int64
	Title     string
	VenueName string
	City      string
	StartsAt  time.Time
}

type EventDetails struct {
	ID             int64
	Title          string
	VenueName      string
	City           string
	StartsAt       time.Time
	SeatsTotal     int64
	SeatsAvailable int64
}

type EventSeatItem struct {
	ID         int64
	Section    string
	RowLabel   string
	SeatNumber string
	Tier       string
	PriceCents int64
	Status     EventSeatStatus
}
