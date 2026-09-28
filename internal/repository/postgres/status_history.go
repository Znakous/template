package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/Znakous/template/internal/model"
	"github.com/Znakous/template/internal/transaction"
)

type StatusHistory struct {
	database transaction.Executor
}

func NewStatusHistory(database transaction.Executor) *StatusHistory {
	return &StatusHistory{database: database}
}

func (r *StatusHistory) Record(ctx context.Context, tripID uuid.UUID, fromStatus *model.TripStatus, toStatus model.TripStatus, reason *string) error {
	var fromValue any
	if fromStatus != nil {
		fromValue = string(*fromStatus)
	}

	query, args, err := psql.Insert("trip_status_history").
		Columns("trip_id", "from_status", "to_status", "reason").
		Values(tripID, fromValue, string(toStatus), reason).
		ToSql()
	if err != nil {
		return fmt.Errorf("build trip status history query: %w", err)
	}
	if _, err := transaction.ExecutorFromContext(ctx, r.database).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("insert trip status history: %w", err)
	}
	return nil
}
