package service

import (
	"context"
	"fmt"

	"github.com/egor-kardash/ticket-booking-service/internal/features/auth/domain"
)

type RegisterInput struct {
	Email    string
	Password string
}

func (s *Service) Register(ctx context.Context, in RegisterInput) (*domain.User, error) {
	user, err := domain.NewUser(in.Email, in.Password)
	if err != nil {
		return nil, err
	}

	hash, err := s.hasher.Hash(in.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	user.PasswordHash = hash

	created, err := s.repo.Create(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("register: %w", err)
	}

	return created, nil
}
