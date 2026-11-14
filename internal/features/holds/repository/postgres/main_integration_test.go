//go:build integration

package postgres_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/egor-kardash/ticket-booking-service/internal/testdb"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx := context.Background()

	cfg, err := testdb.ConfigFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "holds repository tests: %v\n", err)
		return 1
	}

	pool, err := pgxpool.New(ctx, cfg.DSN(cfg.Database))
	if err != nil {
		fmt.Fprintf(os.Stderr, "holds repository tests: create pool: %v\n", err)
		return 1
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "holds repository tests: ping postgres: %v\n", err)
		return 1
	}
	if err := testdb.RequireDatabase(ctx, pool); err != nil {
		fmt.Fprintf(os.Stderr, "holds repository tests: %v\n", err)
		return 1
	}
	if err := testdb.RequireSchema(ctx, pool, "booking.holds"); err != nil {
		fmt.Fprintf(os.Stderr, "holds repository tests: %v\n", err)
		return 1
	}
	if err := testdb.RequireSchema(ctx, pool, "booking.event_seats"); err != nil {
		fmt.Fprintf(os.Stderr, "holds repository tests: %v\n", err)
		return 1
	}
	if err := testdb.RequireSchema(ctx, pool, "booking.users"); err != nil {
		fmt.Fprintf(os.Stderr, "holds repository tests: %v\n", err)
		return 1
	}

	release, err := testdb.AcquireLock(ctx, pool)
	if err != nil {
		fmt.Fprintf(os.Stderr, "holds repository tests: %v\n", err)
		return 1
	}
	defer release()

	testPool = pool

	return m.Run()
}
