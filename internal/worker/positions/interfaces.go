package positions

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/Znakous/template/internal/model"
)

type StaleTripFinder interface {
	FindStaleTrips(ctx context.Context, staleAfter time.Duration) ([]model.StaleTrip, error)
}

type PositionRequester interface {
	RequestPosition(ctx context.Context, driverID, tripID uuid.UUID) error
}

type Metrics interface {
	StaleCheck(ctx context.Context, result string)
	QueueSize(ctx context.Context, size int)
	ActiveWorkers(ctx context.Context, delta int)
	PositionRequest(ctx context.Context, result string, duration time.Duration)
}

type Ticker interface {
	C() <-chan time.Time
	Stop()
}

type Clock interface {
	NewTicker(interval time.Duration) Ticker
}
