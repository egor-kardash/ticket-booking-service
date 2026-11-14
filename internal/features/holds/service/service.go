package service

import (
	"context"
	"time"

	"github.com/egor-kardash/ticket-booking-service/internal/features/holds/domain"
)

type HoldRepository interface {
	Create(ctx context.Context, userID, eventSeatID int64, ttl time.Duration) (*domain.HoldDetails, error)
	GetByID(ctx context.Context, id, userID int64) (*domain.HoldDetails, error)
	Cancel(ctx context.Context, id, userID int64) error
	ExpireBatch(ctx context.Context, limit int) (int, error)
}

type Service struct {
	repo HoldRepository
}

func New(repo HoldRepository) *Service {
	return &Service{repo: repo}
}
