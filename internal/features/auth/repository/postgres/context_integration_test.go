//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/egor-kardash/ticket-booking-service/internal/features/auth/domain"
	authpostgres "github.com/egor-kardash/ticket-booking-service/internal/features/auth/repository/postgres"
)

// callWithin запускает вызов в горутине: если код проигнорирует контекст,
// тест упадёт по таймауту, а не повиснет до таймаута пакета.
func callWithin(t *testing.T, limit time.Duration, call func() error) error {
	t.Helper()

	done := make(chan error, 1)
	go func() {
		done <- call()
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(limit):
		t.Fatalf("call did not return within %v after context was done", limit)

		return nil
	}
}

func TestRepositoryHonorsDoneContext(t *testing.T) {
	resetUsers(t)

	repo := authpostgres.New(testPool)

	t.Run("canceled context on GetByID", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := callWithin(t, 5*time.Second, func() error {
			_, err := repo.GetByID(ctx, 1)
			return err
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	})

	t.Run("expired deadline on Create", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()

		err := callWithin(t, 5*time.Second, func() error {
			_, err := repo.Create(ctx, &domain.User{
				Email:        "ctx@example.com",
				PasswordHash: testHash,
			})
			return err
		})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("error = %v, want context.DeadlineExceeded", err)
		}
	})
}

// TestCreateRespectsDeadlineWhileBlocked блокирует таблицу на ACCESS EXCLUSIVE,
// чтобы INSERT гарантированно ждал в БД. Это проверяет отмену уже выполняющегося
// запроса, а не только отказ от взятия соединения из пула.
func TestCreateRespectsDeadlineWhileBlocked(t *testing.T) {
	resetUsers(t)

	repo := authpostgres.New(testPool)
	ctx := context.Background()

	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin lock transaction: %v", err)
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	if _, err := tx.Exec(ctx, "LOCK TABLE booking.users IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatalf("lock table: %v", err)
	}

	callCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := repo.Create(callCtx, &domain.User{
			Email:        "blocked@example.com",
			PasswordHash: testHash,
		})
		done <- err
	}()

	select {
	case err = <-done:
	case <-time.After(5 * time.Second):
		_ = tx.Rollback(context.Background())
		t.Fatal("Create did not return after context deadline: query cancellation is ignored")
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Create() error = %v, want context.DeadlineExceeded", err)
	}

	// Снимаем блокировку до проверки count, иначе TRUNCATE в cleanup и
	// SELECT count(*) будут ждать.
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("release lock: %v", err)
	}

	if count := countUsers(t); count != 0 {
		t.Errorf("users count = %d, want 0: canceled insert left a row", count)
	}
}
