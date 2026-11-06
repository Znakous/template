package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)

			status := rec.status
			level := slog.LevelInfo
			if status >= 500 {
				level = slog.LevelError
			}
			logger.Log(r.Context(), level, "request handled",
				"method", r.Method,
				"route", routePattern(r),
				"status", status,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}
