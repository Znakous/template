package get_test

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
	"github.com/Znakous/template/internal/http/trip/get"
)

func newLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func tripByID(id uuid.UUID) trip.Trip {
	return trip.Trip{
		ID:             id,
		UserID:         uuid.New(),
		DriverID:       uuid.New(),
		StartLatitude:  55.75,
		StartLongitude: 37.61,
		EndLatitude:    55.76,
		EndLongitude:   37.62,
		Price:          1450,
		Status:         trip.StatusActive,
		StartedAt:      time.Now().UTC(),
	}
}

func TestGetTripOK(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMocktripsUsecase(ctrl)
	h := get.New(mock, newLogger())

	id := uuid.New()
	tr := tripByID(id)
	mock.EXPECT().Get(gomock.Any(), id).Return(tr, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+id.String(), nil)

	h.GetTrip(rec, req, api.TripId(id))

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
}

func TestGetTripNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMocktripsUsecase(ctrl)
	h := get.New(mock, newLogger())

	id := uuid.New()
	mock.EXPECT().Get(gomock.Any(), id).Return(trip.Trip{}, trip.ErrTripNotFound)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+id.String(), nil)

	h.GetTrip(rec, req, api.TripId(id))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	var body api.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode problem body: %v", err)
	}
	if body.Code != "trip_not_found" {
		t.Fatalf("body.code = %q, want %q", body.Code, "trip_not_found")
	}
}