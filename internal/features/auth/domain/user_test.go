package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/egor-kardash/ticket-booking-service/internal/features/auth/domain"
)

// emailOfBytes строит синтаксически корректный адрес ровно из n байт.
// Границы 254/255 проверяются только тогда, когда вход действительно такой длины.
func emailOfBytes(t *testing.T, n int) string {
	t.Helper()

	local := strings.Repeat("a", 64)
	rest := n - len(local) - 1

	var labels []string
	for rest > 0 {
		size := min(rest, 63)
		labels = append(labels, strings.Repeat("b", size))
		rest -= size + 1
	}

	email := local + "@" + strings.Join(labels, ".")
	if len(email) != n {
		t.Fatalf("emailOfBytes(%d) built %d bytes", n, len(email))
	}

	return email
}

func TestNewUser(t *testing.T) {
	const (
		validEmail    = "user@example.com"
		validPassword = "password"
	)

	tests := []struct {
		name         string
		email        string
		password     string
		wantEmail    string
		wantFields   []string
		wantMessages map[string]string
	}{
		{name: "trims and lowercases email", email: " User@Example.COM ", password: validPassword, wantEmail: validEmail},

		// Текст важен: пустой после трима email должен отличаться от невалидного адреса.
		{name: "rejects whitespace-only email", email: "   ", password: validPassword, wantFields: []string{"email"}, wantMessages: map[string]string{"email": "must not be blank"}},
		{name: "rejects email without at sign", email: "userexample.com", password: validPassword, wantFields: []string{"email"}},
		{name: "accepts email of 254 bytes", email: emailOfBytes(t, 254), password: validPassword, wantEmail: emailOfBytes(t, 254)},
		{name: "rejects email of 255 bytes", email: emailOfBytes(t, 255), password: validPassword, wantFields: []string{"email"}},

		{name: "rejects empty password", email: validEmail, password: "", wantFields: []string{"password"}},
		{name: "rejects password of 7 characters", email: validEmail, password: strings.Repeat("p", 7), wantFields: []string{"password"}},
		{name: "accepts password of 8 characters", email: validEmail, password: strings.Repeat("p", 8), wantEmail: validEmail},
		{name: "accepts 8 cyrillic characters", email: validEmail, password: strings.Repeat("п", 8), wantEmail: validEmail},
		{name: "rejects 7 cyrillic characters", email: validEmail, password: strings.Repeat("п", 7), wantFields: []string{"password"}},
		{name: "accepts password of 72 bytes", email: validEmail, password: strings.Repeat("p", 72), wantEmail: validEmail},
		{name: "rejects password of 73 bytes", email: validEmail, password: strings.Repeat("p", 73), wantFields: []string{"password"}},

		{name: "does not trim password whitespace", email: validEmail, password: " 1234567 ", wantEmail: validEmail},
		// Второй текст, который различает поведение: 7 пробелов — не blank, а слишком коротко.
		{name: "treats 7 spaces as too short, not blank", email: validEmail, password: "       ", wantFields: []string{"password"}, wantMessages: map[string]string{"password": "must be at least 8 characters"}},

		{name: "reports both invalid fields at once", email: "not-an-email", password: "short", wantFields: []string{"email", "password"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := domain.NewUser(tt.email, tt.password)

			if len(tt.wantFields) == 0 {
				if err != nil {
					t.Fatalf("NewUser() error = %v, want nil", err)
				}
				if user == nil || user.Email != tt.wantEmail {
					t.Fatalf("NewUser() user = %+v, want email %q", user, tt.wantEmail)
				}
				return
			}

			if err == nil {
				t.Fatalf("NewUser() = %+v, want ValidationError", user)
			}
			if user != nil {
				t.Errorf("NewUser() user = %+v, want nil on validation error", user)
			}

			var validationErr *domain.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("NewUser() error = %T(%v), want *domain.ValidationError", err, err)
			}
			if len(validationErr.Fields) != len(tt.wantFields) {
				t.Fatalf("ValidationError.Fields = %v, want exactly %v", validationErr.Fields, tt.wantFields)
			}
			for _, field := range tt.wantFields {
				if _, ok := validationErr.Fields[field]; !ok {
					t.Fatalf("ValidationError.Fields = %v, missing %q", validationErr.Fields, field)
				}
			}
			for field, want := range tt.wantMessages {
				if got := validationErr.Fields[field]; got != want {
					t.Errorf("ValidationError.Fields[%q] = %q, want %q", field, got, want)
				}
			}
		})
	}
}
