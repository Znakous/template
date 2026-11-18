package positions

import (
	"go.opentelemetry.io/otel/trace"

	"github.com/Znakous/template/internal/model"
)

type task struct {
	trip      model.StaleTrip
	running   bool
	checkSpan trace.SpanContext
}
