package service

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	database     *pgxpool.Pool
	queryTimeout time.Duration
}

func New(database *pgxpool.Pool, queryTimeout time.Duration) *Service {
	return &Service{database: database, queryTimeout: queryTimeout}
}

func (s *Service) CheckReady(ctx context.Context) error {
	queryCtx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	if err := s.database.Ping(queryCtx); err != nil {
		return fmt.Errorf("ping database for readiness: %w", err)
	}
	return nil
}
