package telemetry

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type Provider struct {
	traces  *sdktrace.TracerProvider
	metrics *sdkmetric.MeterProvider
}

func New(ctx context.Context, serviceName, endpoint string) (*Provider, error) {
	parsedEndpoint, err := url.Parse(endpoint)
	if err != nil || parsedEndpoint.Host == "" ||
		(parsedEndpoint.Scheme != "http" && parsedEndpoint.Scheme != "https") {
		return nil, fmt.Errorf("OTEL_EXPORTER_OTLP_ENDPOINT must be an absolute HTTP URL")
	}
	traceEndpoint := *parsedEndpoint
	traceEndpoint.Path = strings.TrimRight(traceEndpoint.Path, "/") + "/v1/traces"
	metricEndpoint := *parsedEndpoint
	metricEndpoint.Path = strings.TrimRight(metricEndpoint.Path, "/") + "/v1/metrics"
	traceEndpoint.RawQuery = ""
	metricEndpoint.RawQuery = ""
	traceEndpoint.Fragment = ""
	metricEndpoint.Fragment = ""
	res, err := resource.New(ctx, resource.WithAttributes(attribute.String("service.name", serviceName)))
	if err != nil {
		return nil, fmt.Errorf("create telemetry resource: %w", err)
	}

	traceOptions := []otlptracehttp.Option{otlptracehttp.WithEndpointURL(traceEndpoint.String())}
	metricOptions := []otlpmetrichttp.Option{otlpmetrichttp.WithEndpointURL(metricEndpoint.String())}
	if parsedEndpoint.Scheme == "http" {
		traceOptions = append(traceOptions, otlptracehttp.WithInsecure())
		metricOptions = append(metricOptions, otlpmetrichttp.WithInsecure())
	}
	traceExporter, err := otlptracehttp.New(ctx, traceOptions...)
	if err != nil {
		return nil, fmt.Errorf("create OTLP trace exporter: %w", err)
	}
	metricExporter, err := otlpmetrichttp.New(ctx, metricOptions...)
	if err != nil {
		return nil, fmt.Errorf("create OTLP metric exporter: %w", err)
	}

	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)
	metricProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithResource(res),
	)
	otel.SetTracerProvider(traceProvider)
	otel.SetMeterProvider(metricProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return &Provider{traces: traceProvider, metrics: metricProvider}, nil
}

func (p *Provider) Shutdown(ctx context.Context) error {
	var traceErr, metricErr error
	if err := p.traces.Shutdown(ctx); err != nil {
		traceErr = fmt.Errorf("shutdown trace provider: %w", err)
	}
	if err := p.metrics.Shutdown(ctx); err != nil {
		metricErr = fmt.Errorf("shutdown metric provider: %w", err)
	}
	if traceErr != nil || metricErr != nil {
		return fmt.Errorf("shutdown telemetry: %w", errors.Join(traceErr, metricErr))
	}
	return nil
}

func HTTPMiddleware(next http.Handler) http.Handler {
	tracer := otel.Tracer("trip-service")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := tracer.Start(ctx, r.Method+" "+r.URL.Path, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
