//go:build integration

package migrations

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/egor-kardash/ticket-booking-service/internal/testdb"
)

//go:embed *.sql
var migrationFiles embed.FS

const (
	initUpFile   = "000001_init.up.sql"
	initDownFile = "000001_init.down.sql"
	authUpFile   = "000002_auth.up.sql"
	authDownFile = "000002_auth.down.sql"

	checkViolationSQLState = "23514"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

// run создаёт отдельную БД для тестов миграций: они делают DROP SCHEMA booking,
// а пакет репозитория параллельно работает с booking_test в другой БД.
func run(m *testing.M) int {
	ctx := context.Background()

	cfg, err := testdb.ConfigFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "migration tests: %v\n", err)
		return 1
	}

	name := strings.TrimSuffix(cfg.Database, "_test") + "_migrations_test"

	adminPool, err := pgxpool.New(ctx, cfg.DSN("postgres"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "migration tests: admin pool: %v\n", err)
		return 1
	}
	defer adminPool.Close()

	quoted := pgx.Identifier{name}.Sanitize()

	// Сносим хвост упавшего прогона, затем создаём чистую БД.
	if _, err := adminPool.Exec(ctx, "DROP DATABASE IF EXISTS "+quoted+" WITH (FORCE)"); err != nil {
		fmt.Fprintf(os.Stderr, "migration tests: drop database: %v\n", err)
		return 1
	}
	if _, err := adminPool.Exec(ctx, "CREATE DATABASE "+quoted); err != nil {
		fmt.Fprintf(os.Stderr, "migration tests: create database %q: %v (user needs CREATEDB)\n", name, err)
		return 1
	}

	testPool, err = pgxpool.New(ctx, cfg.DSN(name))
	if err != nil {
		fmt.Fprintf(os.Stderr, "migration tests: connect: %v\n", err)
		return 1
	}

	code := m.Run()

	testPool.Close()
	if _, err := adminPool.Exec(ctx, "DROP DATABASE IF EXISTS "+quoted+" WITH (FORCE)"); err != nil {
		fmt.Fprintf(os.Stderr, "migration tests: drop database: %v\n", err)
		if code == 0 {
			code = 1
		}
	}

	return code
}

func TestMigrationsUp(t *testing.T) {
	resetSchema(t)

	applyFile(t, initUpFile)
	if !schemaExists(t, "booking") {
		t.Fatalf("after %s schema %q does not exist", initUpFile, "booking")
	}

	applyFile(t, authUpFile)
	assertUsersSchema(t)
}

func TestMigrationsDown(t *testing.T) {
	resetSchema(t)
	applyFile(t, initUpFile)
	applyFile(t, authUpFile)

	applyFile(t, authDownFile)

	if tableExists(t, "booking.users") {
		t.Fatal("after 000002 down table booking.users still exists")
	}
	// 000002 down не должен трогать схему: это ответственность 000001 down.
	if !schemaExists(t, "booking") {
		t.Error("after 000002 down schema booking disappeared")
	}
}

func TestMigrationsInitDown(t *testing.T) {
	resetSchema(t)
	applyFile(t, initUpFile)
	applyFile(t, authUpFile)
	applyFile(t, authDownFile)
	applyFile(t, initDownFile)

	if schemaExists(t, "booking") {
		t.Fatal("after 000001 down schema booking still exists")
	}
}

func TestMigrationsUpAfterDown(t *testing.T) {
	resetSchema(t)
	applyFile(t, initUpFile)
	applyFile(t, authUpFile)
	applyFile(t, authDownFile)
	applyFile(t, authUpFile)

	assertUsersSchema(t)
}

// assertUsersSchema проверяет ключевые инварианты схемы, а не каждую колонку:
// уникальность email и CHECK на нижний регистр.
func assertUsersSchema(t *testing.T) {
	t.Helper()

	if !tableExists(t, "booking.users") {
		t.Fatal("table booking.users does not exist")
	}

	t.Run("unique index users_email_key on email", func(t *testing.T) {
		var unique bool
		var column string

		err := testPool.QueryRow(context.Background(), `
			SELECT ix.indisunique, a.attname
			FROM pg_index AS ix
			JOIN pg_class AS idx ON idx.oid = ix.indexrelid
			JOIN pg_class AS tbl ON tbl.oid = ix.indrelid
			JOIN pg_namespace AS ns ON ns.oid = tbl.relnamespace
			JOIN pg_attribute AS a ON a.attrelid = tbl.oid AND a.attnum = ix.indkey[0]
			WHERE ns.nspname = 'booking'
			  AND tbl.relname = 'users'
			  AND idx.relname = 'users_email_key'
			  AND ix.indnkeyatts = 1`).
			Scan(&unique, &column)
		if err != nil {
			t.Fatalf("query index users_email_key: %v", err)
		}
		if !unique {
			t.Error("users_email_key is not unique")
		}
		if column != "email" {
			t.Errorf("users_email_key column = %q, want email", column)
		}
	})

	t.Run("check constraint rejects uppercase email", func(t *testing.T) {
		var count int
		err := testPool.QueryRow(context.Background(), `
			SELECT count(*)
			FROM pg_constraint
			WHERE conrelid = 'booking.users'::regclass
			  AND conname = 'users_email_lowercase'
			  AND contype = 'c'`).
			Scan(&count)
		if err != nil {
			t.Fatalf("query check constraint: %v", err)
		}
		if count != 1 {
			t.Errorf("users_email_lowercase constraints = %d, want 1", count)
		}

		_, err = testPool.Exec(context.Background(),
			`INSERT INTO booking.users (email, password_hash) VALUES ($1, $2)`,
			"UPPER@Example.COM", "hash")
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != checkViolationSQLState {
			t.Fatalf("insert uppercase email error = %v, want SQLSTATE %s", err, checkViolationSQLState)
		}
	})
}

func resetSchema(t *testing.T) {
	t.Helper()

	exec(t, "DROP SCHEMA IF EXISTS booking CASCADE")
}

func applyFile(t *testing.T, name string) {
	t.Helper()

	sql, err := migrationFiles.ReadFile(name)
	if err != nil {
		t.Fatalf("read migration %q: %v", name, err)
	}

	// Без аргументов pgx использует simple protocol и принимает несколько
	// statement'ов в одном файле.
	if _, err := testPool.Exec(context.Background(), string(sql)); err != nil {
		t.Fatalf("apply %s: %v", name, err)
	}
}

func exec(t *testing.T, sql string) {
	t.Helper()

	if _, err := testPool.Exec(context.Background(), sql); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func schemaExists(t *testing.T, name string) bool {
	t.Helper()

	var exists bool
	if err := testPool.QueryRow(context.Background(),
		"SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = $1)", name).
		Scan(&exists); err != nil {
		t.Fatalf("query schema %q: %v", name, err)
	}

	return exists
}

func tableExists(t *testing.T, name string) bool {
	t.Helper()

	var exists bool
	if err := testPool.QueryRow(context.Background(),
		"SELECT to_regclass($1) IS NOT NULL", name).Scan(&exists); err != nil {
		t.Fatalf("query table %q: %v", name, err)
	}

	return exists
}
