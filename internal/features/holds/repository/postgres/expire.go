package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/egor-kardash/ticket-booking-service/internal/features/holds/domain"
)

func (r *HoldRepository) ExpireBatch(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		return 0, fmt.Errorf("expire batch: limit must be positive, got %d", limit)
	}

	const candidatesQuery = `
		SELECT id, event_seat_id
		FROM booking.holds
		WHERE status = 'active' AND expires_at <= now()
		ORDER BY expires_at
		LIMIT $1`

	rows, err := r.pool.Query(ctx, candidatesQuery, limit)
	if err != nil {
		return 0, fmt.Errorf("select expired holds: %w", err)
	}
	defer rows.Close()

	type candidate struct{ holdID, seatID int64 }

	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.holdID, &c.seatID); err != nil {
			return 0, fmt.Errorf("scan expired hold: %w", err)
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate expired holds: %w", err)
	}

	var (
		expired int
		errs    []error
	)

	for _, c := range candidates {
		ok, err := r.expireOne(ctx, c.holdID, c.seatID)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if ok {
			expired++
		}
	}

	return expired, errors.Join(errs...)
}

func (r *HoldRepository) expireOne(ctx context.Context, holdID, seatID int64) (bool, error) {
	const (
		lockHoldQuery = `
			SELECT status, (expires_at <= now()) AS is_expired
			FROM booking.holds
			WHERE id = $1
			FOR UPDATE`

		expireHoldQuery = `
			UPDATE booking.holds
			SET status = 'expired'
			WHERE id = $1
				AND status = 'active'`
	)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin expire hold %d: %w", holdID, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockEventSeat(ctx, tx, seatID); err != nil {
		return false, err
	}

	var (
		status    string
		isExpired bool
	)
	if err := tx.QueryRow(ctx, lockHoldQuery, holdID).Scan(&status, &isExpired); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("lock hold %d: %w", holdID, err)
	}
	if domain.HoldStatus(status) != domain.HoldActive || !isExpired {
		return false, nil
	}

	tag, err := tx.Exec(ctx, expireHoldQuery, holdID)
	if err != nil {
		return false, fmt.Errorf("expire hold %d: %w", holdID, err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}

	if err := freeEventSeat(ctx, tx, seatID); err != nil {
		return false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit expire hold %d: %w", holdID, err)
	}

	return true, nil
}
