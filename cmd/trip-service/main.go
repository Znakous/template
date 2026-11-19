package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Znakous/template/internal/config"
	"github.com/Znakous/template/internal/gateway/push"
	"github.com/Znakous/template/internal/handler"
	"github.com/Znakous/template/internal/readiness"
	"github.com/Znakous/template/internal/repository/postgres"
	"github.com/Znakous/template/internal/service"
	"github.com/Znakous/template/internal/telemetry"
	"github.com/Znakous/template/internal/transaction"
	"github.com/Znakous/template/internal/worker/positions"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		slog.Error("trip service stopped with an error", "error", err)
		os.Exit(1)
	}
}
func run(ctx context.Context) error {
	appCtx, cancelAppCtx := context.WithCancel(ctx)
	defer cancelAppCtx()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)

	telemetryProvider, telemetryErr := telemetry.New(appCtx, cfg.OTELServiceName, cfg.OTELExporterEndpoint)
	if telemetryErr != nil {
		logger.Warn("telemetry initialization failed; continuing without telemetry", "error", telemetryErr)
	} else {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
			defer cancel()
			if err := telemetryProvider.Shutdown(shutdownCtx); err != nil {
				logger.Warn("telemetry shutdown failed", "error", err)
			}
		}()
	}
	positionMetrics, err := telemetry.NewPositionMetrics()
	if err != nil {
		return fmt.Errorf("initialize position metrics: %w", err)
	}

	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("parse database URL: %w", err)
	}
	poolConfig.MaxConns = cfg.DatabaseMaxConns
	poolConfig.MinConns = cfg.DatabaseMinConns
	poolConfig.MaxConnLifetime = cfg.DatabaseMaxConnLifetime

	database, err := pgxpool.NewWithConfig(appCtx, poolConfig)
	if err != nil {
		return fmt.Errorf("init database pool: %w", err)
	}
	defer database.Close()

	connectCtx, cancelConnect := context.WithTimeout(appCtx, cfg.DatabaseConnectTimeout)
	err = database.Ping(connectCtx)
	cancelConnect()
	if err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	transactions := transaction.New(database, cfg.DatabaseQueryTimeout)
	tripRepository := postgres.NewTrips(database)
	positionRepository := postgres.NewPositions(database)
	statusHistoryRepository := postgres.NewStatusHistory(database)
	idempotencyRepository := postgres.NewIdempotencyKeys(database)
	tripService := service.New(transactions, tripRepository, statusHistoryRepository, idempotencyRepository, positionRepository, cfg.DatabaseQueryTimeout)
	readinessChecker := readiness.New(database, cfg.DatabaseQueryTimeout)

	pushClient, err := push.NewClient(cfg.PushHTTPURL, cfg.PushRequestTimeout)
	if err != nil {
		return fmt.Errorf("initialize Push Service client: %w", err)
	}
	defer pushClient.Close()

	positionPool, err := positions.New(
		tripService,
		pushClient,
		positionMetrics,
		logger,
		nil,
		cfg.PositionCheckInterval,
		cfg.PositionStaleAfter,
		cfg.PositionWorkers,
		cfg.PositionQueueSize,
		cfg.PushRequestTimeout,
	)
	if err != nil {
		return fmt.Errorf("initialize position worker pool: %w", err)
	}
	if err := positionPool.Start(appCtx); err != nil {
		return fmt.Errorf("start position worker pool: %w", err)
	}

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           telemetry.HTTPMiddleware(handler.New(readinessChecker, tripService, positionPool, positionMetrics)),
		ReadTimeout:       cfg.HTTPReadTimeout,
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
	}

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.ListenAndServe()
	}()
	logger.Info("trip service started", "service", "trip-service", "addr", cfg.HTTPAddr)

	select {
	case <-appCtx.Done():
	case err := <-serverErrors:
		shutdownErr := shutdownResources(server, positionPool, cfg.ShutdownTimeout, logger)
		if !errors.Is(err, http.ErrServerClosed) {
			return errors.Join(fmt.Errorf("serve HTTP: %w", err), shutdownErr)
		}
		return shutdownErr
	}

	return shutdownResources(server, positionPool, cfg.ShutdownTimeout, logger)
}

func shutdownResources(server *http.Server, positionPool *positions.Pool, timeout time.Duration, logger *slog.Logger) error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var shutdownErrors []error
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed; closing active HTTP connections", "error", err)
		shutdownErrors = append(shutdownErrors, fmt.Errorf("graceful HTTP shutdown: %w", err))
		if closeErr := server.Close(); closeErr != nil {
			shutdownErrors = append(shutdownErrors, fmt.Errorf("force close HTTP server: %w", closeErr))
		}
	}

	if err := positionPool.Stop(shutdownCtx); err != nil {
		logger.Error("position worker shutdown failed", "error", err)
		shutdownErrors = append(shutdownErrors, err)
	}
	if len(shutdownErrors) > 0 {
		return errors.Join(shutdownErrors...)
	}
	logger.Info("trip service stopped gracefully", "service", "trip-service")
	return nil
}
