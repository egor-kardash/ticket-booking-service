package service

import (
	"context"
	"fmt"

	"github.com/egor-kardash/ticket-booking-service/internal/features/events/domain"
)

func (s *Service) ListSeats(ctx context.Context, eventID int64) ([]domain.EventSeatItem, error) {
	seats, err := s.repo.ListSeats(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("list seats: %w", err)
	}

	if len(seats) > 0 {
		return seats, nil
	}

	exists, err := s.repo.Exists(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("check event exists: %w", err)
	}

	if !exists {
		return nil, fmt.Errorf("list seats: %w", domain.ErrEventNotFound)
	}

	return seats, nil
}
