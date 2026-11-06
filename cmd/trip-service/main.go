package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/Znakous/template/internal/config"
	"github.com/Znakous/template/internal/container"
)

func main() {
	os.Exit(runMain())
}

func runMain() int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "trip service stopped with an error: %v\n", err)
		return 1
	}
	return 0
}

func run(ctx context.Context) error {
	appCtx, cancelAppCtx := context.WithCancel(ctx)
	defer cancelAppCtx()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	app, err := container.New(appCtx, cfg)
	if err != nil {
		return err
	}
	defer app.Close()

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           app.Handler,
		ReadTimeout:       cfg.HTTPReadTimeout,
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
	}

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.ListenAndServe()
	}()
	app.Logger.Info("trip service started", "addr", cfg.HTTPAddr)

	select {
	case <-appCtx.Done():
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	}

	return shutdown(server, app)
}

func shutdown(server *http.Server, app *container.Container) error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), app.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		app.Logger.Error("graceful shutdown failed; closing active HTTP connections", "error", err)
		if closeErr := server.Close(); closeErr != nil {
			return errors.Join(
				fmt.Errorf("graceful HTTP shutdown: %w", err),
				fmt.Errorf("force close HTTP server: %w", closeErr),
			)
		}
		return fmt.Errorf("graceful HTTP shutdown: %w", err)
	}

	app.Logger.Info("trip service stopped gracefully")
	return nil
}
