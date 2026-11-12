package domain_test

import (
	"testing"

	"github.com/egor-kardash/ticket-booking-service/internal/features/events/domain"
)

func TestParseEventSeatStatus(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    domain.EventSeatStatus
		wantErr bool
	}{
		{name: "available", raw: "available", want: domain.EventSeatAvailable},
		{name: "held", raw: "held", want: domain.EventSeatHeld},
		{name: "sold", raw: "sold", want: domain.EventSeatSold},
		{name: "unknown status", raw: "pending", wantErr: true},
		{name: "empty status", raw: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.ParseEventSeatStatus(tt.raw)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseEventSeatStatus(%q) error = nil, want error", tt.raw)
				}
				if got != "" {
					t.Errorf("ParseEventSeatStatus(%q) = %q, want empty on error", tt.raw, got)
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseEventSeatStatus(%q) error = %v, want nil", tt.raw, err)
			}
			if got != tt.want {
				t.Errorf("ParseEventSeatStatus(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
