// Package testdb содержит помощники для интеграционных тестов,
// которые работают с реальной PostgreSQL.
package testdb

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// lockKey сериализует интеграционные пакеты, которые делят одну БД:
// Go запускает пакеты параллельно, а booking.users нельзя использовать одновременно.
// Значение произвольное, но обязано совпадать во всех пакетах, поэтому живёт здесь.
const lockKey int64 = 0x54424B // "TBK"

// lockTimeout ограничивает ожидание advisory lock.
const lockTimeout = 2 * time.Minute

// Config — параметры подключения к тестовой БД из TEST_POSTGRES_*.
type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	Database string
	SSLMode  string
}

// ConfigFromEnv читает TEST_POSTGRES_* и отказывается работать с базой,
// имя которой не оканчивается на _test.
func ConfigFromEnv() (Config, error) {
	database := os.Getenv("TEST_POSTGRES_DB")
	if database == "" {
		return Config{}, errors.New("TEST_POSTGRES_DB is required; run make test-integration")
	}
	if !strings.HasSuffix(database, "_test") {
		return Config{}, fmt.Errorf("TEST_POSTGRES_DB = %q: database name must end with _test", database)
	}

	return Config{
		Host:     envOr("TEST_POSTGRES_HOST", "127.0.0.1"),
		Port:     envOr("TEST_POSTGRES_PORT", "5433"),
		User:     os.Getenv("TEST_POSTGRES_USER"),
		Password: os.Getenv("TEST_POSTGRES_PASSWORD"),
		Database: database,
		SSLMode:  envOr("TEST_POSTGRES_SSLMODE", "disable"),
	}, nil
}

// DSN собирает URL для указанной базы. Креды кодируются через url.UserPassword,
// поэтому спецсимволы в пароле не ломают разбор.
func (c Config) DSN(database string) string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.User, c.Password),
		Host:   net.JoinHostPort(c.Host, c.Port),
		Path:   "/" + database,
	}

	q := u.Query()
	q.Set("sslmode", c.SSLMode)
	u.RawQuery = q.Encode()

	return u.String()
}

// RequireDatabase проверяет фактическую базу подключения, а не намерение из env.
func RequireDatabase(ctx context.Context, pool *pgxpool.Pool) error {
	var name string
	if err := pool.QueryRow(ctx, "SELECT current_database()").Scan(&name); err != nil {
		return fmt.Errorf("read current_database: %w", err)
	}
	if !strings.HasSuffix(name, "_test") {
		return fmt.Errorf("refusing to run against database %q: name must end with _test", name)
	}

	return nil
}

// RequireSchema проверяет, что миграции применены: таблица видна в каталоге.
func RequireSchema(ctx context.Context, pool *pgxpool.Pool, table string) error {
	var migrated bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", table).Scan(&migrated); err != nil {
		return fmt.Errorf("check schema: %w", err)
	}
	if !migrated {
		return fmt.Errorf("schema is not migrated: %q is missing; run make test-integration", table)
	}

	return nil
}

// AcquireLock берёт session-level advisory lock на отдельном соединении.
// Пакет держит его всё время выполнения тестов, поэтому пакеты не мешают друг другу.
func AcquireLock(ctx context.Context, pool *pgxpool.Pool) (func(), error) {
	lockCtx, cancel := context.WithTimeout(ctx, lockTimeout)
	defer cancel()

	conn, err := pool.Acquire(lockCtx)
	if err != nil {
		return nil, fmt.Errorf("acquire lock connection: %w", err)
	}

	if _, err := conn.Exec(lockCtx, "SELECT pg_advisory_lock($1)", lockKey); err != nil {
		conn.Release()
		return nil, fmt.Errorf("acquire advisory lock: %w", err)
	}

	return func() {
		// Background: тестовый контекст уже отменён, а разблокировать нужно.
		_, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", lockKey)
		conn.Release()
	}, nil
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}
