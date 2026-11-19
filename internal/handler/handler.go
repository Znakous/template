package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Znakous/template/internal/generated"
	"github.com/Znakous/template/internal/model"
)

type readinessChecker interface {
	CheckReady(ctx context.Context) error
}

type tripService interface {
	CreateTrip(ctx context.Context, trip model.Trip, idempotencyKey *uuid.UUID) (model.Trip, bool, error)
	GetTrip(ctx context.Context, id uuid.UUID) (model.Trip, error)
	FinishTrip(ctx context.Context, id uuid.UUID) (model.Trip, error)
	SavePosition(ctx context.Context, position model.TripPosition) (model.TripPosition, error)
	ListPositions(ctx context.Context, tripID uuid.UUID) ([]model.TripPosition, error)
}

type positionCanceller interface {
	Cancel(ctx context.Context, tripID uuid.UUID)
}

type positionMetrics interface {
	PositionSaved(ctx context.Context)
}

type handler struct {
	api.Unimplemented
	readiness readinessChecker
	trips     tripService
	canceller positionCanceller
	metrics   positionMetrics
}

func New(readiness readinessChecker, trips tripService, canceller positionCanceller, metrics positionMetrics) http.Handler {
	requestHandler := &handler{
		readiness: readiness,
		trips:     trips,
		canceller: canceller,
		metrics:   metrics,
	}
	router := chi.NewRouter()
	return api.HandlerWithOptions(requestHandler, api.ChiServerOptions{
		BaseRouter:       router,
		ErrorHandlerFunc: requestHandler.handleRequestError,
	})
}

func (h *handler) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}

func (h *handler) Ready(w http.ResponseWriter, r *http.Request) {
	if err := h.readiness.CheckReady(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, api.HealthResponse{Status: api.Unavailable})
		return
	}
	writeJSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}

func (h *handler) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	data, err := decodeTripData(w, r)
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "Invalid request", "invalid_request", "Request validation failed: "+err.Error())
		return
	}
	if data.UserId == uuid.Nil || data.DriverId == uuid.Nil ||
		!validCoordinates(data.StartPoint) || !validCoordinates(data.EndPoint) || data.Price < 0 {
		writeProblem(w, r, http.StatusBadRequest, "Invalid request", "invalid_request", "Request validation failed")
		return
	}

	var idempotencyKey *uuid.UUID
	if params.IdempotencyKey != nil {
		key := uuid.UUID(*params.IdempotencyKey)
		idempotencyKey = &key
	}

	trip, created, err := h.trips.CreateTrip(r.Context(), model.Trip{
		UserID:         uuid.UUID(data.UserId),
		DriverID:       uuid.UUID(data.DriverId),
		StartLatitude:  data.StartPoint.Latitude,
		StartLongitude: data.StartPoint.Longitude,
		EndLatitude:    data.EndPoint.Latitude,
		EndLongitude:   data.EndPoint.Longitude,
		Price:          data.Price,
	}, idempotencyKey)
	if err != nil {
		h.writeTripError(w, r, err)
		return
	}
	if created {
		w.Header().Set("Location", "/api/v1/trips/"+trip.ID.String())
		writeJSON(w, http.StatusCreated, tripResponse(trip))
		return
	}
	writeJSON(w, http.StatusOK, tripResponse(trip))
}

func (h *handler) GetTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	trip, err := h.trips.GetTrip(r.Context(), uuid.UUID(tripID))
	if err != nil {
		h.writeTripError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, tripResponse(trip))
}

func (h *handler) FinishTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	trip, err := h.trips.FinishTrip(r.Context(), uuid.UUID(tripID))
	if err != nil {
		h.writeTripError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, tripResponse(trip))
}

func (h *handler) CreateTripPosition(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	data, err := decodePositionData(w, r)
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "Invalid request", "invalid_request", "Request validation failed: "+err.Error())
		return
	}
	if !validCoordinates(data.Coordinates) || data.RecordedAt.After(time.Now().Add(time.Minute)) {
		writeProblem(w, r, http.StatusBadRequest, "Invalid request", "invalid_request", "Request validation failed")
		return
	}

	position, err := h.trips.SavePosition(r.Context(), model.TripPosition{
		TripID:     uuid.UUID(tripID),
		Latitude:   data.Coordinates.Latitude,
		Longitude:  data.Coordinates.Longitude,
		RecordedAt: data.RecordedAt,
	})
	if err != nil {
		h.writeTripError(w, r, err)
		return
	}
	if h.canceller != nil {
		h.canceller.Cancel(r.Context(), position.TripID)
	}
	if h.metrics != nil {
		h.metrics.PositionSaved(r.Context())
	}
	writeJSON(w, http.StatusCreated, positionResponse(position))
}

func (h *handler) ListTripPositions(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	positions, err := h.trips.ListPositions(r.Context(), uuid.UUID(tripID))
	if err != nil {
		h.writeTripError(w, r, err)
		return
	}
	response := api.TripPositionsResponse{Positions: make([]api.TripPosition, 0, len(positions))}
	for _, position := range positions {
		response.Positions = append(response.Positions, positionResponse(position))
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *handler) writeTripError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, model.ErrDriverBusy):
		writeProblem(w, r, http.StatusConflict, "Driver busy", "driver_busy", "Driver already has an active trip")
	case errors.Is(err, model.ErrIdempotencyConflict):
		writeProblem(w, r, http.StatusConflict, "Idempotency conflict", "idempotency_conflict", "Idempotency-Key was already used with a different request body")
	case errors.Is(err, model.ErrTripNotFound):
		writeProblem(w, r, http.StatusNotFound, "Trip not found", "trip_not_found", "Trip was not found")
	case errors.Is(err, model.ErrTripCompleted):
		writeProblem(w, r, http.StatusConflict, "Trip completed", "trip_completed", "Operation is not allowed for a completed trip")
	default:
		slog.Error("trip request failed", "error", err)
		writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "internal_error", "Internal server error")
	}
}

