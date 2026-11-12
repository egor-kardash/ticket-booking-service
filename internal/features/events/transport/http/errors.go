package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/egor-kardash/ticket-booking-service/internal/features/events/domain"
	"github.com/egor-kardash/ticket-booking-service/internal/features/events/service"
	"github.com/egor-kardash/ticket-booking-service/internal/share/httpserver"
)

var (
	ErrInvalidCursor = errors.New("invalid cursor")
)

func writeError(ctx context.Context, w http.ResponseWriter, err error) {
	var validationErr *domain.ValidationError

	switch {
	case errors.As(err, &validationErr):
		httpserver.WriteErrorFields(ctx, w, http.StatusBadRequest,
			"validation_failed", "request validation failed", validationErr.Fields)
	case errors.Is(err, domain.ErrEventNotFound):
		httpserver.WriteError(ctx, w, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, ErrInvalidCursor):
		httpserver.WriteError(ctx, w, http.StatusBadRequest, "invalid_cursor", "invalid pagination cursor")
	case errors.Is(err, service.ErrInvalidLimit):
		httpserver.WriteError(ctx, w, http.StatusBadRequest, "invalid_limit", "invalid events limit")
	case errors.Is(err, context.DeadlineExceeded):
		httpserver.LoggerFrom(ctx).Error("request timed out", "error", err)
		httpserver.WriteError(ctx, w, http.StatusServiceUnavailable, "service_unavailable", "service unavailable")
	default:
		httpserver.LoggerFrom(ctx).Error("request failed", "error", err)
		httpserver.WriteError(ctx, w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
