package create

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/Znakous/template/internal/domain/trip"
)

const idempotencyKeyTTL = 24 * time.Hour

type Usecase struct {
	transactions txManager
	trips        tripRepository
	history      statusHistoryRepository
	idempotency  idempotencyRepository
	queryTimeout time.Duration
	metrics      metrics
	logger       *slog.Logger
}

func New(
	transactions txManager,
	trips tripRepository,
	history statusHistoryRepository,
	idempotency idempotencyRepository,
	queryTimeout time.Duration,
	metrics metrics,
	logger *slog.Logger,
) *Usecase {
	return &Usecase{
		transactions: transactions,
		trips:        trips,
		history:      history,
		idempotency:  idempotency,
		queryTimeout: queryTimeout,
		metrics:      metrics,
		logger:       logger,
	}
}

func (u *Usecase) Create(ctx context.Context, in Input, idempotencyKey *uuid.UUID) (Output, error) {
	t, err := trip.New(
		uuid.New(), in.UserID, in.DriverID,
		in.StartLatitude, in.StartLongitude, in.EndLatitude, in.EndLongitude, in.Price,
	)
	if err != nil {
		return Output{}, err
	}

	requestHash, err := requestFingerprint(in)
	if err != nil {
		return Output{}, fmt.Errorf("fingerprint trip request: %w", err)
	}

	for range 2 {
		created, err := u.insertTrip(ctx, *t, idempotencyKey, requestHash)
		if err == nil {
			u.metrics.TripCreated(ctx)
			u.logger.Info("trip created", "trip_id", created.ID.String())
			return Output{Trip: created, Created: true}, nil
		}
		if idempotencyKey == nil || !errors.Is(err, trip.ErrIdempotencyKeyExists) {
			return Output{}, err
		}

		replayed, replayedTrip, replayErr := u.replayOrExpire(ctx, *idempotencyKey, requestHash)
		if replayErr != nil {
			return Output{}, replayErr
		}
		if replayed {
			return Output{Trip: replayedTrip}, nil
		}
	}

	return Output{}, fmt.Errorf("create trip: %w", trip.ErrIdempotencyKeyExists)
}

func (u *Usecase) insertTrip(ctx context.Context, t trip.Trip, idempotencyKey *uuid.UUID, requestHash string) (trip.Trip, error) {
	queryCtx, cancel := context.WithTimeout(ctx, u.queryTimeout)
	defer cancel()

	err := u.transactions.Do(queryCtx, func(txCtx context.Context) error {
		if idempotencyKey != nil {
			if err := u.idempotency.Create(txCtx, *idempotencyKey, t.ID, requestHash); err != nil {
				return fmt.Errorf("store idempotency key: %w", err)
			}
		}
		if err := u.trips.Create(txCtx, t); err != nil {
			return fmt.Errorf("insert trip: %w", err)
		}
		if err := u.history.Add(txCtx, t.ID, nil, trip.StatusActive, nil); err != nil {
			return fmt.Errorf("record initial trip status: %w", err)
		}
		return nil
	})
	if err != nil {
		return trip.Trip{}, fmt.Errorf("create trip: %w", err)
	}
	return t, nil
}

func (u *Usecase) replayOrExpire(ctx context.Context, key uuid.UUID, requestHash string) (bool, trip.Trip, error) {
	queryCtx, cancel := context.WithTimeout(ctx, u.queryTimeout)
	defer cancel()

	record, err := u.idempotency.Get(queryCtx, key)
	if errors.Is(err, trip.ErrIdempotencyKeyNotFound) {
		return false, trip.Trip{}, nil
	}
	if err != nil {
		return false, trip.Trip{}, fmt.Errorf("get idempotency key: %w", err)
	}
	if time.Since(record.CreatedAt) > idempotencyKeyTTL {
		if err := u.idempotency.Delete(queryCtx, key); err != nil {
			return false, trip.Trip{}, fmt.Errorf("expire idempotency key: %w", err)
		}
		return false, trip.Trip{}, nil
	}
	if record.RequestHash != requestHash {
		return false, trip.Trip{}, trip.ErrIdempotencyConflict
	}

	t, err := u.trips.GetByID(queryCtx, record.TripID)
	if err != nil {
		return false, trip.Trip{}, fmt.Errorf("get trip by idempotency key: %w", err)
	}
	return true, t, nil
}

func requestFingerprint(in Input) (string, error) {
	payload, err := json.Marshal(struct {
		UserID         uuid.UUID `json:"user_id"`
		DriverID       uuid.UUID `json:"driver_id"`
		StartLatitude  float64   `json:"start_latitude"`
		StartLongitude float64   `json:"start_longitude"`
		EndLatitude    float64   `json:"end_latitude"`
		EndLongitude   float64   `json:"end_longitude"`
		Price          int64     `json:"price"`
	}{
		UserID:         in.UserID,
		DriverID:       in.DriverID,
		StartLatitude:  in.StartLatitude,
		StartLongitude: in.StartLongitude,
		EndLatitude:    in.EndLatitude,
		EndLongitude:   in.EndLongitude,
		Price:          in.Price,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}