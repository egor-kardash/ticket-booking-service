package service

import (
	"context"

	"github.com/egor-kardash/ticket-booking-service/internal/features/events/domain"
)

type EventRepository interface {
	List(ctx context.Context, limit int, cursor *domain.EventCursor) ([]domain.EventListItem, *domain.EventCursor, error)
	GetByID(ctx context.Context, id int64) (*domain.EventDetails, error)
	ListSeats(ctx context.Context, eventID int64) ([]domain.EventSeatItem, error)
	Exists(ctx context.Context, eventID int64) (bool, error)
}

type Service struct {
	repo EventRepository
}

func New(repo EventRepository) *Service {
	return &Service{repo: repo}
}
