package http

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/Znakous/template/internal/generated"
	"github.com/Znakous/template/internal/http/domainerror"
	"github.com/Znakous/template/internal/http/health"
	"github.com/Znakous/template/internal/http/middleware"
	"github.com/Znakous/template/internal/http/trip/create"
	"github.com/Znakous/template/internal/http/trip/finish"
	"github.com/Znakous/template/internal/http/trip/get"
)

type Server struct {
	create *create.Handler
	get    *get.Handler
	finish *finish.Handler
	health *health.Handler
}

func NewRouter(
	create *create.Handler,
	get *get.Handler,
	finish *finish.Handler,
	health *health.Handler,
	logger *slog.Logger,
	tracerProvider *sdktrace.TracerProvider,
	meterProvider *sdkmetric.MeterProvider,
) http.Handler {
	s := &Server{create: create, get: get, finish: finish, health: health}
	router := chi.NewRouter()
	router.Use(
		middleware.Recover(logger),
		middleware.OTelHTTP(tracerProvider, meterProvider),
		middleware.Logging(logger),
	)
	return api.HandlerWithOptions(s, api.ChiServerOptions{
		BaseRouter:       router,
		ErrorHandlerFunc: s.handleRequestError,
	})
}

func (s *Server) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	s.create.CreateTrip(w, r, params)
}

func (s *Server) GetTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	s.get.GetTrip(w, r, tripID)
}

func (s *Server) FinishTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	s.finish.FinishTrip(w, r, tripID)
}

func (s *Server) Health(w http.ResponseWriter, r *http.Request) {
	s.health.Health(w, r)
}

func (s *Server) Ready(w http.ResponseWriter, r *http.Request) {
	s.health.Ready(w, r)
}

func (s *Server) handleRequestError(w http.ResponseWriter, r *http.Request, _ error) {
	domainerror.WriteBadRequest(w, r, "Request validation failed")
}
