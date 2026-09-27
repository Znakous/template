package readiness

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Checker struct {
	database     *pgxpool.Pool
	queryTimeout time.Duration
}

func New(database *pgxpool.Pool, queryTimeout time.Duration) *Checker {
	return &Checker{database: database, queryTimeout: queryTimeout}
}

func (c *Checker) CheckReady(ctx context.Context) error {
	queryCtx, cancel := context.WithTimeout(ctx, c.queryTimeout)
	defer cancel()

	if err := c.database.Ping(queryCtx); err != nil {
		return fmt.Errorf("ping database for readiness: %w", err)
	}
	return nil
}
