package domain

import (
	"errors"
	"fmt"
)

var (
	ErrEventNotFound = errors.New("event not found")
)

type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed: %d field(s)", len(e.Fields))
}
