package container

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/Znakous/template/internal/config"
	httpx "github.com/Znakous/template/internal/http"
	healthh "github.com/Znakous/template/internal/http/health"
	createh "github.com/Znakous/template/internal/http/trip/create"
	finishh "github.com/Znakous/template/internal/http/trip/finish"
	geth "github.com/Znakous/template/internal/http/trip/get"
	"github.com/Znakous/template/internal/infra/postgres"
	"github.com/Znakous/template/internal/infra/postgres/transaction"
	repoIdempotency "github.com/Znakous/template/internal/repository/idempotency"
	repoHistory "github.com/Znakous/template/internal/repository/status_history"
	repoTrip "github.com/Znakous/template/internal/repository/trip"
	createu "github.com/Znakous/template/internal/usecase/trip/create"
	finishu "github.com/Znakous/template/internal/usecase/trip/finish"
	getu "github.com/Znakous/template/internal/usecase/trip/get"
)

type Container struct {
	Logger          *slog.Logger
	Handler         http.Handler
	ShutdownTimeout time.Duration
	close           func()
}

func (c *Container) Close() {
	if c.close != nil {
		c.close()
	}
}

func New(ctx context.Context, cfg config.Config) (*Container, error) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))

	pool, err := postgres.NewPool(ctx, cfg)
	if err != nil {
		return nil, err
	}

	transactions := transaction.New(pool, cfg.DatabaseQueryTimeout)
	tripRepo := repoTrip.New(pool)
	idempotencyRepo := repoIdempotency.New(pool)
	historyRepo := repoHistory.New(pool)

	createUC := createu.New(transactions, tripRepo, historyRepo, idempotencyRepo, cfg.DatabaseQueryTimeout)
	getUC := getu.New(tripRepo, cfg.DatabaseQueryTimeout)
	finishUC := finishu.New(transactions, tripRepo, historyRepo, cfg.DatabaseQueryTimeout)

	readiness := postgres.NewReadiness(pool, cfg.DatabaseQueryTimeout)

	createHandler := createh.New(createUC, logger)
	getHandler := geth.New(getUC, logger)
	finishHandler := finishh.New(finishUC, logger)
	healthHandler := healthh.New(readiness)

	handler := httpx.NewRouter(createHandler, getHandler, finishHandler, healthHandler)

	return &Container{
		Logger:          logger,
		Handler:         handler,
		ShutdownTimeout: cfg.ShutdownTimeout,
		close:           pool.Close,
	}, nil
}