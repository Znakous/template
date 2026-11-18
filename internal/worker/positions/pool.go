package positions

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

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

type realClock struct{}

func (realClock) NewTicker(interval time.Duration) Ticker {
	return realTicker{ticker: time.NewTicker(interval)}
}

type realTicker struct {
	ticker *time.Ticker
}

func (t realTicker) C() <-chan time.Time { return t.ticker.C }
func (t realTicker) Stop()               { t.ticker.Stop() }

type task struct {
	trip      model.StaleTrip
	running   bool
	checkSpan trace.SpanContext
}

type Pool struct {
	finder       StaleTripFinder
	requester    PositionRequester
	metrics      Metrics
	logger       *slog.Logger
	clock        Clock
	checkEvery   time.Duration
	staleAfter   time.Duration
	requestLimit time.Duration
	workerCount  int
	queue        chan *task

	mu       sync.Mutex
	enqueued map[uuid.UUID]*task

	startMu      sync.Mutex
	stopMu       sync.Mutex
	started      bool
	stopped      bool
	scanCancel   context.CancelFunc
	workerCancel context.CancelFunc
	scannerDone  chan struct{}
	workersDone  chan struct{}
}

func New(
	finder StaleTripFinder,
	requester PositionRequester,
	metrics Metrics,
	logger *slog.Logger,
	clock Clock,
	checkEvery time.Duration,
	staleAfter time.Duration,
	workers int,
	queueSize int,
	requestLimit time.Duration,
) (*Pool, error) {
	switch {
	case finder == nil:
		return nil, fmt.Errorf("stale trip finder is required")
	case requester == nil:
		return nil, fmt.Errorf("position requester is required")
	case metrics == nil:
		return nil, fmt.Errorf("position metrics are required")
	case logger == nil:
		return nil, fmt.Errorf("logger is required")
	case clock == nil:
		clock = realClock{}
	}
	if checkEvery <= 0 || staleAfter <= 0 || workers <= 0 || queueSize <= 0 || requestLimit <= 0 {
		return nil, fmt.Errorf("position worker timings and sizes must be positive")
	}
	return &Pool{
		finder:       finder,
		requester:    requester,
		metrics:      metrics,
		logger:       logger,
		clock:        clock,
		checkEvery:   checkEvery,
		staleAfter:   staleAfter,
		requestLimit: requestLimit,
		workerCount:  workers,
		queue:        make(chan *task, queueSize),
		enqueued:     make(map[uuid.UUID]*task),
	}, nil
}

func (p *Pool) Start(ctx context.Context) error {
	p.startMu.Lock()
	defer p.startMu.Unlock()
	if p.started {
		return fmt.Errorf("position worker pool already started")
	}
	p.started = true

	scanCtx, scanCancel := context.WithCancel(ctx)
	workerCtx, workerCancel := context.WithCancel(context.WithoutCancel(ctx))
	p.scanCancel = scanCancel
	p.workerCancel = workerCancel
	p.scannerDone = make(chan struct{})
	p.workersDone = make(chan struct{})
	p.metrics.QueueSize(ctx, 0)
	p.metrics.ActiveWorkers(ctx, 0)

	var workers sync.WaitGroup
	for range p.workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			p.worker(workerCtx)
		}()
	}
	go func() {
		workers.Wait()
		close(p.workersDone)
	}()
	go func() {
		defer close(p.scannerDone)
		p.scan(scanCtx)
	}()
	return nil
}

func (p *Pool) Stop(ctx context.Context) error {
	p.stopMu.Lock()
	defer p.stopMu.Unlock()

	p.startMu.Lock()
	if !p.started || p.stopped {
		p.startMu.Unlock()
		return nil
	}
	scanCancel := p.scanCancel
	workerCancel := p.workerCancel
	scannerDone := p.scannerDone
	workersDone := p.workersDone
	p.startMu.Unlock()

	scanCancel()
	select {
	case <-scannerDone:
	case <-ctx.Done():
		<-scannerDone
		close(p.queue)
		workerCancel()
		<-workersDone
		p.startMu.Lock()
		p.stopped = true
		p.startMu.Unlock()
		return fmt.Errorf("stop positions scanner: %w", ctx.Err())
	}

	close(p.queue)
	select {
	case <-workersDone:
		workerCancel()
		p.startMu.Lock()
		p.stopped = true
		p.startMu.Unlock()
		return nil
	case <-ctx.Done():
		workerCancel()
		<-workersDone
		p.startMu.Lock()
		p.stopped = true
		p.startMu.Unlock()
		return fmt.Errorf("drain position worker queue: %w", ctx.Err())
	}
}

func (p *Pool) Cancel(ctx context.Context, tripID uuid.UUID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if queued, ok := p.enqueued[tripID]; ok && !queued.running {
		delete(p.enqueued, tripID)
		p.metrics.PositionRequest(ctx, "cancelled", 0)
	}
}

func (p *Pool) scan(ctx context.Context) {
	ticker := p.clock.NewTicker(p.checkEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
			checkCtx, span := otel.Tracer("trip-service").Start(ctx, "tripgo.stale_check", trace.WithSpanKind(trace.SpanKindInternal))
			trips, err := p.finder.FindStaleTrips(checkCtx, p.staleAfter)
			if err != nil {
				span.RecordError(err)
				span.End()
				if !errors.Is(err, context.Canceled) {
					p.logger.Error("scan stale trips failed", "service", "trip-service",
						"trace_id", span.SpanContext().TraceID().String(),
						"span_id", span.SpanContext().SpanID().String(),
						"error", err)
				}
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
	defer p.mu.Unlock()
	if _, exists := p.enqueued[trip.TripID]; exists {
		p.metrics.StaleCheck(ctx, "duplicate")
		return
	}
	item := &task{trip: trip, checkSpan: checkSpan}
	p.enqueued[trip.TripID] = item
	select {
	case p.queue <- item:
		p.metrics.StaleCheck(ctx, "queued")
		p.metrics.QueueSize(ctx, len(p.queue))
	default:
		delete(p.enqueued, trip.TripID)
		p.metrics.StaleCheck(ctx, "dropped")
		spanContext := trace.SpanContextFromContext(ctx)
		p.logger.Warn("position request queue is full", "service", "trip-service",
			"trace_id", spanContext.TraceID().String(),
			"span_id", spanContext.SpanID().String(),
			"trip_id", trip.TripID)
	}
}

func (p *Pool) worker(ctx context.Context) {
	for item := range p.queue {
		p.mu.Lock()
		p.metrics.QueueSize(ctx, len(p.queue))
		current, exists := p.enqueued[item.trip.TripID]
		if !exists || current != item {
			p.mu.Unlock()
			continue
		}
		item.running = true
		p.mu.Unlock()

		workerCtx, span := otel.Tracer("trip-service").Start(ctx, "tripgo.position_request.process",
			trace.WithSpanKind(trace.SpanKindInternal),
			trace.WithLinks(trace.Link{SpanContext: item.checkSpan}),
		)
		p.metrics.ActiveWorkers(workerCtx, 1)
		started := time.Now()
		requestCtx, cancel := context.WithTimeout(workerCtx, p.requestLimit)
		err := p.requester.RequestPosition(requestCtx, item.trip.DriverID, item.trip.TripID)
		cancel()
		duration := time.Since(started)
		p.metrics.ActiveWorkers(workerCtx, -1)

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
		span.End()

		p.mu.Lock()
		if p.enqueued[item.trip.TripID] == item {
			delete(p.enqueued, item.trip.TripID)
		}
		p.mu.Unlock()
	}
}
