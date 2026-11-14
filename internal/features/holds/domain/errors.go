package domain

import (
	"errors"
	"fmt"
)

var (
	ErrHoldNotFound      = errors.New("hold not found")
	ErrEventSeatNotFound = errors.New("event seat not found")
	ErrSeatTaken         = errors.New("seat taken")
	ErrUnauthorized      = errors.New("unauthorized")
)

type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed: %d field(s)", len(e.Fields))
}
