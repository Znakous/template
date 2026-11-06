package trip

import (
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Znakous/template/internal/domain/trip"
)

func scanTrip(row pgx.Row) (trip.Trip, error) {
	var t trip.Trip
	var finishedAt pgtype.Timestamptz
	if err := row.Scan(
		&t.ID,
		&t.UserID,
		&t.DriverID,
		&t.StartLatitude,
		&t.StartLongitude,
		&t.EndLatitude,
		&t.EndLongitude,
		&t.Price,
		&t.Status,
		&t.StartedAt,
		&finishedAt,
	); err != nil {
		return trip.Trip{}, fmt.Errorf("scan trip: %w", err)
	}
	if finishedAt.Valid {
		t.FinishedAt = &finishedAt.Time
	}
	return t, nil
}
