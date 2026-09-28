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
	"github.com/Znakous/template/internal/handler"
	"github.com/Znakous/template/internal/readiness"
	"github.com/Znakous/template/internal/repository/postgres"
	"github.com/Znakous/template/internal/service"
	"github.com/Znakous/template/internal/transaction"
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
	statusHistoryRepository := postgres.NewStatusHistory(database)
	idempotencyRepository := postgres.NewIdempotencyKeys(database)
	tripService := service.New(transactions, tripRepository, statusHistoryRepository, idempotencyRepository, cfg.DatabaseQueryTimeout)
	readinessChecker := readiness.New(database, cfg.DatabaseQueryTimeout)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler.New(readinessChecker, tripService),
		ReadTimeout:       cfg.HTTPReadTimeout,
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
	}

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.ListenAndServe()
	}()
	logger.Info("trip service started", "addr", cfg.HTTPAddr)

	select {
	case <-appCtx.Done():
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	}

	return shutdownResources(server, cfg.ShutdownTimeout, logger)
}

func shutdownResources(server *http.Server, timeout time.Duration, logger *slog.Logger) error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed; closing active HTTP connections", "error", err)
		if closeErr := server.Close(); closeErr != nil {
			return errors.Join(
				fmt.Errorf("graceful HTTP shutdown: %w", err),
				fmt.Errorf("force close HTTP server: %w", closeErr),
			)
		}
		return fmt.Errorf("graceful HTTP shutdown: %w", err)
	}

	logger.Info("trip service stopped gracefully")
	return nil
}
