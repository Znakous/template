package trip

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Znakous/template/internal/domain/trip"
	"github.com/Znakous/template/internal/infra/postgres/transaction"
)

func (r *Trips) Create(ctx context.Context, t trip.Trip) error {
	query, args, err := psql.Insert("trips").
		Columns("id", "user_id", "driver_id", "start_latitude", "start_longitude", "end_latitude", "end_longitude", "price", "status", "started_at", "finished_at").
		Values(t.ID, t.UserID, t.DriverID, t.StartLatitude, t.StartLongitude, t.EndLatitude, t.EndLongitude, t.Price, t.Status, t.StartedAt, t.FinishedAt).
		ToSql()
	if err != nil {
		return fmt.Errorf("build create trip query: %w", err)
	}
	if _, err := transaction.ExecutorFromContext(ctx, r.database).Exec(ctx, query, args...); err != nil {
		var postgresErr *pgconn.PgError
		if errors.As(err, &postgresErr) && postgresErr.Code == "23505" && postgresErr.ConstraintName == "trips_one_active_per_driver_idx" {
			return fmt.Errorf("driver already has an active trip: %w", trip.ErrDriverBusy)
		}
		return fmt.Errorf("insert trip: %w", err)
	}
	return nil
}

func (r *Trips) Finish(ctx context.Context, id uuid.UUID, finishedAt time.Time) (trip.Trip, error) {
	query, args, err := psql.Update("trips").
		Set("status", trip.StatusCompleted).
		Set("finished_at", finishedAt).
		Set("updated_at", finishedAt).
		Where(squirrel.Eq{"id": id}).
		Where(squirrel.Eq{"status": trip.StatusActive}).
		Suffix("RETURNING " + tripColumns).
		ToSql()
	if err != nil {
		return trip.Trip{}, fmt.Errorf("build finish trip query: %w", err)
	}

	t, err := scanTrip(transaction.ExecutorFromContext(ctx, r.database).QueryRow(ctx, query, args...))
	if !errors.Is(err, pgx.ErrNoRows) {
		if err != nil {
			return trip.Trip{}, err
		}
		return t, nil
	}

	if _, readErr := r.GetByID(ctx, id); errors.Is(readErr, trip.ErrTripNotFound) {
		return trip.Trip{}, trip.ErrTripNotFound
	} else if readErr != nil {
		return trip.Trip{}, fmt.Errorf("check trip after conditional finish: %w", readErr)
	}
	return trip.Trip{}, trip.ErrTripCompleted
}
