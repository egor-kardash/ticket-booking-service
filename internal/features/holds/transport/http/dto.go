package http

import "github.com/egor-kardash/ticket-booking-service/internal/features/holds/domain"

type createHoldRequest struct {
	EventSeatID int64 `json:"event_seat_id"`
}

type holdResponse struct {
	ID          int64  `json:"id"`
	EventSeatID int64  `json:"event_seat_id"`
	ExpiresAt   string `json:"expires_at"`
	PriceCents  int64  `json:"price_cents"`
}

func toHoldResponse(hold domain.HoldDetails) holdResponse {
	return holdResponse{
		ID:          hold.ID,
		EventSeatID: hold.EventSeatID,
		ExpiresAt:   formatDate(hold.ExpiresAt),
		PriceCents:  hold.PriceCents,
	}
}
