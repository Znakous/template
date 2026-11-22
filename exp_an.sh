#!/bin/sh
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
EXPLAIN (ANALYZE, BUFFERS)
SELECT id, driver_id
FROM trips
WHERE status = 'active'
  AND COALESCE(last_position_at, started_at)
      < now() - (60000000::bigint * interval '1 microsecond')
ORDER BY COALESCE(last_position_at, started_at) ASC, id ASC;
SQL