package hasher_test

import (
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/egor-kardash/ticket-booking-service/internal/share/hasher"
)

const testPassword = "correct horse battery staple"

func newHasher(t *testing.T, cost int) *hasher.Bcrypt {
	t.Helper()

	h, err := hasher.New(cost)
	if err != nil {
		t.Fatalf("New(%d) error = %v, want nil", cost, err)
	}

	return h
}

func TestNew(t *testing.T) {
	// 4 — bcrypt.MinCost, 32 — на единицу больше bcrypt.MaxCost (31).
	// Стоимость 12 проверяется в тесте конфига; здесь она была бы слишком медленной.
	tests := []struct {
		name    string
		cost    int
		wantErr bool
	}{
		{name: "accepts minimum cost", cost: 4},
		// bcrypt сам молча поднимает cost 3 до DefaultCost 10, поэтому без
		// собственной проверки New(3) вернул бы дорогой хешер вместо ошибки.
		{name: "rejects cost below minimum", cost: 3, wantErr: true},
		{name: "rejects cost above maximum", cost: 32, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := hasher.New(tt.cost)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("New(%d) = %v, want error", tt.cost, h)
				}
				if h != nil {
					t.Errorf("New(%d) hasher = %v, want nil on error", tt.cost, h)
				}
				return
			}

			if err != nil {
				t.Fatalf("New(%d) error = %v, want nil", tt.cost, err)
			}
			if h == nil {
				t.Fatalf("New(%d) hasher = nil, want non-nil", tt.cost)
			}
		})
	}
}

func TestHashAndCompare(t *testing.T) {
	h := newHasher(t, 4)

	hash, err := h.Hash(testPassword)
	if err != nil {
		t.Fatalf("Hash() error = %v, want nil", err)
	}
	if hash == testPassword {
		t.Fatal("Hash() returned plaintext password")
	}

	t.Run("uses configured cost", func(t *testing.T) {
		cost, err := bcrypt.Cost([]byte(hash))
		if err != nil {
			t.Fatalf("bcrypt.Cost() error = %v", err)
		}
		if cost != 4 {
			t.Errorf("hash cost = %d, want 4: Hash() ignored configured cost", cost)
		}
	})

	t.Run("accepts correct password", func(t *testing.T) {
		match, err := h.Compare(hash, testPassword)
		if err != nil {
			t.Fatalf("Compare() error = %v, want nil", err)
		}
		if !match {
			t.Error("Compare() match = false, want true for correct password")
		}
	})

	t.Run("rejects wrong password without error", func(t *testing.T) {
		match, err := h.Compare(hash, "wrong password")
		if err != nil {
			t.Fatalf("Compare() error = %v, want nil: mismatch is not a failure", err)
		}
		if match {
			t.Error("Compare() match = true, want false for wrong password")
		}
	})

	t.Run("reports corrupted hash as error", func(t *testing.T) {
		tests := []struct {
			name string
			hash string
		}{
			{name: "empty", hash: ""},
			{name: "not a hash", hash: "not-a-bcrypt-hash"},
			{name: "truncated header", hash: "$2a$04$"},
			{name: "invalid base64 payload", hash: "$2a$04$!!!!"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				match, err := h.Compare(tt.hash, testPassword)
				if err == nil {
					t.Fatal("Compare() error = nil, want error: corrupted hash must not look like a wrong password")
				}
				if match {
					t.Error("Compare() match = true, want false for corrupted hash")
				}
			})
		}
	})
}
