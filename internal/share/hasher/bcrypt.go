package hasher

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

const dummyPassword = "dummy-password-for-timing"

type Bcrypt struct {
	cost      int
	dummyHash []byte
}

func New(cost int) (*Bcrypt, error) {
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		return nil, fmt.Errorf("bcrypt cost %d: must be in range %d..%d",
			cost, bcrypt.MinCost, bcrypt.MaxCost)
	}

	dummyHash, err := bcrypt.GenerateFromPassword([]byte(dummyPassword), cost)
	if err != nil {
		return nil, fmt.Errorf("generate dummy hash: %w", err)
	}

	return &Bcrypt{cost: cost, dummyHash: dummyHash}, nil
}

func (b *Bcrypt) Hash(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), b.cost)
	return string(bytes), err
}

func (b *Bcrypt) Compare(hash, password string) (bool, error) {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return false, nil
	}

	return false, fmt.Errorf("compare password hash: %w", err)
}

// CompareDummy выполняет bcrypt-сравнение с фиксированным хешем.
// Результат не важен, т.к. метод нужен только для выравнивания времени ответа.
func (b *Bcrypt) CompareDummy(password string) {
	_ = bcrypt.CompareHashAndPassword(b.dummyHash, []byte(password))
}
