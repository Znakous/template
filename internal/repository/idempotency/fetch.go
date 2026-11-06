package idempotency

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Znakous/template/internal/domain/trip"
	"github.com/Znakous/template/internal/infra/postgres/transaction"
	"github.com/Znakous/template/internal/model"
)

func (r *Keys) Get(ctx context.Context, key uuid.UUID) (model.IdempotencyRecord, error) {
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
		return model.IdempotencyRecord{}, trip.ErrIdempotencyKeyNotFound
	}
	if err != nil {
		return model.IdempotencyRecord{}, fmt.Errorf("scan idempotency key: %w", err)
	}
	return record, nil
}