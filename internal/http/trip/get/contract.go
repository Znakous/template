//go:generate mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package get

import (
	"context"

	"github.com/google/uuid"

	"github.com/Znakous/template/internal/domain/trip"
)

type tripsUsecase interface {
	Get(ctx context.Context, id uuid.UUID) (trip.Trip, error)
}
