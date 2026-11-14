package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/egor-kardash/ticket-booking-service/internal/config"
	authpostgres "github.com/egor-kardash/ticket-booking-service/internal/features/auth/repository/postgres"
	authservice "github.com/egor-kardash/ticket-booking-service/internal/features/auth/service"
	authhttp "github.com/egor-kardash/ticket-booking-service/internal/features/auth/transport/http"
	eventspostgres "github.com/egor-kardash/ticket-booking-service/internal/features/events/repository/postgres"
	eventsservice "github.com/egor-kardash/ticket-booking-service/internal/features/events/service"
	eventshttp "github.com/egor-kardash/ticket-booking-service/internal/features/events/transport/http"
	holdspostgres "github.com/egor-kardash/ticket-booking-service/internal/features/holds/repository/postgres"
	holdsservice "github.com/egor-kardash/ticket-booking-service/internal/features/holds/service"
	holdshttp "github.com/egor-kardash/ticket-booking-service/internal/features/holds/transport/http"
	"github.com/egor-kardash/ticket-booking-service/internal/share/hasher"
	"github.com/egor-kardash/ticket-booking-service/internal/share/httpserver"
	"github.com/egor-kardash/ticket-booking-service/internal/share/postgres"
	"github.com/egor-kardash/ticket-booking-service/internal/share/token"
)

const (
	expireInterval = 30 * time.Second
	expireTimeout  = 5 * time.Second
)

type App struct {
	pool            *pgxpool.Pool
	log             *slog.Logger
	server          *http.Server
	holds           *holdsservice.Service
	shutdownTimeout time.Duration
}

func New(ctx context.Context, cfg *config.Config, log *slog.Logger) (*App, error) {
	pool, err := postgres.NewPool(ctx, cfg.Database.DSN())
	if err != nil {
		return nil, fmt.Errorf("init postgres: %w", err)
	}

	router, holdsService, err := NewRouter(cfg, pool, log)
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
		holds:           holdsService,
		shutdownTimeout: cfg.HTTPServer.ShutdownTimeout,
	}, nil
}

func NewRouter(
	cfg *config.Config,
	pool *pgxpool.Pool,
	log *slog.Logger,
) (chi.Router, *holdsservice.Service, error) {
	passwordHasher, err := hasher.New(cfg.Auth.BcryptCost)
	if err != nil {
		return nil, nil, fmt.Errorf("init password hasher: %w", err)
	}

	issuer := token.New(cfg.Auth.JWTSecret, cfg.Auth.TokenDuration)

	userRepo := authpostgres.New(pool)
	userService := authservice.New(userRepo, passwordHasher, issuer)
	userHandler := authhttp.New(userService)

	eventRepo := eventspostgres.New(pool)
	eventService := eventsservice.New(eventRepo)
	eventHandler := eventshttp.New(eventService)

	holdsRepo := holdspostgres.New(pool)
	holdsService := holdsservice.New(holdsRepo)
	holdsHandler := holdshttp.New(holdsService)

	router := httpserver.NewRouter(log, pool)

	router.Route("/api/v1", func(router chi.Router) {
		protected := router.Group(func(protected chi.Router) {
			protected.Use(httpserver.Auth(issuer))
		})

		userHandler.Routes(router, protected)
		eventHandler.Routes(router)
		holdsHandler.Routes(protected)
	})

	return router, holdsService, nil
}

func (a *App) Run(ctx context.Context) error {
	expireCtx, stopExpirer := context.WithCancel(ctx)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		a.runExpirer(expireCtx)
	}()

	defer func() {
		stopExpirer()
		wg.Wait()
	}()

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

func (a *App) runExpirer(ctx context.Context) {
	a.expireHoldsOnce(ctx)

	ticker := time.NewTicker(expireInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.expireHoldsOnce(ctx)
		}
	}
}

func (a *App) expireHoldsOnce(ctx context.Context) {
	iterCtx, cancel := context.WithTimeout(ctx, expireTimeout)
	defer cancel()

	n, err := a.holds.ExpireHolds(iterCtx)

	switch {
	case err != nil && !errors.Is(err, context.Canceled):
		a.log.Error("expire holds", "error", err)
	case n > 0:
		a.log.Info("holds expired", "count", n)
	}
}
