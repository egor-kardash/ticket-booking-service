package domain

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound           = errors.New("not found")
	ErrInvalidCredentials = errors.New("incorrect email or password")
	ErrEmailTaken         = errors.New("email is taken")
)

type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("user validation failed: %d field(s)", len(e.Fields))
}
