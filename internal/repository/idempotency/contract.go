package idempotency

import (
	"context"

	"github.com/google/uuid"

	"github.com/Znakous/template/internal/model"
)

type Repository interface {
	Create(ctx context.Context, key, tripID uuid.UUID, requestHash string) error
	Get(ctx context.Context, key uuid.UUID) (model.IdempotencyRecord, error)
	Delete(ctx context.Context, key uuid.UUID) error
}
