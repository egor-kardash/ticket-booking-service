package service_test

import (
	"context"
	"time"

	"github.com/egor-kardash/ticket-booking-service/internal/features/auth/domain"
)

const (
	testUserID   int64 = 7
	testEmail          = "user@example.com"
	testPassword       = "secret-password"
	testHash           = "$2a$12$stub-hash-value"
)

type fakeUserRepository struct {
	createResult *domain.User
	createErr    error
	createInput  *domain.User
	createCalls  int

	getByEmailResult *domain.User
	getByEmailErr    error
	getByEmailArg    string

	getByIDResult *domain.User
	getByIDErr    error
	getByIDArg    int64
}

func (f *fakeUserRepository) Create(_ context.Context, user *domain.User) (*domain.User, error) {
	f.createCalls++
	f.createInput = user

	if f.createErr != nil {
		return nil, f.createErr
	}
	if f.createResult != nil {
		return f.createResult, nil
	}

	return user, nil
}

func (f *fakeUserRepository) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	f.getByEmailArg = email

	if f.getByEmailErr != nil {
		return nil, f.getByEmailErr
	}

	return f.getByEmailResult, nil
}

func (f *fakeUserRepository) GetByID(_ context.Context, id int64) (*domain.User, error) {
	f.getByIDArg = id

	if f.getByIDErr != nil {
		return nil, f.getByIDErr
	}

	return f.getByIDResult, nil
}

type fakePasswordHasher struct {
	hashResult string
	hashErr    error
	hashArg    string
	hashCalls  int

	compareResult      bool
	compareErr         error
	compareHashArg     string
	comparePasswordArg string

	compareDummyCalls int
	compareDummyArg   string
}

func (f *fakePasswordHasher) Hash(password string) (string, error) {
	f.hashCalls++
	f.hashArg = password

	if f.hashErr != nil {
		return "", f.hashErr
	}

	return f.hashResult, nil
}

func (f *fakePasswordHasher) Compare(hash, password string) (bool, error) {
	f.compareHashArg = hash
	f.comparePasswordArg = password

	if f.compareErr != nil {
		return false, f.compareErr
	}

	return f.compareResult, nil
}

func (f *fakePasswordHasher) CompareDummy(password string) {
	f.compareDummyCalls++
	f.compareDummyArg = password
}

type fakeTokenIssuer struct {
	token     string
	expiresIn time.Duration
	err       error

	userID int64
	calls  int
}

func (f *fakeTokenIssuer) IssueToken(userID int64) (string, time.Duration, error) {
	f.calls++
	f.userID = userID

	if f.err != nil {
		return "", 0, f.err
	}

	return f.token, f.expiresIn, nil
}
