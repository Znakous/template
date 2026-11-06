package finish_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/Znakous/template/internal/domain/trip"
	"github.com/Znakous/template/internal/usecase/trip/finish"
)

func newUsecase(t *testing.T, ctrl *gomock.Controller) (
	*finish.Usecase,
	*MocktxManager,
	*MocktripRepository,
	*MockstatusHistoryRepository,
	*Mockmetrics,
) {
	t.Helper()
	tx := NewMocktxManager(ctrl)
	trips := NewMocktripRepository(ctrl)
	history := NewMockstatusHistoryRepository(ctrl)
	metrics := NewMockmetrics(ctrl)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	uc := finish.New(tx, trips, history, time.Second, metrics, logger)
	return uc, tx, trips, history, metrics
}

func TestFinish_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	uc, tx, trips, history, metrics := newUsecase(t, ctrl)

	finished := trip.Trip{
		ID:        uuid.New(),
		Status:    trip.StatusActive,
		StartedAt: time.Now().UTC(),
	}

	tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, fn func(context.Context) error) error { return fn(context.Background()) },
	)
	trips.EXPECT().Finish(gomock.Any(), gomock.Any(), gomock.Any()).Return(finished, nil)
	history.EXPECT().Add(gomock.Any(), gomock.Any(), gomock.Any(), trip.StatusCompleted, nil).Return(nil)
	metrics.EXPECT().TripCompleted(gomock.Any())

	got, err := uc.Finish(context.Background(), finished.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != finished.ID {
		t.Fatalf("expected trip %s, got %s", finished.ID, got.ID)
	}
}

func TestFinish_Completed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	uc, tx, trips, _, metrics := newUsecase(t, ctrl)

	metrics.EXPECT().TripCompleted(gomock.Any()).Times(0)
	tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, fn func(context.Context) error) error { return fn(context.Background()) },
	)
	trips.EXPECT().Finish(gomock.Any(), gomock.Any(), gomock.Any()).Return(trip.Trip{}, trip.ErrTripCompleted)

	_, err := uc.Finish(context.Background(), uuid.New())
	if !errors.Is(err, trip.ErrTripCompleted) {
		t.Fatalf("expected ErrTripCompleted, got %v", err)
	}
}

func TestFinish_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	uc, tx, trips, _, _ := newUsecase(t, ctrl)

	tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, fn func(context.Context) error) error { return fn(context.Background()) },
	)
	trips.EXPECT().Finish(gomock.Any(), gomock.Any(), gomock.Any()).Return(trip.Trip{}, trip.ErrTripNotFound)

	_, err := uc.Finish(context.Background(), uuid.New())
	if !errors.Is(err, trip.ErrTripNotFound) {
		t.Fatalf("expected ErrTripNotFound, got %v", err)
	}
}
