package create

import (
	"context"

	"github.com/google/uuid"

	usecase "github.com/Znakous/template/internal/usecase/trip/create"
)

type tripsUsecase interface {
	Create(ctx context.Context, in usecase.Input, idempotencyKey *uuid.UUID) (usecase.Output, error)
}