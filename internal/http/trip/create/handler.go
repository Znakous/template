package create

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"

	"github.com/google/uuid"

	"github.com/Znakous/template/internal/generated"
	"github.com/Znakous/template/internal/http/domainerror"
	"github.com/Znakous/template/internal/http/respond"
)

type Handler struct {
	trips  tripsUsecase
	logger *slog.Logger
}

func New(trips tripsUsecase, logger *slog.Logger) *Handler {
	return &Handler{trips: trips, logger: logger}
}

func (h *Handler) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	data, err := decodeTripData(w, r)
	if err != nil {
		domainerror.WriteBadRequest(w, r, err.Error())
		return
	}

	var idempotencyKey *uuid.UUID
	if params.IdempotencyKey != nil {
		key := uuid.UUID(*params.IdempotencyKey)
		idempotencyKey = &key
	}

	out, err := h.trips.Create(r.Context(), toInput(data), idempotencyKey)
	if err != nil {
		domainerror.Write(w, r, h.logger, err)
		return
	}

	if out.Created {
		w.Header().Set("Location", "/api/v1/trips/"+out.Trip.ID.String())
		respond.JSON(w, http.StatusCreated, fromTrip(out.Trip))
		return
	}
	respond.JSON(w, http.StatusOK, fromTrip(out.Trip))
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

func isJSONNull(value json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}