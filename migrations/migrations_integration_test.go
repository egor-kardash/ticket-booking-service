//go:build integration

package migrations

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
	initUpFile     = "000001_init.up.sql"
	initDownFile   = "000001_init.down.sql"
	authUpFile     = "000002_auth.up.sql"
	authDownFile   = "000002_auth.down.sql"
	eventsUpFile   = "000003_events.up.sql"
	eventsDownFile = "000003_events.down.sql"

	checkViolationSQLState      = "23514"
	uniqueViolationSQLState     = "23505"
	foreignKeyViolationSQLState = "23503"
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

func TestMigrationsEventsUp(t *testing.T) {
	resetSchema(t)
	applyFile(t, initUpFile)
	applyFile(t, authUpFile)
	applyFile(t, eventsUpFile)

	tables := []string{"booking.venues", "booking.events", "booking.seats", "booking.event_seats"}
	for _, table := range tables {
		if !tableExists(t, table) {
			t.Fatalf("after %s table %q does not exist", eventsUpFile, table)
		}
	}

	venueA := insertVenue(t, "Venue A")
	venueB := insertVenue(t, "Venue B")
	eventA := insertEvent(t, venueA, "Event A")
	eventB := insertEvent(t, venueB, "Event B")

	t.Run("event_seats_unique is event_id plus seat_id", func(t *testing.T) {
		var definition string
		err := testPool.QueryRow(context.Background(), `
			SELECT pg_get_constraintdef(oid)
			FROM pg_constraint
			WHERE conrelid = 'booking.event_seats'::regclass
			  AND conname = 'event_seats_unique'
			  AND contype = 'u'`).Scan(&definition)
		if err != nil {
			t.Fatalf("query event_seats_unique: %v", err)
		}
		if !strings.Contains(definition, "UNIQUE (event_id, seat_id)") {
			t.Errorf("constraint definition = %q, want UNIQUE (event_id, seat_id)", definition)
		}
	})

	t.Run("event_seats_unique rejects duplicate pair", func(t *testing.T) {
		seat := insertSeat(t, venueA, "unique")
		if err := insertEventSeat(t, eventA, seat, venueA, 1000, "available"); err != nil {
			t.Fatalf("first insert: %v", err)
		}

		requirePgError(t, insertEventSeat(t, eventA, seat, venueA, 2000, "held"), uniqueViolationSQLState)
	})

	t.Run("price_cents must be positive", func(t *testing.T) {
		for i, price := range []int64{0, -1} {
			seat := insertSeat(t, venueA, fmt.Sprintf("p%d", i))
			requirePgError(t, insertEventSeat(t, eventA, seat, venueA, price, "available"), checkViolationSQLState)
		}
	})

	t.Run("status allows only known values", func(t *testing.T) {
		for i, status := range []string{"available", "held", "sold"} {
			seat := insertSeat(t, venueA, fmt.Sprintf("st%d", i))
			if err := insertEventSeat(t, eventA, seat, venueA, 1000, status); err != nil {
				t.Errorf("status %q rejected: %v", status, err)
			}
		}

		seat := insertSeat(t, venueA, "stbad")
		requirePgError(t, insertEventSeat(t, eventA, seat, venueA, 1000, "pending"), checkViolationSQLState)
	})

	t.Run("status defaults to available", func(t *testing.T) {
		seat := insertSeat(t, venueA, "stdef")
		if err := insertEventSeat(t, eventA, seat, venueA, 1000, ""); err != nil {
			t.Fatalf("insert without status: %v", err)
		}

		var status string
		if err := testPool.QueryRow(context.Background(), `
			SELECT status FROM booking.event_seats WHERE event_id = $1 AND seat_id = $2`,
			eventA, seat).Scan(&status); err != nil {
			t.Fatalf("read default status: %v", err)
		}
		if status != "available" {
			t.Errorf("status = %q, want available", status)
		}
	})

	// venue_id должен совпадать и с event, и с seat: оба composite FK.
	t.Run("rejects seat from another venue", func(t *testing.T) {
		foreignSeat := insertSeat(t, venueB, "xseat")
		requirePgError(t, insertEventSeat(t, eventA, foreignSeat, venueA, 1000, "available"), foreignKeyViolationSQLState)
	})

	t.Run("rejects event from another venue", func(t *testing.T) {
		seat := insertSeat(t, venueA, "xevent")
		requirePgError(t, insertEventSeat(t, eventB, seat, venueA, 1000, "available"), foreignKeyViolationSQLState)
	})
}

func TestMigrationsEventsDown(t *testing.T) {
	resetSchema(t)
	applyFile(t, initUpFile)
	applyFile(t, authUpFile)
	applyFile(t, eventsUpFile)

	// Порядок drop внутри файла проверяется самим выполнением:
	// неверный порядок упрётся в FK и упадёт на applyFile.
	applyFile(t, eventsDownFile)

	for _, table := range []string{"booking.event_seats", "booking.seats", "booking.events", "booking.venues"} {
		if tableExists(t, table) {
			t.Errorf("after %s table %q still exists", eventsDownFile, table)
		}
	}
	if !schemaExists(t, "booking") {
		t.Error("after 000003 down schema booking disappeared")
	}
}

func TestMigrationsEventsUpAfterDown(t *testing.T) {
	resetSchema(t)
	applyFile(t, initUpFile)
	applyFile(t, authUpFile)
	applyFile(t, eventsUpFile)
	applyFile(t, eventsDownFile)
	applyFile(t, eventsUpFile)

	for _, table := range []string{"booking.venues", "booking.events", "booking.seats", "booking.event_seats"} {
		if !tableExists(t, table) {
			t.Errorf("after up -> down -> up table %q does not exist", table)
		}
	}
}

