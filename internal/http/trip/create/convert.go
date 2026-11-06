package create

import (
	"github.com/Znakous/template/internal/domain/trip"
	"github.com/Znakous/template/internal/generated"
	usecase "github.com/Znakous/template/internal/usecase/trip/create"
)

func toInput(data api.TripData) usecase.Input {
	return usecase.Input{
		UserID:         data.UserId,
		DriverID:       data.DriverId,
		StartLatitude:  data.StartPoint.Latitude,
		StartLongitude: data.StartPoint.Longitude,
		EndLatitude:    data.EndPoint.Latitude,
		EndLongitude:   data.EndPoint.Longitude,
		Price:          data.Price,
	}
}

func fromTrip(t trip.Trip) api.Trip {
	return api.Trip{
		Id:         t.ID,
		UserId:     t.UserID,
		DriverId:   t.DriverID,
		StartPoint: api.Coordinates{Latitude: t.StartLatitude, Longitude: t.StartLongitude},
		EndPoint:   api.Coordinates{Latitude: t.EndLatitude, Longitude: t.EndLongitude},
		Price:      t.Price,
		Status:     api.TripStatus(t.Status),
		StartedAt:  t.StartedAt,
		FinishedAt: t.FinishedAt,
	}
}
