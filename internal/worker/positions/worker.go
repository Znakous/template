package positions

import (
	"context"
	"errors"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

func (p *Pool) worker(ctx context.Context) {
	for item := range p.queue {
		p.mu.Lock()
		current, exists := p.enqueued[item.trip.TripID]
		if !exists || current != item {
			p.mu.Unlock()
			p.metrics.QueueSize(ctx, len(p.queue))
			continue
		}
		item.running = true
		p.mu.Unlock()
		p.metrics.QueueSize(ctx, len(p.queue))

		p.process(ctx, item)

		p.mu.Lock()
		if p.enqueued[item.trip.TripID] == item {
			delete(p.enqueued, item.trip.TripID)
		}
		p.mu.Unlock()
	}
}

func (p *Pool) process(ctx context.Context, item *task) {
	workerCtx, span := otel.Tracer("trip-service").Start(
		ctx,
		"tripgo.position_request.process",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithLinks(trace.Link{SpanContext: item.checkSpan}),
	)
	defer span.End()

	p.metrics.ActiveWorkers(workerCtx, 1)
	defer p.metrics.ActiveWorkers(workerCtx, -1)

	started := time.Now()
	requestCtx, cancel := context.WithTimeout(workerCtx, p.requestLimit)
	err := p.requester.RequestPosition(requestCtx, item.trip.DriverID, item.trip.TripID)
	cancel()
	duration := time.Since(started)

	result := "success"
	if err != nil {
		result = "error"
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			result = "cancelled"
		}
		span.RecordError(err)
		p.logger.Warn("request driver position failed", "service", "trip-service",
			"trace_id", span.SpanContext().TraceID().String(),
			"span_id", span.SpanContext().SpanID().String(),
			"trip_id", item.trip.TripID, "error", err)
	}
	p.metrics.PositionRequest(workerCtx, result, duration)
}
