package http_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/egor-kardash/ticket-booking-service/internal/features/auth/domain"
	"github.com/egor-kardash/ticket-booking-service/internal/features/auth/service"
)

// TestMalformedJSONIsRejectedBeforeService проверяет wiring, а не декодер:
// каждый публичный хендлер обязан разобрать тело до вызова сервиса.
func TestMalformedJSONIsRejectedBeforeService(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		calls func(*fakeUserService) int
	}{
		{
			name:  "register",
			path:  "/api/v1/auth/register",
			calls: func(s *fakeUserService) int { return s.registerCalls },
		},
		{
			name:  "login",
			path:  "/api/v1/auth/login",
			calls: func(s *fakeUserService) int { return s.loginCalls },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Успешные результаты нужны, чтобы при регрессе хендлер не упал
			// в nil dereference, а тест показал понятное расхождение.
			svc := &fakeUserService{
				registerResult: &domain.User{ID: testUserID, Email: testEmail},
				loginResult:    &service.LoginOutput{AccessToken: "token", ExpiresIn: time.Minute},
			}
			api := newTestAPI(svc, nil)

			rec := serve(t, api, postJSON(t, tt.path, `{"email":"user@example.com"`))

			if got := tt.calls(svc); got != 0 {
				t.Fatalf("service calls = %d, want 0: body must be decoded before service call", got)
			}
			requireError(t, rec, http.StatusBadRequest, "invalid_json")
		})
	}
}
