package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/egor-kardash/ticket-booking-service/internal/features/auth/domain"
	"github.com/egor-kardash/ticket-booking-service/internal/share/httpserver"
)

func writeError(ctx context.Context, w http.ResponseWriter, err error) {
	var validationErr *domain.ValidationError

	switch {
	case errors.As(err, &validationErr):
		httpserver.WriteErrorFields(ctx, w, http.StatusBadRequest,
			"validation_failed", "request validation failed", validationErr.Fields)
	case errors.Is(err, domain.ErrEmailTaken):
		httpserver.WriteError(ctx, w, http.StatusConflict, "email_taken", "email is already taken")
	case errors.Is(err, domain.ErrNotFound):
		httpserver.WriteError(ctx, w, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, domain.ErrInvalidCredentials):
		httpserver.WriteError(ctx, w, http.StatusUnauthorized, "invalid_credentials", "incorrect email or password")
	case errors.Is(err, context.DeadlineExceeded):
		httpserver.LoggerFrom(ctx).Error("user request timed out", "error", err)
		httpserver.WriteError(ctx, w, http.StatusServiceUnavailable, "service_unavailable", "service unavailable")
	default:
		httpserver.LoggerFrom(ctx).Error("user request failed", "error", err)
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
