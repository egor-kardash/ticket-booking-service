package service_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/egor-kardash/ticket-booking-service/internal/features/holds/domain"
	"github.com/egor-kardash/ticket-booking-service/internal/features/holds/service"
)

func TestCreateHoldValidationShortCircuits(t *testing.T) {
	t.Run("invalid input does not touch repository", func(t *testing.T) {
		tests := []struct {
			name       string
			userID     int64
			eventSeat  int64
			wantFields []string
		}{
			{name: "zero user", userID: 0, eventSeat: 1, wantFields: []string{"user_id"}},
			{name: "zero seat", userID: 1, eventSeat: 0, wantFields: []string{"event_seat_id"}},
			{name: "both invalid", userID: -1, eventSeat: -1, wantFields: []string{"user_id", "event_seat_id"}},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				repo := &fakeHoldRepository{}
				svc := service.New(repo)

				hold, err := svc.CreateHold(t.Context(), tt.userID, tt.eventSeat)
				if hold != nil {
					t.Errorf("CreateHold() = %+v, want nil", hold)
				}

				var validationErr *domain.ValidationError
				if !errors.As(err, &validationErr) {
					t.Fatalf("CreateHold() error = %T(%v), want *ValidationError", err, err)
				}
				if len(validationErr.Fields) != len(tt.wantFields) {
					t.Fatalf("Fields = %v, want exactly %v", validationErr.Fields, tt.wantFields)
				}
				for _, field := range tt.wantFields {
					if _, ok := validationErr.Fields[field]; !ok {
						t.Errorf("Fields = %v, missing %q", validationErr.Fields, field)
					}
				}

				if repo.createCalls != 0 {
					t.Errorf("Create() calls = %d, want 0 before validation passes", repo.createCalls)
				}
			})
		}
	})

	t.Run("valid input reaches repository", func(t *testing.T) {
		want := &domain.HoldDetails{ID: 10, EventSeatID: 11}
		repo := &fakeHoldRepository{createResult: want}
		svc := service.New(repo)

		hold, err := svc.CreateHold(t.Context(), 7, 11)
		if err != nil {
			t.Fatalf("CreateHold() error = %v, want nil", err)
		}
		if hold != want {
			t.Errorf("CreateHold() = %+v, want repository result", hold)
		}
		if repo.createCalls != 1 || repo.createUserID != 7 || repo.createEventSeatID != 11 {
			t.Errorf("Create() args = (%d, %d), calls = %d, want (7, 11), 1",
				repo.createUserID, repo.createEventSeatID, repo.createCalls)
		}
		if repo.createTTL != 10*time.Minute {
			t.Errorf("Create() ttl = %v, want 10m", repo.createTTL)
		}
	})

	t.Run("repository error is wrapped", func(t *testing.T) {
		dbErr := errors.New("connection refused")
		repoErr := fmt.Errorf("insert hold: %w", dbErr)
		repo := &fakeHoldRepository{createErr: repoErr}
		svc := service.New(repo)

		_, err := svc.CreateHold(t.Context(), 7, 11)

		if !errors.Is(err, dbErr) {
			t.Fatalf("CreateHold() error = %v, want errors.Is(dbErr)", err)
		}
		if err == repoErr {
			t.Error("CreateHold() returned repository error without wrapping")
		}
	})
}
