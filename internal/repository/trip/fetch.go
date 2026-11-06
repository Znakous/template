package trip

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Znakous/template/internal/domain/trip"
	"github.com/Znakous/template/internal/infra/postgres/transaction"
)

func (r *Trips) GetByID(ctx context.Context, id uuid.UUID) (trip.Trip, error) {
	query, args, err := psql.Select(tripColumns).
		From("trips").
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return trip.Trip{}, fmt.Errorf("build get trip query: %w", err)
	}

	t, err := scanTrip(transaction.ExecutorFromContext(ctx, r.database).QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return trip.Trip{}, trip.ErrTripNotFound
	}
	return t, err
}
