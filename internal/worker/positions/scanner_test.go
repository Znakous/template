package positions

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/Znakous/template/internal/model"
)

type blockingMetrics struct {
	*testMetrics
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (m *blockingMetrics) StaleCheck(ctx context.Context, result string) {
	m.testMetrics.StaleCheck(ctx, result)
	if result == "duplicate" {
		m.once.Do(func() { close(m.entered) })
		<-m.release
	}
}

func TestScannerDeduplicatesAcrossTicks(t *testing.T) {
	trip := model.StaleTrip{TripID: uuid.New(), DriverID: uuid.New()}
	ticker := &testTicker{ticks: make(chan time.Time, 2)}
	metrics := newTestMetrics()
	requester := &testRequester{entered: make(chan struct{}, 1), release: make(chan struct{}, 1)}
	pool := newTestPool(t, testFinder{trips: []model.StaleTrip{trip}}, requester, testClock{ticker}, 1, 2, metrics)
	if err := pool.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	ticker.ticks <- time.Now()
	awaitSignal(t, requester.entered)
	ticker.ticks <- time.Now()
	awaitCount(t, metrics, "duplicate", 1)

	requester.release <- struct{}{}
	stopPool(t, pool)
	if got := requester.calls.Load(); got != 1 {
		t.Fatalf("request count = %d, want 1", got)
	}
}

func TestDuplicateMetricDoesNotHoldQueueMutex(t *testing.T) {
	baseMetrics := newTestMetrics()
	metrics := &blockingMetrics{
		testMetrics: baseMetrics,
		entered:     make(chan struct{}),
		release:     make(chan struct{}),
	}
	pool := newTestPool(t, testFinder{}, &testRequester{}, nil, 1, 1, metrics)
	trip := model.StaleTrip{TripID: uuid.New(), DriverID: uuid.New()}
	ctx := context.Background()

	pool.enqueue(ctx, trip, trace.SpanContext{})
	enqueueDone := make(chan struct{})
	go func() {
		pool.enqueue(ctx, trip, trace.SpanContext{})
		close(enqueueDone)
	}()
	awaitSignal(t, metrics.entered)

	cancelDone := make(chan struct{})
	go func() {
		pool.Cancel(ctx, trip.TripID)
		close(cancelDone)
	}()
	select {
	case <-cancelDone:
	case <-time.After(time.Second):
		close(metrics.release)
		<-enqueueDone
		<-cancelDone
		t.Fatal("Cancel blocked while duplicate metric was recorded")
	}

	close(metrics.release)
	awaitSignal(t, enqueueDone)
	if got := len(pool.enqueued); got != 0 {
		t.Fatalf("enqueued task count = %d, want 0 after Cancel", got)
	}
}

func TestScannerDropsWhenQueueIsFull(t *testing.T) {
	metrics := newTestMetrics()
	pool := newTestPool(t, testFinder{}, &testRequester{}, nil, 1, 1, metrics)
	ctx := context.Background()
	pool.enqueue(ctx, model.StaleTrip{TripID: uuid.New()}, trace.SpanContext{})
	pool.enqueue(ctx, model.StaleTrip{TripID: uuid.New()}, trace.SpanContext{})

	if got := metrics.count("dropped"); got != 1 {
		t.Fatalf("dropped count = %d, want 1", got)
	}
	if got := len(pool.queue); got != 1 {
		t.Fatalf("queue length = %d, want 1", got)
	}
}

func TestCoordinateCancelsQueuedRequest(t *testing.T) {
	metrics := newTestMetrics()
	requester := &testRequester{entered: make(chan struct{}, 1), release: make(chan struct{}, 1)}
	pool := newTestPool(t, testFinder{}, requester, nil, 1, 1, metrics)
	trip := model.StaleTrip{TripID: uuid.New(), DriverID: uuid.New()}
	pool.enqueue(context.Background(), trip, trace.SpanContext{})
	pool.Cancel(context.Background(), trip.TripID)
	close(pool.queue)
	pool.worker(context.Background())

	if got := requester.calls.Load(); got != 0 {
		t.Fatalf("request count = %d, want 0", got)
	}
	if got := metrics.requestCount("cancelled"); got != 1 {
		t.Fatalf("cancelled count = %d, want 1", got)
	}
}
