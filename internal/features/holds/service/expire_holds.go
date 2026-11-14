package service

import (
	"context"
	"fmt"
)

const expireBatchSize = 100

func (s *Service) ExpireHolds(ctx context.Context) (int, error) {
	n, err := s.repo.ExpireBatch(ctx, expireBatchSize)
	if err != nil {
		return n, fmt.Errorf("expire holds: %w", err)
	}

	return n, nil
}
