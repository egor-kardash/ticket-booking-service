package service

import (
	"context"
	"time"

	"github.com/egor-kardash/ticket-booking-service/internal/features/auth/domain"
)

type UserRepository interface {
	Create(ctx context.Context, user *domain.User) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	GetByID(ctx context.Context, id int64) (*domain.User, error)
}

type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(hash, password string) (bool, error)
	CompareDummy(password string)
}

type TokenIssuer interface {
	IssueToken(userID int64) (string, time.Duration, error)
}

type Service struct {
	repo   UserRepository
	hasher PasswordHasher
	issuer TokenIssuer
}

func New(repo UserRepository, hasher PasswordHasher, issuer TokenIssuer) *Service {
	return &Service{
		repo:   repo,
		hasher: hasher,
		issuer: issuer,
	}
}
