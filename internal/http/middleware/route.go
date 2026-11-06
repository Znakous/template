package middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func routePattern(r *http.Request) string {
	if rc := chi.RouteContext(r.Context()); rc != nil {
		if p := rc.RoutePattern(); p != "" {
			return p
		}
	}
	return r.URL.Path
}