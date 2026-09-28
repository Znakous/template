package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type TripStatus string

const (
	TripStatusActive    TripStatus = "active"
	TripStatusCompleted TripStatus = "completed"
)

var (
	ErrDriverBusy             = errors.New("driver already has an active trip")
	ErrTripNotFound           = errors.New("trip not found")
	ErrTripCompleted          = errors.New("trip already completed")
	ErrIdempotencyConflict    = errors.New("idempotency key already used with a different request")
	ErrIdempotencyKeyExists   = errors.New("idempotency key already exists")
	ErrIdempotencyKeyNotFound = errors.New("idempotency key not found")
)

type IdempotencyRecord struct {
	Key         uuid.UUID
	TripID      uuid.UUID
	RequestHash string
	CreatedAt   time.Time
}

type Trip struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	DriverID       uuid.UUID
	StartLatitude  float64
	StartLongitude float64
	EndLatitude    float64
	EndLongitude   float64
	Price          int64
	Status         TripStatus
	StartedAt      time.Time
	FinishedAt     *time.Time
}
