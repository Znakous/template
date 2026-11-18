package positions

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/Znakous/template/internal/model"
)

func TestWorkerPoolUsesConfiguredCountAndDrainsQueue(t *testing.T) {
	const workerCount = 3
	const taskCount = 6
	metrics := newTestMetrics()
	requester := &testRequester{
		entered: make(chan struct{}, taskCount),
		release: make(chan struct{}, taskCount),
	}
	ticker := &testTicker{ticks: make(chan time.Time)}
	pool := newTestPool(t, testFinder{}, requester, testClock{ticker}, workerCount, taskCount, metrics)
	for range taskCount {
		pool.enqueue(context.Background(), model.StaleTrip{TripID: uuid.New(), DriverID: uuid.New()}, trace.SpanContext{})
	}
	if err := pool.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	for range workerCount {
		awaitSignal(t, requester.entered)
	}
	for range taskCount {
		requester.release <- struct{}{}
	}
	for range taskCount - workerCount {
		awaitSignal(t, requester.entered)
	}
	stopPool(t, pool)
	if got := requester.calls.Load(); got != taskCount {
		t.Fatalf("request count = %d, want %d", got, taskCount)
	}
}
