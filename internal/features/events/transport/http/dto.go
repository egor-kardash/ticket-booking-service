package http

import (
	"github.com/egor-kardash/ticket-booking-service/internal/features/events/domain"
)

type listEventsResponse struct {
	Items      []eventListItemResponse `json:"items"`
	NextCursor string                  `json:"next_cursor,omitempty"`
}

type eventListItemResponse struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Venue    string `json:"venue"`
	City     string `json:"city"`
	StartsAt string `json:"starts_at"`
}

type eventResponse struct {
	ID             int64  `json:"id"`
	Title          string `json:"title"`
	Venue          string `json:"venue"`
	City           string `json:"city"`
	StartsAt       string `json:"starts_at"`
	SeatsTotal     int64  `json:"seats_total"`
	SeatsAvailable int64  `json:"seats_available"`
}

type listEventSeatsResponse struct {
	Items []eventSeatItemResponse `json:"items"`
}

type eventSeatItemResponse struct {
	ID         int64                  `json:"id"`
	Section    string                 `json:"section"`
	RowLabel   string                 `json:"row"`
	SeatNumber string                 `json:"seat"`
	Tier       string                 `json:"tier"`
	PriceCents int64                  `json:"price_cents"`
	Status     domain.EventSeatStatus `json:"status"`
}

func toEventResponse(event domain.EventDetails) eventResponse {
	return eventResponse{
		ID:             event.ID,
		Title:          event.Title,
		Venue:          event.VenueName,
		City:           event.City,
		StartsAt:       formatDate(event.StartsAt),
		SeatsTotal:     event.SeatsTotal,
		SeatsAvailable: event.SeatsAvailable,
	}
}

func toListEventsResponse(items []domain.EventListItem, next *domain.EventCursor) listEventsResponse {
	out := listEventsResponse{
		Items:      make([]eventListItemResponse, 0, len(items)),
		NextCursor: encodeEventCursor(next),
	}
	for _, item := range items {
		out.Items = append(out.Items, eventListItemResponse{
			ID:       item.ID,
			Title:    item.Title,
			Venue:    item.VenueName,
			City:     item.City,
			StartsAt: formatDate(item.StartsAt),
		})
	}

	return out
}

func toListEventSeatsResponse(items []domain.EventSeatItem) listEventSeatsResponse {
	out := listEventSeatsResponse{
		Items: make([]eventSeatItemResponse, 0, len(items)),
	}

	for _, item := range items {
		out.Items = append(out.Items, eventSeatItemResponse{
			ID:         item.ID,
			Section:    item.Section,
			RowLabel:   item.RowLabel,
			SeatNumber: item.SeatNumber,
			Tier:       item.Tier,
			PriceCents: item.PriceCents,
			Status:     item.Status,
		})
	}

	return out
}
