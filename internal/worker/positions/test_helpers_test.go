package positions

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/goleak"

	"github.com/Znakous/template/internal/model"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

type testFinder struct {
	trips []model.StaleTrip
}

func (f testFinder) FindStaleTrips(context.Context, time.Duration) ([]model.StaleTrip, error) {
	return f.trips, nil
}

type testTicker struct {
	ticks chan time.Time
}

func (t *testTicker) C() <-chan time.Time { return t.ticks }
func (t *testTicker) Stop()               {}

type testClock struct {
	ticker *testTicker
}

func (c testClock) NewTicker(time.Duration) Ticker { return c.ticker }

type testMetrics struct {
	mu       sync.Mutex
	results  map[string]int
	requests map[string]int
	changed  chan struct{}
}

func newTestMetrics() *testMetrics {
	return &testMetrics{
		results:  make(map[string]int),
		requests: make(map[string]int),
		changed:  make(chan struct{}, 100),
	}
}

func (m *testMetrics) StaleCheck(_ context.Context, result string) {
	m.mu.Lock()
	m.results[result]++
	m.mu.Unlock()
	m.changed <- struct{}{}
}

func (*testMetrics) QueueSize(context.Context, int)     {}
func (*testMetrics) ActiveWorkers(context.Context, int) {}
func (m *testMetrics) PositionRequest(_ context.Context, result string, _ time.Duration) {
	m.mu.Lock()
	m.requests[result]++
	m.mu.Unlock()
	m.changed <- struct{}{}
}

func (m *testMetrics) count(result string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.results[result]
}

func (m *testMetrics) requestCount(result string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.requests[result]
}

type testRequester struct {
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int64
}

func (r *testRequester) RequestPosition(ctx context.Context, _, _ uuid.UUID) error {
	r.calls.Add(1)
	r.entered <- struct{}{}
	select {
	case <-r.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func newTestPool(
	t *testing.T,
	finder StaleTripFinder,
	requester PositionRequester,
	clock Clock,
	workers, queueSize int,
	metrics Metrics,
) *Pool {
	t.Helper()
	pool, err := New(
		finder,
		requester,
		metrics,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		clock,
		time.Second,
		time.Minute,
		workers,
		queueSize,
		time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	return pool
}

func awaitSignal[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for signal")
		var zero T
		return zero
	}
}

func awaitCount(t *testing.T, metrics *testMetrics, result string, count int) {
	t.Helper()
	for metrics.count(result) < count {
		awaitSignal(t, metrics.changed)
	}
}

func stopPool(t *testing.T, pool *Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := pool.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}
