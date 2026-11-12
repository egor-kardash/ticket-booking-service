package service_test

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/egor-kardash/ticket-booking-service/internal/features/events/domain"
	"github.com/egor-kardash/ticket-booking-service/internal/features/events/service"
)

func TestListEvents(t *testing.T) {
	items := []domain.EventListItem{{ID: 1, Title: "Concert"}}

	t.Run("rejects invalid limit before repository", func(t *testing.T) {
		for _, limit := range []int{0, 101, -1} {
			t.Run(strconv.Itoa(limit), func(t *testing.T) {
				repo := &fakeEventRepository{listResult: items}
				svc := service.New(repo)

				got, _, err := svc.ListEvents(t.Context(), limit, nil)

				if !errors.Is(err, service.ErrInvalidLimit) {
					t.Fatalf("ListEvents() error = %v, want ErrInvalidLimit", err)
				}
				if got != nil {
					t.Errorf("ListEvents() items = %+v, want nil on error", got)
				}
				if repo.listCalls != 0 {
					t.Errorf("List() calls = %d, want 0 for invalid limit", repo.listCalls)
				}
			})
		}
	})

	t.Run("passes cursor to repository unchanged", func(t *testing.T) {
		cursor := &domain.EventCursor{
			StartsAt: time.Date(2026, time.November, 12, 18, 0, 0, 0, time.UTC),
			ID:       42,
		}
		repo := &fakeEventRepository{listResult: items}
		svc := service.New(repo)

		if _, _, err := svc.ListEvents(t.Context(), 20, cursor); err != nil {
			t.Fatalf("ListEvents() error = %v, want nil", err)
		}

		if repo.listCursor != cursor {
			t.Fatalf("List() cursor = %+v, want the same pointer", repo.listCursor)
		}
		if !repo.listCursor.StartsAt.Equal(cursor.StartsAt) || repo.listCursor.ID != cursor.ID {
			t.Errorf("List() cursor = %+v, want %+v", repo.listCursor, cursor)
		}
	})

	t.Run("wraps repository error", func(t *testing.T) {
		dbErr := errors.New("connection refused")
		repoErr := fmt.Errorf("query events: %w", dbErr)
		repo := &fakeEventRepository{listErr: repoErr}
		svc := service.New(repo)

		items, next, err := svc.ListEvents(t.Context(), 20, nil)

		if !errors.Is(err, dbErr) {
			t.Fatalf("ListEvents() error = %v, want errors.Is(dbErr)", err)
		}
		if err == repoErr {
			t.Error("ListEvents() returned repository error without wrapping")
		}
		if items != nil || next != nil {
			t.Errorf("ListEvents() = (%+v, %+v), want nils on error", items, next)
		}
	})
}

// Отдельная проверка границ успешного диапазона: репозиторий получает
// ровно тот limit, который передали, и возвращает элементы без изменений.
func TestListEventsAcceptedLimits(t *testing.T) {
	items := []domain.EventListItem{{ID: 1, Title: "Concert"}}

	for _, limit := range []int{1, 20, 100} {
		t.Run(strconv.Itoa(limit), func(t *testing.T) {
			repo := &fakeEventRepository{listResult: items}
			svc := service.New(repo)

			got, _, err := svc.ListEvents(t.Context(), limit, nil)
			if err != nil {
				t.Fatalf("ListEvents() error = %v, want nil", err)
			}
			if repo.listCalls != 1 || repo.listLimit != limit {
				t.Errorf("List() calls = %d with limit %d, want 1 with %d",
					repo.listCalls, repo.listLimit, limit)
			}
			if !reflect.DeepEqual(got, items) {
				t.Errorf("ListEvents() items = %+v, want %+v", got, items)
			}
		})
	}
}
