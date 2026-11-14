package service

import (
	"context"
	"fmt"
)

func (s *Service) CancelHold(ctx context.Context, id, userID int64) error {
	err := s.repo.Cancel(ctx, id, userID)
	if err != nil {
		return fmt.Errorf("cancel hold: %w", err)
	}

	return nil
}
