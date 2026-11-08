package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/egor-kardash/ticket-booking-service/internal/config"
	authpostgres "github.com/egor-kardash/ticket-booking-service/internal/features/auth/repository/postgres"
	authservice "github.com/egor-kardash/ticket-booking-service/internal/features/auth/service"
	authhttp "github.com/egor-kardash/ticket-booking-service/internal/features/auth/transport/http"
	"github.com/egor-kardash/ticket-booking-service/internal/share/hasher"
	"github.com/egor-kardash/ticket-booking-service/internal/share/httpserver"
	"github.com/egor-kardash/ticket-booking-service/internal/share/postgres"
	"github.com/egor-kardash/ticket-booking-service/internal/share/token"
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

	router, err := NewRouter(cfg, pool, log)
	if err != nil {
		pool.Close()
		return nil, err
	}

	server, err := httpserver.New(cfg.HTTPServer, cfg.TLS, router, log)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("init http server: %w", err)
	}

	return &App{
		pool:            pool,
		log:             log,
		server:          server,
		shutdownTimeout: cfg.HTTPServer.ShutdownTimeout,
	}, nil
}

func NewRouter(cfg *config.Config, pool *pgxpool.Pool, log *slog.Logger) (chi.Router, error) {
	passwordHasher, err := hasher.New(cfg.Auth.BcryptCost)
	if err != nil {
		return nil, fmt.Errorf("init password hasher: %w", err)
	}

	issuer := token.New(cfg.Auth.JWTSecret, cfg.Auth.TokenDuration)

	userRepo := authpostgres.New(pool)
	userService := authservice.New(userRepo, passwordHasher, issuer)
	userHandler := authhttp.New(userService)

	router := httpserver.NewRouter(log, pool)

	router.Route("/api/v1", func(router chi.Router) {
		protected := router.Group(func(protected chi.Router) {
			protected.Use(httpserver.Auth(issuer))
		})

		userHandler.Routes(router, protected)
	})

	return router, nil
}

func (a *App) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", a.server.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", a.server.Addr, err)
	}

	a.log.Info("http server listening", "addr", ln.Addr().String())

	serveErr := make(chan error, 1)

	go func() {
		if a.server.TLSConfig != nil {
			serveErr <- a.server.ServeTLS(ln, "", "")
		} else {
			serveErr <- a.server.Serve(ln)
		}
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
