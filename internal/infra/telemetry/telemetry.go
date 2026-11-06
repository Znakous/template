package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.34.0"

	"github.com/Znakous/template/internal/config"
)

const meterName = "github.com/Znakous/template/internal/infra/telemetry"

type Business struct {
	tripsCreated   metric.Int64Counter
	tripsCompleted metric.Int64Counter
}

func (b *Business) TripCreated(ctx context.Context) {
	b.tripsCreated.Add(ctx, 1)
}

func (b *Business) TripCompleted(ctx context.Context) {
	b.tripsCompleted.Add(ctx, 1)
}

type Telemetry struct {
	Logger         *slog.Logger
	TracerProvider *sdktrace.TracerProvider
	MeterProvider  *sdkmetric.MeterProvider
	Business       *Business
}

func New(cfg config.Config) (*Telemetry, func(context.Context) error, error) {
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(cfg.OTELServiceName),
		semconv.ServiceVersion("dev"),
		semconv.ServiceNamespace("tripgo"),
		attribute.String("deployment.environment", "local"),
	)

	traceExporter, err := otlptracehttp.New(context.Background(), otlptracehttp.WithEndpointURL(cfg.OTELExporterOTLPEndpoint))
	if err != nil {
		return nil, nil, fmt.Errorf("create OTLP trace exporter: %w", err)
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithSampler(parseSampler(cfg.OTLPTracesSampler)),
		sdktrace.WithResource(res),
	)

	metricExporter, err := otlpmetrichttp.New(context.Background(), otlpmetrichttp.WithEndpointURL(cfg.OTELExporterOTLPEndpoint))
	if err != nil {
		return nil, nil, fmt.Errorf("create OTLP metric exporter: %w", err)
	}

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter, sdkmetric.WithInterval(10*time.Second))),
	)

	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	meter := meterProvider.Meter(meterName)
	tripsCreated, err := meter.Int64Counter("tripgo.trips.created")
	if err != nil {
		return nil, nil, fmt.Errorf("create tripgo.trips.created counter: %w", err)
	}
	tripsCompleted, err := meter.Int64Counter("tripgo.trips.completed")
	if err != nil {
		return nil, nil, fmt.Errorf("create tripgo.trips.completed counter: %w", err)
	}

	logger := newLogger(cfg)

	shutdown := func(ctx context.Context) error {
		var errs []error
		if err := tracerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("shutdown tracer provider: %w", err))
		}
		if err := meterProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("shutdown meter provider: %w", err))
		}
		return errors.Join(errs...)
	}

	return &Telemetry{
		Logger:         logger,
		TracerProvider: tracerProvider,
		MeterProvider:  meterProvider,
		Business: &Business{
			tripsCreated:   tripsCreated,
			tripsCompleted: tripsCompleted,
		},
	}, shutdown, nil
}

func parseSampler(value string) sdktrace.Sampler {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "always_on":
		return sdktrace.AlwaysSample()
	case "parentbased_always_on", "":
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	default:
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	}
}

type traceHandler struct {
	inner slog.Handler
}

func (h *traceHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.inner.Handle(ctx, r)
}

func (h *traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &traceHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *traceHandler) WithGroup(name string) slog.Handler {
	return &traceHandler{inner: h.inner.WithGroup(name)}
}

func newLogger(cfg config.Config) *slog.Logger {
	base := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})
	return slog.New(&traceHandler{
		inner: base.WithAttrs([]slog.Attr{slog.String("service", cfg.OTELServiceName)}),
	})
}