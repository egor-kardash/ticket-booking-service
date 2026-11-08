package http_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/egor-kardash/ticket-booking-service/internal/features/auth/domain"
	"github.com/egor-kardash/ticket-booking-service/internal/features/auth/service"
)

func loginRequest(t *testing.T, api http.Handler, email, password string) *httptest.ResponseRecorder {
	t.Helper()

	body := fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)

	return serve(t, api, postJSON(t, "/api/v1/auth/login", body))
}

func TestLogin(t *testing.T) {
	t.Run("returns access token", func(t *testing.T) {
		svc := &fakeUserService{
			loginResult: &service.LoginOutput{
				AccessToken: "signed-jwt",
				ExpiresIn:   15 * time.Minute,
			},
		}
		api := newTestAPI(svc, nil)

		rec := loginRequest(t, api, testEmail, testPassword)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %q)", rec.Code, http.StatusOK, rec.Body.String())
		}
		requireNoStore(t, rec)

		var body loginBody
		decodeBody(t, rec, &body)
		if body.AccessToken != "signed-jwt" {
			t.Errorf("access_token = %q, want %q", body.AccessToken, "signed-jwt")
		}
		if body.ExpiresIn != 900 {
			t.Errorf("expires_in = %d, want 900 seconds", body.ExpiresIn)
		}
	})

	t.Run("ignores authorization header", func(t *testing.T) {
		svc := &fakeUserService{
			loginResult: &service.LoginOutput{AccessToken: "signed-jwt", ExpiresIn: time.Minute},
		}
		api := newTestAPI(svc, nil)

		req := postJSON(t, "/api/v1/auth/login",
			`{"email":"user@example.com","password":"secret-password"}`)
		req.Header.Set("Authorization", "Basic Z2FyYmFnZTo=")
		rec := serve(t, api, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d: login must not depend on Authorization", rec.Code, http.StatusOK)
		}
	})

	t.Run("does not reveal whether email exists", func(t *testing.T) {
		// Сервис возвращает одну и ту же ошибку в обоих случаях;
		// транспорт обязан не добавлять различий.
		unknownEmail := loginRequest(t,
			newTestAPI(&fakeUserService{loginErr: domain.ErrInvalidCredentials}, nil),
			"nobody@example.com", testPassword)
		wrongPassword := loginRequest(t,
			newTestAPI(&fakeUserService{loginErr: domain.ErrInvalidCredentials}, nil),
			testEmail, "wrong-password")

		requireError(t, unknownEmail, http.StatusUnauthorized, "invalid_credentials")
		requireError(t, wrongPassword, http.StatusUnauthorized, "invalid_credentials")

		if !bytes.Equal(unknownEmail.Body.Bytes(), wrongPassword.Body.Bytes()) {
			t.Errorf("responses differ:\nunknown email: %s\nwrong password: %s",
				unknownEmail.Body.String(), wrongPassword.Body.String())
		}
	})

}
