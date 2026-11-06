package create_test

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/Znakous/template/internal/domain/trip"
	"github.com/Znakous/template/internal/generated"
	"github.com/Znakous/template/internal/http/trip/create"
	usecase "github.com/Znakous/template/internal/usecase/trip/create"
)

func newLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func validTrip() trip.Trip {
	return trip.Trip{
		ID:             uuid.New(),
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

func validRequestBody() string {
	userID := uuid.New().String()
	driverID := uuid.New().String()
	return `{
		"user_id": "` + userID + `",
		"driver_id": "` + driverID + `",
		"start_point": {"latitude": 55.75, "longitude": 37.61},
		"end_point": {"latitude": 55.76, "longitude": 37.62},
		"price": 1450
	}`
}

func TestCreateTripCreated(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMocktripsUsecase(ctrl)
	h := create.New(mock, newLogger())

	tr := validTrip()
	mock.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(usecase.Output{Trip: tr, Created: true}, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", strings.NewReader(validRequestBody()))
	req.Header.Set("Content-Type", "application/json")

	h.CreateTrip(rec, req, api.CreateTripParams{})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if got := rec.Header().Get("Location"); got != "/api/v1/trips/"+tr.ID.String() {
		t.Fatalf("Location = %q, want %q", got, "/api/v1/trips/"+tr.ID.String())
	}

	var body api.Trip
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Id.String() != tr.ID.String() {
		t.Fatalf("body.id = %s, want %s", body.Id, tr.ID)
	}
	if body.Status != api.Active {
		t.Fatalf("body.status = %q, want %q", body.Status, api.Active)
	}
}

func TestCreateTripDriverBusy(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMocktripsUsecase(ctrl)
	h := create.New(mock, newLogger())

	mock.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(usecase.Output{}, trip.ErrDriverBusy)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", strings.NewReader(validRequestBody()))
	req.Header.Set("Content-Type", "application/json")

	h.CreateTrip(rec, req, api.CreateTripParams{})

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
	assertProblemCode(t, rec, "driver_busy")
}

func TestCreateTripInvalidBody(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMocktripsUsecase(ctrl)
	h := create.New(mock, newLogger())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", strings.NewReader(`{"malformed":`))
	req.Header.Set("Content-Type", "application/json")

	h.CreateTrip(rec, req, api.CreateTripParams{})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	assertProblemCode(t, rec, "invalid_request")
}

func TestCreateTripInternalError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMocktripsUsecase(ctrl)
	h := create.New(mock, newLogger())

	mock.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(usecase.Output{}, errors.New("db exploded"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", strings.NewReader(validRequestBody()))
	req.Header.Set("Content-Type", "application/json")

	h.CreateTrip(rec, req, api.CreateTripParams{})

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if strings.Contains(rec.Body.String(), "db exploded") {
		t.Fatalf("body leaks internal error: %s", rec.Body.String())
	}
	assertProblemCode(t, rec, "internal_error")
}

func assertProblemCode(t *testing.T, rec *httptest.ResponseRecorder, wantCode string) {
	t.Helper()
	var body api.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode problem body: %v", err)
	}
	if body.Code != wantCode {
		t.Fatalf("body.code = %q, want %q", body.Code, wantCode)
	}
}
