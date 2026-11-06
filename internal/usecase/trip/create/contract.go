//go:generate mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package create

import (
	"context"

	"github.com/google/uuid"

	"github.com/Znakous/template/internal/domain/trip"
	"github.com/Znakous/template/internal/model"
)

type Input struct {
	UserID         uuid.UUID
	DriverID       uuid.UUID
	StartLatitude  float64
	StartLongitude float64
	EndLatitude    float64
	EndLongitude   float64
	Price          int64
}

type Output struct {
	Trip    trip.Trip
	Created bool
}

type (
	tripRepository interface {
		Create(ctx context.Context, t trip.Trip) error
		GetByID(ctx context.Context, id uuid.UUID) (trip.Trip, error)
	}

	idempotencyRepository interface {
		Create(ctx context.Context, key, tripID uuid.UUID, requestHash string) error
		Get(ctx context.Context, key uuid.UUID) (model.IdempotencyRecord, error)
		Delete(ctx context.Context, key uuid.UUID) error
	}

	statusHistoryRepository interface {
		Add(ctx context.Context, tripID uuid.UUID, fromStatus *trip.Status, toStatus trip.Status, reason *string) error
	}

	txManager interface {
		Do(ctx context.Context, fn func(ctx context.Context) error) error
	}

	metrics interface {
		TripCreated(ctx context.Context)
	}
)
