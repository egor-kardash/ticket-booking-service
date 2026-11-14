package service_test

import (
	"context"
	"time"

	"github.com/egor-kardash/ticket-booking-service/internal/features/holds/domain"
)

type fakeHoldRepository struct {
	createResult *domain.HoldDetails
	createErr    error

	createUserID      int64
	createEventSeatID int64
	createTTL         time.Duration
	createCalls       int

	cancelErr error

	cancelID     int64
	cancelUserID int64
	cancelCalls  int
}

func (f *fakeHoldRepository) Create(_ context.Context, userID, eventSeatID int64, ttl time.Duration) (*domain.HoldDetails, error) {
	f.createCalls++
	f.createUserID = userID
	f.createEventSeatID = eventSeatID
	f.createTTL = ttl

	return f.createResult, f.createErr
}

func (f *fakeHoldRepository) GetByID(context.Context, int64, int64) (*domain.HoldDetails, error) {
	return nil, nil
}

func (f *fakeHoldRepository) Cancel(_ context.Context, id, userID int64) error {
	f.cancelCalls++
	f.cancelID = id
	f.cancelUserID = userID

	return f.cancelErr
}

func (f *fakeHoldRepository) ExpireBatch(context.Context, int) (int, error) {
	return 0, nil
}
