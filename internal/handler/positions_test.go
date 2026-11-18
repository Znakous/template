package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Znakous/template/internal/model"
)

type positionTestService struct {
	saved     model.TripPosition
	positions []model.TripPosition
	saveErr   error
	listErr   error
	saveCalls int
	listCalls int
}

func (*positionTestService) CreateTrip(context.Context, model.Trip, *uuid.UUID) (model.Trip, bool, error) {
	return model.Trip{}, false, nil
}
func (*positionTestService) GetTrip(context.Context, uuid.UUID) (model.Trip, error) {
	return model.Trip{}, nil
}
func (*positionTestService) FinishTrip(context.Context, uuid.UUID) (model.Trip, error) {
	return model.Trip{}, nil
}
func (s *positionTestService) SavePosition(_ context.Context, position model.TripPosition) (model.TripPosition, error) {
	s.saveCalls++
	if s.saveErr != nil {
		return model.TripPosition{}, s.saveErr
	}
	position.ID = 17
	s.saved = position
	return position, nil
}
func (s *positionTestService) ListPositions(context.Context, uuid.UUID) ([]model.TripPosition, error) {
	s.listCalls++
	return s.positions, s.listErr
}

type positionTestReadiness struct{}

func (positionTestReadiness) CheckReady(context.Context) error { return nil }

type positionTestCanceller struct {
	tripID uuid.UUID
	calls  int
}

func (c *positionTestCanceller) Cancel(_ context.Context, tripID uuid.UUID) {
	c.tripID = tripID
	c.calls++
}

type positionTestMetrics struct {
	saved int
}

func (m *positionTestMetrics) PositionSaved(context.Context) { m.saved++ }

func TestCreateTripPosition(t *testing.T) {
	tripID := uuid.New()
	service := &positionTestService{}
	canceller := &positionTestCanceller{}
	metrics := &positionTestMetrics{}
	handler := New(positionTestReadiness{}, service, canceller, metrics)
	body := `{"coordinates":{"latitude":59.935,"longitude":30.335},"recorded_at":"` +
		time.Now().UTC().Format(time.RFC3339Nano) + `"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+tripID.String()+"/positions", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusCreated, response.Body)
	}
	var result struct {
		ID       int64     `json:"id"`
		TripID   uuid.UUID `json:"trip_id"`
		Recorded time.Time `json:"recorded_at"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.ID != 17 || result.TripID != tripID {
		t.Fatalf("unexpected position response: %+v", result)
	}
	if service.saveCalls != 1 || canceller.calls != 1 || canceller.tripID != tripID || metrics.saved != 1 {
		t.Fatalf("save/cancel/metric calls = %d/%d/%d, expected 1/1/1", service.saveCalls, canceller.calls, metrics.saved)
	}
}

func TestCreateTripPositionRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "missing recorded_at",
			body: `{"coordinates":{"latitude":59.935,"longitude":30.335}}`,
		},
		{
			name: "latitude out of range",
			body: `{"coordinates":{"latitude":91,"longitude":30},"recorded_at":"2026-08-24T12:00:05Z"}`,
		},
		{
			name: "too far in future",
			body: `{"coordinates":{"latitude":59,"longitude":30},"recorded_at":"` +
				time.Now().Add(2*time.Minute).UTC().Format(time.RFC3339Nano) + `"}`,
		},
		{
			name: "unknown field",
			body: `{"coordinates":{"latitude":59,"longitude":30},"recorded_at":"2026-08-24T12:00:05Z","extra":true}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &positionTestService{}
			handler := New(positionTestReadiness{}, service, &positionTestCanceller{}, &positionTestMetrics{})
			request := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+uuid.NewString()+"/positions", strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusBadRequest, response.Body)
			}
			if service.saveCalls != 0 {
				t.Fatalf("SavePosition called %d times for invalid input", service.saveCalls)
			}
		})
	}
}

func TestListTripPositionsReturnsAllPoints(t *testing.T) {
	tripID := uuid.New()
	service := &positionTestService{
		positions: []model.TripPosition{
			{ID: 1, TripID: tripID, Latitude: 59.9, Longitude: 30.3, RecordedAt: time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)},
			{ID: 2, TripID: tripID, Latitude: 59.8, Longitude: 30.2, RecordedAt: time.Date(2026, 8, 24, 12, 1, 0, 0, time.UTC)},
		},
	}
	handler := New(positionTestReadiness{}, service, &positionTestCanceller{}, &positionTestMetrics{})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+tripID.String()+"/positions", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusOK, response.Body)
	}
	var result struct {
		Positions []json.RawMessage `json:"positions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Positions) != 2 || service.listCalls != 1 {
		t.Fatalf("positions returned = %d, list calls = %d; want 2 and 1", len(result.Positions), service.listCalls)
	}
}
