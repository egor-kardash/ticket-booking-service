package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func lockEventSeat(ctx context.Context, tx pgx.Tx, seatID int64) error {
	const query = `
		SELECT 1
		FROM booking.event_seats
		WHERE id = $1
		FOR UPDATE`

	var one int
	if err := tx.QueryRow(ctx, query, seatID).Scan(&one); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("lock event seat %d: seat row is missing", seatID)
		}
		return fmt.Errorf("lock event seat %d: %w", seatID, err)
	}

	return nil
}

func freeEventSeat(ctx context.Context, tx pgx.Tx, seatID int64) error {
	const query = `
		UPDATE booking.event_seats
		SET status = 'available'
		WHERE id = $1
			AND status = 'held'
			AND NOT EXISTS (
			    SELECT 1
			    FROM booking.holds
			    WHERE event_seat_id = $1
			    AND status = 'active'
			)`

	if _, err := tx.Exec(ctx, query, seatID); err != nil {
		return fmt.Errorf("free event seat %d: %w", seatID, err)
	}

	return nil
}
