package domain

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	MaxVenueNameChars = 120
	MaxVenueCityChars = 120
)

type Venue struct {
	ID   int64
	Name string
	City string
}

func NewVenue(name, city string) (*Venue, error) {
	name = strings.TrimSpace(name)
	city = strings.TrimSpace(city)

	fields := make(map[string]string, 2)

	switch {
	case name == "":
		fields["name"] = "must not be blank"
	case utf8.RuneCountInString(name) > MaxVenueNameChars:
		fields["name"] = fmt.Sprintf("must be at most %d characters", MaxVenueNameChars)
	}

	switch {
	case city == "":
		fields["city"] = "must not be blank"
	case utf8.RuneCountInString(city) > MaxVenueCityChars:
		fields["city"] = fmt.Sprintf("must be at most %d characters", MaxVenueCityChars)
	}

	if len(fields) > 0 {
		return nil, &ValidationError{Fields: fields}
	}

	return &Venue{
		Name: name,
		City: city,
	}, nil
}
