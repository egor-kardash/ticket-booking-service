package http

import (
	"context"
	"net/http"
	"time"

	"github.com/egor-kardash/ticket-booking-service/internal/features/auth/service"
	"github.com/egor-kardash/ticket-booking-service/internal/share/httpserver"
)

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := httpserver.DecodeJSON(w, r, &req); err != nil {
		writeDecodeError(r.Context(), w, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), dbTimeout)
	defer cancel()

	out, err := h.user.Login(ctx, service.LoginInput{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		writeError(r.Context(), w, err)
		return
	}

	writeAuthJSON(r.Context(), w, http.StatusOK, loginResponse{
		AccessToken: out.AccessToken,
		ExpiresIn:   int64(out.ExpiresIn / time.Second),
	})
}
