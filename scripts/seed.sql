-- Seed для локальной/тестовой БД.
-- Одна площадка, два события, по 100 мест на событие.
-- Повторный запуск не создаёт дубликаты.

BEGIN;

DO $$
DECLARE
    c_venue_name      CONSTANT text := 'Seed Arena';
    c_venue_city      CONSTANT text := 'Moscow';
    c_event_one_title CONSTANT text := 'Seed Concert';
    c_event_two_title CONSTANT text := 'Seed Show';

    v_venue_id       bigint;
    v_event_one_id   bigint;
    v_event_two_id   bigint;
BEGIN
    -- 1. Площадка.
    SELECT id
    INTO v_venue_id
    FROM booking.venues
    WHERE name = c_venue_name
      AND city = c_venue_city
    LIMIT 1;

    IF v_venue_id IS NULL THEN
        INSERT INTO booking.venues (name, city)
        VALUES (c_venue_name, c_venue_city)
        RETURNING id INTO v_venue_id;
    END IF;

    -- 2. События.
    SELECT id
    INTO v_event_one_id
    FROM booking.events
    WHERE venue_id = v_venue_id
      AND title = c_event_one_title
    LIMIT 1;

    IF v_event_one_id IS NULL THEN
        INSERT INTO booking.events (venue_id, title, starts_at)
        VALUES (v_venue_id, c_event_one_title, timestamptz '2027-01-01 19:00:00+03')
        RETURNING id INTO v_event_one_id;
    END IF;

    SELECT id
    INTO v_event_two_id
    FROM booking.events
    WHERE venue_id = v_venue_id
      AND title = c_event_two_title
    LIMIT 1;

    IF v_event_two_id IS NULL THEN
        INSERT INTO booking.events (venue_id, title, starts_at)
        VALUES (v_venue_id, c_event_two_title, timestamptz '2027-01-08 19:00:00+03')
        RETURNING id INTO v_event_two_id;
    END IF;

    -- 3. 100 физических мест площадки: ряд 1..10, место 1..10, сектор A.
    INSERT INTO booking.seats (venue_id, section, row_label, seat_number)
    SELECT v_venue_id, 'A', rows.row_no::text, seats.seat_no::text
    FROM generate_series(1, 10) AS rows(row_no)
    CROSS JOIN generate_series(1, 10) AS seats(seat_no)
    ON CONFLICT (venue_id, section, row_label, seat_number) DO NOTHING;

    -- 4. Привязка всех 100 мест к каждому событию.
    --    Повторный запуск обновляет tier/price, но не трогает status:
    --    занятые/проданные места не должны «оживать» после seed.
    INSERT INTO booking.event_seats (event_id, seat_id, venue_id, tier, price_cents)
    SELECT
        e.id,
        s.id,
        v_venue_id,
        CASE WHEN rows.row_no <= 3 THEN 'vip' ELSE 'standard' END,
        CASE WHEN rows.row_no <= 3 THEN 500000 ELSE 250000 END
    FROM generate_series(1, 10) AS rows(row_no)
    CROSS JOIN generate_series(1, 10) AS seats(seat_no)
    JOIN booking.seats AS s
      ON s.venue_id = v_venue_id
     AND s.section = 'A'
     AND s.row_label = rows.row_no::text
     AND s.seat_number = seats.seat_no::text
    CROSS JOIN (VALUES (v_event_one_id), (v_event_two_id)) AS e(id)
    ON CONFLICT (event_id, seat_id) DO UPDATE
        SET tier = EXCLUDED.tier,
            price_cents = EXCLUDED.price_cents;
END
$$;

COMMIT;
