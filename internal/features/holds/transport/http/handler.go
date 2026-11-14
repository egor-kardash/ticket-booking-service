package http

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/egor-kardash/ticket-booking-service/internal/features/holds/domain"
)

const dbTimeout = 3 * time.Second

type HoldService interface {
	CreateHold(ctx context.Context, userID, eventSeatID int64) (*domain.HoldDetails, error)
	GetHold(ctx context.Context, id, userID int64) (*domain.HoldDetails, error)
	CancelHold(ctx context.Context, id, userID int64) error
}

type Handler struct {
	hold HoldService
}

func New(hold HoldService) *Handler {
	return &Handler{hold: hold}
}

func (h *Handler) Routes(r chi.Router) {
	r.Get("/holds/{id}", h.GetHold)
	r.Delete("/holds/{id}", h.DeleteHold)
	r.Post("/holds", h.CreateHold)
}

func parseHoldID(r *http.Request) (int64, error) {
	fields := make(map[string]string, 1)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		fields["id"] = "must be integer"
	} else if id <= 0 {
		fields["id"] = "must be positive"
	}

	if len(fields) > 0 {
		return 0, &domain.ValidationError{Fields: fields}
	}

	return id, nil
}

func formatDate(date time.Time) string {
	return date.UTC().Format(time.RFC3339)
}
