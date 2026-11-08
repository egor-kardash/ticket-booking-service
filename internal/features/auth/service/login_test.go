package service_test

import (
	"errors"
	"testing"
	"time"

	"github.com/egor-kardash/ticket-booking-service/internal/features/auth/domain"
	"github.com/egor-kardash/ticket-booking-service/internal/features/auth/service"
)

func TestLogin(t *testing.T) {
	t.Run("normalises email and returns issued token", func(t *testing.T) {
		repo := &fakeUserRepository{
			getByEmailResult: &domain.User{ID: testUserID, Email: testEmail, PasswordHash: testHash},
		}
		hasher := &fakePasswordHasher{compareResult: true}
		issuer := &fakeTokenIssuer{token: "signed-jwt", expiresIn: time.Hour}
		svc := service.New(repo, hasher, issuer)

		out, err := svc.Login(t.Context(), service.LoginInput{
			Email:    " USER@Example.COM ",
			Password: testPassword,
		})
		if err != nil {
			t.Fatalf("Login() error = %v, want nil", err)
		}
		if repo.getByEmailArg != testEmail {
			t.Errorf("GetByEmail() email = %q, want %q", repo.getByEmailArg, testEmail)
		}
		if hasher.compareHashArg != testHash || hasher.comparePasswordArg != testPassword {
			t.Errorf("Compare() args = (%q, %q), want (%q, %q)",
				hasher.compareHashArg, hasher.comparePasswordArg, testHash, testPassword)
		}
		if issuer.userID != testUserID {
			t.Errorf("IssueToken() userID = %d, want %d", issuer.userID, testUserID)
		}
		if out == nil || out.AccessToken != "signed-jwt" || out.ExpiresIn != time.Hour {
			t.Errorf("Login() output = %+v, want token %q with expiry %v", out, "signed-jwt", time.Hour)
		}
	})

	t.Run("unknown email compares dummy and returns invalid credentials", func(t *testing.T) {
		repo := &fakeUserRepository{
			getByEmailErr: domain.ErrNotFound,
		}
		hasher := &fakePasswordHasher{}
		svc := service.New(repo, hasher, &fakeTokenIssuer{})

		out, err := svc.Login(t.Context(), service.LoginInput{
			Email:    testEmail,
			Password: testPassword,
		})
		if out != nil {
			t.Errorf("Login() output = %+v, want nil", out)
		}
		if !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("Login() error = %v, want ErrInvalidCredentials", err)
		}

		// Без dummy-сравнения время ответа выдавало бы существование пользователя.
		if hasher.compareDummyCalls != 1 {
			t.Errorf("CompareDummy() calls = %d, want 1", hasher.compareDummyCalls)
		}
		if hasher.compareDummyArg != testPassword {
			t.Errorf("CompareDummy() password = %q, want %q", hasher.compareDummyArg, testPassword)
		}
	})

	t.Run("wrong password returns invalid credentials without token", func(t *testing.T) {
		repo := &fakeUserRepository{
			getByEmailResult: &domain.User{ID: testUserID, Email: testEmail, PasswordHash: testHash},
		}
		issuer := &fakeTokenIssuer{}
		svc := service.New(repo, &fakePasswordHasher{compareResult: false}, issuer)

		_, err := svc.Login(t.Context(), service.LoginInput{
			Email:    testEmail,
			Password: "wrong-password",
		})
		if !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("Login() error = %v, want ErrInvalidCredentials", err)
		}
		if issuer.calls != 0 {
			t.Errorf("IssueToken() calls = %d, want 0: token must not be issued", issuer.calls)
		}
	})

	t.Run("compare error is internal, not invalid credentials", func(t *testing.T) {
		compareErr := errors.New("bcrypt: hashedSecret too short")
		repo := &fakeUserRepository{
			getByEmailResult: &domain.User{ID: testUserID, Email: testEmail, PasswordHash: "broken"},
		}
		svc := service.New(repo, &fakePasswordHasher{compareErr: compareErr}, &fakeTokenIssuer{})

		_, err := svc.Login(t.Context(), service.LoginInput{
			Email:    testEmail,
			Password: testPassword,
		})
		if !errors.Is(err, compareErr) {
			t.Fatalf("Login() error = %v, want wrapped %v", err, compareErr)
		}
		if errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatal("corrupted hash must not map to 401")
		}
	})

	t.Run("propagates token issuing error", func(t *testing.T) {
		issueErr := errors.New("sign token: key is empty")
		repo := &fakeUserRepository{
			getByEmailResult: &domain.User{ID: testUserID, Email: testEmail, PasswordHash: testHash},
		}
		issuer := &fakeTokenIssuer{err: issueErr}
		svc := service.New(repo, &fakePasswordHasher{compareResult: true}, issuer)

		out, err := svc.Login(t.Context(), service.LoginInput{
			Email:    testEmail,
			Password: testPassword,
		})
		if out != nil {
			t.Errorf("Login() output = %+v, want nil", out)
		}
		if !errors.Is(err, issueErr) {
			t.Fatalf("Login() error = %v, want wrapped %v", err, issueErr)
		}
	})
}
