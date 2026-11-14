//go:build integration

package app_test

import (
	"context"
	"log/slog"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/egor-kardash/ticket-booking-service/app"
	"github.com/egor-kardash/ticket-booking-service/internal/config"
	"github.com/egor-kardash/ticket-booking-service/internal/testdb"
)

const testJWTSecret = "0123456789abcdef0123456789abcdef"

// logSink превращает логи app.Run в события, чтобы дождаться старта сервера
// без time.Sleep и фиксированных портов.
type logSink struct {
	messages chan string
}

func (s *logSink) Enabled(context.Context, slog.Level) bool { return true }

func (s *logSink) Handle(_ context.Context, record slog.Record) error {
	select {
	case s.messages <- record.Message:
	default:
	}

	return nil
}

func (s *logSink) WithAttrs([]slog.Attr) slog.Handler { return s }
func (s *logSink) WithGroup(string) slog.Handler      { return s }

func testAppConfig(t *testing.T) *config.Config {
	t.Helper()

	db, err := testdb.ConfigFromEnv()
	if err != nil {
		t.Fatalf("test database config: %v", err)
	}

	port, err := strconv.Atoi(db.Port)
	if err != nil {
		t.Fatalf("parse test database port: %v", err)
	}

	return &config.Config{
		HTTPServer: config.HTTPServer{
			Address:         "127.0.0.1:0",
			Timeout:         5 * time.Second,
			IdleTimeout:     30 * time.Second,
			ShutdownTimeout: 5 * time.Second,
		},
		Database: config.Database{
			Host: db.Host, Port: port, User: db.User, Password: db.Password, Name: db.Database, SSLMode: db.SSLMode,
		},
		Log:  config.Log{Level: "info", Format: "text"},
		Auth: config.Auth{BcryptCost: bcrypt.MinCost, JWTSecret: testJWTSecret, TokenDuration: 15 * time.Minute},
	}
}

// acquireTestDatabaseLock защищает от гонки с другими интеграционными пакетами:
// Run сразу запускает expiry worker, который пишет в booking.holds.
func acquireTestDatabaseLock(t *testing.T) {
	t.Helper()

	cfg, err := testdb.ConfigFromEnv()
	if err != nil {
		t.Fatalf("test database config: %v", err)
	}

	pool, err := pgxpool.New(context.Background(), cfg.DSN(cfg.Database))
	if err != nil {
		t.Fatalf("create lock pool: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := testdb.RequireDatabase(context.Background(), pool); err != nil {
		t.Fatalf("require test database: %v", err)
	}

	release, err := testdb.AcquireLock(context.Background(), pool)
	if err != nil {
		t.Fatalf("acquire test database lock: %v", err)
	}
	t.Cleanup(release)
}

func TestAppGracefulShutdown(t *testing.T) {
	acquireTestDatabaseLock(t)

	logs := &logSink{messages: make(chan string, 16)}
	logger := slog.New(logs)

	application, err := app.New(context.Background(), testAppConfig(t), logger)
	if err != nil {
		t.Fatalf("app.New() error = %v", err)
	}
	t.Cleanup(func() {
		if err := application.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() {
		runDone <- application.Run(ctx)
	}()

	select {
	case message := <-logs.messages:
		if message != "http server listening" {
			t.Fatalf("unexpected startup log %q", message)
		}
	case err := <-runDone:
		t.Fatalf("Run() returned before listening: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("server did not start")
	}

	cancel()

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run() error = %v, want nil after graceful shutdown", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return after context cancellation")
	}
}
