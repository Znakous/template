package statushistory

import (
	"context"

	"github.com/google/uuid"

	"github.com/Znakous/template/internal/domain/trip"
)

type Repository interface {
	Add(ctx context.Context, tripID uuid.UUID, fromStatus *trip.Status, toStatus trip.Status, reason *string) error
}
