package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/egor-kardash/ticket-booking-service/internal/share/httpserver"
)

func (h *Handler) CreateHold(w http.ResponseWriter, r *http.Request) {
	var req createHoldRequest
	if err := httpserver.DecodeJSON(w, r, &req); err != nil {
		writeDecodeError(r.Context(), w, err)
		return
	}

	userID, ok := httpserver.UserIDFrom(r.Context())
	if !ok {
		writeError(r.Context(), w, errors.New("user id missing in context"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), dbTimeout)
	defer cancel()

	hold, err := h.hold.CreateHold(ctx, userID, req.EventSeatID)
	if err != nil {
		writeError(r.Context(), w, err)
		return
	}

	w.Header().Set("Location", fmt.Sprintf("%s/%d", r.URL.Path, hold.ID))
	writeNoStoreJSON(r.Context(), w, http.StatusCreated, toHoldResponse(*hold))
}
