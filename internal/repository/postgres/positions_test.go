package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Znakous/template/internal/model"
	"github.com/Znakous/template/internal/transaction"
)

func TestPositionSaveIsAtomicAndDoesNotMoveTimestampBackward(t *testing.T) {
	pool := testDatabase(t)
	ctx := context.Background()
	tripID := insertTestTrip(t, pool, model.TripStatusActive, time.Now().Add(-time.Hour))
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM trips WHERE id = $1", tripID)
	}()

	repository := NewPositions(pool)
	txManager := transaction.New(pool, time.Second)
	recordedAt := time.Now().UTC().Truncate(time.Microsecond)
	position := model.TripPosition{TripID: tripID, Latitude: 59.935, Longitude: 30.335, RecordedAt: recordedAt}
	rollbackCause := errors.New("force rollback")
	err := txManager.Do(ctx, func(txCtx context.Context) error {
		if _, err := repository.Save(txCtx, position); err != nil {
			return err
		}
		return rollbackCause
	})
	if !errors.Is(err, rollbackCause) {
		t.Fatalf("transaction error = %v, want rollback cause", err)
	}

	var lastPositionAt *time.Time
	var count int
	if err := pool.QueryRow(ctx, "SELECT last_position_at FROM trips WHERE id = $1", tripID).Scan(&lastPositionAt); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM trip_positions WHERE trip_id = $1", tripID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if lastPositionAt != nil || count != 0 {
		t.Fatalf("rollback left last_position_at=%v and %d positions", lastPositionAt, count)
	}

	savePosition := func(at time.Time) {
		t.Helper()
		if err := txManager.Do(ctx, func(txCtx context.Context) error {
			_, err := repository.Save(txCtx, model.TripPosition{
				TripID: tripID, Latitude: 59.935, Longitude: 30.335, RecordedAt: at,
			})
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	savePosition(recordedAt.Add(time.Second))
	savePosition(recordedAt)
	if err := pool.QueryRow(ctx, "SELECT last_position_at FROM trips WHERE id = $1", tripID).Scan(&lastPositionAt); err != nil {
		t.Fatal(err)
	}
	if lastPositionAt == nil || !lastPositionAt.Equal(recordedAt.Add(time.Second)) {
		t.Fatalf("last_position_at = %v, want %v", lastPositionAt, recordedAt.Add(time.Second))
	}
}

func TestFindStaleTripsFiltersAndOrdersInDatabase(t *testing.T) {
	pool := testDatabase(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	noPosition := insertTestTrip(t, pool, model.TripStatusActive, now.Add(-2*time.Minute))
	oldPosition := insertTestTrip(t, pool, model.TripStatusActive, now)
	fresh := insertTestTrip(t, pool, model.TripStatusActive, now)
	completed := insertTestTrip(t, pool, model.TripStatusCompleted, now.Add(-time.Hour))
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM trips WHERE id = ANY($1)", []uuid.UUID{noPosition, oldPosition, fresh, completed})
	}()

	repository := NewPositions(pool)
	txManager := transaction.New(pool, time.Second)
	if err := txManager.Do(ctx, func(txCtx context.Context) error {
		_, err := repository.Save(txCtx, model.TripPosition{
			TripID: oldPosition, Latitude: 59, Longitude: 30, RecordedAt: now.Add(-time.Minute),
		})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		"UPDATE trips SET status = 'completed', finished_at = $1 WHERE id = $2",
		now.Add(-time.Hour), completed,
	); err != nil {
		t.Fatal(err)
	}

	stale, err := repository.FindStaleTrips(ctx, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	expected := map[uuid.UUID]bool{noPosition: true, oldPosition: true}
	found := make(map[uuid.UUID]int, len(expected))
	for _, trip := range stale {
		if expected[trip.TripID] {
			found[trip.TripID] = len(found)
		}
	}
	if _, ok := found[noPosition]; !ok {
		t.Fatal("trip without coordinates was not selected")
	}
	if _, ok := found[oldPosition]; !ok {
		t.Fatal("trip with stale coordinates was not selected")
	}
	if found[noPosition] >= found[oldPosition] {
		t.Fatalf("stale trips not ordered oldest first: no-position=%d old-position=%d", found[noPosition], found[oldPosition])
	}
	for _, trip := range stale {
		if trip.TripID == fresh || trip.TripID == completed {
			t.Fatalf("fresh or completed trip %s was selected", trip.TripID)
		}
	}
}

func testDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func insertTestTrip(t *testing.T, pool *pgxpool.Pool, status model.TripStatus, startedAt time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	var finishedAt any
	if status == model.TripStatusCompleted {
		finishedAt = startedAt.Add(time.Minute)
	}
	_, err := pool.Exec(context.Background(), `
INSERT INTO trips (
    id, user_id, driver_id, start_latitude, start_longitude, end_latitude,
    end_longitude, price, status, started_at, finished_at
) VALUES ($1, $2, $3, 59, 30, 60, 31, 100, $4, $5, $6)`,
		id, uuid.New(), uuid.New(), status, startedAt, finishedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
