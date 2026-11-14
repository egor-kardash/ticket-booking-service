package service

import (
	"context"
	"fmt"
	"time"

	"github.com/egor-kardash/ticket-booking-service/internal/features/holds/domain"
)

const holdTTL = 10 * time.Minute

func (s *Service) CreateHold(ctx context.Context, userID, eventSeatID int64) (*domain.HoldDetails, error) {
	if err := domain.ValidateNewHold(userID, eventSeatID); err != nil {
		return nil, fmt.Errorf("create hold: %w", err)
	}

	hold, err := s.repo.Create(ctx, userID, eventSeatID, holdTTL)
	if err != nil {
		return nil, fmt.Errorf("create hold: %w", err)
	}

	return hold, nil
}
