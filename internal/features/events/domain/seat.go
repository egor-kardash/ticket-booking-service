package domain

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	MaxSeatSectionChars  = 10
	MaxSeatRowLabelChars = 10
	MaxSeatNumberChars   = 10
)

type Seat struct {
	ID         int64
	VenueID    int64
	Section    string
	RowLabel   string
	SeatNumber string
}

func NewSeat(venueID int64, section, rowLabel, seatNumber string) (*Seat, error) {
	section = strings.ToUpper(strings.TrimSpace(section))
	rowLabel = strings.ToUpper(strings.TrimSpace(rowLabel))
	seatNumber = strings.ToUpper(strings.TrimSpace(seatNumber))

	fields := make(map[string]string, 4)

	if venueID <= 0 {
		fields["venue_id"] = "must be positive"
	}

	switch {
	case section == "":
		fields["section"] = "must not be blank"
	case utf8.RuneCountInString(section) > MaxSeatSectionChars:
		fields["section"] = fmt.Sprintf("must be at most %d characters", MaxSeatSectionChars)
	}

	switch {
	case rowLabel == "":
		fields["row_label"] = "must not be blank"
	case utf8.RuneCountInString(rowLabel) > MaxSeatRowLabelChars:
		fields["row_label"] = fmt.Sprintf("must be at most %d characters", MaxSeatRowLabelChars)
	}

	switch {
	case seatNumber == "":
		fields["seat_number"] = "must not be blank"
	case utf8.RuneCountInString(seatNumber) > MaxSeatNumberChars:
		fields["seat_number"] = fmt.Sprintf("must be at most %d characters", MaxSeatNumberChars)
	}

	if len(fields) > 0 {
		return nil, &ValidationError{Fields: fields}
	}

	return &Seat{
		VenueID:    venueID,
		Section:    section,
		RowLabel:   rowLabel,
		SeatNumber: seatNumber,
	}, nil
}
