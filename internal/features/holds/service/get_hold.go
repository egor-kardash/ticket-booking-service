package service

import (
	"context"
	"fmt"

	"github.com/egor-kardash/ticket-booking-service/internal/features/holds/domain"
)

func (s *Service) GetHold(ctx context.Context, id, userID int64) (*domain.HoldDetails, error) {
	holdDetails, err := s.repo.GetByID(ctx, id, userID)
	if err != nil {
		return nil, fmt.Errorf("get hold %d: %w", id, err)
	}

	return holdDetails, nil
}
