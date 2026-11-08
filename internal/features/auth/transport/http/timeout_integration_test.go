//go:build integration

package http_test

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// TestIntegrationDBTimeoutReturns503 держит ACCESS EXCLUSIVE на booking.users
// дольше dbTimeout хендлера: INSERT не успевает выполниться, контекст истекает.
func TestIntegrationDBTimeoutReturns503(t *testing.T) {
	resetUsers(t)

	env := newIntegrationEnv(t)
	ctx := context.Background()

	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin lock transaction: %v", err)
	}

	locked := true
	unlock := func() {
		if !locked {
			return
		}
		locked = false

		if err := tx.Rollback(context.Background()); err != nil {
			t.Errorf("release table lock: %v", err)
		}
	}
	t.Cleanup(unlock)

	if _, err := tx.Exec(ctx, "LOCK TABLE booking.users IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatalf("lock table: %v", err)
	}

	type attempt struct {
		resp response
		err  error
	}

	done := make(chan attempt, 1)
	go func() {
		resp, err := env.tryRegister("timeout@example.com", integrationPassword)
		done <- attempt{resp: resp, err: err}
	}()

	var result attempt
	select {
	case result = <-done:
	case <-time.After(10 * time.Second):
		unlock()
		t.Fatal("register did not return after handler timeout: goroutine is stuck")
	}

	if result.err != nil {
		t.Fatalf("request error: %v", result.err)
	}
	if result.resp.status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d (body %q)",
			result.resp.status, http.StatusServiceUnavailable, result.resp.body)
	}
	var envelope errorEnvelope
	decodeJSONResponse(t, result.resp, &envelope)
	if envelope.Error.Code != "service_unavailable" {
		t.Errorf("error code = %q, want service_unavailable", envelope.Error.Code)
	}
	if envelope.Error.Message != "service unavailable" {
		t.Errorf("message = %q, want generic message", envelope.Error.Message)
	}

	unlock()

	// Отменённый INSERT не должен оставить строку.
	if count := countUsers(t); count != 0 {
		t.Errorf("users count = %d, want 0 after canceled insert", count)
	}

	// Пул и сервер живы: следующий запрос проходит без перезапуска.
	after := env.register(t, "after-timeout@example.com", integrationPassword)
	if after.status != http.StatusCreated {
		t.Errorf("follow-up register status = %d, want %d (body %q)",
			after.status, http.StatusCreated, after.body)
	}
}
