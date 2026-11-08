package service_test

import (
	"errors"
	"testing"

	"github.com/egor-kardash/ticket-booking-service/internal/features/auth/domain"
	"github.com/egor-kardash/ticket-booking-service/internal/features/auth/service"
)

func TestGetMe(t *testing.T) {
	t.Run("returns user by id", func(t *testing.T) {
		want := &domain.User{ID: testUserID, Email: testEmail}
		repo := &fakeUserRepository{getByIDResult: want}
		svc := service.New(repo, &fakePasswordHasher{}, &fakeTokenIssuer{})

		user, err := svc.GetMe(t.Context(), testUserID)
		if err != nil {
			t.Fatalf("GetMe() error = %v, want nil", err)
		}
		if user != want {
			t.Fatalf("GetMe() user = %+v, want %+v", user, want)
		}
		if repo.getByIDArg != testUserID {
			t.Errorf("GetByID() id = %d, want %d", repo.getByIDArg, testUserID)
		}
	})

	t.Run("preserves ErrNotFound through wrapping", func(t *testing.T) {
		repo := &fakeUserRepository{getByIDErr: errors.Join(domain.ErrNotFound, errors.New("query"))}
		svc := service.New(repo, &fakePasswordHasher{}, &fakeTokenIssuer{})

		user, err := svc.GetMe(t.Context(), testUserID)
		if user != nil {
			t.Errorf("GetMe() user = %+v, want nil", user)
		}
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("GetMe() error = %v, want errors.Is(ErrNotFound)", err)
		}
	})
}
