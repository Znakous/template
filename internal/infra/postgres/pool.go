package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Znakous/template/internal/config"
)

func NewPool(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	poolConfig.MaxConns = cfg.DatabaseMaxConns
	poolConfig.MinConns = cfg.DatabaseMinConns
	poolConfig.MaxConnLifetime = cfg.DatabaseMaxConnLifetime
	poolConfig.ConnConfig.Tracer = otelpgx.NewTracer()

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("init database pool: %w", err)
	}

	connectCtx, cancel := context.WithTimeout(ctx, cfg.DatabaseConnectTimeout)
	defer cancel()
	if err := pool.Ping(connectCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return pool, nil
}

type Readiness struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func NewReadiness(pool *pgxpool.Pool, queryTimeout time.Duration) *Readiness {
	return &Readiness{pool: pool, queryTimeout: queryTimeout}
}

func (c *Readiness) CheckReady(ctx context.Context) error {
	queryCtx, cancel := context.WithTimeout(ctx, c.queryTimeout)
	defer cancel()

	if err := c.pool.Ping(queryCtx); err != nil {
		return fmt.Errorf("ping database for readiness: %w", err)
	}
	return nil
}