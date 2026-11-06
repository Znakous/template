package model

import (
	"time"

	"github.com/google/uuid"
)

type IdempotencyRecord struct {
	Key         uuid.UUID
	TripID      uuid.UUID
	RequestHash string
	CreatedAt   time.Time
}