package service_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/egor-kardash/ticket-booking-service/internal/features/events/domain"
	"github.com/egor-kardash/ticket-booking-service/internal/features/events/service"
)

func TestGetEvent(t *testing.T) {
	t.Run("returns event", func(t *testing.T) {
		details := &domain.EventDetails{ID: 7, Title: "Concert"}
		repo := &fakeEventRepository{getByIDResult: details}
		svc := service.New(repo)

		got, err := svc.GetEvent(t.Context(), 7)
		if err != nil {
			t.Fatalf("GetEvent() error = %v, want nil", err)
		}
		if got != details {
			t.Errorf("GetEvent() = %+v, want %+v", got, details)
		}
		if repo.getByIDArg != 7 || repo.getByIDCalls != 1 {
			t.Errorf("GetByID() arg = %d, calls = %d, want 7 and 1", repo.getByIDArg, repo.getByIDCalls)
		}
	})

	t.Run("preserves ErrEventNotFound", func(t *testing.T) {
		repoErr := fmt.Errorf("query event: %w", domain.ErrEventNotFound)
		repo := &fakeEventRepository{getByIDErr: repoErr}
		svc := service.New(repo)

		got, err := svc.GetEvent(t.Context(), 7)
		if got != nil {
			t.Errorf("GetEvent() = %+v, want nil", got)
		}
		if !errors.Is(err, domain.ErrEventNotFound) {
			t.Fatalf("GetEvent() error = %v, want errors.Is(ErrEventNotFound)", err)
		}
		if err == repoErr {
			t.Error("GetEvent() returned repository error without wrapping")
		}
	})

	t.Run("wraps internal error", func(t *testing.T) {
		dbErr := errors.New("connection refused")
		repoErr := fmt.Errorf("query event: %w", dbErr)
		repo := &fakeEventRepository{getByIDErr: repoErr}
		svc := service.New(repo)

		got, err := svc.GetEvent(t.Context(), 7)
		if got != nil {
			t.Errorf("GetEvent() = %+v, want nil", got)
		}
		if !errors.Is(err, dbErr) {
			t.Fatalf("GetEvent() error = %v, want errors.Is(dbErr)", err)
		}
		if err == repoErr {
			t.Error("GetEvent() returned repository error without wrapping")
		}
	})
}
