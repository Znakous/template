package trip

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusActive    Status = "active"
	StatusCompleted Status = "completed"
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
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
	Status         Status
	StartedAt      time.Time
	FinishedAt     *time.Time
}

func New(id, userID, driverID uuid.UUID, startLatitude, startLongitude, endLatitude, endLongitude float64, price int64) (*Trip, error) {
	if userID == uuid.Nil {
		return nil, &ValidationError{Field: "user_id", Message: "must not be empty"}
	}
	if driverID == uuid.Nil {
		return nil, &ValidationError{Field: "driver_id", Message: "must not be empty"}
	}
	if !validLatitude(startLatitude) || !validLatitude(endLatitude) {
		return nil, &ValidationError{Field: "latitude", Message: "must be within [-90, 90]"}
	}
	if !validLongitude(startLongitude) || !validLongitude(endLongitude) {
		return nil, &ValidationError{Field: "longitude", Message: "must be within [-180, 180]"}
	}
	if price < 0 {
		return nil, &ValidationError{Field: "price", Message: "must not be negative"}
	}

	return &Trip{
		ID:             id,
		UserID:         userID,
		DriverID:       driverID,
		StartLatitude:  startLatitude,
		StartLongitude: startLongitude,
		EndLatitude:    endLatitude,
		EndLongitude:   endLongitude,
		Price:          price,
		Status:         StatusActive,
		StartedAt:      time.Now().UTC(),
	}, nil
}

func (t *Trip) Finish(at time.Time) error {
	if t.Status != StatusActive {
		return ErrTripCompleted
	}
	t.Status = StatusCompleted
	t.FinishedAt = &at
	return nil
}

func validLatitude(v float64) bool {
	return v >= -90 && v <= 90
}

func validLongitude(v float64) bool {
	return v >= -180 && v <= 180
}
