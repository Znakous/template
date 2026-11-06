package get_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/Znakous/template/internal/domain/trip"
	"github.com/Znakous/template/internal/usecase/trip/get"
)

func TestGet_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	trips := NewMocktripRepository(ctrl)
	uc := get.New(trips, time.Second)

	want := trip.Trip{ID: uuid.New(), Status: trip.StatusActive}
	trips.EXPECT().GetByID(gomock.Any(), want.ID).Return(want, nil)

	got, err := uc.Get(context.Background(), want.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != want.ID {
		t.Fatalf("expected trip %s, got %s", want.ID, got.ID)
	}
}

func TestGet_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	trips := NewMocktripRepository(ctrl)
	uc := get.New(trips, time.Second)

	id := uuid.New()
	trips.EXPECT().GetByID(gomock.Any(), id).Return(trip.Trip{}, trip.ErrTripNotFound)

	_, err := uc.Get(context.Background(), id)
	if !errors.Is(err, trip.ErrTripNotFound) {
		t.Fatalf("expected ErrTripNotFound, got %v", err)
	}
}
