package main

import (
	"SignalForge/internal/app"
	"SignalForge/internal/infra/config"
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// Setup structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Load configuration
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Initialize application
	application, err := app.New(cfg, logger)
	if err != nil {
		slog.Error("failed to initialize app", "error", err)
		os.Exit(1)
	}

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start application
	errChan := make(chan error, 1)
	go func() {
		slog.Info("starting signalforge")
		if err := application.Run(ctx); err != nil {
			errChan <- err
		}
	}()

	// Wait for shutdown signal or error
	select {
	case sig := <-sigChan:
		slog.Info("received shutdown signal", "signal", sig)
	case err := <-errChan:
		slog.Error("application error", "error", err)
	}

	// Graceful shutdown with timeout
	slog.Info("shutting down gracefully...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := application.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
		os.Exit(1)
	}

	slog.Info("shutdown complete")
}
