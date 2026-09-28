-- +goose Up
CREATE TABLE trip_idempotency_keys (
    key           UUID PRIMARY KEY,
    trip_id       UUID NOT NULL REFERENCES trips(id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
    request_hash  TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX trip_idempotency_keys_created_at_idx ON trip_idempotency_keys (created_at);

-- +goose Down
DROP TABLE IF EXISTS trip_idempotency_keys;
