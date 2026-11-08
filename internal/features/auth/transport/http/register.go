package http

import (
	"context"
	"net/http"
	"path"

	"github.com/egor-kardash/ticket-booking-service/internal/features/auth/service"
	"github.com/egor-kardash/ticket-booking-service/internal/share/httpserver"
)

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := httpserver.DecodeJSON(w, r, &req); err != nil {
		writeDecodeError(r.Context(), w, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), dbTimeout)
	defer cancel()

	user, err := h.user.Register(ctx, service.RegisterInput{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		writeError(r.Context(), w, err)
		return
	}

	w.Header().Set("Location", path.Dir(r.URL.Path)+"/me")
	writeAuthJSON(r.Context(), w, http.StatusCreated, toUserResponse(*user))
}
