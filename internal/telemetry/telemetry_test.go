package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestNewUsesOTLPHTTPSignalPaths(t *testing.T) {
	var mu sync.Mutex
	paths := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths[r.URL.Path]++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	provider, err := New(ctx, "test-service", server.URL+"/prefix")
	if err != nil {
		t.Fatal(err)
	}
	_, span := otel.Tracer("test").Start(ctx, "test-span")
	span.End()
	counter, err := otel.Meter("test").Int64Counter("test.counter")
	if err != nil {
		t.Fatal(err)
	}
	counter.Add(ctx, 1, metric.WithAttributes(attribute.String("result", "ok")))
	if err := provider.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	otel.SetMeterProvider(metricnoop.NewMeterProvider())

	mu.Lock()
	defer mu.Unlock()
	if paths["/prefix/v1/traces"] == 0 || paths["/prefix/v1/metrics"] == 0 {
		t.Fatalf("OTLP signal paths = %v, want /prefix/v1/traces and /prefix/v1/metrics", paths)
	}
}
