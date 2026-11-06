//go:build integration

package integration

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Znakous/template/internal/domain/trip"
	"github.com/Znakous/template/internal/infra/postgres/transaction"
	repoIdempotency "github.com/Znakous/template/internal/repository/idempotency"
	repoHistory "github.com/Znakous/template/internal/repository/status_history"
	repoTrip "github.com/Znakous/template/internal/repository/trip"
)

const defaultDSN = "postgres://tripgo:tripgo@localhost:5432/tripgo?sslmode=disable" //nolint:gosec // локальный тестовый DSN со заглушками

func dsn() string {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v
	}
	return defaultDSN
}

func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn())
	if err != nil {
		t.Fatalf("connect pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping db: %v", err)
	}
	return pool
}

func newTrip(driverID uuid.UUID) trip.Trip {
	return trip.Trip{
		ID:             uuid.New(),
		UserID:         uuid.New(),
		DriverID:       driverID,
		StartLatitude:  55.0,
		StartLongitude: 37.0,
		EndLatitude:    56.0,
		EndLongitude:   38.0,
		Price:          100,
		Status:         trip.StatusActive,
		StartedAt:      time.Now().UTC(),
	}
}

func cleanup(t *testing.T, pool *pgxpool.Pool, tripIDs ...uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	for _, id := range tripIDs {
		_, _ = pool.Exec(ctx, `DELETE FROM trip_status_history WHERE trip_id = $1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM trips WHERE id = $1`, id)
	}
}

func TestTripCreateGet(t *testing.T) {
	pool := newPool(t)
	repo := repoTrip.New(pool)
	tr := newTrip(uuid.New())
	t.Cleanup(func() { cleanup(t, pool, tr.ID) })

	if err := repo.Create(context.Background(), tr); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := repo.GetByID(context.Background(), tr.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != tr.ID || got.Status != trip.StatusActive {
		t.Fatalf("unexpected trip: %+v", got)
	}
}

func TestTripCreateDriverBusy(t *testing.T) {
	pool := newPool(t)
	repo := repoTrip.New(pool)
	driverID := uuid.New()
	t1 := newTrip(driverID)
	t2 := newTrip(driverID)
	t.Cleanup(func() { cleanup(t, pool, t1.ID, t2.ID) })

	if err := repo.Create(context.Background(), t1); err != nil {
		t.Fatalf("create first: %v", err)
	}
	if err := repo.Create(context.Background(), t2); !errors.Is(err, trip.ErrDriverBusy) {
		t.Fatalf("expected ErrDriverBusy, got %v", err)
	}
}

func TestTripFinish(t *testing.T) {
	pool := newPool(t)
	repo := repoTrip.New(pool)
	tr := newTrip(uuid.New())
	t.Cleanup(func() { cleanup(t, pool, tr.ID) })

	if err := repo.Create(context.Background(), tr); err != nil {
		t.Fatalf("create: %v", err)
	}
	finished, err := repo.Finish(context.Background(), tr.ID, time.Now().UTC())
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if finished.Status != trip.StatusCompleted {
		t.Fatalf("expected completed, got %q", finished.Status)
	}

	_, err = repo.Finish(context.Background(), tr.ID, time.Now().UTC())
	if !errors.Is(err, trip.ErrTripCompleted) {
		t.Fatalf("expected ErrTripCompleted, got %v", err)
	}

	_, err = repo.Finish(context.Background(), uuid.New(), time.Now().UTC())
	if !errors.Is(err, trip.ErrTripNotFound) {
		t.Fatalf("expected ErrTripNotFound, got %v", err)
	}
}

func TestIdempotencyRepo(t *testing.T) {
	pool := newPool(t)
	repo := repoIdempotency.New(pool)
	trips := repoTrip.New(pool)

	tr := newTrip(uuid.New())
	if err := trips.Create(context.Background(), tr); err != nil {
		t.Fatalf("create trip: %v", err)
	}
	key := uuid.New()
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM trip_idempotency_keys WHERE key = $1`, key)
		_, _ = pool.Exec(ctx, `DELETE FROM trips WHERE id = $1`, tr.ID)
	})

	if err := repo.Create(context.Background(), key, tr.ID, "hash"); err != nil {
		t.Fatalf("create key: %v", err)
	}
	if err := repo.Create(context.Background(), key, tr.ID, "hash"); !errors.Is(err, trip.ErrIdempotencyKeyExists) {
		t.Fatalf("expected ErrIdempotencyKeyExists, got %v", err)
	}

	rec, err := repo.Get(context.Background(), key)
	if err != nil {
		t.Fatalf("get key: %v", err)
	}
	if rec.TripID != tr.ID || rec.RequestHash != "hash" {
		t.Fatalf("unexpected record: %+v", rec)
	}

	if err := repo.Delete(context.Background(), key); err != nil {
		t.Fatalf("delete key: %v", err)
	}
	if _, err := repo.Get(context.Background(), key); !errors.Is(err, trip.ErrIdempotencyKeyNotFound) {
		t.Fatalf("expected ErrIdempotencyKeyNotFound after delete, got %v", err)
	}
}

func TestStatusHistoryAdd(t *testing.T) {
	pool := newPool(t)
	repo := repoTrip.New(pool)
	history := repoHistory.New(pool)
	tr := newTrip(uuid.New())
	t.Cleanup(func() { cleanup(t, pool, tr.ID) })

	if err := repo.Create(context.Background(), tr); err != nil {
		t.Fatalf("create: %v", err)
	}
	from := trip.StatusActive
	if err := history.Add(context.Background(), tr.ID, &from, trip.StatusCompleted, nil); err != nil {
		t.Fatalf("add history: %v", err)
	}

	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM trip_status_history WHERE trip_id = $1`, tr.ID).Scan(&count); err != nil {
		t.Fatalf("count history: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 history row, got %d", count)
	}
}

func TestTransactionRollback(t *testing.T) {
	pool := newPool(t)
	repo := repoTrip.New(pool)
	history := repoHistory.New(pool)
	manager := transaction.New(pool, 3*time.Second)

	tr := newTrip(uuid.New())
	t.Cleanup(func() { cleanup(t, pool, tr.ID) })

	ctx := context.Background()
	err := manager.Do(ctx, func(txCtx context.Context) error {
		if err := repo.Create(txCtx, tr); err != nil {
			return err
		}
		if err := history.Add(txCtx, tr.ID, nil, trip.StatusActive, nil); err != nil {
			return err
		}
		return errors.New("boom")
	})
	if err == nil {
		t.Fatal("expected tx error")
	}

	var tripCount, historyCount int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM trips WHERE id = $1`, tr.ID).Scan(&tripCount)
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM trip_status_history WHERE trip_id = $1`, tr.ID).Scan(&historyCount)
	if tripCount != 0 || historyCount != 0 {
		t.Fatalf("expected rollback (trip=%d history=%d), got (trip=%d history=%d)", 0, 0, tripCount, historyCount)
	}
}
