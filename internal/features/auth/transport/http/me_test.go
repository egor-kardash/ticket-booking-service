package http_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/egor-kardash/ticket-booking-service/internal/features/auth/domain"
)

func meRequest(t *testing.T, api http.Handler, authorization string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}

	return serve(t, api, req)
}

func TestMe(t *testing.T) {
	t.Run("returns current user", func(t *testing.T) {
		svc := &fakeUserService{
			getMeResult: &domain.User{
				ID:           testUserID,
				Email:        testEmail,
				PasswordHash: "bcrypt-hash-value",
			},
		}
		api := newTestAPI(svc, &fakeTokenVerifier{userID: testUserID})

		rec := meRequest(t, api, "Bearer valid-token")

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %q)", rec.Code, http.StatusOK, rec.Body.String())
		}
		requireNoStore(t, rec)
		requireNoLeak(t, rec, "bcrypt-hash-value", "password_hash")

		var body userBody
		decodeBody(t, rec, &body)
		if body.ID != testUserID || body.Email != testEmail {
			t.Errorf("body = %+v, want {ID:%d Email:%q}", body, testUserID, testEmail)
		}
		if svc.getMeID != testUserID {
			t.Errorf("GetMe() id = %d, want %d from auth context", svc.getMeID, testUserID)
		}
	})

	t.Run("returns internal error without user id in context", func(t *testing.T) {
		svc := &fakeUserService{}
		// verifier == nil: protected-роут смонтирован без Auth, контекст пустой.
		api := newTestAPI(svc, nil)

		rec := meRequest(t, api, "")

		requireError(t, rec, http.StatusInternalServerError, "internal_error")
	})

	t.Run("maps missing user to not found", func(t *testing.T) {
		// Обёртка как в сервисе: errors.Is должен пройти цепочку.
		svc := &fakeUserService{getMeErr: fmt.Errorf("get me: %w", domain.ErrNotFound)}
		api := newTestAPI(svc, &fakeTokenVerifier{userID: testUserID})

		rec := meRequest(t, api, "Bearer valid-token")

		requireError(t, rec, http.StatusNotFound, "not_found")
	})

}
