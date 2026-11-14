package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/egor-kardash/ticket-booking-service/internal/features/holds/domain"
)

const (
	uniqueViolationSQLState     = "23505"
	foreignKeyViolationSQLState = "23503"
	holdsActiveSeatConstraint   = "holds_active_seat_idx"
	holdsUserIDConstraint       = "holds_user_id_fkey"
)

type HoldRepository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *HoldRepository {
	return &HoldRepository{pool: pool}
}

func (r *HoldRepository) Create(
	ctx context.Context,
	userID, eventSeatID int64,
	ttl time.Duration,
) (*domain.HoldDetails, error) {
	if ttl < time.Second {
		return nil, fmt.Errorf("insert hold: ttl must be at least 1s, got %s", ttl)
	}

	const (
		lockSeatQuery = `
			SELECT price_cents
			FROM booking.event_seats
			WHERE id = $1
			FOR UPDATE`

		expireStaleQuery = `
			UPDATE booking.holds
			SET status = 'expired'
			WHERE event_seat_id = $1
				AND status = 'active'
				AND expires_at <= now()`

		insertHoldQuery = `
			INSERT INTO booking.holds (user_id, event_seat_id, expires_at)
			SELECT $1, $2, now() + $3 * interval '1 second'
			FROM booking.event_seats
			WHERE id = $2
				AND status = 'available'
			RETURNING id, expires_at`

		markSeatHeldQuery = `
			UPDATE booking.event_seats
			SET status = 'held'
			WHERE id = $1
				AND status = 'available'`
	)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin create hold: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var priceCents int64
	if err := tx.QueryRow(ctx, lockSeatQuery, eventSeatID).Scan(&priceCents); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("create hold: %w", domain.ErrEventSeatNotFound)
		}
		return nil, fmt.Errorf("lock event seat %d: %w", eventSeatID, err)
	}

	if _, err := tx.Exec(ctx, expireStaleQuery, eventSeatID); err != nil {
		return nil, fmt.Errorf("expire stale holds of seat %d: %w", eventSeatID, err)
	}

	if err := freeEventSeat(ctx, tx, eventSeatID); err != nil {
		return nil, err
	}

	details := domain.HoldDetails{
		EventSeatID: eventSeatID,
		PriceCents:  priceCents,
	}

	err = tx.QueryRow(ctx, insertHoldQuery, userID, eventSeatID, int64(ttl/time.Second)).Scan(
		&details.ID,
		&details.ExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("create hold: %w", domain.ErrSeatTaken)
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) &&
			pgErr.Code == uniqueViolationSQLState &&
			pgErr.ConstraintName == holdsActiveSeatConstraint {
			return nil, fmt.Errorf("create hold: %w", domain.ErrSeatTaken)
		}
		if errors.As(err, &pgErr) &&
			pgErr.Code == foreignKeyViolationSQLState &&
			pgErr.ConstraintName == holdsUserIDConstraint {
			return nil, fmt.Errorf("create hold: %w", domain.ErrUnauthorized)
		}

		return nil, fmt.Errorf("insert hold: %w", err)
	}

	tag, err := tx.Exec(ctx, markSeatHeldQuery, eventSeatID)
	if err != nil {
		return nil, fmt.Errorf("mark event seat %d held: %w", eventSeatID, err)
	}
	if tag.RowsAffected() != 1 {
		return nil, fmt.Errorf("mark event seat %d held: expected 1 row affected, got %d", eventSeatID, tag.RowsAffected())
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create hold: %w", err)
	}

	return &details, nil
}

func (r *HoldRepository) GetByID(ctx context.Context, id, userID int64) (*domain.HoldDetails, error) {
	const query = `
		SELECT h.id, h.event_seat_id, h.expires_at, es.price_cents
		FROM booking.holds h
		JOIN booking.event_seats es ON es.id = h.event_seat_id
		WHERE h.id = $1
			AND h.user_id = $2
			AND h.status = 'active'
			AND h.expires_at > now()`

	var details domain.HoldDetails

	err := r.pool.QueryRow(ctx, query, id, userID).Scan(
		&details.ID,
		&details.EventSeatID,
		&details.ExpiresAt,
		&details.PriceCents,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("select hold %d: %w", id, domain.ErrHoldNotFound)
		}
		return nil, fmt.Errorf("select hold %d: %w", id, err)
	}

	return &details, nil
}

func (r *HoldRepository) Cancel(ctx context.Context, id, userID int64) error {
	const (
		findHoldQuery = `
			SELECT event_seat_id
			FROM booking.holds
			WHERE id = $1
				AND user_id = $2
				AND status = 'active'
				AND expires_at > now()`

		cancelHoldQuery = `
			UPDATE booking.holds
			SET status = 'canceled'
			WHERE id = $1
				AND user_id = $2
				AND status = 'active'
				AND expires_at > now()`
	)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin cancel hold: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var eventSeatID int64
	if err := tx.QueryRow(ctx, findHoldQuery, id, userID).Scan(&eventSeatID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("cancel hold %d: %w", id, domain.ErrHoldNotFound)
		}
		return fmt.Errorf("select hold %d: %w", id, err)
	}

	if err := lockEventSeat(ctx, tx, eventSeatID); err != nil {
		return err
	}

	tag, err := tx.Exec(ctx, cancelHoldQuery, id, userID)
	if err != nil {
		return fmt.Errorf("cancel hold %d: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("cancel hold %d: %w", id, domain.ErrHoldNotFound)
	}

	if err := freeEventSeat(ctx, tx, eventSeatID); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit cancel hold: %w", err)
	}

	return nil
}

func (r *HoldRepository) ExpireBatch(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		return 0, fmt.Errorf("expire holds: limit must be positive, got %d", limit)
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
