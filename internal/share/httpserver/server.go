package httpserver

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/egor-kardash/ticket-booking-service/internal/config"
)

func New(
	cfg config.HTTPServer,
	tlsCfg config.TLS,
	handler http.Handler,
	log *slog.Logger,
) (*http.Server, error) {
	srv := &http.Server{
		Addr:              cfg.Address,
		Handler:           handler,
		ReadHeaderTimeout: cfg.Timeout,
		ReadTimeout:       cfg.Timeout,
		WriteTimeout:      cfg.Timeout,
		IdleTimeout:       cfg.IdleTimeout,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelError),
	}

	if tlsCfg.Enabled() {
		cert, err := tls.LoadX509KeyPair(tlsCfg.CertPath, tlsCfg.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("load tls key pair: %w", err)
		}

		srv.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}
	}

	return srv, nil
}
