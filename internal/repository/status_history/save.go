package statushistory

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/Znakous/template/internal/domain/trip"
	"github.com/Znakous/template/internal/infra/postgres/transaction"
)

func (h *History) Add(ctx context.Context, tripID uuid.UUID, fromStatus *trip.Status, toStatus trip.Status, reason *string) error {
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
	if _, err := transaction.ExecutorFromContext(ctx, h.database).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("insert trip status history: %w", err)
	}
	return nil
}
