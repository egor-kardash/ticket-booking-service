package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/egor-kardash/ticket-booking-service/internal/config"
	"github.com/egor-kardash/ticket-booking-service/internal/share/httpserver"
	"github.com/egor-kardash/ticket-booking-service/internal/share/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

type App struct {
	pool            *pgxpool.Pool
	log             *slog.Logger
	server          *http.Server
	shutdownTimeout time.Duration
}

func New(ctx context.Context, cfg *config.Config, log *slog.Logger) (*App, error) {
	pool, err := postgres.NewPool(ctx, cfg.Database.DSN())
	if err != nil {
		return nil, fmt.Errorf("init postgres: %w", err)
	}

	router := httpserver.NewRouter(log, pool)

	server := httpserver.New(cfg.HTTPServer, router, log)

	return &App{
		pool:            pool,
		log:             log,
		server:          server,
		shutdownTimeout: cfg.HTTPServer.ShutdownTimeout,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", a.server.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", a.server.Addr, err)
	}

	a.log.Info("http server listening", "addr", ln.Addr().String())

	serveErr := make(chan error, 1)

	go func() {
		serveErr <- a.server.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), a.shutdownTimeout)
		defer cancel()

		if err := a.server.Shutdown(shutdownCtx); err != nil {
			_ = a.server.Close()
			return fmt.Errorf("shutdown http server: %w", err)
		}

		a.log.Info("http server stopped")
		return nil

	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve http: %w", err)
	}
}

func (a *App) Close() error {
	a.pool.Close()
	return nil
}
