package token

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	testSecret = "test-secret"
	testTTL    = 15 * time.Minute
)

var testNow = time.Date(2026, time.November, 8, 12, 0, 0, 0, time.UTC)

// newTestManager создаёт менеджер с фиксированными часами: без этого
// проверки exp/iat зависели бы от реального времени запуска.
func newTestManager() *JWTManager {
	m := New(testSecret, testTTL)
	m.now = func() time.Time { return testNow }

	return m
}

// testClaims собирает claims по нулевым значениям: zero time = claim отсутствует.
func testClaims(sub string, iat, exp time.Time) jwt.RegisteredClaims {
	claims := jwt.RegisteredClaims{Subject: sub}

	if !iat.IsZero() {
		claims.IssuedAt = jwt.NewNumericDate(iat)
	}
	if !exp.IsZero() {
		claims.ExpiresAt = jwt.NewNumericDate(exp)
	}

	return claims
}

func signToken(t *testing.T, method jwt.SigningMethod, key any, claims jwt.RegisteredClaims) string {
	t.Helper()

	signed, err := jwt.NewWithClaims(method, claims).SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	return signed
}

func TestIssueTokenAndVerify(t *testing.T) {
	m := newTestManager()

	signed, expiresIn, err := m.IssueToken(42)
	if err != nil {
		t.Fatalf("IssueToken() error = %v, want nil", err)
	}
	if signed == "" {
		t.Fatal("IssueToken() token = empty, want signed token")
	}
	if expiresIn != testTTL {
		t.Errorf("IssueToken() expiresIn = %v, want %v", expiresIn, testTTL)
	}

	userID, err := m.VerifyToken(signed)
	if err != nil {
		t.Fatalf("VerifyToken() error = %v, want nil", err)
	}
	if userID != 42 {
		t.Errorf("VerifyToken() userID = %d, want 42", userID)
	}

	t.Run("token is still valid just before ttl", func(t *testing.T) {
		m := New(testSecret, testTTL)
		m.now = func() time.Time { return testNow.Add(testTTL - time.Second) }

		if _, err := m.VerifyToken(signed); err != nil {
			t.Fatalf("VerifyToken() error = %v, want nil before expiry", err)
		}
	})

	t.Run("token is rejected after ttl", func(t *testing.T) {
		m := New(testSecret, testTTL)
		m.now = func() time.Time { return testNow.Add(testTTL + time.Second) }

		if _, err := m.VerifyToken(signed); err == nil {
			t.Fatal("VerifyToken() error = nil, want expiry error: exp does not match reported ttl")
		}
	})
}

func TestVerifyToken(t *testing.T) {
	valid := func(sub string) jwt.RegisteredClaims {
		return testClaims(sub, testNow, testNow.Add(testTTL))
	}

	tests := []struct {
		name   string
		token  string
		wantID int64
		wantOK bool
	}{
		{
			name:   "accepts token with sub 1",
			token:  signToken(t, jwt.SigningMethodHS256, []byte(testSecret), valid("1")),
			wantID: 1,
			wantOK: true,
		},
		{
			name:  "rejects expired token",
			token: signToken(t, jwt.SigningMethodHS256, []byte(testSecret), testClaims("42", testNow.Add(-2*time.Hour), testNow.Add(-time.Hour))),
		},
		{
			// Без exp токен был бы бессрочным: WithExpirationRequired обязателен.
			name:  "rejects token without exp",
			token: signToken(t, jwt.SigningMethodHS256, []byte(testSecret), testClaims("42", testNow, time.Time{})),
		},
		{
			// Парсер iat не требует, поэтому отсутствие проверяет сам VerifyToken.
			name:  "rejects token without iat",
			token: signToken(t, jwt.SigningMethodHS256, []byte(testSecret), testClaims("42", time.Time{}, testNow.Add(testTTL))),
		},
		{
			name:  "rejects token issued in the future",
			token: signToken(t, jwt.SigningMethodHS256, []byte(testSecret), testClaims("42", testNow.Add(time.Minute), testNow.Add(time.Hour))),
		},
		{
			name:  "rejects signature with another secret",
			token: signToken(t, jwt.SigningMethodHS256, []byte("other-secret"), valid("42")),
		},
		{
			name:  "rejects HS512",
			token: signToken(t, jwt.SigningMethodHS512, []byte(testSecret), valid("42")),
		},
		{
			name:  "rejects alg none",
			token: signToken(t, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, valid("42")),
		},
		{
			name:  "rejects non-numeric sub",
			token: signToken(t, jwt.SigningMethodHS256, []byte(testSecret), valid("abc")),
		},
		{
			name:  "rejects zero sub",
			token: signToken(t, jwt.SigningMethodHS256, []byte(testSecret), valid("0")),
		},
		{
			name:  "rejects negative sub",
			token: signToken(t, jwt.SigningMethodHS256, []byte(testSecret), valid("-1")),
		},
	}

	m := newTestManager()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID, err := m.VerifyToken(tt.token)

			if !tt.wantOK {
				if err == nil {
					t.Fatalf("VerifyToken() = %d, want error", userID)
				}
				if userID != 0 {
					t.Errorf("VerifyToken() userID = %d, want 0 on error", userID)
				}

				return
			}

			if err != nil {
				t.Fatalf("VerifyToken() error = %v, want nil", err)
			}
			if userID != tt.wantID {
				t.Errorf("VerifyToken() userID = %d, want %d", userID, tt.wantID)
			}
		})
	}
}
