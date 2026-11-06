package create_test

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
	"github.com/Znakous/template/internal/usecase/trip/create"
)

func testUsecase(t *testing.T, ctrl *gomock.Controller) (
	*create.Usecase,
	*MocktxManager,
	*MocktripRepository,
	*MockstatusHistoryRepository,
	*Mockmetrics,
) {
	t.Helper()
	tx := NewMocktxManager(ctrl)
	trips := NewMocktripRepository(ctrl)
	history := NewMockstatusHistoryRepository(ctrl)
	idem := NewMockidempotencyRepository(ctrl)
	metrics := NewMockmetrics(ctrl)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	uc := create.New(tx, trips, history, idem, time.Second, metrics, logger)
	return uc, tx, trips, history, metrics
}

func runInTx() gomock.Matcher {
	return gomock.Any()
}

func validInput() create.Input {
	return create.Input{
		UserID:         uuid.New(),
		DriverID:       uuid.New(),
		StartLatitude:  55.0,
		StartLongitude: 37.0,
		EndLatitude:    56.0,
		EndLongitude:   38.0,
		Price:          100,
	}
}

func TestCreate_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	uc, tx, trips, history, metrics := testUsecase(t, ctrl)

	tx.EXPECT().Do(gomock.Any(), runInTx()).DoAndReturn(
		func(_ context.Context, fn func(context.Context) error) error { return fn(context.Background()) },
	)
	trips.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)
	history.EXPECT().Add(gomock.Any(), gomock.Any(), nil, trip.StatusActive, nil).Return(nil)
	metrics.EXPECT().TripCreated(gomock.Any())

	out, err := uc.Create(context.Background(), validInput(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Created {
		t.Fatal("expected created trip")
	}
	if out.Trip.Status != trip.StatusActive {
		t.Fatalf("expected status %q, got %q", trip.StatusActive, out.Trip.Status)
	}
}

func TestCreate_DriverBusy(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	uc, tx, trips, _, _ := testUsecase(t, ctrl)

	tx.EXPECT().Do(gomock.Any(), runInTx()).DoAndReturn(
		func(_ context.Context, fn func(context.Context) error) error { return fn(context.Background()) },
	)
	trips.EXPECT().Create(gomock.Any(), gomock.Any()).Return(trip.ErrDriverBusy)

	_, err := uc.Create(context.Background(), validInput(), nil)
	if !errors.Is(err, trip.ErrDriverBusy) {
		t.Fatalf("expected ErrDriverBusy, got %v", err)
	}
}

func TestCreate_InvalidInput(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	uc, _, _, _, _ := testUsecase(t, ctrl)

	in := validInput()
	in.UserID = uuid.Nil

	_, err := uc.Create(context.Background(), in, nil)
	var validationErr *trip.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *ValidationError, got %v", err)
	}
	if validationErr.Field != "user_id" {
		t.Fatalf("expected field user_id, got %q", validationErr.Field)
	}
}

func TestCreate_RepositoryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	uc, tx, trips, _, metrics := testUsecase(t, ctrl)

	metrics.EXPECT().TripCreated(gomock.Any()).Times(0)
	tx.EXPECT().Do(gomock.Any(), runInTx()).DoAndReturn(
		func(_ context.Context, fn func(context.Context) error) error { return fn(context.Background()) },
	)
	trips.EXPECT().Create(gomock.Any(), gomock.Any()).Return(errors.New("db exploded"))

	_, err := uc.Create(context.Background(), validInput(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
}
