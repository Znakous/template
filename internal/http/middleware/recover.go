package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/Znakous/template/internal/http/domainerror"
)

func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic recovered",
						"error", rec,
						"stack", string(debug.Stack()),
					)
					domainerror.WriteInternal(w, r)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
