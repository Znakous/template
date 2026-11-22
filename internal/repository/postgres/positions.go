package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Znakous/template/internal/model"
	"github.com/Znakous/template/internal/transaction"
)

type Positions struct {
	database transaction.Executor
}

func NewPositions(database transaction.Executor) *Positions {
	return &Positions{database: database}
}

func (r *Positions) Save(ctx context.Context, position model.TripPosition) (model.TripPosition, error) {
	executor := transaction.ExecutorFromContext(ctx, r.database)

	updateQuery, updateArgs, err := psql.Update("trips").
		Set("last_position_at", squirrel.Expr(
			"CASE WHEN last_position_at IS NULL OR last_position_at < ? THEN ? ELSE last_position_at END",
			position.RecordedAt, position.RecordedAt,
		)).
		Where(squirrel.Eq{"id": position.TripID}).
		Where(squirrel.Eq{"status": model.TripStatusActive}).
		ToSql()
	if err != nil {
		return model.TripPosition{}, fmt.Errorf("build update last position query: %w", err)
	}

	tag, err := executor.Exec(ctx, updateQuery, updateArgs...)
	if err != nil {
		return model.TripPosition{}, fmt.Errorf("update last position timestamp: %w", err)
	}
	if tag.RowsAffected() == 0 {
		var status model.TripStatus
		err := executor.QueryRow(ctx, "SELECT status FROM trips WHERE id = $1", position.TripID).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) {
			return model.TripPosition{}, model.ErrTripNotFound
		}
		if err != nil {
			return model.TripPosition{}, fmt.Errorf("check trip before saving position: %w", err)
		}
		return model.TripPosition{}, model.ErrTripCompleted
	}

	query, args, err := psql.Insert("trip_positions").
		Columns("trip_id", "latitude", "longitude", "recorded_at").
		Values(position.TripID, position.Latitude, position.Longitude, position.RecordedAt).
		Suffix("RETURNING id").
		ToSql()
	if err != nil {
		return model.TripPosition{}, fmt.Errorf("build save position query: %w", err)
	}
	if err := executor.QueryRow(ctx, query, args...).Scan(&position.ID); err != nil {
		return model.TripPosition{}, fmt.Errorf("insert trip position: %w", err)
	}
	return position, nil
}

func (r *Positions) List(ctx context.Context, tripID uuid.UUID) ([]model.TripPosition, error) {
	query, args, err := psql.Select("id", "trip_id", "latitude", "longitude", "recorded_at").
		From("trip_positions").
		Where(squirrel.Eq{"trip_id": tripID}).
		OrderBy("recorded_at ASC", "id ASC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build list positions query: %w", err)
	}

	rows, err := transaction.ExecutorFromContext(ctx, r.database).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list trip positions: %w", err)
	}
	defer rows.Close()

	positions := make([]model.TripPosition, 0)
	for rows.Next() {
		var position model.TripPosition
		if err := rows.Scan(
			&position.ID,
			&position.TripID,
			&position.Latitude,
			&position.Longitude,
			&position.RecordedAt,
		); err != nil {
			return nil, fmt.Errorf("scan trip position: %w", err)
		}
		positions = append(positions, position)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trip positions: %w", err)
	}
	return positions, nil
}

func (r *Positions) FindStaleTrips(ctx context.Context, staleAfter time.Duration) ([]model.StaleTrip, error) {
	query, args, err := psql.Select("id", "driver_id").
		From("trips").
		Where("status = 'active'").
		Where(squirrel.Expr(
			"COALESCE(last_position_at, started_at) < now() - (?::bigint * interval '1 microsecond')",
			staleAfter.Microseconds(),
		)).
		OrderBy("COALESCE(last_position_at, started_at) ASC", "id ASC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build stale trips query: %w", err)
	}

	rows, err := transaction.ExecutorFromContext(ctx, r.database).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("find stale trips: %w", err)
	}
	defer rows.Close()

	trips := make([]model.StaleTrip, 0)
	for rows.Next() {
		var trip model.StaleTrip
		if err := rows.Scan(&trip.TripID, &trip.DriverID); err != nil {
			return nil, fmt.Errorf("scan stale trip: %w", err)
		}
		trips = append(trips, trip)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stale trips: %w", err)
	}
	return trips, nil
}
