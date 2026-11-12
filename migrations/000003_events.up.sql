CREATE TABLE booking.venues (
    id   BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name VARCHAR(120) NOT NULL,
    city VARCHAR(120) NOT NULL
);

CREATE TABLE booking.events (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    venue_id   BIGINT NOT NULL REFERENCES booking.venues(id) ON DELETE RESTRICT,
    title      VARCHAR(200) NOT NULL,
    starts_at  TIMESTAMPTZ NOT NULL,

    CONSTRAINT events_id_venue_unique UNIQUE (id, venue_id)
);
CREATE INDEX events_starts_idx ON booking.events (starts_at, id);

CREATE TABLE booking.seats (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    venue_id    BIGINT NOT NULL REFERENCES booking.venues(id) ON DELETE RESTRICT,
    section     VARCHAR(10) NOT NULL,
    row_label   VARCHAR(10) NOT NULL,
    seat_number VARCHAR(10) NOT NULL,

    CONSTRAINT seats_id_venue_unique UNIQUE (id, venue_id),
    CONSTRAINT seats_unique UNIQUE (venue_id, section, row_label, seat_number)
);

CREATE TABLE booking.event_seats (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_id    BIGINT NOT NULL,
    seat_id     BIGINT NOT NULL,
    venue_id    BIGINT NOT NULL,
    tier        VARCHAR(20) NOT NULL,
    price_cents BIGINT NOT NULL CHECK (price_cents > 0),
    status      TEXT NOT NULL DEFAULT 'available'
                CHECK (status IN ('available', 'held', 'sold')),

    CONSTRAINT event_seats_event_venue_fk
        FOREIGN KEY (event_id, venue_id)
        REFERENCES booking.events (id, venue_id)
        ON DELETE RESTRICT,

    CONSTRAINT event_seats_seat_venue_fk
        FOREIGN KEY (seat_id, venue_id)
        REFERENCES booking.seats (id, venue_id)
        ON DELETE RESTRICT,

    CONSTRAINT event_seats_unique UNIQUE (event_id, seat_id)
);
