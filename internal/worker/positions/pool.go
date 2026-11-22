package positions

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

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
