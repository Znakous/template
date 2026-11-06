package finish

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/Znakous/template/internal/domain/trip"
)

type (
	tripRepository interface {
		Finish(ctx context.Context, id uuid.UUID, finishedAt time.Time) (trip.Trip, error)
	}

	statusHistoryRepository interface {
		Add(ctx context.Context, tripID uuid.UUID, fromStatus *trip.Status, toStatus trip.Status, reason *string) error
	}

	txManager interface {
		Do(ctx context.Context, fn func(ctx context.Context) error) error
	}
)