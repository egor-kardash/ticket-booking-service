package service_test

import (
	"context"

	"github.com/egor-kardash/ticket-booking-service/internal/features/events/domain"
)

type fakeEventRepository struct {
	listResult []domain.EventListItem
	listNext   *domain.EventCursor
	listErr    error

	listLimit  int
	listCursor *domain.EventCursor
	listCalls  int

	getByIDResult *domain.EventDetails
	getByIDErr    error
	getByIDArg    int64
	getByIDCalls  int

	seatsResult []domain.EventSeatItem
	seatsErr    error
	seatsArg    int64
	seatsCalls  int

	existsResult bool
	existsErr    error
	existsArg    int64
	existsCalls  int
}

func (f *fakeEventRepository) List(_ context.Context, limit int, cursor *domain.EventCursor) ([]domain.EventListItem, *domain.EventCursor, error) {
	f.listCalls++
	f.listLimit = limit
	f.listCursor = cursor

	return f.listResult, f.listNext, f.listErr
}

func (f *fakeEventRepository) GetByID(_ context.Context, id int64) (*domain.EventDetails, error) {
	f.getByIDCalls++
	f.getByIDArg = id

	return f.getByIDResult, f.getByIDErr
}

func (f *fakeEventRepository) ListSeats(_ context.Context, eventID int64) ([]domain.EventSeatItem, error) {
	f.seatsCalls++
	f.seatsArg = eventID

	return f.seatsResult, f.seatsErr
}

func (f *fakeEventRepository) Exists(_ context.Context, eventID int64) (bool, error) {
	f.existsCalls++
	f.existsArg = eventID

	return f.existsResult, f.existsErr
}
