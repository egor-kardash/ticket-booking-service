package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/egor-kardash/ticket-booking-service/internal/features/events/domain"
)

type EventRepository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *EventRepository {
	return &EventRepository{pool: pool}
}

func (r *EventRepository) List(
	ctx context.Context,
	limit int,
	cursor *domain.EventCursor,
) ([]domain.EventListItem, *domain.EventCursor, error) {
	const eventSelectBase = `
		SELECT e.id, e.title, v.name, v.city, e.starts_at
		FROM booking.events e
		JOIN booking.venues v ON v.id = e.venue_id`

	const firstPageQuery = eventSelectBase + `
		ORDER BY e.starts_at, e.id
		LIMIT $1`

	const nextPageQuery = eventSelectBase + `
		WHERE (e.starts_at, e.id) > ($1, $2)
		ORDER BY e.starts_at, e.id
		LIMIT $3`

	if limit <= 0 {
		return nil, nil, fmt.Errorf("list events: limit must be positive, got %d", limit)
	}

	query := firstPageQuery
	args := []any{limit + 1}

	if cursor != nil {
		query = nextPageQuery
		args = []any{cursor.StartsAt, cursor.ID, limit + 1}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	items := make([]domain.EventListItem, 0, limit+1)

	for rows.Next() {
		var item domain.EventListItem
		if err := rows.Scan(
			&item.ID,
			&item.Title,
			&item.VenueName,
			&item.City,
			&item.StartsAt,
		); err != nil {
			return nil, nil, fmt.Errorf("scan event: %w", err)
		}

		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate events: %w", err)
	}

	var next *domain.EventCursor
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		next = &domain.EventCursor{
			StartsAt: last.StartsAt,
			ID:       last.ID,
		}
	}

	return items, next, nil
}

func (r *EventRepository) GetByID(ctx context.Context, id int64) (*domain.EventDetails, error) {
	const query = `
	    SELECT
			e.id, e.title, v.name, v.city, e.starts_at,
	        COUNT(es.id) AS seats_total,
	        COUNT(es.id) FILTER (WHERE es.status = 'available') AS seats_available
	    FROM booking.events e
	    JOIN booking.venues v ON v.id = e.venue_id
	    LEFT JOIN booking.event_seats es ON es.event_id = e.id
	    WHERE e.id = $1
	    GROUP BY e.id, e.title, v.name, v.city, e.starts_at`

	var details domain.EventDetails

	err := r.pool.QueryRow(ctx, query, id).Scan(
		&details.ID,
		&details.Title,
		&details.VenueName,
		&details.City,
		&details.StartsAt,
		&details.SeatsTotal,
		&details.SeatsAvailable,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("select event %d: %w", id, domain.ErrEventNotFound)
		}
		return nil, fmt.Errorf("select event %d: %w", id, err)
	}

	return &details, nil
}

func (r *EventRepository) ListSeats(
	ctx context.Context,
	eventID int64,
) ([]domain.EventSeatItem, error) {
	const seatsCapacity = 100
	const query = `
	    SELECT
	        es.id,
	        s.section,
	        s.row_label,
	        s.seat_number,
	        es.tier,
	        es.price_cents,
	        es.status
	    FROM booking.event_seats es
	    JOIN booking.seats s ON s.id = es.seat_id
	    WHERE es.event_id = $1
	    ORDER BY s.section,
	        s.row_label,
	        CASE WHEN s.seat_number ~ '^[0-9]+$' THEN s.seat_number::bigint END NULLS LAST,
	        s.seat_number,
	        es.id`

	rows, err := r.pool.Query(ctx, query, eventID)
	if err != nil {
		return nil, fmt.Errorf("list seats of event %d: %w", eventID, err)
	}
	defer rows.Close()

	items := make([]domain.EventSeatItem, 0, seatsCapacity)

	for rows.Next() {
		var item domain.EventSeatItem
		var rawStatus string

		if err := rows.Scan(
			&item.ID,
			&item.Section,
			&item.RowLabel,
			&item.SeatNumber,
			&item.Tier,
			&item.PriceCents,
			&rawStatus,
		); err != nil {
			return nil, fmt.Errorf("scan event seat: %w", err)
		}

		status, err := domain.ParseEventSeatStatus(rawStatus)
		if err != nil {
			return nil, fmt.Errorf("parse event seat %d status: %w", item.ID, err)
		}
		item.Status = status

		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate event seats: %w", err)
	}

	return items, nil
}

func (r *EventRepository) Exists(ctx context.Context, eventID int64) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1
			FROM booking.events
			WHERE id = $1
		)`

	var exists bool
	err := r.pool.QueryRow(ctx, query, eventID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("select exists event: %w", err)
	}

	return exists, nil
}
