package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Znakous/template/internal/generated"
)

type readinessChecker interface {
	CheckReady(ctx context.Context) error
}

type handler struct {
	api.Unimplemented
	service readinessChecker
}

func New(service readinessChecker) http.Handler {
	requestHandler := &handler{service: service}
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
	if err := h.service.CheckReady(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, api.HealthResponse{Status: api.Unavailable})
		return
	}
	writeJSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
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
		Type:     "https://tripgo.example/problems/invalid-request",
		Title:    title,
		Status:   int32(status),
		Detail:   &detail,
		Instance: &r.URL.Path,
		Code:     code,
	})
}
