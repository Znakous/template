package trip

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNew(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		id := uuid.New()
		userID := uuid.New()
		driverID := uuid.New()

		got, err := New(id, userID, driverID, 55.75, 37.61, 55.76, 37.62, 1450)
		if err != nil {
			t.Fatalf("New returned unexpected error: %v", err)
		}
		if got == nil {
			t.Fatal("New returned nil trip")
		}
		if got.Status != StatusActive {
			t.Fatalf("Status = %q, want %q", got.Status, StatusActive)
		}
		if got.StartedAt.IsZero() {
			t.Fatal("StartedAt not set")
		}
		if got.ID != id || got.UserID != userID || got.DriverID != driverID {
			t.Fatalf("IDs not propagated: got %+v", got)
		}
	})

	tests := []struct {
		name       string
		userID     uuid.UUID
		driverID   uuid.UUID
		startLat   float64
		startLng   float64
		endLat     float64
		endLng     float64
		price      int64
		wantField  string
	}{
		{
			name:      "empty userID",
			driverID:  uuid.New(),
			startLat:  55.75,
			startLng:  37.61,
			endLat:    55.76,
			endLng:    37.62,
			price:     100,
			wantField: "user_id",
		},
		{
			name:      "empty driverID",
			userID:    uuid.New(),
			startLat:  55.75,
			startLng:  37.61,
			endLat:    55.76,
			endLng:    37.62,
			price:     100,
			wantField: "driver_id",
		},
		{
			name:      "start latitude too high",
			userID:    uuid.New(),
			driverID:  uuid.New(),
			startLat:  91,
			startLng:  37.61,
			endLat:    55.76,
			endLng:    37.62,
			price:     100,
			wantField: "latitude",
		},
		{
			name:      "end latitude too low",
			userID:    uuid.New(),
			driverID:  uuid.New(),
			startLat:  55.75,
			startLng:  37.61,
			endLat:    -91,
			endLng:    37.62,
			price:     100,
			wantField: "latitude",
		},
		{
			name:      "start longitude too high",
			userID:    uuid.New(),
			driverID:  uuid.New(),
			startLat:  55.75,
			startLng:  181,
			endLat:    55.76,
			endLng:    37.62,
			price:     100,
			wantField: "longitude",
		},
		{
			name:      "end longitude too low",
			userID:    uuid.New(),
			driverID:  uuid.New(),
			startLat:  55.75,
			startLng:  37.61,
			endLat:    55.76,
			endLng:    -181,
			price:     100,
			wantField: "longitude",
		},
		{
			name:      "negative price",
			userID:    uuid.New(),
			driverID:  uuid.New(),
			startLat:  55.75,
			startLng:  37.61,
			endLat:    55.76,
			endLng:    37.62,
			price:     -5,
			wantField: "price",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := uuid.New()
			got, err := New(id, tt.userID, tt.driverID, tt.startLat, tt.startLng, tt.endLat, tt.endLng, tt.price)
			if err == nil {
				t.Fatal("New returned nil error, want *ValidationError")
			}
			ve, ok := err.(*ValidationError)
			if !ok {
				t.Fatalf("error type = %T, want *ValidationError", err)
			}
			if ve.Field != tt.wantField {
				t.Fatalf("Field = %q, want %q", ve.Field, tt.wantField)
			}
			if got != nil {
				t.Fatalf("New returned non-nil trip on error: %+v", got)
			}
		})
	}
}

func TestTripFinish(t *testing.T) {
	userID := uuid.New()
	driverID := uuid.New()
	tr, err := New(uuid.New(), userID, driverID, 55.75, 37.61, 55.76, 37.62, 1450)
	if err != nil {
		t.Fatalf("New returned unexpected error: %v", err)
	}

	at := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	if err := tr.Finish(at); err != nil {
		t.Fatalf("Finish returned unexpected error: %v", err)
	}
	if tr.Status != StatusCompleted {
		t.Fatalf("Status = %q, want %q", tr.Status, StatusCompleted)
	}
	if tr.FinishedAt == nil {
		t.Fatal("FinishedAt not set")
	}
	if !tr.FinishedAt.Equal(at) {
		t.Fatalf("FinishedAt = %v, want %v", *tr.FinishedAt, at)
	}

	if err := tr.Finish(at.Add(time.Hour)); err != ErrTripCompleted {
		t.Fatalf("second Finish error = %v, want ErrTripCompleted", err)
	}
	if tr.Status != StatusCompleted {
		t.Fatalf("Status changed on completed trip: %q", tr.Status)
	}
}