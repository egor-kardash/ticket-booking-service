package http

import (
	"context"
	"net/http"

	"github.com/egor-kardash/ticket-booking-service/internal/share/httpserver"
)

func (h *Handler) ListSeats(w http.ResponseWriter, r *http.Request) {
	id, err := parseEventID(r)
	if err != nil {
		writeError(r.Context(), w, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), dbTimeout)
	defer cancel()

	seats, err := h.event.ListSeats(ctx, id)
	if err != nil {
		writeError(r.Context(), w, err)
		return
	}

	httpserver.WriteJSON(r.Context(), w, http.StatusOK, toListEventSeatsResponse(seats))
}
