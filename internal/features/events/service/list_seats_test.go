package service_test

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/egor-kardash/ticket-booking-service/internal/features/events/domain"
	"github.com/egor-kardash/ticket-booking-service/internal/features/events/service"
)

func TestListSeats(t *testing.T) {
	seats := []domain.EventSeatItem{{ID: 1, Status: domain.EventSeatAvailable}}

	t.Run("non-empty list skips Exists", func(t *testing.T) {
		repo := &fakeEventRepository{seatsResult: seats}
		svc := service.New(repo)

		got, err := svc.ListSeats(t.Context(), 7)
		if err != nil {
			t.Fatalf("ListSeats() error = %v, want nil", err)
		}
		if !reflect.DeepEqual(got, seats) {
			t.Errorf("ListSeats() = %+v, want %+v", got, seats)
		}
		if repo.seatsArg != 7 || repo.seatsCalls != 1 {
			t.Errorf("ListSeats() arg = %d, calls = %d, want 7 and 1", repo.seatsArg, repo.seatsCalls)
		}
		if repo.existsCalls != 0 {
			t.Errorf("Exists() calls = %d, want 0 for non-empty list", repo.existsCalls)
		}
	})

	t.Run("empty list for existing event", func(t *testing.T) {
		repo := &fakeEventRepository{
			seatsResult:  []domain.EventSeatItem{},
			existsResult: true,
		}
		svc := service.New(repo)

		got, err := svc.ListSeats(t.Context(), 7)
		if err != nil {
			t.Fatalf("ListSeats() error = %v, want nil", err)
		}
		if len(got) != 0 {
			t.Errorf("ListSeats() = %+v, want empty list", got)
		}
		if repo.existsArg != 7 || repo.existsCalls != 1 {
			t.Errorf("Exists() arg = %d, calls = %d, want 7 and 1", repo.existsArg, repo.existsCalls)
		}
	})

	t.Run("empty list for missing event", func(t *testing.T) {
		repo := &fakeEventRepository{
			seatsResult:  []domain.EventSeatItem{},
			existsResult: false,
		}
		svc := service.New(repo)

		got, err := svc.ListSeats(t.Context(), 7)
		if got != nil {
			t.Errorf("ListSeats() = %+v, want nil", got)
		}
		if !errors.Is(err, domain.ErrEventNotFound) {
			t.Fatalf("ListSeats() error = %v, want errors.Is(ErrEventNotFound)", err)
		}
	})

	t.Run("propagates Exists error", func(t *testing.T) {
		dbErr := errors.New("connection refused")
		repo := &fakeEventRepository{
			seatsResult: []domain.EventSeatItem{},
			existsErr:   fmt.Errorf("check exists: %w", dbErr),
		}
		svc := service.New(repo)

		got, err := svc.ListSeats(t.Context(), 7)
		if got != nil {
			t.Errorf("ListSeats() = %+v, want nil", got)
		}
		if !errors.Is(err, dbErr) {
			t.Fatalf("ListSeats() error = %v, want errors.Is(dbErr)", err)
		}
		if errors.Is(err, domain.ErrEventNotFound) {
			t.Error("Exists error must not map to ErrEventNotFound")
		}
	})
}
