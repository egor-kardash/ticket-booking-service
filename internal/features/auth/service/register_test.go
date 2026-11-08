package service_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/egor-kardash/ticket-booking-service/internal/features/auth/domain"
	"github.com/egor-kardash/ticket-booking-service/internal/features/auth/service"
)

func TestRegister(t *testing.T) {
	t.Run("normalises email and stores hash", func(t *testing.T) {
		created := &domain.User{ID: testUserID, Email: testEmail}
		repo := &fakeUserRepository{createResult: created}
		hasher := &fakePasswordHasher{hashResult: testHash}
		svc := service.New(repo, hasher, &fakeTokenIssuer{})

		user, err := svc.Register(t.Context(), service.RegisterInput{
			Email:    " User@Example.COM ",
			Password: testPassword,
		})
		if err != nil {
			t.Fatalf("Register() error = %v, want nil", err)
		}
		if user != created {
			t.Fatalf("Register() user = %+v, want repository result %+v", user, created)
		}
		if hasher.hashArg != testPassword {
			t.Errorf("Hash() password = %q, want raw %q", hasher.hashArg, testPassword)
		}
		if repo.createInput.Email != testEmail {
			t.Errorf("Create() email = %q, want %q", repo.createInput.Email, testEmail)
		}
		if repo.createInput.PasswordHash != testHash {
			t.Errorf("Create() password hash = %q, want %q", repo.createInput.PasswordHash, testHash)
		}
		if repo.createInput.PasswordHash == testPassword {
			t.Error("Create() received plaintext password instead of a hash")
		}
	})

	t.Run("returns validation error before hashing or writing", func(t *testing.T) {
		repo := &fakeUserRepository{}
		hasher := &fakePasswordHasher{}
		svc := service.New(repo, hasher, &fakeTokenIssuer{})

		user, err := svc.Register(t.Context(), service.RegisterInput{
			Email:    "not-an-email",
			Password: "short",
		})
		if user != nil {
			t.Errorf("Register() user = %+v, want nil", user)
		}

		var validationErr *domain.ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("Register() error = %T(%v), want *domain.ValidationError", err, err)
		}
		for _, field := range []string{"email", "password"} {
			if _, ok := validationErr.Fields[field]; !ok {
				t.Errorf("ValidationError.Fields = %v, missing %q", validationErr.Fields, field)
			}
		}

		// Порядок важен: валидация обязана идти до bcrypt и до записи.
		if hasher.hashCalls != 0 || repo.createCalls != 0 {
			t.Errorf("hash calls = %d, create calls = %d, want 0 before validation passes",
				hasher.hashCalls, repo.createCalls)
		}
	})

	t.Run("returns hasher error as internal, not validation", func(t *testing.T) {
		hashErr := errors.New("entropy source unavailable")
		repo := &fakeUserRepository{}
		svc := service.New(repo, &fakePasswordHasher{hashErr: hashErr}, &fakeTokenIssuer{})

		_, err := svc.Register(t.Context(), service.RegisterInput{
			Email:    testEmail,
			Password: testPassword,
		})
		if !errors.Is(err, hashErr) {
			t.Fatalf("Register() error = %v, want wrapped %v", err, hashErr)
		}

		var validationErr *domain.ValidationError
		if errors.As(err, &validationErr) {
			t.Fatalf("Register() error = %v, must not be ValidationError: transport would answer 400 instead of 500", err)
		}
	})
}

func TestRegisterRepositoryErrors(t *testing.T) {
	dbErr := errors.New("connection refused")

	tests := []struct {
		name    string
		repoErr error
		wantErr error
	}{
		{
			name:    "preserves ErrEmailTaken",
			repoErr: fmt.Errorf("insert user: %w", domain.ErrEmailTaken),
			wantErr: domain.ErrEmailTaken,
		},
		{
			name:    "wraps internal error",
			repoErr: fmt.Errorf("insert user: %w", dbErr),
			wantErr: dbErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := service.New(
				&fakeUserRepository{createErr: tt.repoErr},
				&fakePasswordHasher{hashResult: testHash},
				&fakeTokenIssuer{},
			)

			user, err := svc.Register(t.Context(), service.RegisterInput{
				Email:    testEmail,
				Password: testPassword,
			})
			if user != nil {
				t.Errorf("Register() user = %+v, want nil", user)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Register() error = %v, want errors.Is(%v)", err, tt.wantErr)
			}
		})
	}
}
