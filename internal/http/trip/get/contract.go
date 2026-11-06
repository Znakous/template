package get

import (
	"context"

	"github.com/google/uuid"

	"github.com/Znakous/template/internal/domain/trip"
)

type tripsUsecase interface {
	Get(ctx context.Context, id uuid.UUID) (trip.Trip, error)
}