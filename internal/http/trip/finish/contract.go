//go:generate mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package finish

import (
	"context"

	"github.com/google/uuid"

	"github.com/Znakous/template/internal/domain/trip"
)

type tripsUsecase interface {
	Finish(ctx context.Context, id uuid.UUID) (trip.Trip, error)
}
