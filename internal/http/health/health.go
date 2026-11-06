package health

import (
	"net/http"

	"github.com/Znakous/template/internal/generated"
	"github.com/Znakous/template/internal/http/respond"
)

type Handler struct {
	readiness readinessChecker
}

func New(readiness readinessChecker) *Handler {
	return &Handler{readiness: readiness}
}

func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	respond.JSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	if err := h.readiness.CheckReady(r.Context()); err != nil {
		respond.JSON(w, http.StatusServiceUnavailable, api.HealthResponse{Status: api.Unavailable})
		return
	}
	respond.JSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}
