package http

import (
	"context"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/egor-kardash/ticket-booking-service/internal/features/events/domain"
)

const dbTimeout = 3 * time.Second

type EventService interface {
	GetEvent(ctx context.Context, id int64) (*domain.EventDetails, error)
	ListEvents(ctx context.Context, limit int, cursor *domain.EventCursor) ([]domain.EventListItem, *domain.EventCursor, error)
	ListSeats(ctx context.Context, eventID int64) ([]domain.EventSeatItem, error)
}

type Handler struct {
	event EventService
}

func New(event EventService) *Handler {
	return &Handler{event: event}
}

func (h *Handler) Routes(r chi.Router) {
	r.Get("/events", h.ListEvents)
	r.Get("/events/{id}", h.GetEvent)
	r.Get("/events/{id}/seats", h.ListSeats)
}
