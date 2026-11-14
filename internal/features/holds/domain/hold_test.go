package domain_test

import (
	"errors"
	"testing"

	"github.com/egor-kardash/ticket-booking-service/internal/features/holds/domain"
)

func TestValidateNewHold(t *testing.T) {
	tests := []struct {
		name       string
		userID     int64
		eventSeat  int64
		wantFields []string
	}{
		{name: "valid", userID: 1, eventSeat: 1},
		{name: "zero user", userID: 0, eventSeat: 1, wantFields: []string{"user_id"}},
		{name: "negative user", userID: -1, eventSeat: 1, wantFields: []string{"user_id"}},
		{name: "zero event seat", userID: 1, eventSeat: 0, wantFields: []string{"event_seat_id"}},
		{name: "negative event seat", userID: 1, eventSeat: -1, wantFields: []string{"event_seat_id"}},
		{name: "both invalid", userID: 0, eventSeat: 0, wantFields: []string{"user_id", "event_seat_id"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := domain.ValidateNewHold(tt.userID, tt.eventSeat)

			if len(tt.wantFields) == 0 {
				if err != nil {
					t.Fatalf("ValidateNewHold() error = %v, want nil", err)
				}
				return
			}

			var validationErr *domain.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("ValidateNewHold() error = %T(%v), want *ValidationError", err, err)
			}
			if len(validationErr.Fields) != len(tt.wantFields) {
				t.Fatalf("Fields = %v, want exactly %v", validationErr.Fields, tt.wantFields)
			}
			for _, field := range tt.wantFields {
				if _, ok := validationErr.Fields[field]; !ok {
					t.Errorf("Fields = %v, missing %q", validationErr.Fields, field)
				}
			}
		})
	}
}
