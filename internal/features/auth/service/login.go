package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/egor-kardash/ticket-booking-service/internal/features/auth/domain"
)

type LoginInput struct {
	Email    string
	Password string
}

type LoginOutput struct {
	AccessToken string
	ExpiresIn   time.Duration
}

func (s *Service) Login(ctx context.Context, in LoginInput) (*LoginOutput, error) {
	email := domain.NormalizeEmail(in.Email)

	user, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			s.hasher.CompareDummy(in.Password)
			return nil, domain.ErrInvalidCredentials
		}
		return nil, fmt.Errorf("login: %w", err)
	}

	isMatch, err := s.hasher.Compare(user.PasswordHash, in.Password)
	if err != nil {
		return nil, fmt.Errorf("compare password: %w", err)
	}
	if !isMatch {
		return nil, domain.ErrInvalidCredentials
	}

	accessToken, expiresIn, err := s.issuer.IssueToken(user.ID)
	if err != nil {
		return nil, fmt.Errorf("issue token: %w", err)
	}

	return &LoginOutput{
		AccessToken: accessToken,
		ExpiresIn:   expiresIn,
	}, nil
}
