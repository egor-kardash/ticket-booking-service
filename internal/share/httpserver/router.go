package httpserver

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

var allowedMethods = [...]string{
	http.MethodGet,
	http.MethodHead,
	http.MethodPost,
	http.MethodPut,
	http.MethodPatch,
	http.MethodDelete,
	http.MethodOptions,
}

func NewRouter(log *slog.Logger, db Pinger) chi.Router {
	router := chi.NewRouter()

	router.Use(middleware.RequestID)
	router.Use(RequestIDHeader())
	router.Use(WithLogger(log)) // после RequestID, иначе request_id будет пустым
	router.Use(RequestLogger()) // после WithLogger, иначе LoggerFrom вернет default
	router.Use(Recoverer())

	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		WriteError(r.Context(), w, http.StatusNotFound, "not_found", "resource not found")
	})

	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		if allow := allowedMethodsFor(router, r.URL.Path); allow != "" {
			w.Header().Set("Allow", allow)
		}
		WriteError(r.Context(), w, http.StatusMethodNotAllowed,
			"method_not_allowed", "method not allowed")
	})

	router.Get("/healthz", healthz)
	router.Get("/readyz", readyz(db))

	return router
}

func allowedMethodsFor(mux *chi.Mux, path string) string {
	var allowed []string
	for _, method := range allowedMethods {
		if mux.Match(chi.NewRouteContext(), method, path) {
			allowed = append(allowed, method)
		}
	}
	return strings.Join(allowed, ", ")
}
