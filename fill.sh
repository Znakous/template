#!/bin/sh


# тут старался чат жбт, просто заполнение таблички в бд
# выглядит ужасно, но работает
set -eu

if [ ! -f .env ]; then
    echo ".env not found; run this script from the repository root" >&2
    exit 1
fi

set -a
. ./.env
set +a

: "${DATABASE_URL:?DATABASE_URL is required in .env}"

psql "$DATABASE_URL" -v ON_ERROR_STOP=1 <<'SQL'
BEGIN;

INSERT INTO trips (
    id, user_id, driver_id,
    start_latitude, start_longitude, end_latitude, end_longitude,
    price, status, started_at
)
SELECT
    ('f3110000-0000-4000-8000-' || lpad(g::text, 12, '0'))::uuid,
    'f3110000-0000-4000-8fff-ffffffffffff'::uuid,
    ('f3110000-0000-4000-9000-' || lpad(g::text, 12, '0'))::uuid,
    0, 0, 1, 1, 100, 'active',
    CASE
        WHEN g % 100 = 0 THEN now() - interval '10 minutes'
        ELSE now() - interval '10 seconds'
    END
FROM generate_series(1, 10000) AS series(g)
ON CONFLICT (id) DO NOTHING;

COMMIT;

ANALYZE trips;
SQL
