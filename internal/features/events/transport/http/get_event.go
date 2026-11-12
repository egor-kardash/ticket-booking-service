package http

import (
	"context"
	"net/http"

	"github.com/egor-kardash/ticket-booking-service/internal/share/httpserver"
)

func (h *Handler) GetEvent(w http.ResponseWriter, r *http.Request) {
	id, err := parseEventID(r)
	if err != nil {
		writeError(r.Context(), w, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), dbTimeout)
	defer cancel()

	event, err := h.event.GetEvent(ctx, id)
	if err != nil {
		writeError(r.Context(), w, err)
		return
	}

	httpserver.WriteJSON(r.Context(), w, http.StatusOK, toEventResponse(*event))
}
