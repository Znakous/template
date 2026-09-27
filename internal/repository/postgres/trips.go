package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Znakous/template/internal/model"
	"github.com/Znakous/template/internal/transaction"
)

var psql = squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)

const tripColumns = "id, user_id, driver_id, start_latitude, start_longitude, end_latitude, end_longitude, price, status, started_at, finished_at"

type Trips struct {
	database transaction.Executor
}

func NewTrips(database transaction.Executor) *Trips {
	return &Trips{database: database}
}

func (r *Trips) Create(ctx context.Context, trip model.Trip) error {
	query, args, err := psql.Insert("trips").
		Columns("id", "user_id", "driver_id", "start_latitude", "start_longitude", "end_latitude", "end_longitude", "price", "status", "started_at", "finished_at").
		Values(trip.ID, trip.UserID, trip.DriverID, trip.StartLatitude, trip.StartLongitude, trip.EndLatitude, trip.EndLongitude, trip.Price, trip.Status, trip.StartedAt, trip.FinishedAt).
		ToSql()
	if err != nil {
		return fmt.Errorf("build create trip query: %w", err)
	}
	if _, err := transaction.ExecutorFromContext(ctx, r.database).Exec(ctx, query, args...); err != nil {
		var postgresErr *pgconn.PgError
		if errors.As(err, &postgresErr) && postgresErr.Code == "23505" && postgresErr.ConstraintName == "trips_one_active_per_driver_idx" {
			return fmt.Errorf("driver already has an active trip: %w", model.ErrDriverBusy)
		}
		return fmt.Errorf("insert trip: %w", err)
	}
	return nil
}

func (r *Trips) GetByID(ctx context.Context, id uuid.UUID) (model.Trip, error) {
	query, args, err := psql.Select(tripColumns).
		From("trips").
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return model.Trip{}, fmt.Errorf("build get trip query: %w", err)
	}
	trip, err := scanTrip(transaction.ExecutorFromContext(ctx, r.database).QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Trip{}, model.ErrTripNotFound
	}
	return trip, err
}

func (r *Trips) Finish(ctx context.Context, id uuid.UUID, finishedAt time.Time) (model.Trip, error) {
	query, args, err := psql.Update("trips").
		Set("status", model.TripStatusCompleted).
		Set("finished_at", finishedAt).
		Set("updated_at", finishedAt).
		Where(squirrel.Eq{"id": id}).
		Where(squirrel.Eq{"status": model.TripStatusActive}).
		Suffix("RETURNING " + tripColumns).
		ToSql()
	if err != nil {
		return model.Trip{}, fmt.Errorf("build finish trip query: %w", err)
	}

	trip, err := scanTrip(transaction.ExecutorFromContext(ctx, r.database).QueryRow(ctx, query, args...))
	if !errors.Is(err, pgx.ErrNoRows) {
		if err != nil {
			return model.Trip{}, err
		}
		return trip, nil
	}

	if _, readErr := r.GetByID(ctx, id); errors.Is(readErr, model.ErrTripNotFound) {
		return model.Trip{}, model.ErrTripNotFound
	} else if readErr != nil {
		return model.Trip{}, fmt.Errorf("check trip after conditional finish: %w", readErr)
	}
	return model.Trip{}, model.ErrTripCompleted
}

func scanTrip(row pgx.Row) (model.Trip, error) {
	var trip model.Trip
	var finishedAt pgtype.Timestamptz
	if err := row.Scan(
		&trip.ID,
		&trip.UserID,
		&trip.DriverID,
		&trip.StartLatitude,
		&trip.StartLongitude,
		&trip.EndLatitude,
		&trip.EndLongitude,
		&trip.Price,
		&trip.Status,
		&trip.StartedAt,
		&finishedAt,
	); err != nil {
		return model.Trip{}, fmt.Errorf("scan trip: %w", err)
	}
	if finishedAt.Valid {
		trip.FinishedAt = &finishedAt.Time
	}
	return trip, nil
}
