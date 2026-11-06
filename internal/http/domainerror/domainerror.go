package domainerror

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Znakous/template/internal/domain/trip"
	"github.com/Znakous/template/internal/generated"
	"github.com/Znakous/template/internal/http/respond"
)

func Problem(err error) (status int, body api.Problem) {
	var validationErr *trip.ValidationError
	switch {
	case errors.As(err, &validationErr):
		return http.StatusBadRequest, problem(http.StatusBadRequest, "Invalid request", "invalid_request", validationErr.Error())
	case errors.Is(err, trip.ErrDriverBusy):
		return http.StatusConflict, problem(http.StatusConflict, "Driver busy", "driver_busy", "Driver already has an active trip")
	case errors.Is(err, trip.ErrIdempotencyConflict):
		return http.StatusConflict, problem(http.StatusConflict, "Idempotency conflict", "idempotency_conflict", "Idempotency-Key was already used with a different request body")
	case errors.Is(err, trip.ErrTripNotFound):
		return http.StatusNotFound, problem(http.StatusNotFound, "Trip not found", "trip_not_found", "Trip was not found")
	case errors.Is(err, trip.ErrTripCompleted):
		return http.StatusConflict, problem(http.StatusConflict, "Trip completed", "trip_completed", "Operation is not allowed for a completed trip")
	default:
		return http.StatusInternalServerError, problem(http.StatusInternalServerError, "Internal Server Error", "internal_error", "Internal server error")
	}
}

func Write(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	status, body := Problem(err)
	instance := r.URL.Path
	body.Instance = &instance
	if status >= http.StatusInternalServerError {
		logger.Error("request failed", "error", err)
	}
	respond.Problem(w, status, body)
}

func WriteBadRequest(w http.ResponseWriter, r *http.Request, detail string) {
	var status int32 = http.StatusBadRequest
	body := problem(status, "Invalid request", "invalid_request", detail)
	instance := r.URL.Path
	body.Instance = &instance
	respond.Problem(w, int(status), body)
}

func WriteInternal(w http.ResponseWriter, r *http.Request) {
	var status int32 = http.StatusInternalServerError
	body := problem(status, "Internal Server Error", "internal_error", "Internal server error")
	instance := r.URL.Path
	body.Instance = &instance
	respond.Problem(w, int(status), body)
}

func problem(status int32, title, code, detail string) api.Problem {
	return api.Problem{
		Type:   "https://tripgo.example/problems/" + strings.ReplaceAll(code, "_", "-"),
		Title:  title,
		Status: status,
		Detail: &detail,
		Code:   code,
	}
}
