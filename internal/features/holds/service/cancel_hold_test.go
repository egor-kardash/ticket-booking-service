package service_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/egor-kardash/ticket-booking-service/internal/features/holds/domain"
	"github.com/egor-kardash/ticket-booking-service/internal/features/holds/service"
)

func TestCancelHoldPropagates(t *testing.T) {
	repoErr := fmt.Errorf("repository: %w", domain.ErrHoldNotFound)
	repo := &fakeHoldRepository{cancelErr: repoErr}
	svc := service.New(repo)

	err := svc.CancelHold(t.Context(), 42, 7)

	if !errors.Is(err, domain.ErrHoldNotFound) {
		t.Fatalf("CancelHold() error = %v, want errors.Is(ErrHoldNotFound)", err)
	}
	if err == repoErr {
		t.Error("CancelHold() returned repository error without wrapping")
	}
	if repo.cancelCalls != 1 || repo.cancelID != 42 || repo.cancelUserID != 7 {
		t.Errorf("Cancel() args = (%d, %d), calls = %d, want (42, 7), 1",
			repo.cancelID, repo.cancelUserID, repo.cancelCalls)
	}
}
