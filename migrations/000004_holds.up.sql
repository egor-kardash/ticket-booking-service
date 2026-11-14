CREATE TABLE booking.holds (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id       BIGINT NOT NULL REFERENCES booking.users(id) ON DELETE RESTRICT,
    event_seat_id BIGINT NOT NULL REFERENCES booking.event_seats(id) ON DELETE RESTRICT,
    status        TEXT NOT NULL DEFAULT 'active'
                  CHECK (status IN ('active', 'expired', 'canceled', 'consumed')),
    expires_at    TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT holds_expires_after_created CHECK (expires_at > created_at)
);
CREATE UNIQUE INDEX holds_active_seat_idx
    ON booking.holds (event_seat_id) WHERE status = 'active';
CREATE INDEX holds_expires_idx
    ON booking.holds (expires_at) WHERE status = 'active';
