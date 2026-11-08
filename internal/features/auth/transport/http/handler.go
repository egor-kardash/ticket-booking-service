package http

import (
	"context"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/egor-kardash/ticket-booking-service/internal/features/auth/domain"
	"github.com/egor-kardash/ticket-booking-service/internal/features/auth/service"
)

const dbTimeout = 3 * time.Second

type UserService interface {
	Register(ctx context.Context, in service.RegisterInput) (*domain.User, error)
	Login(ctx context.Context, in service.LoginInput) (*service.LoginOutput, error)
	GetMe(ctx context.Context, userID int64) (*domain.User, error)
}

type Handler struct {
	user UserService
}

func New(user UserService) *Handler {
	return &Handler{user: user}
}

func (h *Handler) Routes(r, protected chi.Router) {
	r.Post("/auth/register", h.register)
	r.Post("/auth/login", h.login)

	protected.Get("/auth/me", h.me)
}
