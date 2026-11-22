package positions

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/Znakous/template/internal/model"
)

func (p *Pool) scan(ctx context.Context) {
	ticker := p.clock.NewTicker(p.checkEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
			checkCtx, span := otel.Tracer("trip-service").Start(
				ctx,
				"tripgo.stale_check",
				trace.WithSpanKind(trace.SpanKindInternal),
			)
			trips, err := p.finder.FindStaleTrips(checkCtx, p.staleAfter)
			if err != nil {
				span.RecordError(err)
				if !errors.Is(err, context.Canceled) {
					p.logger.Error("scan stale trips failed", "service", "trip-service",
						"trace_id", span.SpanContext().TraceID().String(),
						"span_id", span.SpanContext().SpanID().String(),
						"error", err)
				}
				span.End()
				continue
			}
			for _, trip := range trips {
				if checkCtx.Err() != nil {
					span.End()
					return
				}
				p.enqueue(checkCtx, trip, span.SpanContext())
			}
			span.End()
		}
	}
}

func (p *Pool) enqueue(ctx context.Context, trip model.StaleTrip, checkSpan trace.SpanContext) {
	p.mu.Lock()
	if _, exists := p.enqueued[trip.TripID]; exists {
		p.mu.Unlock()
		p.metrics.StaleCheck(ctx, "duplicate")
		return
	}
	item := &task{trip: trip, checkSpan: checkSpan}
	p.enqueued[trip.TripID] = item
	result := "queued"
	dropped := false
	select {
	case p.queue <- item:
	default:
		delete(p.enqueued, trip.TripID)
		result = "dropped"
		dropped = true
	}
	queueSize := len(p.queue)
	p.mu.Unlock()

	p.metrics.StaleCheck(ctx, result)
	p.metrics.QueueSize(ctx, queueSize)
	if dropped {
		logContext := trace.SpanContextFromContext(ctx)
		p.logger.Warn("position request queue is full", "service", "trip-service",
			"trace_id", logContext.TraceID().String(),
			"span_id", logContext.SpanID().String(),
			"trip_id", trip.TripID)
	}
}

func (p *Pool) Cancel(ctx context.Context, tripID uuid.UUID) {
	p.mu.Lock()
	cancelled := false
	if queued, ok := p.enqueued[tripID]; ok && !queued.running {
		delete(p.enqueued, tripID)
		cancelled = true
	}
	p.mu.Unlock()

	if cancelled {
		p.metrics.PositionRequest(ctx, "cancelled", 0)
	}
}
