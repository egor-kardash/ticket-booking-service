//go:build integration

package http_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/egor-kardash/ticket-booking-service/app"
	"github.com/egor-kardash/ticket-booking-service/internal/config"
	"github.com/egor-kardash/ticket-booking-service/internal/testdb"
)

const (
	integrationPassword  = "secret-password"
	integrationJWTSecret = "0123456789abcdef0123456789abcdef"
	integrationTTL       = 15 * time.Minute
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	os.Exit(runIntegration(m))
}

func runIntegration(m *testing.M) int {
	ctx := context.Background()

	cfg, err := testdb.ConfigFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "http integration tests: %v\n", err)
		return 1
	}

	pool, err := pgxpool.New(ctx, cfg.DSN(cfg.Database))
	if err != nil {
		fmt.Fprintf(os.Stderr, "http integration tests: create pool: %v\n", err)
		return 1
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "http integration tests: ping postgres: %v\n", err)
		return 1
	}
	if err := testdb.RequireDatabase(ctx, pool); err != nil {
		fmt.Fprintf(os.Stderr, "http integration tests: %v\n", err)
		return 1
	}
	if err := testdb.RequireSchema(ctx, pool, "booking.users"); err != nil {
		fmt.Fprintf(os.Stderr, "http integration tests: %v\n", err)
		return 1
	}

	// Тесты репозитория работают с той же БД: пакеты должны идти по очереди.
	release, err := testdb.AcquireLock(ctx, pool)
	if err != nil {
		fmt.Fprintf(os.Stderr, "http integration tests: %v\n", err)
		return 1
	}
	defer release()

	testPool = pool

	return m.Run()
}

// integrationEnv повторяет проводку app.New: реальные репозиторий, сервис,
// bcrypt и JWT, только router собирается здесь, без config и TLS.
type integrationEnv struct {
	server *httptest.Server
	client *http.Client
}

func newIntegrationEnv(t *testing.T) *integrationEnv {
	t.Helper()

	// Та же проводка, что в app.New: маршруты, Auth-middleware, порядок.
	// Bcrypt cost 4 — только чтобы тесты не ждали production-стоимость.
	cfg := &config.Config{Auth: config.Auth{
		BcryptCost:    bcrypt.MinCost,
		JWTSecret:     integrationJWTSecret,
		TokenDuration: integrationTTL,
	}}

	router, err := app.NewRouter(cfg, testPool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("build router: %v", err)
	}

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	client := server.Client()
	// Без таймаута зависший запрос повесил бы весь тестовый пакет.
	client.Timeout = 10 * time.Second

	return &integrationEnv{server: server, client: client}
}

type response struct {
	status int
	header http.Header
	body   []byte
}

func (e *integrationEnv) do(t *testing.T, method, path, contentType, body, bearer string) response {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), method, e.server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	return response{status: resp.StatusCode, header: resp.Header.Clone(), body: data}
}

func (e *integrationEnv) postJSON(t *testing.T, path, body string) response {
	t.Helper()

	return e.do(t, http.MethodPost, path, "application/json", body, "")
}

func (e *integrationEnv) get(t *testing.T, path, bearer string) response {
	t.Helper()

	return e.do(t, http.MethodGet, path, "", "", bearer)
}

func (e *integrationEnv) register(t *testing.T, email, password string) response {
	t.Helper()

	return e.postJSON(t, "/api/v1/auth/register",
		fmt.Sprintf(`{"email":%q,"password":%q}`, email, password))
}

func (e *integrationEnv) login(t *testing.T, email, password string) response {
	t.Helper()

	return e.postJSON(t, "/api/v1/auth/login",
		fmt.Sprintf(`{"email":%q,"password":%q}`, email, password))
}

func decodeJSONResponse(t *testing.T, resp response, dst any) {
	t.Helper()

	if err := json.Unmarshal(resp.body, dst); err != nil {
		t.Fatalf("decode response: %v (body %q)", err, resp.body)
	}
}

func resetUsers(t *testing.T) {
	t.Helper()

	truncate := func() {
		if _, err := testPool.Exec(context.Background(), "TRUNCATE booking.users RESTART IDENTITY CASCADE"); err != nil {
			t.Errorf("truncate booking.users: %v", err)
		}
	}

	truncate()
	t.Cleanup(truncate)
}

func countUsers(t *testing.T) int {
	t.Helper()

	var count int
	if err := testPool.QueryRow(context.Background(), "SELECT count(*) FROM booking.users").Scan(&count); err != nil {
		t.Fatalf("count users: %v", err)
	}

	return count
}

func findUser(t *testing.T, email string) (int64, string) {
	t.Helper()

	var id int64
	var hash string
	if err := testPool.QueryRow(context.Background(),
		"SELECT id, password_hash FROM booking.users WHERE email = $1", email).
		Scan(&id, &hash); err != nil {
		t.Fatalf("find user %q: %v", email, err)
	}

	return id, hash
}
