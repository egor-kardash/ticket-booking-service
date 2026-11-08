//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/egor-kardash/ticket-booking-service/internal/features/auth/domain"
	authpostgres "github.com/egor-kardash/ticket-booking-service/internal/features/auth/repository/postgres"
)

const testHash = "bcrypt-hash-value"

// resetUsers чистит таблицу до и после теста: до — чтобы хвосты упавшего
// прогона не влияли на результат, после — чтобы не зависеть от порядка тестов.
func resetUsers(t *testing.T) {
	t.Helper()

	truncate := func() {
		// context.Background, а не t.Context: тестовый контекст уже отменён,
		// когда выполняются t.Cleanup.
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

func TestCreateAndRead(t *testing.T) {
	resetUsers(t)

	repo := authpostgres.New(testPool)
	ctx := t.Context()

	const password = "secret-password"

	// Настоящий bcrypt с минимальной стоимостью: репозиторий не должен
	// ни хешировать повторно, ни обрезать значение колонкой.
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt hash: %v", err)
	}

	created, err := repo.Create(ctx, &domain.User{
		Email:        "user@example.com",
		PasswordHash: string(hash),
	})
	if err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	if created.ID <= 0 {
		t.Errorf("Create() id = %d, want positive identity value", created.ID)
	}
	if created.Email != "user@example.com" {
		t.Errorf("Create() email = %q, want %q", created.Email, "user@example.com")
	}
	if created.CreatedAt.IsZero() {
		t.Error("Create() created_at is zero, want value set by database")
	}
	if delta := time.Since(created.CreatedAt); delta > time.Minute || delta < -time.Minute {
		t.Errorf("Create() created_at = %v, want close to now()", created.CreatedAt)
	}

	byEmail, err := repo.GetByEmail(ctx, "user@example.com")
	if err != nil {
		t.Fatalf("GetByEmail() error = %v, want nil", err)
	}
	if byEmail.ID != created.ID {
		t.Errorf("GetByEmail() id = %d, want %d", byEmail.ID, created.ID)
	}
	if byEmail.PasswordHash == password {
		t.Fatal("GetByEmail() returned plaintext password, want bcrypt hash")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(byEmail.PasswordHash), []byte(password)); err != nil {
		t.Errorf("stored password hash does not match password: %v", err)
	}

	// Читаем колонку напрямую: значение должно дойти до БД без изменений.
	var stored string
	if err := testPool.QueryRow(ctx,
		"SELECT password_hash FROM booking.users WHERE id = $1", created.ID).
		Scan(&stored); err != nil {
		t.Fatalf("read stored password hash: %v", err)
	}
	if stored != string(hash) {
		t.Errorf("stored password hash differs from inserted value")
	}
	if stored == password {
		t.Error("database contains plaintext password")
	}
	if !byEmail.CreatedAt.Equal(created.CreatedAt) {
		t.Errorf("GetByEmail() created_at = %v, want %v", byEmail.CreatedAt, created.CreatedAt)
	}

	byID, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v, want nil", err)
	}
	if byID.Email != created.Email {
		t.Errorf("GetByID() email = %q, want %q", byID.Email, created.Email)
	}
	// GetByID не выбирает password_hash: хеш не должен покидать слой,
	// которому он не нужен.
	if byID.PasswordHash != "" {
		t.Errorf("GetByID() password hash = %q, want empty", byID.PasswordHash)
	}
}

func TestGetNotFound(t *testing.T) {
	resetUsers(t)

	repo := authpostgres.New(testPool)
	ctx := t.Context()

	if _, err := repo.GetByEmail(ctx, "missing@example.com"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("GetByEmail() error = %v, want errors.Is(ErrNotFound)", err)
	}
	if _, err := repo.GetByID(ctx, 404); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("GetByID() error = %v, want errors.Is(ErrNotFound)", err)
	}
}

func TestCreateEmailTaken(t *testing.T) {
	resetUsers(t)

	repo := authpostgres.New(testPool)
	ctx := t.Context()

	if _, err := repo.Create(ctx, &domain.User{Email: "dup@example.com", PasswordHash: testHash}); err != nil {
		t.Fatalf("first Create() error = %v, want nil", err)
	}

	_, err := repo.Create(ctx, &domain.User{Email: "dup@example.com", PasswordHash: testHash})
	if !errors.Is(err, domain.ErrEmailTaken) {
		t.Fatalf("second Create() error = %v, want errors.Is(ErrEmailTaken)", err)
	}
	if count := countUsers(t); count != 1 {
		t.Errorf("users count = %d, want 1: duplicate must not be inserted", count)
	}
}

// TestCreateEnforcesLowercaseInvariant фиксирует CHECK из миграции:
// репозиторий нормализацию не делает, её обеспечивает схема.
func TestCreateEnforcesLowercaseInvariant(t *testing.T) {
	resetUsers(t)

	repo := authpostgres.New(testPool)
	ctx := t.Context()

	_, err := repo.Create(ctx, &domain.User{Email: "User@Example.COM", PasswordHash: testHash})
	if err == nil {
		t.Fatal("Create() = nil, want CHECK constraint violation")
	}
	if errors.Is(err, domain.ErrEmailTaken) {
		t.Error("CHECK violation is not a duplicate: mapping would answer 409 instead of 500")
	}
	if count := countUsers(t); count != 0 {
		t.Errorf("users count = %d, want 0: failed insert left a row", count)
	}
}