func TestSeedSmoke(t *testing.T) {
	resetSchema(t)
	applyFile(t, initUpFile)
	applyFile(t, authUpFile)
	applyFile(t, eventsUpFile)

	applySeed(t)
	assertSeedCounts(t)
	assertSeedPrices(t)

	// Занятые/проданные места не должны «оживать» после повторного seed.
	heldIDs := markEventSeatStatus(t, "held", 5, 0)
	soldIDs := markEventSeatStatus(t, "sold", 5, 5)

	applySeed(t)

	assertSeedCounts(t)
	assertSeedPrices(t)

	for status, ids := range map[string][]int64{"held": heldIDs, "sold": soldIDs} {
		for _, id := range ids {
			var got string
			if err := testPool.QueryRow(context.Background(),
				"SELECT status FROM booking.event_seats WHERE id = $1", id).Scan(&got); err != nil {
				t.Fatalf("read status of event_seat %d: %v", id, err)
			}
			if got != status {
				t.Errorf("event_seat %d status = %q, want %q after second seed", id, got, status)
			}
		}
	}
}

func applySeed(t *testing.T) {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}

	path := filepath.Join(filepath.Dir(file), "..", "scripts", "seed.sql")
	sql, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	if _, err := testPool.Exec(context.Background(), string(sql)); err != nil {
		t.Fatalf("apply seed: %v", err)
	}
}

func assertSeedCounts(t *testing.T) {
	t.Helper()

	counts := map[string]int{
		"booking.venues":      1,
		"booking.events":      2,
		"booking.seats":       100,
		"booking.event_seats": 200,
	}

	for table, want := range counts {
		var got int
		if err := testPool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&got); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if got != want {
			t.Errorf("%s count = %d, want %d", table, got, want)
		}
	}
}

func assertSeedPrices(t *testing.T) {
	t.Helper()

	var nonPositive int
	if err := testPool.QueryRow(context.Background(),
		"SELECT count(*) FROM booking.event_seats WHERE price_cents <= 0").Scan(&nonPositive); err != nil {
		t.Fatalf("check price_cents: %v", err)
	}
	if nonPositive != 0 {
		t.Errorf("event_seats with price_cents <= 0: %d", nonPositive)
	}

	var vip, standard int
	if err := testPool.QueryRow(context.Background(), `
		SELECT
			count(*) FILTER (WHERE tier = 'vip' AND price_cents = 500000),
			count(*) FILTER (WHERE tier = 'standard' AND price_cents = 250000)
		FROM booking.event_seats`).Scan(&vip, &standard); err != nil {
		t.Fatalf("check tiers: %v", err)
	}
	if vip != 60 || standard != 140 {
		t.Errorf("tiers = %d vip / %d standard, want 60/140", vip, standard)
	}
}

func markEventSeatStatus(t *testing.T, status string, limit, offset int) []int64 {
	t.Helper()

	rows, err := testPool.Query(context.Background(), `
		UPDATE booking.event_seats
		SET status = $1
		WHERE id IN (SELECT id FROM booking.event_seats ORDER BY id LIMIT $2 OFFSET $3)
		RETURNING id`, status, limit, offset)
	if err != nil {
		t.Fatalf("mark %s seats: %v", status, err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan marked seat: %v", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate marked seats: %v", err)
	}
	if len(ids) != limit {
		t.Fatalf("marked %d seats, want %d", len(ids), limit)
	}

	return ids
}

func insertVenue(t *testing.T, name string) int64 {
	t.Helper()

	var id int64
	err := testPool.QueryRow(context.Background(),
		`INSERT INTO booking.venues (name, city) VALUES ($1, 'Moscow') RETURNING id`, name).Scan(&id)
	if err != nil {
		t.Fatalf("insert venue %q: %v", name, err)
	}

	return id
}

func insertEvent(t *testing.T, venueID int64, title string) int64 {
	t.Helper()

	var id int64
	err := testPool.QueryRow(context.Background(),
		`INSERT INTO booking.events (venue_id, title, starts_at) VALUES ($1, $2, now()) RETURNING id`,
		venueID, title).Scan(&id)
	if err != nil {
		t.Fatalf("insert event %q: %v", title, err)
	}

	return id
}

func insertSeat(t *testing.T, venueID int64, section string) int64 {
	t.Helper()

	var id int64
	err := testPool.QueryRow(context.Background(),
		`INSERT INTO booking.seats (venue_id, section, row_label, seat_number) VALUES ($1, $2, '1', '1') RETURNING id`,
		venueID, section).Scan(&id)
	if err != nil {
		t.Fatalf("insert seat in section %q: %v", section, err)
	}

	return id
}

// insertEventSeat не валит тест: ожидаемые нарушения ограничений проверяет caller.
func insertEventSeat(t *testing.T, eventID, seatID, venueID, price int64, status string) error {
	t.Helper()

	if status == "" {
		_, err := testPool.Exec(context.Background(), `
			INSERT INTO booking.event_seats (event_id, seat_id, venue_id, tier, price_cents)
			VALUES ($1, $2, $3, 'standard', $4)`, eventID, seatID, venueID, price)
		return err
	}

	_, err := testPool.Exec(context.Background(), `
		INSERT INTO booking.event_seats (event_id, seat_id, venue_id, tier, price_cents, status)
		VALUES ($1, $2, $3, 'standard', $4, $5)`, eventID, seatID, venueID, price, status)

	return err
}

func requirePgError(t *testing.T, err error, wantCode string) {
	t.Helper()

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("error = %v, want *pgconn.PgError", err)
	}
	if pgErr.Code != wantCode {
		t.Errorf("SQLSTATE = %q, want %q (%v)", pgErr.Code, wantCode, err)
	}
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
