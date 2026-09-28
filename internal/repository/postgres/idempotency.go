package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Znakous/template/internal/model"
	"github.com/Znakous/template/internal/transaction"
)

type IdempotencyKeys struct {
	database transaction.Executor
}

func NewIdempotencyKeys(database transaction.Executor) *IdempotencyKeys {
	return &IdempotencyKeys{database: database}
}

func (r *IdempotencyKeys) Create(ctx context.Context, key, tripID uuid.UUID, requestHash string) error {
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
			return fmt.Errorf("insert idempotency key: %w", model.ErrIdempotencyKeyExists)
		}
		return fmt.Errorf("insert idempotency key: %w", err)
	}
	return nil
}

func (r *IdempotencyKeys) Get(ctx context.Context, key uuid.UUID) (model.IdempotencyRecord, error) {
	query, args, err := psql.Select("key", "trip_id", "request_hash", "created_at").
		From("trip_idempotency_keys").
		Where(squirrel.Eq{"key": key}).
		ToSql()
	if err != nil {
		return model.IdempotencyRecord{}, fmt.Errorf("build get idempotency key query: %w", err)
	}

	var record model.IdempotencyRecord
	err = transaction.ExecutorFromContext(ctx, r.database).QueryRow(ctx, query, args...).Scan(
		&record.Key,
		&record.TripID,
		&record.RequestHash,
		&record.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.IdempotencyRecord{}, model.ErrIdempotencyKeyNotFound
	}
	if err != nil {
		return model.IdempotencyRecord{}, fmt.Errorf("scan idempotency key: %w", err)
	}
	return record, nil
}

func (r *IdempotencyKeys) Delete(ctx context.Context, key uuid.UUID) error {
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
