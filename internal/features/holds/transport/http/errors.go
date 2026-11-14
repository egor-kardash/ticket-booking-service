package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/egor-kardash/ticket-booking-service/internal/features/holds/domain"
	"github.com/egor-kardash/ticket-booking-service/internal/share/httpserver"
)

func writeError(ctx context.Context, w http.ResponseWriter, err error) {
	var validationErr *domain.ValidationError

	switch {
	case errors.As(err, &validationErr):
		httpserver.WriteErrorFields(ctx, w, http.StatusBadRequest,
			"validation_failed", "request validation failed", validationErr.Fields)
	case errors.Is(err, domain.ErrSeatTaken):
		httpserver.WriteError(ctx, w, http.StatusConflict, "seat_taken", "seat is already held")
	case errors.Is(err, domain.ErrHoldNotFound):
		httpserver.WriteError(ctx, w, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, domain.ErrEventSeatNotFound):
		httpserver.WriteError(ctx, w, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, domain.ErrUnauthorized):
		httpserver.WriteError(ctx, w, http.StatusUnauthorized, "unauthorized", "invalid or expired token")
	case errors.Is(err, context.Canceled):
		return
	case errors.Is(err, context.DeadlineExceeded):
		httpserver.LoggerFrom(ctx).Error("request timed out", "error", err)
		httpserver.WriteError(ctx, w, http.StatusServiceUnavailable, "service_unavailable", "service unavailable")
	default:
		httpserver.LoggerFrom(ctx).Error("request failed", "error", err)
		httpserver.WriteError(ctx, w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func writeDecodeError(ctx context.Context, w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		httpserver.WriteError(ctx, w, http.StatusRequestEntityTooLarge,
			"request_too_large", "request body is too large")
		return
	}

	if errors.Is(err, httpserver.ErrUnsupportedMediaType) {
		httpserver.WriteError(ctx, w, http.StatusUnsupportedMediaType,
			"unsupported_media_type", "content type must be application/json")
		return
	}

	httpserver.WriteError(ctx, w, http.StatusBadRequest,
		"invalid_json", "request body must be a single JSON object")
}
