package idempotency

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Znakous/template/internal/domain/trip"
	"github.com/Znakous/template/internal/infra/postgres/transaction"
)

func (r *Keys) Create(ctx context.Context, key, tripID uuid.UUID, requestHash string) error {
	query, args, err := psql.Insert("trip_idempotency_keys").
		Columns("key", "trip_id", "request_hash").
		Values(key, tripID, requestHash).
		ToSql()
	if err != nil {
		return fmt.Errorf("build create idempotency key query: %w", err)
	}
	if _, err := transaction.ExecutorFromContext(ctx, r.database).Exec(ctx, query, args...); err != nil {
		var postgresErr *pgconn.PgError
		if errors.As(err, &postgresErr) && postgresErr.Code == "23505" {
			return fmt.Errorf("insert idempotency key: %w", trip.ErrIdempotencyKeyExists)
		}
		return fmt.Errorf("insert idempotency key: %w", err)
	}
	return nil
}

func (r *Keys) Delete(ctx context.Context, key uuid.UUID) error {
	query, args, err := psql.Delete("trip_idempotency_keys").
		Where(squirrel.Eq{"key": key}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build delete idempotency key query: %w", err)
	}
	if _, err := transaction.ExecutorFromContext(ctx, r.database).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("delete idempotency key: %w", err)
	}
	return nil
}