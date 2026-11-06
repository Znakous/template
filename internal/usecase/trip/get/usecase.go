package get

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Znakous/template/internal/domain/trip"
)

type Usecase struct {
	trips        tripRepository
	queryTimeout time.Duration
}

func New(trips tripRepository, queryTimeout time.Duration) *Usecase {
	return &Usecase{trips: trips, queryTimeout: queryTimeout}
}

func (u *Usecase) Get(ctx context.Context, id uuid.UUID) (trip.Trip, error) {
	queryCtx, cancel := context.WithTimeout(ctx, u.queryTimeout)
	defer cancel()

	t, err := u.trips.GetByID(queryCtx, id)
	if err != nil {
		return trip.Trip{}, fmt.Errorf("get trip: %w", err)
	}
	return t, nil
}