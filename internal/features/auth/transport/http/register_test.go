package http_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/egor-kardash/ticket-booking-service/internal/features/auth/domain"
)

func TestRegister(t *testing.T) {
	t.Run("creates user", func(t *testing.T) {
		svc := &fakeUserService{
			registerResult: &domain.User{
				ID:    testUserID,
				Email: testEmail,
				// Пароль и хеш не должны попасть в ответ.
				PasswordHash: "bcrypt-hash-value",
			},
		}
		api := newTestAPI(svc, nil)

		req := postJSON(t, "/api/v1/auth/register",
			`{"email":" User@Example.COM ","password":"secret-password"}`)
		rec := serve(t, api, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d (body %q)", rec.Code, http.StatusCreated, rec.Body.String())
		}
		if got := rec.Header().Get("Location"); got != "/api/v1/auth/me" {
			t.Errorf("Location = %q, want %q", got, "/api/v1/auth/me")
		}
		requireNoStore(t, rec)
		requireNoLeak(t, rec, "bcrypt-hash-value", "password_hash", "secret-password")

		var body userBody
		decodeBody(t, rec, &body)
		if body.ID != testUserID || body.Email != testEmail {
			t.Errorf("body = %+v, want {ID:%d Email:%q}", body, testUserID, testEmail)
		}

		// Нормализация — забота домена, транспорт передаёт ввод как есть.
		if svc.registerInput.Email != " User@Example.COM " || svc.registerInput.Password != "secret-password" {
			t.Errorf("Register() input = %+v, want raw credentials", svc.registerInput)
		}
	})

	t.Run("returns validation fields", func(t *testing.T) {
		svc := &fakeUserService{
			registerErr: &domain.ValidationError{Fields: map[string]string{
				"email":    "invalid email address",
				"password": "must be at least 8 characters",
			}},
		}
		api := newTestAPI(svc, nil)

		rec := serve(t, api, postJSON(t, "/api/v1/auth/register",
			`{"email":"not-an-email","password":"short"}`))

		envelope := requireError(t, rec, http.StatusBadRequest, "validation_failed")
		want := map[string]string{
			"email":    "invalid email address",
			"password": "must be at least 8 characters",
		}
		if len(envelope.Error.Fields) != len(want) {
			t.Fatalf("fields = %v, want %v", envelope.Error.Fields, want)
		}
		for field, message := range want {
			if got := envelope.Error.Fields[field]; got != message {
				t.Errorf("fields[%q] = %q, want %q", field, got, message)
			}
		}
	})

	t.Run("maps taken email to conflict", func(t *testing.T) {
		// Обёртка как в сервисе и репозитории: errors.Is должен пройти цепочку.
		svc := &fakeUserService{
			registerErr: fmt.Errorf("register: %w", domain.ErrEmailTaken),
		}
		api := newTestAPI(svc, nil)

		rec := serve(t, api, postJSON(t, "/api/v1/auth/register",
			`{"email":"user@example.com","password":"secret-password"}`))

		requireError(t, rec, http.StatusConflict, "email_taken")
	})

}
