package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Znakous/template/internal/model"
	"github.com/Znakous/template/internal/transaction"
)

const IdempotencyKeyTTL = 24 * time.Hour

type TripRepository interface {
	Create(ctx context.Context, trip model.Trip) error
	GetByID(ctx context.Context, id uuid.UUID) (model.Trip, error)
	Finish(ctx context.Context, id uuid.UUID, finishedAt time.Time) (model.Trip, error)
}

type StatusHistoryRepository interface {
	Record(ctx context.Context, tripID uuid.UUID, fromStatus *model.TripStatus, toStatus model.TripStatus, reason *string) error
}

type IdempotencyRepository interface {
	Create(ctx context.Context, key, tripID uuid.UUID, requestHash string) error
	Get(ctx context.Context, key uuid.UUID) (model.IdempotencyRecord, error)
	Delete(ctx context.Context, key uuid.UUID) error
}

type Service struct {
	transactions transaction.TxManager
	trips        TripRepository
	history      StatusHistoryRepository
	idempotency  IdempotencyRepository
	queryTimeout time.Duration
}

func New(
	transactions transaction.TxManager,
	trips TripRepository,
	history StatusHistoryRepository,
	idempotency IdempotencyRepository,
	queryTimeout time.Duration,
) *Service {
	return &Service{
		transactions: transactions,
		trips:        trips,
		history:      history,
		idempotency:  idempotency,
		queryTimeout: queryTimeout,
	}
}

func (s *Service) CreateTrip(ctx context.Context, trip model.Trip, idempotencyKey *uuid.UUID) (model.Trip, bool, error) {
	requestHash, err := requestFingerprint(trip)
	if err != nil {
		return model.Trip{}, false, fmt.Errorf("fingerprint trip request: %w", err)
	}

	for range 2 {
		createdTrip, err := s.insertTrip(ctx, trip, idempotencyKey, requestHash)
		if err == nil {
			return createdTrip, true, nil
		}
		if idempotencyKey == nil || !errors.Is(err, model.ErrIdempotencyKeyExists) {
			return model.Trip{}, false, err
		}

		replayed, replayedTrip, replayErr := s.replayOrExpire(ctx, *idempotencyKey, requestHash)
		if replayErr != nil {
			return model.Trip{}, false, replayErr
		}
		if replayed {
			return replayedTrip, false, nil
		}
	}

	return model.Trip{}, false, fmt.Errorf("create trip: %w", model.ErrIdempotencyKeyExists)
}

func (s *Service) insertTrip(ctx context.Context, trip model.Trip, idempotencyKey *uuid.UUID, requestHash string) (model.Trip, error) {
	trip.ID = uuid.New()
	trip.Status = model.TripStatusActive
	trip.StartedAt = time.Now().UTC()
	trip.FinishedAt = nil

	queryCtx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	err := s.transactions.Do(queryCtx, func(txCtx context.Context) error {
		if idempotencyKey != nil {
			if err := s.idempotency.Create(txCtx, *idempotencyKey, trip.ID, requestHash); err != nil {
				return fmt.Errorf("store idempotency key: %w", err)
			}
		}
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

func (s *Service) replayOrExpire(ctx context.Context, key uuid.UUID, requestHash string) (bool, model.Trip, error) {
	queryCtx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	record, err := s.idempotency.Get(queryCtx, key)
	if errors.Is(err, model.ErrIdempotencyKeyNotFound) {
		return false, model.Trip{}, nil
	}
	if err != nil {
		return false, model.Trip{}, fmt.Errorf("get idempotency key: %w", err)
	}
	if time.Since(record.CreatedAt) > IdempotencyKeyTTL {
		if err := s.idempotency.Delete(queryCtx, key); err != nil {
			return false, model.Trip{}, fmt.Errorf("expire idempotency key: %w", err)
		}
		return false, model.Trip{}, nil
	}
	if record.RequestHash != requestHash {
		return false, model.Trip{}, model.ErrIdempotencyConflict
	}

	trip, err := s.trips.GetByID(queryCtx, record.TripID)
	if err != nil {
		return false, model.Trip{}, fmt.Errorf("get trip by idempotency key: %w", err)
	}
	return true, trip, nil
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

func requestFingerprint(trip model.Trip) (string, error) {
	payload, err := json.Marshal(struct {
		UserID         uuid.UUID `json:"user_id"`
		DriverID       uuid.UUID `json:"driver_id"`
		StartLatitude  float64   `json:"start_latitude"`
		StartLongitude float64   `json:"start_longitude"`
		EndLatitude    float64   `json:"end_latitude"`
		EndLongitude   float64   `json:"end_longitude"`
		Price          int64     `json:"price"`
	}{
		UserID:         trip.UserID,
		DriverID:       trip.DriverID,
		StartLatitude:  trip.StartLatitude,
		StartLongitude: trip.StartLongitude,
		EndLatitude:    trip.EndLatitude,
		EndLongitude:   trip.EndLongitude,
		Price:          trip.Price,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}
