package domainerror_test

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Znakous/template/internal/domain/trip"
	"github.com/Znakous/template/internal/generated"
	"github.com/Znakous/template/internal/http/domainerror"
)

func TestProblem(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "driver busy", err: trip.ErrDriverBusy, wantStatus: http.StatusConflict, wantCode: "driver_busy"},
		{name: "trip not found", err: trip.ErrTripNotFound, wantStatus: http.StatusNotFound, wantCode: "trip_not_found"},
		{name: "trip completed", err: trip.ErrTripCompleted, wantStatus: http.StatusConflict, wantCode: "trip_completed"},
		{name: "idempotency conflict", err: trip.ErrIdempotencyConflict, wantStatus: http.StatusConflict, wantCode: "idempotency_conflict"},
		{name: "validation error", err: &trip.ValidationError{Field: "x", Message: "y"}, wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "generic error", err: errors.New("boom"), wantStatus: http.StatusInternalServerError, wantCode: "internal_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := domainerror.Problem(tt.err)
			if status != tt.wantStatus {
				t.Fatalf("status = %d, want %d", status, tt.wantStatus)
			}
			if body.Code != tt.wantCode {
				t.Fatalf("body.Code = %q, want %q", body.Code, tt.wantCode)
			}
			if int(body.Status) != tt.wantStatus {
				t.Fatalf("body.Status = %d, want %d", body.Status, tt.wantStatus)
			}
		})
	}
}

func TestProblemDoesNotLeakMessage(t *testing.T) {
	status, body := domainerror.Problem(errors.New("boom"))
	if status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", status, http.StatusInternalServerError)
	}
	if body.Detail != nil && strings.Contains(*body.Detail, "boom") {
		t.Fatalf("body.Detail leaks original error message: %q", *body.Detail)
	}
}

func TestWrite(t *testing.T) {
	t.Run("content type", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/abc", nil)
		logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

		domainerror.Write(rec, req, logger, trip.ErrTripNotFound)

		ct := rec.Header().Get("Content-Type")
		if !strings.Contains(ct, "application/problem+json") {
			t.Fatalf("Content-Type = %q, want to contain application/problem+json", ct)
		}
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("5xx does not leak message", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", nil)
		logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

		domainerror.Write(rec, req, logger, errors.New("secret db exploded"))

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}
		if strings.Contains(rec.Body.String(), "secret db exploded") {
			t.Fatalf("body leaks original error: %s", rec.Body.String())
		}
		var body api.Problem
		if err := decodeBody(rec.Body.String(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.Code != "internal_error" {
			t.Fatalf("body.Code = %q, want %q", body.Code, "internal_error")
		}
	})
}

func TestWriteBadRequest(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", nil)

	domainerror.WriteBadRequest(rec, req, "missing required field")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	var body api.Problem
	if err := decodeBody(rec.Body.String(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Code != "invalid_request" {
		t.Fatalf("body.Code = %q, want %q", body.Code, "invalid_request")
	}
}

func decodeBody(s string, out any) error {
	return json.Unmarshal([]byte(s), out)
}