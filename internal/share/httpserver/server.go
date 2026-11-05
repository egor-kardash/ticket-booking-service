package httpserver

import (
	"log/slog"
	"net/http"

	"github.com/egor-kardash/ticket-booking-service/internal/config"
)

func New(cfg config.HTTPServer, handler http.Handler, log *slog.Logger) *http.Server {
	return &http.Server{
		Addr:              cfg.Address,
		Handler:           handler,
		ReadHeaderTimeout: cfg.Timeout,
		ReadTimeout:       cfg.Timeout,
		WriteTimeout:      cfg.Timeout,
		IdleTimeout:       cfg.IdleTimeout,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelError),
	}
}
