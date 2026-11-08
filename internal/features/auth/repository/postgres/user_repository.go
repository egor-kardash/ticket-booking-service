package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/egor-kardash/ticket-booking-service/internal/features/auth/domain"
)

const (
	uniqueViolationSQLState = "23505"
	usersEmailConstraint    = "users_email_key"
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) Create(ctx context.Context, user *domain.User) (*domain.User, error) {
	query := `
		INSERT INTO booking.users (email, password_hash)
		VALUES ($1, $2)
		RETURNING id, email, created_at`

	var created domain.User
	err := r.pool.QueryRow(ctx, query, user.Email, user.PasswordHash).Scan(
		&created.ID,
		&created.Email,
		&created.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) &&
			pgErr.Code == uniqueViolationSQLState &&
			pgErr.ConstraintName == usersEmailConstraint {
			return nil, fmt.Errorf("insert user: %w", domain.ErrEmailTaken)
		}
		return nil, fmt.Errorf("insert user: %w", err)
	}

	return &created, nil
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `
		SELECT id, email, password_hash, created_at
		FROM booking.users
		WHERE email = $1`

	var user domain.User
	err := r.pool.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get user by email: %w", domain.ErrNotFound)
		}
		return nil, fmt.Errorf("get user by email: %w", err)
	}

	return &user, nil
}

func (r *UserRepository) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	query := `
		SELECT id, email, created_at
		FROM booking.users
		WHERE id = $1`

	var user domain.User
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&user.ID,
		&user.Email,
		&user.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get user by id: %w", domain.ErrNotFound)
		}
		return nil, fmt.Errorf("get user by id: %w", err)
	}

	return &user, nil
}
