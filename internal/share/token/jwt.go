package token

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type JWTManager struct {
	secretKey     []byte
	tokenDuration time.Duration
	now           func() time.Time
}

func New(secretKey string, duration time.Duration) *JWTManager {
	return &JWTManager{
		secretKey:     []byte(secretKey),
		tokenDuration: duration,
		now:           time.Now,
	}
}

func (m *JWTManager) IssueToken(userID int64) (string, time.Duration, error) {
	now := m.now()
	issuedAt := jwt.NewNumericDate(now)
	expiresAt := jwt.NewNumericDate(now.Add(m.tokenDuration))

	claims := jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(userID, 10),
		IssuedAt:  issuedAt,
		ExpiresAt: expiresAt,
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secretKey)
	if err != nil {
		return "", 0, fmt.Errorf("sign token: %w", err)
	}

	return signed, expiresAt.Sub(issuedAt.Time), nil
}

func (m *JWTManager) VerifyToken(tokenStr string) (int64, error) {
	claims := &jwt.RegisteredClaims{}

	_, err := jwt.ParseWithClaims(tokenStr, claims,
		func(token *jwt.Token) (any, error) {
			if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, fmt.Errorf("unexpected signing method: %s", token.Method.Alg())
			}
			return m.secretKey, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithTimeFunc(m.now),
	)
	if err != nil {
		return 0, fmt.Errorf("parse token: %w", err)
	}

	if claims.IssuedAt == nil {
		return 0, errors.New("iat claim is required")
	}

	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse sub claim: %w", err)
	}
	if userID <= 0 {
		return 0, errors.New("sub claim must be positive")
	}

	return userID, nil
}
