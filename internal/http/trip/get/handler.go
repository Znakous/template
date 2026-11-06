package get

import (
	"log/slog"
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

func (h *Handler) GetTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	t, err := h.trips.Get(r.Context(), uuid.UUID(tripID))
	if err != nil {
		domainerror.Write(w, r, h.logger, err)
		return
	}
	respond.JSON(w, http.StatusOK, fromTrip(t))
}