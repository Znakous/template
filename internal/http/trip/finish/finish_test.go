package finish_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/Znakous/template/internal/domain/trip"
	"github.com/Znakous/template/internal/generated"
	"github.com/Znakous/template/internal/http/trip/finish"
)

func newLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func completedTrip(id uuid.UUID) trip.Trip {
	now := time.Now().UTC()
	return trip.Trip{
		ID:             id,
		UserID:         uuid.New(),
		DriverID:       uuid.New(),
		StartLatitude:  55.75,
		StartLongitude: 37.61,
		EndLatitude:    55.76,
		EndLongitude:   37.62,
		Price:          1450,
		Status:         trip.StatusCompleted,
		StartedAt:      now.Add(-time.Hour),
		FinishedAt:     &now,
	}
}

func TestFinishTripOK(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMocktripsUsecase(ctrl)
	h := finish.New(mock, newLogger())

	id := uuid.New()
	tr := completedTrip(id)
	mock.EXPECT().Finish(gomock.Any(), id).Return(tr, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+id.String()+"/finish", nil)

	h.FinishTrip(rec, req, api.TripId(id))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body api.Trip
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Id.String() != id.String() {
		t.Fatalf("body.id = %s, want %s", body.Id, id)
	}
	if body.Status != api.Completed {
		t.Fatalf("body.status = %q, want %q", body.Status, api.Completed)
	}
}

func TestFinishTripCompleted(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMocktripsUsecase(ctrl)
	h := finish.New(mock, newLogger())

	id := uuid.New()
	mock.EXPECT().Finish(gomock.Any(), id).Return(trip.Trip{}, trip.ErrTripCompleted)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+id.String()+"/finish", nil)

	h.FinishTrip(rec, req, api.TripId(id))

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
	var body api.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode problem body: %v", err)
	}
	if body.Code != "trip_completed" {
		t.Fatalf("body.code = %q, want %q", body.Code, "trip_completed")
	}
}