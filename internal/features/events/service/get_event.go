package service

import (
	"context"
	"fmt"

	"github.com/egor-kardash/ticket-booking-service/internal/features/events/domain"
)

func (s *Service) GetEvent(ctx context.Context, id int64) (*domain.EventDetails, error) {
	eventDetails, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get event %d: %w", id, err)
	}

	return eventDetails, nil
}
