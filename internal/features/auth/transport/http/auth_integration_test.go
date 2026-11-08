//go:build integration

package http_test

import (
	"net/http"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/egor-kardash/ticket-booking-service/internal/share/token"
)

// TestIntegrationAuthFlow проходит happy path целиком: register → login → me.
// Заодно проверяет то, что не проверить unit-тестом: нормализацию email,
// bcrypt-хеш в БД и валидность реально выпущенного токена.
func TestIntegrationAuthFlow(t *testing.T) {
	resetUsers(t)

	env := newIntegrationEnv(t)

	registered := env.register(t, " MiXeD@Example.COM ", integrationPassword)
	if registered.status != http.StatusCreated {
		t.Fatalf("register status = %d, want %d (body %q)", registered.status, http.StatusCreated, registered.body)
	}
	if got := registered.header.Get("Location"); got != "/api/v1/auth/me" {
		t.Errorf("Location = %q, want /api/v1/auth/me", got)
	}
	if got := registered.header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("register Cache-Control = %q, want no-store", got)
	}

	var profile userBody
	decodeJSONResponse(t, registered, &profile)
	if profile.ID <= 0 {
		t.Errorf("register id = %d, want positive", profile.ID)
	}
	if profile.Email != "mixed@example.com" {
		t.Errorf("register email = %q, want normalized %q", profile.Email, "mixed@example.com")
	}

	dbID, hash := findUser(t, "mixed@example.com")
	if dbID != profile.ID {
		t.Errorf("database id = %d, want %d", dbID, profile.ID)
	}
	if hash == integrationPassword {
		t.Error("database contains plaintext password")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(integrationPassword)); err != nil {
		t.Errorf("stored hash does not match password: %v", err)
	}

	login := env.login(t, "MIXED@EXAMPLE.COM", integrationPassword)
	if login.status != http.StatusOK {
		t.Fatalf("login status = %d, want %d (body %q)", login.status, http.StatusOK, login.body)
	}

	var session loginBody
	decodeJSONResponse(t, login, &session)
	if session.AccessToken == "" {
		t.Fatal("access_token is empty")
	}
	if session.ExpiresIn != 900 {
		t.Errorf("expires_in = %d, want 900 seconds", session.ExpiresIn)
	}
	if got := login.header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("login Cache-Control = %q, want no-store", got)
	}

	me := env.get(t, "/api/v1/auth/me", session.AccessToken)
	if me.status != http.StatusOK {
		t.Fatalf("me status = %d, want %d (body %q)", me.status, http.StatusOK, me.body)
	}
	if got := me.header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("me Cache-Control = %q, want no-store", got)
	}

	var current userBody
	decodeJSONResponse(t, me, &current)
	if current.ID != profile.ID || current.Email != profile.Email {
		t.Errorf("me = %+v, want {ID:%d Email:%q}", current, profile.ID, profile.Email)
	}
}

func TestIntegrationRegisterRejectsDuplicateEmail(t *testing.T) {
	resetUsers(t)

	env := newIntegrationEnv(t)

	if first := env.register(t, "user@example.com", integrationPassword); first.status != http.StatusCreated {
		t.Fatalf("first register status = %d, want %d (body %q)", first.status, http.StatusCreated, first.body)
	}

	second := env.register(t, "user@example.com", integrationPassword)
	if second.status != http.StatusConflict {
		t.Fatalf("second register status = %d, want %d (body %q)", second.status, http.StatusConflict, second.body)
	}

	var envelope errorEnvelope
	decodeJSONResponse(t, second, &envelope)
	if envelope.Error.Code != "email_taken" {
		t.Errorf("error code = %q, want email_taken", envelope.Error.Code)
	}
	if count := countUsers(t); count != 1 {
		t.Errorf("users count = %d, want 1", count)
	}
}

// TestIntegrationMeForDeletedUser фиксирует согласованное поведение:
// удаления пользователя в API пока нет, поэтому строка удаляется SQL-ом,
// а /me отвечает 404, как для любого отсутствующего ресурса.
func TestIntegrationMeForDeletedUser(t *testing.T) {
	resetUsers(t)

	env := newIntegrationEnv(t)

	if resp := env.register(t, "user@example.com", integrationPassword); resp.status != http.StatusCreated {
		t.Fatalf("register status = %d, want %d (body %q)", resp.status, http.StatusCreated, resp.body)
	}

	userID, _ := findUser(t, "user@example.com")

	login := env.login(t, "user@example.com", integrationPassword)
	if login.status != http.StatusOK {
		t.Fatalf("login status = %d, want %d (body %q)", login.status, http.StatusOK, login.body)
	}

	var session loginBody
	decodeJSONResponse(t, login, &session)

	if _, err := testPool.Exec(t.Context(), "DELETE FROM booking.users WHERE id = $1", userID); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	resp := env.get(t, "/api/v1/auth/me", session.AccessToken)
	if resp.status != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body %q)", resp.status, http.StatusNotFound, resp.body)
	}

	var envelope errorEnvelope
	decodeJSONResponse(t, resp, &envelope)
	if envelope.Error.Code != "not_found" {
		t.Errorf("error code = %q, want not_found", envelope.Error.Code)
	}
}

// TestIntegrationMeRejectsExpiredToken проверяет связку реального JWT-менеджера
// и middleware: токен с прошедшим exp не проходит /me.
func TestIntegrationMeRejectsExpiredToken(t *testing.T) {
	resetUsers(t)

	env := newIntegrationEnv(t)

	if resp := env.register(t, "user@example.com", integrationPassword); resp.status != http.StatusCreated {
		t.Fatalf("register status = %d, want %d (body %q)", resp.status, http.StatusCreated, resp.body)
	}

	userID, _ := findUser(t, "user@example.com")

	expiredIssuer := token.New(integrationJWTSecret, -time.Minute)
	expiredToken, _, err := expiredIssuer.IssueToken(userID)
	if err != nil {
		t.Fatalf("issue expired token: %v", err)
	}

	resp := env.get(t, "/api/v1/auth/me", expiredToken)
	if resp.status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body %q)", resp.status, http.StatusUnauthorized, resp.body)
	}
	if got := resp.header.Get("WWW-Authenticate"); got != `Bearer error="invalid_token"` {
		t.Errorf("WWW-Authenticate = %q, want invalid_token challenge", got)
	}
}
