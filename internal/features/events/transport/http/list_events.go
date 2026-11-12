package http

import (
	"context"
	"net/http"

	"github.com/egor-kardash/ticket-booking-service/internal/share/httpserver"
)

func (h *Handler) ListEvents(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parseListEventsParams(r)
	if err != nil {
		writeError(r.Context(), w, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), dbTimeout)
	defer cancel()

	items, next, err := h.event.ListEvents(ctx, limit, cursor)
	if err != nil {
		writeError(r.Context(), w, err)
		return
	}

	httpserver.WriteJSON(r.Context(), w, http.StatusOK, toListEventsResponse(items, next))
}
