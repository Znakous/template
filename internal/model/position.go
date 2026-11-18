package model

import (
	"time"

	"github.com/google/uuid"
)

type TripPosition struct {
	ID         int64
	TripID     uuid.UUID
	Latitude   float64
	Longitude  float64
	RecordedAt time.Time
}

type StaleTrip struct {
	TripID   uuid.UUID
	DriverID uuid.UUID
}
