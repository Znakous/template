package trip

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/Znakous/template/internal/domain/trip"
)

type Repository interface {
	Create(ctx context.Context, t trip.Trip) error
	GetByID(ctx context.Context, id uuid.UUID) (trip.Trip, error)
	Finish(ctx context.Context, id uuid.UUID, finishedAt time.Time) (trip.Trip, error)
}