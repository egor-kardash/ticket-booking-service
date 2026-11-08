package http

import (
	"context"
	"net/http"

	"github.com/egor-kardash/ticket-booking-service/internal/share/httpserver"
)

func writeAuthJSON(ctx context.Context, w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	httpserver.WriteJSON(ctx, w, status, payload)
}
