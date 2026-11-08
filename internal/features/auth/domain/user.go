package domain

import (
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxEmailBytes    = 254
	MinPasswordChars = 8
	MaxPasswordBytes = 72
)

type User struct {
	ID           int64
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

func NewUser(email, password string) (*User, error) {
	email = NormalizeEmail(email)

	fields := make(map[string]string, 2)

	switch {
	case email == "":
		fields["email"] = "must not be blank"
	case len(email) > MaxEmailBytes:
		fields["email"] = fmt.Sprintf("must be at most %d bytes", MaxEmailBytes)
	case !isValidEmail(email):
		fields["email"] = "invalid email address"
	}

	switch {
	case password == "":
		fields["password"] = "must not be blank"
	case utf8.RuneCountInString(password) < MinPasswordChars:
		fields["password"] = fmt.Sprintf("must be at least %d characters", MinPasswordChars)
	case len(password) > MaxPasswordBytes:
		fields["password"] = fmt.Sprintf("must be at most %d bytes", MaxPasswordBytes)
	}

	if len(fields) > 0 {
		return nil, &ValidationError{Fields: fields}
	}

	return &User{
		Email: email,
	}, nil
}

func isValidEmail(email string) bool {
	addr, err := mail.ParseAddress(email)
	if err != nil {
		return false
	}

	return addr.Address == email
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