func (h *handler) handleRequestError(w http.ResponseWriter, r *http.Request, _ error) {
	writeProblem(w, r, http.StatusBadRequest, "Invalid request", "invalid_request", "Request validation failed")
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeProblem(w http.ResponseWriter, r *http.Request, status int, title, code, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(api.Problem{
		Type:     "https://tripgo.example/problems/" + strings.ReplaceAll(code, "_", "-"),
		Title:    title,
		Status:   int32(status),
		Detail:   &detail,
		Instance: &r.URL.Path,
		Code:     code,
	})
}

func decodeTripData(w http.ResponseWriter, r *http.Request) (api.TripData, error) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return api.TripData{}, fmt.Errorf("content type must be application/json")
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		return api.TripData{}, fmt.Errorf("read request body: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var data api.TripData
	if err := decoder.Decode(&data); err != nil {
		return api.TripData{}, fmt.Errorf("decode request body: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return api.TripData{}, fmt.Errorf("request body must contain one JSON value")
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil || fields == nil {
		return api.TripData{}, fmt.Errorf("request body must be a JSON object")
	}
	for _, key := range []string{"user_id", "driver_id", "start_point", "end_point", "price"} {
		value, ok := fields[key]
		if !ok {
			return api.TripData{}, fmt.Errorf("missing required field %q", key)
		}
		if isJSONNull(value) {
			return api.TripData{}, fmt.Errorf("required field %q cannot be null", key)
		}
	}
	for _, key := range []string{"start_point", "end_point"} {
		var coordinates map[string]json.RawMessage
		if err := json.Unmarshal(fields[key], &coordinates); err != nil || coordinates == nil {
			return api.TripData{}, fmt.Errorf("%s must be an object", key)
		}
		latitude, ok := coordinates["latitude"]
		if !ok {
			return api.TripData{}, fmt.Errorf("%s.latitude is required", key)
		}
		if isJSONNull(latitude) {
			return api.TripData{}, fmt.Errorf("%s.latitude cannot be null", key)
		}
		longitude, ok := coordinates["longitude"]
		if !ok {
			return api.TripData{}, fmt.Errorf("%s.longitude is required", key)
		}
		if isJSONNull(longitude) {
			return api.TripData{}, fmt.Errorf("%s.longitude cannot be null", key)
		}
	}
	return data, nil
}

func decodePositionData(w http.ResponseWriter, r *http.Request) (api.PositionData, error) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return api.PositionData{}, fmt.Errorf("content type must be application/json")
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		return api.PositionData{}, fmt.Errorf("read request body: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var data api.PositionData
	if err := decoder.Decode(&data); err != nil {
		return api.PositionData{}, fmt.Errorf("decode request body: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return api.PositionData{}, fmt.Errorf("request body must contain one JSON value")
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil || fields == nil {
		return api.PositionData{}, fmt.Errorf("request body must be a JSON object")
	}
	for _, key := range []string{"coordinates", "recorded_at"} {
		value, ok := fields[key]
		if !ok || isJSONNull(value) {
			return api.PositionData{}, fmt.Errorf("required field %q is missing or null", key)
		}
	}
	var coordinates map[string]json.RawMessage
	if err := json.Unmarshal(fields["coordinates"], &coordinates); err != nil || coordinates == nil {
		return api.PositionData{}, fmt.Errorf("coordinates must be an object")
	}
	for _, key := range []string{"latitude", "longitude"} {
		value, ok := coordinates[key]
		if !ok || isJSONNull(value) {
			return api.PositionData{}, fmt.Errorf("coordinates.%s is missing or null", key)
		}
	}
	return data, nil
}

func isJSONNull(value json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}

func validCoordinates(coordinates api.Coordinates) bool {
	return coordinates.Latitude >= -90 && coordinates.Latitude <= 90 &&
		coordinates.Longitude >= -180 && coordinates.Longitude <= 180
}

func tripResponse(trip model.Trip) api.Trip {
	return api.Trip{
		Id:             api.TripId(trip.ID),
		UserId:         uuid.UUID(trip.UserID),
		DriverId:       uuid.UUID(trip.DriverID),
		StartPoint:     api.Coordinates{Latitude: trip.StartLatitude, Longitude: trip.StartLongitude},
		EndPoint:       api.Coordinates{Latitude: trip.EndLatitude, Longitude: trip.EndLongitude},
		Price:          trip.Price,
		Status:         api.TripStatus(trip.Status),
		StartedAt:      trip.StartedAt,
		FinishedAt:     trip.FinishedAt,
		LastPositionAt: trip.LastPositionAt,
	}
}

func positionResponse(position model.TripPosition) api.TripPosition {
	return api.TripPosition{
		Id:     position.ID,
		TripId: api.TripId(position.TripID),
		Coordinates: api.Coordinates{
			Latitude:  position.Latitude,
			Longitude: position.Longitude,
		},
		RecordedAt: position.RecordedAt,
	}
}
