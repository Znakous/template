package telemetry

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type PositionMetrics struct {
	staleChecks     metric.Int64Counter
	queueSize       metric.Int64Gauge
	activeWorkers   metric.Int64Gauge
	positionRequest metric.Int64Counter
	requestDuration metric.Float64Histogram
	positionsSaved  metric.Int64Counter
	activeCount     atomic.Int64
}

func NewPositionMetrics() (*PositionMetrics, error) {
	meter := otel.Meter("trip-service")
	metrics := &PositionMetrics{}
	var err error
	if metrics.staleChecks, err = meter.Int64Counter("tripgo.stale_checks"); err != nil {
		return nil, fmt.Errorf("create stale check counter: %w", err)
	}
	if metrics.queueSize, err = meter.Int64Gauge("tripgo.position_requests.queue.size"); err != nil {
		return nil, fmt.Errorf("create position queue gauge: %w", err)
	}
	if metrics.activeWorkers, err = meter.Int64Gauge("tripgo.position_requests.workers.active"); err != nil {
		return nil, fmt.Errorf("create active worker gauge: %w", err)
	}
	if metrics.positionRequest, err = meter.Int64Counter("tripgo.position_requests"); err != nil {
		return nil, fmt.Errorf("create position request counter: %w", err)
	}
	if metrics.requestDuration, err = meter.Float64Histogram("tripgo.position_requests.duration", metric.WithUnit("s")); err != nil {
		return nil, fmt.Errorf("create position request duration histogram: %w", err)
	}
	if metrics.positionsSaved, err = meter.Int64Counter("tripgo.positions.saved"); err != nil {
		return nil, fmt.Errorf("create saved positions counter: %w", err)
	}
	return metrics, nil
}

func (m *PositionMetrics) StaleCheck(ctx context.Context, result string) {
	m.staleChecks.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
}

func (m *PositionMetrics) QueueSize(ctx context.Context, size int) {
	m.queueSize.Record(ctx, int64(size))
}

func (m *PositionMetrics) ActiveWorkers(ctx context.Context, delta int) {
	m.activeWorkers.Record(ctx, m.activeCount.Add(int64(delta)))
}

func (m *PositionMetrics) PositionRequest(ctx context.Context, result string, duration time.Duration) {
	attrs := metric.WithAttributes(attribute.String("result", result))
	m.positionRequest.Add(ctx, 1, attrs)
	if duration > 0 {
		m.requestDuration.Record(ctx, duration.Seconds(), attrs)
	}
}

func (m *PositionMetrics) PositionSaved(ctx context.Context) {
	m.positionsSaved.Add(ctx, 1)
}
