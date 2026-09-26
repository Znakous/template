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

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Znakous/template/internal/config"
	"github.com/Znakous/template/internal/handler"
	"github.com/Znakous/template/internal/service"
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

	database, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return fmt.Errorf("create database pool: %w", err)
	}
	defer database.Close()

	connectCtx, cancelConnect := context.WithTimeout(ctx, cfg.DatabaseConnectTimeout)
	err = database.Ping(connectCtx)
	cancelConnect()
	if err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler.New(service.New(database, cfg.DatabaseQueryTimeout)),
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
	case <-ctx.Done():
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownTimeout)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown timed out; forcing HTTP server close", "error", err)
		if closeErr := server.Close(); closeErr != nil {
			return fmt.Errorf("force close HTTP server after shutdown error: %w", closeErr)
		}
		return fmt.Errorf("graceful HTTP shutdown: %w", err)
	}

	logger.Info("trip service stopped")
	return nil
}
