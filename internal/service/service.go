package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Znakous/template/internal/model"
	"github.com/Znakous/template/internal/transaction"
)

type TripRepository interface {
	Create(ctx context.Context, trip model.Trip) error
	GetByID(ctx context.Context, id uuid.UUID) (model.Trip, error)
	Finish(ctx context.Context, id uuid.UUID, finishedAt time.Time) (model.Trip, error)
}

type StatusHistoryRepository interface {
	Record(ctx context.Context, tripID uuid.UUID, fromStatus *model.TripStatus, toStatus model.TripStatus, reason *string) error
}

type Service struct {
	transactions transaction.TxManager
	trips        TripRepository
	history      StatusHistoryRepository
	queryTimeout time.Duration
}

func New(transactions transaction.TxManager, trips TripRepository, history StatusHistoryRepository, queryTimeout time.Duration) *Service {
	return &Service{
		transactions: transactions,
		trips:        trips,
		history:      history,
		queryTimeout: queryTimeout,
	}
}

func (s *Service) CreateTrip(ctx context.Context, trip model.Trip) (model.Trip, error) {
	trip.ID = uuid.New()
	trip.Status = model.TripStatusActive
	trip.StartedAt = time.Now().UTC()
	trip.FinishedAt = nil

	queryCtx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	err := s.transactions.Do(queryCtx, func(txCtx context.Context) error {
		if err := s.trips.Create(txCtx, trip); err != nil {
			return fmt.Errorf("insert trip: %w", err)
		}
		if err := s.history.Record(txCtx, trip.ID, nil, model.TripStatusActive, nil); err != nil {
			return fmt.Errorf("record initial trip status: %w", err)
		}
		return nil
	})
	if err != nil {
		return model.Trip{}, fmt.Errorf("create trip: %w", err)
	}
	return trip, nil
}

func (s *Service) GetTrip(ctx context.Context, id uuid.UUID) (model.Trip, error) {
	queryCtx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	trip, err := s.trips.GetByID(queryCtx, id)
	if err != nil {
		return model.Trip{}, fmt.Errorf("get trip: %w", err)
	}
	return trip, nil
}

func (s *Service) FinishTrip(ctx context.Context, id uuid.UUID) (model.Trip, error) {
	queryCtx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	finishedAt := time.Now().UTC()
	var trip model.Trip
	err := s.transactions.Do(queryCtx, func(txCtx context.Context) error {
		var err error
		trip, err = s.trips.Finish(txCtx, id, finishedAt)
		if err != nil {
			return fmt.Errorf("finish trip: %w", err)
		}
		fromStatus := model.TripStatusActive
		if err := s.history.Record(txCtx, trip.ID, &fromStatus, model.TripStatusCompleted, nil); err != nil {
			return fmt.Errorf("record completed trip status: %w", err)
		}
		return nil
	})
	if err != nil {
		return model.Trip{}, fmt.Errorf("finish trip transaction: %w", err)
	}
	return trip, nil
}
