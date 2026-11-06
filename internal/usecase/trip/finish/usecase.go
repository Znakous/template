package finish

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/Znakous/template/internal/domain/trip"
)

type Usecase struct {
	transactions txManager
	trips        tripRepository
	history      statusHistoryRepository
	queryTimeout time.Duration
	metrics      metrics
	logger       *slog.Logger
}

func New(
	transactions txManager,
	trips tripRepository,
	history statusHistoryRepository,
	queryTimeout time.Duration,
	metrics metrics,
	logger *slog.Logger,
) *Usecase {
	return &Usecase{
		transactions: transactions,
		trips:        trips,
		history:      history,
		queryTimeout: queryTimeout,
		metrics:      metrics,
		logger:       logger,
	}
}

func (u *Usecase) Finish(ctx context.Context, id uuid.UUID) (trip.Trip, error) {
	queryCtx, cancel := context.WithTimeout(ctx, u.queryTimeout)
	defer cancel()

	finishedAt := time.Now().UTC()
	var result trip.Trip
	err := u.transactions.Do(queryCtx, func(txCtx context.Context) error {
		t, err := u.trips.Finish(txCtx, id, finishedAt)
		if err != nil {
			return fmt.Errorf("finish trip: %w", err)
		}
		fromStatus := trip.StatusActive
		if err := u.history.Add(txCtx, t.ID, &fromStatus, trip.StatusCompleted, nil); err != nil {
			return fmt.Errorf("record completed trip status: %w", err)
		}
		result = t
		return nil
	})
	if err != nil {
		return trip.Trip{}, fmt.Errorf("finish trip transaction: %w", err)
	}
	u.metrics.TripCompleted(ctx)
	u.logger.Info("trip completed", "trip_id", result.ID.String())
	return result, nil
}