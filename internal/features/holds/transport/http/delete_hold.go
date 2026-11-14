package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/egor-kardash/ticket-booking-service/internal/share/httpserver"
)

func (h *Handler) DeleteHold(w http.ResponseWriter, r *http.Request) {
	userID, ok := httpserver.UserIDFrom(r.Context())
	if !ok {
		writeError(r.Context(), w, errors.New("user id missing in context"))
		return
	}

	id, err := parseHoldID(r)
	if err != nil {
		writeError(r.Context(), w, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), dbTimeout)
	defer cancel()

	if err := h.hold.CancelHold(ctx, id, userID); err != nil {
		writeError(r.Context(), w, err)
		return
	}

	httpserver.WriteNoContent(w)
}
