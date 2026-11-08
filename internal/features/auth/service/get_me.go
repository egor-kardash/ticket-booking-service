package service

import (
	"context"
	"fmt"

	"github.com/egor-kardash/ticket-booking-service/internal/features/auth/domain"
)

func (s *Service) GetMe(ctx context.Context, userID int64) (*domain.User, error) {
	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get me: %w", err)
	}

	return user, nil
}
