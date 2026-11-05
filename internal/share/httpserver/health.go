package httpserver

import (
	"context"
	"net/http"
	"time"
)

type statusResponse struct {
	Status string `json:"status"`
}

type Pinger interface {
	Ping(ctx context.Context) error
}

const readinessTimeout = 2 * time.Second

func healthz(w http.ResponseWriter, r *http.Request) {
	WriteJSON(r.Context(), w, http.StatusOK, statusResponse{Status: "ok"})
}

func readyz(db Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
		defer cancel()

		if err := db.Ping(ctx); err != nil {
			LoggerFrom(r.Context()).Error("readiness check failed", "error", err)
			WriteError(r.Context(), w, http.StatusServiceUnavailable, "database_unavailable", "database is not reachable")
			return
		}

		WriteJSON(r.Context(), w, http.StatusOK, statusResponse{Status: "ready"})
	}
}
