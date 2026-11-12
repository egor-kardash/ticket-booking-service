package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/egor-kardash/ticket-booking-service/internal/features/events/domain"
)

const maxLimit = 100

var ErrInvalidLimit = errors.New("invalid limit")

func (s *Service) ListEvents(ctx context.Context, limit int, cursor *domain.EventCursor) ([]domain.EventListItem, *domain.EventCursor, error) {
	if limit < 1 || limit > maxLimit {
		return nil, nil, fmt.Errorf("%w: %d", ErrInvalidLimit, limit)
	}

	events, eventCursor, err := s.repo.List(ctx, limit, cursor)
	if err != nil {
		return nil, nil, fmt.Errorf("list events: %w", err)
	}

	return events, eventCursor, nil
}
