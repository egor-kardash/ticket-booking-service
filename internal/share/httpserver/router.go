package httpserver

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(log *slog.Logger, db Pinger) chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(RequestIDHeader())
	r.Use(WithLogger(log)) // после RequestID, иначе request_id будет пустым
	r.Use(RequestLogger()) // после WithLogger, иначе LoggerFrom вернет default
	r.Use(Recoverer())

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		WriteError(r.Context(), w, http.StatusNotFound, "not_found", "resource not found")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		WriteError(r.Context(), w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	})

	r.Get("/healthz", healthz)
	r.Get("/readyz", readyz(db))

	return r
}
