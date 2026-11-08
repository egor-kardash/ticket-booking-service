package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/egor-kardash/ticket-booking-service/internal/share/httpserver"
)

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	userID, ok := httpserver.UserIDFrom(r.Context())
	if !ok {
		writeError(r.Context(), w, errors.New("user id missing in context"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), dbTimeout)
	defer cancel()

	user, err := h.user.GetMe(ctx, userID)
	if err != nil {
		writeError(r.Context(), w, err)
		return
	}

	writeAuthJSON(r.Context(), w, http.StatusOK, toUserResponse(*user))
}
