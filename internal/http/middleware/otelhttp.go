package middleware

import (
	"context"
	"net/http"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.34.0"
)

func OTelHTTP(tp *sdktrace.TracerProvider, mp *sdkmetric.MeterProvider) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		handler := otelhttp.NewHandler(next, "",
			otelhttp.WithTracerProvider(tp),
			otelhttp.WithMeterProvider(mp),
			otelhttp.WithSpanNameFormatter(spanNameFormatter),
		)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route := routePattern(r)
			ctx := context.WithValue(r.Context(), semconv.HTTPRouteKey, route)

			labeler, _ := otelhttp.LabelerFromContext(ctx)
			labeler.Add(semconv.HTTPRoute(route))
			ctx = otelhttp.ContextWithLabeler(ctx, labeler)

			handler.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func spanNameFormatter(_ string, r *http.Request) string {
	method := strings.ToUpper(r.Method)
	route, _ := r.Context().Value(semconv.HTTPRouteKey).(string)
	if route == "" {
		route = r.URL.Path
	}
	return method + " " + route
}
